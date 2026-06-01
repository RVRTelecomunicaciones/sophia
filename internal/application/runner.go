package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
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
		in.ArtifactStore = domain.ArtifactStoreMemoryEngine
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
		return r.Observe(ctx, res, r.deps.Sink)
	}

	// Kick off the first SDD phase (explore) so the operator gets a
	// real cycle from a single `sophia run "..."` invocation. Before
	// this, run only created the change and the observer immediately
	// failed with "snapshot has no current_phase_id" because nothing
	// started a phase. The message becomes the task description for
	// the explore prompt.
	//
	// Failures here are surfaced as ExitError(3) — they're not the
	// observer's fault and there's nothing to observe if the phase
	// never started.
	if _, err := r.deps.Orch.RunPhase(ctx, created.ID, "explore", outbound.RunPhaseInput{
		TaskDescription: in.Message,
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = r.deps.Sink.OnError(context.WithoutCancel(ctx), err)
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
		return res, &ExitError{Code: 3, Err: fmt.Errorf("run: trigger explore: %w", err)}
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

// streamWithSink drives the per-Change observation loop. It is the
// phase-stream multiplexer (Phase 4 Task 4.3 / D-M10-05): the SSE
// transport is per-phase, but the Change moves through phases over its
// lifetime. The multiplexer:
//
//  1. fetches the Change snapshot to learn current_phase_id;
//  2. subscribes to /api/v1/phases/{current_phase_id}/events;
//  3. forwards events to the sink until the stream ends;
//  4. refreshes the snapshot;
//  5. if the Change is terminal → finish;
//  6. if current_phase_id changed → re-subscribe to the new phase
//     (loop back to step 3);
//  7. if it didn't change but the change is non-terminal → bail out
//     with a "stream ended before terminal" error.
//
// 410 phase_terminal_no_events from the orchestrator (sophia-wire-v1
// §9.2) is observed as a closed channel; the snapshot path then drives
// the next decision (advance to a new phase, or finish).
// maxResubscribeAttempts caps how many times the run observer will
// re-subscribe to the SAME phase after an SSE drop while the phase is
// still running. A slow LLM call can easily outlive the upstream SSE
// keepalive (validated 2026-05-19: gpt-5.4 on SDD prompts ~110-237s,
// claude-opus on complex tasks 5-10min). Without re-subscription, the
// observer bailed with "stream ended before terminal status" and the
// CLI returned exit 4 even though the work completed fine.
//
// The cap exists to bound a genuine pathology: SSE handshake works
// every time but the orch never moves the phase. After this many
// consecutive non-terminal GetPhase results we treat it as an upstream
// fault and surface the original error so the operator notices.
const maxResubscribeAttempts = 12

func (r *Runner) streamWithSink(ctx context.Context, id domain.ChangeID, sink inbound.EventSink) (domain.ChangeStatus, error) {
	currentPhase := ""
	// lastPhaseTerminalStatus captures the envelope_status of the most
	// recent `phase.*` terminal event observed on the SSE stream
	// (DONE / DONE_WITH_CONCERNS / DONE_WITH_REJECTIONS / BLOCKED /
	// NEEDS_CONTEXT / FAILED / TIMED_OUT). Used as the fallback when
	// the stream ends, the change has NOT advanced, and no further
	// phases are queued — interpret the last phase outcome as the
	// run's outcome rather than erroring out.
	lastPhaseTerminalStatus := ""
	// resubscribeAttempts counts how many times we re-subscribed to
	// THE SAME currentPhase after an SSE drop while the phase was
	// still running. Resets to 0 when currentPhase advances. Capped
	// by maxResubscribeAttempts to bound runaway re-subscription if
	// the orch is misbehaving.
	resubscribeAttempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Resolve the phase to subscribe to. On the first iteration we
		// fetch a fresh snapshot; on subsequent iterations the previous
		// snapshot decision (below) already updated currentPhase.
		if currentPhase == "" {
			snap, err := r.snapshotChange(ctx, id)
			if err != nil {
				return "", err
			}
			if err := sink.OnSnapshot(ctx, snap); err != nil {
				_ = sink.OnError(ctx, err)
			}
			if snap.Status.IsTerminal() {
				return snap.Status, nil
			}
			if snap.CurrentPhaseID == "" {
				return "", fmt.Errorf("snapshot has no current_phase_id (status=%q)", snap.Status)
			}
			currentPhase = snap.CurrentPhaseID
		}

		ch, stop, err := r.deps.EventStream.Subscribe(ctx, outbound.StreamTarget{
			ChangeID: id, PhaseID: currentPhase,
		}, outbound.SubscribeOptions{})
		if err != nil {
			return "", fmt.Errorf("subscribe: %w", err)
		}

		// Drain events until the channel closes or ctx ends.
		streamLoop := func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ev, ok := <-ch:
					if !ok {
						return nil // stream ended; outer loop refreshes
					}
					if ts := phaseTerminalStatusFromEvent(ev); ts != "" {
						lastPhaseTerminalStatus = ts
					}
					r.dispatchEventWithSink(ctx, ev, sink)
				}
			}
		}
		err = streamLoop()
		_ = stop()
		if err != nil {
			return "", err
		}

		// Stream ended → snapshot to decide: terminal? phase advanced?
		snap, snapErr := r.snapshotChange(ctx, id)
		if snapErr != nil {
			return "", snapErr
		}
		if err := sink.OnSnapshot(ctx, snap); err != nil {
			_ = sink.OnError(ctx, err)
		}
		if snap.Status.IsTerminal() {
			return snap.Status, nil
		}
		if snap.CurrentPhaseID != "" && snap.CurrentPhaseID != currentPhase {
			currentPhase = snap.CurrentPhaseID
			// Phase advanced — reset re-subscription budget so the
			// new phase gets a fresh allowance.
			resubscribeAttempts = 0
			continue
		}
		// The change is still "active" (multi-phase pipeline hasn't
		// advanced to a new phase) but the SSE stream we were
		// subscribed to ended. Two real cases:
		//
		//   (a) The phase we observed reached a terminal outcome
		//       (DONE / DONE_WITH_* / BLOCKED / NEEDS_CONTEXT /
		//       FAILED / TIMED_OUT) and nothing auto-advances the
		//       change to the next phase. This is the common case for
		//       `sophia run "..."` which kicks off ONE phase and
		//       expects the run to finish with that phase's outcome.
		//   (b) The stream dropped without a terminal event AND the
		//       orch didn't advance. This is genuinely abnormal —
		//       preserve the original error so the operator notices.
		//
		// We disambiguate via lastPhaseTerminalStatus captured from
		// the SSE events themselves first. If that was empty (common
		// when a fast-completing phase finished before the SSE
		// subscription delivered any events — the orch then returns
		// 410 phase_terminal_no_events to the late subscriber, the
		// client sees a closed channel with no payload), fall back
		// to a direct GET /api/v1/phases/{id} on the phase we were
		// observing. That endpoint surfaces the orch's authoritative
		// terminal status regardless of SSE delivery timing.
		if mapped := mapPhaseTerminalToChange(lastPhaseTerminalStatus); mapped != "" {
			return mapped, nil
		}
		if currentPhase != "" {
			ph, phErr := r.deps.Orch.GetPhase(ctx, currentPhase)
			if phErr == nil && ph != nil {
				if mapped := mapPhaseLowercaseTerminalToChange(ph.Status); mapped != "" {
					return mapped, nil
				}
				// Phase is NOT terminal yet (typically still "running"
				// or "interrupted"). The SSE stream dropped before
				// the LLM finished — common when the dispatcher call
				// outlives the upstream stream keepalive (slow SDD
				// prompts on opus/gpt-5 can run 2-10 minutes).
				// Re-subscribe to the same phase rather than bailing.
				// Capped by maxResubscribeAttempts to avoid an infinite
				// loop if the orch is wedged.
				if isPhaseInFlight(ph.Status) && resubscribeAttempts < maxResubscribeAttempts {
					resubscribeAttempts++
					_ = sink.OnError(ctx, fmt.Errorf("re-subscribing to phase %s (attempt %d/%d, phase still %s)",
						currentPhase, resubscribeAttempts, maxResubscribeAttempts, ph.Status))
					continue
				}
			}
		}
		return "", fmt.Errorf("stream ended before terminal status (current=%q)", snap.Status)
	}
}

// isPhaseInFlight reports whether a GET /api/v1/phases/{id} status
// indicates the phase is NOT terminal yet — i.e. re-subscribing to
// SSE for it can still yield events. "interrupted" is a non-terminal
// state in the orch's enum (a phase awaiting manual resume) but
// counts as in-flight because the orch may resume it asynchronously.
func isPhaseInFlight(phaseStatus string) bool {
	switch phaseStatus {
	case "pending", "running", "interrupted":
		return true
	}
	return false
}

// mapPhaseLowercaseTerminalToChange is the GetPhase counterpart of
// mapPhaseTerminalToChange. It maps a canonical phase status (from
// GET /api/v1/phases/{id}) to a ChangeStatus for the fallback path
// when the SSE stream ends before a terminal event is received.
//
// Canonical phase status set (sophia-wire-v1 §6.1, 7 values):
//
//	pending, running, done, done_with_concerns, blocked, needs_context, interrupted
//
// Important: "failed" is NOT a phase status — it is the phase.failed
// SSE event (§5.3). When a phase fails, its persisted status is "blocked".
// Mapping "blocked" to ChangeStatusBlocked here is intentional and correct:
// a late subscriber cannot distinguish a hard failure from an approval wait
// using GET /phases/{id} alone. The event-driven path (mapPhaseTerminalToChange
// via phaseTerminalStatusFromEvent) is the only reliable way to surface
// ChangeStatusFailed — it requires observing the phase.failed SSE event.
func mapPhaseLowercaseTerminalToChange(phaseStatus string) domain.ChangeStatus {
	switch phaseStatus {
	case "done", "done_with_concerns":
		return domain.ChangeStatusDone
	case "blocked", "needs_context":
		return domain.ChangeStatusBlocked
	}
	return ""
}

// phaseTerminalStatusFromEvent returns the envelope_status string when
// ev is a terminal phase lifecycle event, "" otherwise. The orch emits
// phase.completed / phase.completed_with_concerns / phase.failed /
// phase.needs_context (sophia-wire-v1 §5.3) with payload field
// `envelope_status` carrying the underlying envelope.Status value.
func phaseTerminalStatusFromEvent(ev domain.Event) string {
	switch ev.Type {
	case "phase.completed", "phase.completed_with_concerns",
		"phase.failed", "phase.needs_context":
		// payload may be nil if the orch redacted it; nothing to do.
		if ev.Payload == nil {
			return ""
		}
		if s, ok := ev.Payload["envelope_status"].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// mapPhaseTerminalToChange translates an envelope.Status string into
// the corresponding ChangeStatus for finishWithSink to map to an exit
// code. Returns "" when the input is empty or unrecognized (caller
// falls back to the defensive "stream ended" error).
func mapPhaseTerminalToChange(envelopeStatus string) domain.ChangeStatus {
	switch envelopeStatus {
	case "DONE", "DONE_WITH_CONCERNS", "DONE_WITH_REJECTIONS":
		// All three success-ish outcomes map to exit 0. The CLI
		// preserves the distinction via OnEvent (the operator sees
		// the original envelope_status in the JSON stream).
		return domain.ChangeStatusDone
	case "BLOCKED", "NEEDS_CONTEXT":
		// Phase couldn't continue without operator input — surface as
		// blocked so the exit code (1) signals "needs attention",
		// distinct from a hard failure.
		return domain.ChangeStatusBlocked
	case "FAILED", "TIMED_OUT":
		return domain.ChangeStatusFailed
	}
	return ""
}

// snapshotChange wraps a bounded GetChange call. Uses
// SnapshotRefreshTimeout so a hung orchestrator doesn't block the
// multiplexer indefinitely between phases.
func (r *Runner) snapshotChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	rctx, cancel := context.WithTimeout(ctx, r.opts.SnapshotRefreshTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(rctx, id)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	return snap, nil
}

// dispatchEventWithSink forwards a single event to the given sink. Heartbeats
// are dropped (defensive — the SSE client also filters them). Approval events
// get translated into OnApprovalGate AND emitted via OnEvent (D-M5-02).
// OnEvent always fires first; OnApprovalGate follows for approval.required.
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

// finishWithSink emits OnComplete to the given sink and maps the terminal
// status to the spec §2.3 ExitError code.
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
	return persistChangeID(ctx, r.deps.State, r.deps.Git, project, id)
}

// persistChangeID writes id to global last_change_id and, when in a git repo
// with a non-empty project name, also to the project-scoped record. Used by
// both Runner.Run (after CreateChange) and Attacher.Attach (after GetChange)
// per spec §3.5. Outside a repo or with empty project, only the global
// record is updated and a nil error is returned (best-effort).
func persistChangeID(ctx context.Context, state outbound.StateStore, git outbound.GitInspector, project string, id domain.ChangeID) error {
	if err := state.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := git.RepoRoot(ctx, ".")
	if err != nil {
		return nil // outside a repo — global-only is fine
	}
	if project == "" {
		return nil // no project context — global-only is fine
	}
	remote, _ := git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := state.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
