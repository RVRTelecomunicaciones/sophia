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
//	4 → transient error (stream ended without terminal, ctx canceled, retry budget exhausted)
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
	Orch        outbound.OrchestratorClient
	State       outbound.StateStore
	Git         outbound.GitInspector
	Sink        inbound.EventSink
	EventStream outbound.EventStreamClient
}

// RunnerOptions tunes the runner.
//
// SnapshotRefreshTimeout caps how long the post-stream GetChange may take
// before the runner gives up and reports exit 4.
type RunnerOptions struct {
	SnapshotRefreshTimeout time.Duration
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

// Runner orchestrates `sophia run` per spec §2.2 (SSE-first in M5).
type Runner struct {
	deps RunnerDeps
	opts RunnerOptions
}

// NewRunner constructs a Runner.
func NewRunner(d RunnerDeps, opts RunnerOptions) *Runner {
	if opts.SnapshotRefreshTimeout <= 0 {
		opts.SnapshotRefreshTimeout = 10 * time.Second
	}
	return &Runner{deps: d, opts: opts}
}

// Run creates a Change and observes it via SSE until terminal status.
// Returns RunResult and either nil (DONE) or *ExitError with the spec code.
func (r *Runner) Run(ctx context.Context, in RunInput) (RunResult, error) {
	if in.Message == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: --message required")}
	}
	if in.Project == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: project not set")}
	}
	if r.deps.EventStream == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: event stream not wired")}
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = r.deps.Sink.OnError(context.WithoutCancel(ctx), err)
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
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
		res.FinalStatus = created.Status
	}

	return r.Observe(ctx, res, r.deps.Sink)
}

// Observe drives the post-create observation loop on an existing or just-
// created Change. The caller is responsible for calling OnSnapshot with the
// initial Change snapshot and persisting last_change_id BEFORE calling
// Observe. Observe will subscribe to SSE, dispatch events to the sink, and
// on stream-end refresh the snapshot to determine terminal status.
//
// If res.FinalStatus is already terminal, Observe short-circuits to
// finishWithSink without subscribing.
//
// Returns RunResult with FinalStatus populated. Error is nil on DONE,
// *ExitError on other terminal/transient/ctx-canceled paths.
func (r *Runner) Observe(ctx context.Context, res RunResult, sink inbound.EventSink) (RunResult, error) {
	if res.FinalStatus.IsTerminal() {
		return r.finishWithSink(ctx, res, res.FinalStatus, sink)
	}

	final, err := r.streamWithSink(ctx, res.ChangeID, sink)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = sink.OnError(context.WithoutCancel(ctx), err)
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = sink.OnError(ctx, err)
		return res, &ExitError{Code: 4, Err: err}
	}
	return r.finishWithSink(ctx, res, final, sink)
}

// stream subscribes to the SSE feed for id and forwards events to the sink
// until the channel closes (either graceful server close or retry budget
// exhausted) or ctx is canceled. After the channel closes, it refreshes the
// change snapshot to determine terminal status.
func (r *Runner) stream(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	return r.streamWithSink(ctx, id, r.deps.Sink)
}

func (r *Runner) streamWithSink(ctx context.Context, id domain.ChangeID, sink inbound.EventSink) (domain.ChangeStatus, error) {
	ch, stop, err := r.deps.EventStream.Subscribe(ctx, outbound.StreamTarget{ChangeID: id}, outbound.SubscribeOptions{})
	if err != nil {
		return "", fmt.Errorf("subscribe: %w", err)
	}
	defer stop() //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return r.refreshAfterStreamEndWithSink(ctx, id, sink)
			}
			r.dispatchEventWithSink(ctx, ev, sink)
		}
	}
}

// dispatchEvent forwards a single event to the sink. Heartbeats are dropped
// (defensive — the SSE client also filters them). Approval events get
// translated into OnApprovalGate AND emitted via OnEvent (D-M5-02).
// OnEvent always fires first; OnApprovalGate follows for approval.required.
func (r *Runner) dispatchEvent(ctx context.Context, ev domain.Event) {
	r.dispatchEventWithSink(ctx, ev, r.deps.Sink)
}

func (r *Runner) dispatchEventWithSink(ctx context.Context, ev domain.Event, sink inbound.EventSink) {
	if ev.Type == "heartbeat" {
		return
	}
	if err := sink.OnEvent(ctx, ev); err != nil {
		_ = sink.OnError(ctx, err)
	}
	if ev.Type == "approval.required" {
		gate := approvalGateFromEvent(ev)
		if err := sink.OnApprovalGate(ctx, gate); err != nil {
			_ = sink.OnError(ctx, err)
		}
	}
}

// approvalGateFromEvent extracts an ApprovalGate from a redacted payload.
// Missing fields default to zero values per the parser's tolerance rules.
func approvalGateFromEvent(ev domain.Event) domain.ApprovalGate {
	gate := domain.ApprovalGate{TraceID: ev.TraceID}
	if ev.Payload == nil {
		return gate
	}
	gate.URL, _ = ev.Payload["gate_url"].(string)
	gate.Reason, _ = ev.Payload["reason"].(string)
	gate.Risk, _ = ev.Payload["risk"].(string)
	gate.Policy, _ = ev.Payload["policy"].(string)
	if ph, ok := ev.Payload["phase"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	}
	if cid, ok := ev.Payload["change_id"].(string); ok {
		gate.ChangeID = domain.ChangeID(cid)
	}
	return gate
}

// refreshAfterStreamEnd issues a final GetChange to determine terminal status.
// Per D-M5-03 this is the only place mid-run snapshots happen — not on every
// reconnect.
func (r *Runner) refreshAfterStreamEnd(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	return r.refreshAfterStreamEndWithSink(ctx, id, r.deps.Sink)
}

func (r *Runner) refreshAfterStreamEndWithSink(ctx context.Context, id domain.ChangeID, sink inbound.EventSink) (domain.ChangeStatus, error) {
	rctx, cancel := context.WithTimeout(ctx, r.opts.SnapshotRefreshTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(rctx, id)
	if err != nil {
		return "", fmt.Errorf("post-stream snapshot: %w", err)
	}
	if err := sink.OnSnapshot(ctx, snap); err != nil {
		_ = sink.OnError(ctx, err)
	}
	if !snap.Status.IsTerminal() {
		return "", fmt.Errorf("stream ended before terminal status (current=%q)", snap.Status)
	}
	return snap.Status, nil
}

func (r *Runner) finish(ctx context.Context, res RunResult, st domain.ChangeStatus) (RunResult, error) {
	return r.finishWithSink(ctx, res, st, r.deps.Sink)
}

func (r *Runner) finishWithSink(ctx context.Context, res RunResult, st domain.ChangeStatus, sink inbound.EventSink) (RunResult, error) {
	res.FinalStatus = st
	_ = sink.OnComplete(ctx, st)
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
