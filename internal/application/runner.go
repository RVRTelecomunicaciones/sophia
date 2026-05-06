package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ExitError carries a process exit code per spec §2.3.
//
//	0 → terminal DONE
//	1 → terminal BLOCKED/FAILED
//	3 → config / orchestrator-unreachable / change-not-found
//	4 → transient error (poll-loop ctx canceled, network mid-run)
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
}

// Unwrap allows errors.Is/As traversal.
func (e *ExitError) Unwrap() error { return e.Err }

// RunnerDeps groups the ports the Runner needs.
type RunnerDeps struct {
	Orch  outbound.OrchestratorClient
	State outbound.StateStore
	Git   outbound.GitInspector
	Sink  inbound.EventSink
}

// RunnerOptions tunes the polling cadence.
type RunnerOptions struct {
	PollMin time.Duration // default 1s
	PollMax time.Duration // default 5s
}

// RunInput controls Run.
type RunInput struct {
	Project       string
	Message       string
	BaseRef       string
	ArtifactStore domain.ArtifactStoreMode
}

// RunResult reports the terminal outcome.
type RunResult struct {
	ChangeID    domain.ChangeID
	FinalStatus domain.ChangeStatus
}

// Runner orchestrates `sophia run` per spec §2.2 (polling-only V1 in M4).
type Runner struct {
	deps RunnerDeps
	opts RunnerOptions
}

// NewRunner constructs a Runner.
func NewRunner(d RunnerDeps, opts RunnerOptions) *Runner {
	if opts.PollMin <= 0 {
		opts.PollMin = time.Second
	}
	if opts.PollMax <= 0 {
		opts.PollMax = 5 * time.Second
	}
	return &Runner{deps: d, opts: opts}
}

// Run creates a Change and observes it via polling until terminal status.
// Returns RunResult and either nil (DONE) or *ExitError with the spec code.
func (r *Runner) Run(ctx context.Context, in RunInput) (RunResult, error) {
	if in.Message == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: --message required")}
	}
	if in.Project == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: project not set")}
	}
	if in.ArtifactStore == "" {
		in.ArtifactStore = domain.ArtifactStoreEngram
	}
	if in.BaseRef == "" {
		in.BaseRef = "main"
	}

	defer r.deps.Sink.Close() //nolint:errcheck // best-effort

	created, err := r.deps.Orch.CreateChange(ctx, outbound.CreateChangeInput{
		Name:              in.Message,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: string(in.ArtifactStore),
	})
	if err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		return RunResult{}, &ExitError{Code: 3, Err: err}
	}

	res := RunResult{ChangeID: created.ID}
	if err := r.persistChangeID(ctx, in.Project, created.ID); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if err := r.deps.Sink.OnSnapshot(ctx, created); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}

	if created.Status.IsTerminal() {
		return r.finish(ctx, res, created.Status)
	}

	final, err := r.poll(ctx, created.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
		return res, &ExitError{Code: 3, Err: err}
	}
	return r.finish(ctx, res, final)
}

// poll runs the GET-snapshot loop until terminal status.
func (r *Runner) poll(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	delay := r.opts.PollMin
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
		snap, err := r.deps.Orch.GetChange(ctx, id)
		if err != nil {
			return "", err
		}
		if err := r.deps.Sink.OnSnapshot(ctx, snap); err != nil {
			// Sink errors are non-fatal but emitted.
			_ = r.deps.Sink.OnError(ctx, err)
		}
		if snap.Status.IsTerminal() {
			return snap.Status, nil
		}
		// Exponential backoff up to PollMax. Cap at PollMax once reached.
		if snap.CurrentPhaseID == "" {
			delay *= 2
			if delay > r.opts.PollMax {
				delay = r.opts.PollMax
			}
		} else {
			// Active phase → keep at PollMin.
			delay = r.opts.PollMin
		}
	}
}

func (r *Runner) finish(ctx context.Context, res RunResult, st domain.ChangeStatus) (RunResult, error) {
	res.FinalStatus = st
	_ = r.deps.Sink.OnComplete(ctx, st)
	switch st {
	case domain.ChangeStatusDone:
		return res, nil
	case domain.ChangeStatusBlocked, domain.ChangeStatusFailed:
		return res, &ExitError{Code: 1, Err: fmt.Errorf("change ended %s", st)}
	default:
		return res, &ExitError{Code: 4, Err: fmt.Errorf("unexpected non-terminal status %q", st)}
	}
}

func (r *Runner) persistChangeID(ctx context.Context, project string, id domain.ChangeID) error {
	if err := r.deps.State.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo — keep only the global record. Not fatal.
		return nil
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := r.deps.State.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
