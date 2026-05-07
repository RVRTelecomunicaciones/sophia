package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
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

// Attach observes an existing Change through to terminal status. Validates
// changeID, fetches the snapshot via OrchestratorClient.GetChange, then
// delegates to AttachFromSnapshot. Returns RunResult and either nil (DONE)
// or *ExitError with the spec §2.3 code (3 for ChangeNotFound / unreachable
// / wiring error, 4 for ctx-cancel / deadline, 1 for BLOCKED/FAILED).
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
		// path; here we close on the GetChange-failure branch only. Both
		// branches return immediately, so direct calls beat defer.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = sink.OnError(context.WithoutCancel(ctx), err)
			_ = sink.Close()
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		_ = sink.OnError(ctx, err)
		_ = sink.Close()
		return RunResult{}, &ExitError{Code: 3, Err: fmt.Errorf("get change: %w", err)}
	}

	return a.AttachFromSnapshot(ctx, snap, in.Project, sink)
}

// AttachFromSnapshot is the second entry point used by cli.attachJSONL
// (D-M8-13) to skip a redundant GetChange when the CLI must scan the
// snapshot for blocked phases and eager-arm approvalTimeoutSink before
// observation begins. Owns sink.Close on every exit path; callers MUST NOT
// double-close.
func (a *Attacher) AttachFromSnapshot(ctx context.Context, snap *domain.Change, project string, sink inbound.EventSink) (RunResult, error) {
	if a.deps.Runner == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: runner not wired")}
	}
	if snap == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: snapshot required")}
	}

	defer sink.Close() //nolint:errcheck // best-effort

	res := RunResult{ChangeID: snap.ID}

	if err := persistChangeID(ctx, a.deps.State, a.deps.Git, project, snap.ID); err != nil {
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
