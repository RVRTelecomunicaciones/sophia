package contract

// SSE event type constants for sophia-wire-v1 §5.3.
//
// Required events MUST be emitted by spec-conformant servers and
// MUST be handled by spec-conformant clients. Optional events MAY
// be emitted; clients MUST tolerate their absence and MUST NOT
// fail on their presence.
//
// Phase 1.5 amendment (2026-05-07): the orch-internal extension
// events (PhaseCompletedWithConcerns, PhaseNeedsContext,
// AgentEnvelopeReceived) and the apply.* family are documented
// here as Optional so that the CLI can recognize them without
// "unknown event type" log noise.
const (
	// Required events.
	EventHeartbeat        = "heartbeat"
	EventPhaseStarted     = "phase.started"
	EventPhaseCompleted   = "phase.completed"
	EventPhaseFailed      = "phase.failed"
	EventApprovalRequired = "approval.required"
	EventApprovalResolved = "approval.resolved"

	// Optional task / agent events.
	EventTaskCreated     = "task.created"
	EventTaskStarted     = "task.started"
	EventTaskCompleted   = "task.completed"
	EventTaskFailed      = "task.failed"
	EventAgentDispatched = "agent.dispatched"
	EventAgentCompleted  = "agent.completed"

	// Optional connection-live signal (server emits at SSE stream open).
	EventOpen = "open"

	// Optional orch-internal extensions (Phase 1.5 amendment).
	EventPhaseCompletedWithConcerns = "phase.completed_with_concerns"
	EventPhaseNeedsContext          = "phase.needs_context"
	EventAgentEnvelopeReceived      = "agent.envelope.received"

	// Optional apply.* diagnostic family (Phase 1.5 amendment).
	EventApplyBoardCreated    = "apply.board.created"
	EventApplyGroupCompleted  = "apply.group.completed"
	EventApplyGroupFailed     = "apply.group.failed"
	EventApplyBoardSaveFailed = "apply.board.save_failed"
	EventApplyWorktreeError   = "apply.worktree.error"

	// Optional team-lead + implement-agent + per-task lifecycle events.
	// Mirror of sophia-orchestrator internal/ports/inbound/event_types.go
	// — adding any new event there REQUIRES mirroring here so the CLI's
	// IsKnownEvent recognises it (otherwise the consumer logs "unknown
	// event type" noise on every occurrence).
	EventApplyTeamLeadSpawned             = "apply.team_lead.spawned"
	EventApplyImplementSpawnFailed        = "apply.implement.spawn_failed"
	EventApplyImplementSpawnGovernorError = "apply.implement.spawn_governor_error"
	EventApplyTaskClaimed                 = "apply.task.claimed"
	EventApplyTaskClaimSkipped            = "apply.task.claim_skipped"
	EventApplyTaskEscalated               = "apply.task.escalated"
	EventApplyTaskRetry                   = "apply.task.retry"
	EventApplyDispatchError               = "apply.dispatch.error"
	EventApplyEnvelopeValidationFailed    = "apply.envelope.validation_failed"
	EventRuntimeDispatchFailed            = "runtime.dispatch_failed"
	EventGovernanceDecision               = "governance.decision"
)

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

// IsKnownEvent reports whether the event type is documented in
// sophia-wire-v1 (required OR optional). Unknown event types should
// be logged + skipped per §10.2.
func IsKnownEvent(eventType string) bool {
	switch eventType {
	case EventHeartbeat,
		EventPhaseStarted,
		EventPhaseCompleted,
		EventPhaseFailed,
		EventApprovalRequired,
		EventApprovalResolved,
		EventTaskCreated,
		EventTaskStarted,
		EventTaskCompleted,
		EventTaskFailed,
		EventAgentDispatched,
		EventAgentCompleted,
		EventOpen,
		EventPhaseCompletedWithConcerns,
		EventPhaseNeedsContext,
		EventAgentEnvelopeReceived,
		EventApplyBoardCreated,
		EventApplyGroupCompleted,
		EventApplyGroupFailed,
		EventApplyBoardSaveFailed,
		EventApplyWorktreeError,
		EventApplyTeamLeadSpawned,
		EventApplyImplementSpawnFailed,
		EventApplyImplementSpawnGovernorError,
		EventApplyTaskClaimed,
		EventApplyTaskClaimSkipped,
		EventApplyTaskEscalated,
		EventApplyTaskRetry,
		EventApplyDispatchError,
		EventApplyEnvelopeValidationFailed,
		EventRuntimeDispatchFailed,
		EventGovernanceDecision:
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
