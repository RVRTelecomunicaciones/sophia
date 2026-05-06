package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// AttacherDeps groups the ports the Attacher needs.
//
// Runner is the M8 shared observation engine. Attacher delegates the
// stream/refresh/finish loop to Runner.Observe; it owns only the GetChange +
// persistChangeID + initial OnSnapshot prelude.
type AttacherDeps struct {
	Orch   outbound.OrchestratorClient
	State  outbound.StateStore
	Git    outbound.GitInspector
	Runner *Runner
}

// AttachInput carries the user-facing inputs for `sophia attach`.
//
// Project is used solely for persisting project-scoped last_change_id (spec
// §3.5). It's optional: when empty (e.g. attaching outside a repo or with a
// changed cwd), Attacher persists only the global last_change_id.
type AttachInput struct {
	ChangeID domain.ChangeID
	Project  string
}

// Attacher implements `sophia attach`.
type Attacher struct {
	deps AttacherDeps
}

// NewAttacher constructs an Attacher.
func NewAttacher(d AttacherDeps) *Attacher { return &Attacher{deps: d} }

// Attach fetches the snapshot, then delegates to AttachFromSnapshot. The sink
// argument lets the CLI pick TUI bridge vs JSONL at command time, just like
// RunnerFactory does for `run`.
//
// Errors map to spec §2.3 exit codes:
//
//   - empty ChangeID                       → exit 3
//   - GetChange ErrChangeNotFound (404)    → exit 3
//   - GetChange transport / 5xx            → exit 3 (orchestrator unreachable)
//   - GetChange ctx canceled / deadline    → exit 4
//   - Observe exit codes (0/1/4)           → forwarded as-is
func (a *Attacher) Attach(ctx context.Context, in AttachInput, sink inbound.EventSink) (RunResult, error) {
	if a.deps.Runner == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: runner not wired")}
	}
	if in.ChangeID.IsZero() {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: change-id required")}
	}

	snap, err := a.deps.Orch.GetChange(ctx, in.ChangeID)
	if err != nil {
		// sink.Close is the caller's responsibility on the AttachFromSnapshot
		// path; here we close on the GetChange-failure branch only.
		defer sink.Close() //nolint:errcheck
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = sink.OnError(context.WithoutCancel(ctx), err)
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		_ = sink.OnError(ctx, err)
		return RunResult{}, &ExitError{Code: 3, Err: fmt.Errorf("get change: %w", err)}
	}

	return a.AttachFromSnapshot(ctx, snap, in.Project, sink)
}

// AttachFromSnapshot is the second entry point for `sophia attach`. It accepts
// a pre-fetched snapshot — the CLI's `attachJSONL` uses this to do its own
// `GetChange`, scan for `PhaseStatusBlocked` to eager-arm `approvalTimeoutSink`
// (D-M8-13), and only then hand the snapshot to the Attacher. This avoids a
// double `GetChange` round-trip while preserving the "approval-timeout starts
// at attach time" guarantee.
//
// Behavior matches Attach minus the GetChange + 404/timeout handling:
//
//  1. persistChangeID (spec §3.5)
//  2. sink.OnSnapshot
//  3. Runner.Observe
func (a *Attacher) AttachFromSnapshot(ctx context.Context, snap *domain.Change, project string, sink inbound.EventSink) (RunResult, error) {
	if a.deps.Runner == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: runner not wired")}
	}
	if snap == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: snapshot required")}
	}

	defer sink.Close() //nolint:errcheck // best-effort

	res := RunResult{ChangeID: snap.ID}

	if err := a.persistChangeID(ctx, project, snap.ID); err != nil {
		// Persistence failure is NOT fatal — surface as a sink error and continue.
		_ = sink.OnError(ctx, err)
	}
	if err := sink.OnSnapshot(ctx, snap); err != nil {
		_ = sink.OnError(ctx, err)
	}

	if snap.Status.IsTerminal() {
		// Prime res.FinalStatus so Observe takes the short-circuit branch.
		res.FinalStatus = snap.Status
	}

	return a.deps.Runner.Observe(ctx, res, sink)
}

// persistChangeID mirrors Runner.persistChangeID exactly (spec §3.5).
// Duplicated rather than shared because Runner.persistChangeID is unexported
// and the shape is small enough that exposing it would overgeneralize the
// API.
func (a *Attacher) persistChangeID(ctx context.Context, project string, id domain.ChangeID) error {
	if err := a.deps.State.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := a.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo — keep only the global record. Not fatal.
		return nil
	}
	if project == "" {
		return nil
	}
	remote, _ := a.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := a.deps.State.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
