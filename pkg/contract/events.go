package contract

// SSE event type constants for sophia-wire-v1 §5.3.
// Restructured per change cli-orch-event-wire-realign.

// =============================================================================
// SECTION 1: Mirrored from sophia-orchestator/internal/ports/inbound/event_types.go
// Adding any event here REQUIRES adding the matching constant in the orch and
// vice-versa. The wire_alignment_test.go enforces this at CI time.
// =============================================================================

const (
	// Phase lifecycle (emitted by application/phase/service.go).
	EventPhaseStarted               = "phase.started"
	EventPhaseCompleted             = "phase.completed"
	EventPhaseCompletedWithConcerns = "phase.completed_with_concerns"
	EventPhaseFailed                = "phase.failed"
	EventPhaseNeedsContext          = "phase.needs_context"

	// Approval.
	EventApprovalRequired = "approval.required"
	EventApprovalResolved = "approval.resolved"

	// Governance.
	EventGovernanceDecision = "governance.decision"

	// Agent lifecycle.
	EventAgentDispatched       = "agent.dispatched"
	EventAgentEnvelopeReceived = "agent.envelope.received"

	// Apply board lifecycle.
	EventApplyBoardCreated    = "apply.board.created"
	EventApplyBoardSaveFailed = "apply.board.save_failed"
	EventApplyWorktreeError   = "apply.worktree.error"

	// Apply group lifecycle.
	EventApplyGroupCompleted = "apply.group.completed"
	EventApplyGroupFailed    = "apply.group.failed"
	EventApplyGroupDegraded  = "apply.group.degraded" // NEW (BUG-30)

	// Apply team-lead / implement / task lifecycle.
	EventApplyTeamLeadSpawned             = "apply.team_lead.spawned"
	EventApplyImplementSpawnFailed        = "apply.implement.spawn_failed"
	EventApplyImplementSpawnGovernorError = "apply.implement.spawn_governor_error"
	EventApplyTaskClaimed                 = "apply.task.claimed"
	EventApplyTaskClaimSkipped            = "apply.task.claim_skipped"
	EventApplyTaskEscalated               = "apply.task.escalated"
	EventApplyTaskRetry                   = "apply.task.retry"

	// Dispatch / envelope errors.
	EventApplyDispatchError            = "apply.dispatch.error"
	EventApplyEnvelopeValidationFailed = "apply.envelope.validation_failed"
	EventRuntimeDispatchFailed         = "runtime.dispatch_failed"

	// Materialize (BUG-29) — NEW.
	EventApplyMaterializeStarted   = "apply.materialize.started"
	EventApplyMaterializeCompleted = "apply.materialize.completed"
	EventApplyMaterializeError     = "apply.materialize.error"

	// Memory integration — NEW.
	EventMemoryArtifactPersistFailed = "memory.artifact_persist_failed"
)

// =============================================================================
// SECTION 2: CLI-only SSE protocol events
// These are NOT in the orch event_types.go. They live here because the CLI's
// SSE client behavior expects them (Heartbeat keepalive, Open connection-live).
// The wire_alignment_test.go allowlists these as "CLI-only".
// =============================================================================

const (
	EventHeartbeat = "heartbeat"
	EventOpen      = "open"
)

// =============================================================================
// SECTION 3: Aspirational events from sophia-wire-v1 spec
// The orch does NOT emit these today. The constants stay declared for
// compile-time references in CLI code (TUI handlers reference them; those
// dormant branches are tracked by follow-up change cli-tui-event-realign).
// =============================================================================

const (
	// TODO(sophia-wire-v1): orch does not emit today; spec-aspirational.
	EventTaskCreated = "task.created"
	// TODO(sophia-wire-v1): orch does not emit today; spec-aspirational.
	EventTaskStarted = "task.started"
	// TODO(sophia-wire-v1): orch does not emit today; spec-aspirational.
	EventTaskCompleted = "task.completed"
	// TODO(sophia-wire-v1): orch does not emit today; spec-aspirational.
	EventTaskFailed = "task.failed"
)

// =============================================================================
// Validation
// =============================================================================

// knownEvents is the canonical set of events recognized by IsKnownEvent.
// Aspirational events (Section 3) are intentionally NOT in this set — orch
// does not emit them, so the CLI treats them as unknown today.
var knownEvents = map[string]struct{}{
	// Section 1: Mirrored from orch.
	EventPhaseStarted:                     {},
	EventPhaseCompleted:                   {},
	EventPhaseCompletedWithConcerns:       {},
	EventPhaseFailed:                      {},
	EventPhaseNeedsContext:                {},
	EventApprovalRequired:                 {},
	EventApprovalResolved:                 {},
	EventGovernanceDecision:               {},
	EventAgentDispatched:                  {},
	EventAgentEnvelopeReceived:            {},
	EventApplyBoardCreated:                {},
	EventApplyBoardSaveFailed:             {},
	EventApplyWorktreeError:               {},
	EventApplyGroupCompleted:              {},
	EventApplyGroupFailed:                 {},
	EventApplyGroupDegraded:               {},
	EventApplyTeamLeadSpawned:             {},
	EventApplyImplementSpawnFailed:        {},
	EventApplyImplementSpawnGovernorError: {},
	EventApplyTaskClaimed:                 {},
	EventApplyTaskClaimSkipped:            {},
	EventApplyTaskEscalated:               {},
	EventApplyTaskRetry:                   {},
	EventApplyDispatchError:               {},
	EventApplyEnvelopeValidationFailed:    {},
	EventRuntimeDispatchFailed:            {},
	EventApplyMaterializeStarted:          {},
	EventApplyMaterializeCompleted:        {},
	EventApplyMaterializeError:            {},
	EventMemoryArtifactPersistFailed:      {},

	// Section 2: CLI-only SSE protocol events (allowlisted by wire_alignment_test.go).
	EventHeartbeat: {},
	EventOpen:      {},
}

// IsKnownEvent reports whether the event type is documented in
// sophia-wire-v1 (required OR optional). Unknown event types should
// be logged + skipped per §10.2.
func IsKnownEvent(eventType string) bool {
	_, ok := knownEvents[eventType]
	return ok
}

// IsRequiredEvent reports whether the given event type is in the
// required-by-spec set. Useful for clients that want to log a warning
// when a required event is malformed (versus silently skipping an
// optional one).
func IsRequiredEvent(eventType string) bool {
	switch eventType {
	case EventHeartbeat,
		EventPhaseStarted,
		EventPhaseCompleted,
		EventPhaseFailed,
		EventApprovalRequired,
		EventApprovalResolved:
		return true
	}
	return false
}

// PhaseStatus values used in PhaseDTO and on phase-related events.
// See sophia-wire-v1 §6.1.
const (
	PhaseStatusPending = "pending"
	PhaseStatusRunning = "running"
	PhaseStatusBlocked = "blocked"
	PhaseStatusDone    = "done"
	PhaseStatusFailed  = "failed"
)

// ChangeStatus values used in ChangeResponse.
// See sophia-wire-v1 §6.1.
const (
	ChangeStatusPending = "pending"
	ChangeStatusRunning = "running"
	ChangeStatusDone    = "done"
	ChangeStatusBlocked = "blocked"
	ChangeStatusFailed  = "failed"
	ChangeStatusAborted = "aborted"
)

// Decision values used in ApprovalResolvedPayload.
const (
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
)
