package contract

import "time"

// ChangeResponse is the JSON shape returned by POST /api/v1/changes
// and GET /api/v1/changes/{id}, and the `items` element of
// GET /api/v1/changes. See sophia-wire-v1 §6.1.
type ChangeResponse struct {
	ChangeID          string     `json:"change_id"`
	Name              string     `json:"name,omitempty"`
	Project           string     `json:"project,omitempty"`
	BaseRef           string     `json:"base_ref,omitempty"`
	ArtifactStoreMode string     `json:"artifact_store_mode,omitempty"`
	Status            string     `json:"status,omitempty"`
	CurrentPhaseID    string     `json:"current_phase_id,omitempty"`
	Phases            []PhaseDTO `json:"phases,omitempty"`
	CreatedAt         time.Time  `json:"created_at,omitzero"`
	UpdatedAt         time.Time  `json:"updated_at,omitzero"`
}

// PhaseDTO is the embedded phase shape inside ChangeResponse.Phases.
// Standalone phase fetches use PhaseResponse instead.
type PhaseDTO struct {
	PhaseID    string    `json:"phase_id"`
	PhaseType  string    `json:"phase_type"`
	Status     string    `json:"status"`
	Confidence float64   `json:"confidence,omitempty"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	EndedAt    time.Time `json:"ended_at,omitzero"`
}

// PhaseResponse is the JSON shape returned by GET /api/v1/phases/{id}.
// See sophia-wire-v1 §6.2.
type PhaseResponse struct {
	PhaseID       string    `json:"phase_id"`
	ChangeID      string    `json:"change_id"`
	PhaseType     string    `json:"phase_type"`
	Status        string    `json:"status"`
	Confidence    float64   `json:"confidence,omitempty"`
	Attempts      int       `json:"attempts,omitempty"`
	RetryBudget   int       `json:"retry_budget,omitempty"`
	StartedAt     time.Time `json:"started_at,omitzero"`
	EndedAt       time.Time `json:"ended_at,omitzero"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
}

// BoardResponse is the JSON shape returned by GET /api/v1/phases/{id}/board.
// See sophia-wire-v1 §6.3.
type BoardResponse struct {
	BoardID string       `json:"board_id"`
	PhaseID string       `json:"phase_id"`
	Status  string       `json:"status"`
	Groups  []BoardGroup `json:"groups,omitempty"`
}

// BoardGroup is one apply-phase coordination group.
type BoardGroup struct {
	GroupID string      `json:"group_id"`
	Name    string      `json:"name"`
	Status  string      `json:"status"`
	Tasks   []BoardTask `json:"tasks,omitempty"`
}

// BoardTask is one apply-phase task.
type BoardTask struct {
	TaskID string       `json:"task_id"`
	Name   string       `json:"name"`
	Status string       `json:"status"`
	Agents []BoardAgent `json:"agents,omitempty"`
}

// BoardAgent is one dispatched agent.
type BoardAgent struct {
	AgentID string `json:"agent_id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
}

// CreateChangeRequest is the JSON request body for POST /api/v1/changes.
type CreateChangeRequest struct {
	Name              string `json:"name"`
	Project           string `json:"project"`
	BaseRef           string `json:"base_ref,omitempty"`
	ArtifactStoreMode string `json:"artifact_store_mode,omitempty"`
}

// AbortChangeRequest is the JSON request body for POST /api/v1/changes/{id}/abort.
type AbortChangeRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ApprovalDecisionRequest is the JSON request body for both
// POST /api/v1/phases/{id}/approve and POST /api/v1/phases/{id}/reject.
type ApprovalDecisionRequest struct {
	Approver string `json:"approver"`
	Reason   string `json:"reason,omitempty"`
}

// ListChangesResponse is the JSON shape returned by GET /api/v1/changes.
type ListChangesResponse struct {
	Items  []ChangeResponse `json:"items"`
	Total  int              `json:"total,omitempty"`
	Limit  int              `json:"limit,omitempty"`
	Offset int              `json:"offset,omitempty"`
}

// HealthResponse is the JSON shape returned by GET /api/v1/health.
type HealthResponse struct {
	Status        string    `json:"status"`
	Version       string    `json:"version,omitempty"`
	UptimeSeconds int64     `json:"uptime_seconds,omitempty"`
	CheckedAt     time.Time `json:"checked_at,omitzero"`
}

// ReadyResponse is the JSON shape returned by GET /api/v1/ready.
type ReadyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
	Error  string            `json:"error,omitempty"`
}

// ErrorResponse is the canonical error envelope per sophia-wire-v1 §9.1.
type ErrorResponse struct {
	Code    string         `json:"code"`
	Error   string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

// ApprovalRequiredPayload is the JSON shape of the `approval.required`
// SSE event payload. Sophia-wire-v1 §5.3 + Phase 1.5 amendment: Risk
// and Policy are Optional; the orchestrator MAY emit them when the
// underlying governance decision carries the data.
type ApprovalRequiredPayload struct {
	PhaseID string `json:"phase_id"`
	GateURL string `json:"gate_url"`
	Reason  string `json:"reason,omitempty"`
	Risk    string `json:"risk,omitempty"`
	Policy  string `json:"policy,omitempty"`
}

// ApprovalResolvedPayload is the JSON shape of the `approval.resolved`
// SSE event payload.
type ApprovalResolvedPayload struct {
	PhaseID   string    `json:"phase_id"`
	Decision  string    `json:"decision"` // "approved" | "rejected"
	Approver  string    `json:"approver"`
	Reason    string    `json:"reason,omitempty"`
	DecidedAt time.Time `json:"decided_at,omitzero"`
}

// PhaseLifecyclePayload is the JSON shape of `phase.started`,
// `phase.completed`, `phase.failed` events.
type PhaseLifecyclePayload struct {
	PhaseID    string    `json:"phase_id"`
	PhaseType  string    `json:"phase_type"`
	Status     string    `json:"status,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	EndedAt    time.Time `json:"ended_at,omitzero"`
	Error      string    `json:"error,omitempty"`
}

// HeartbeatPayload is the JSON shape of `heartbeat` events.
type HeartbeatPayload struct {
	Timestamp time.Time `json:"ts"`
}

// OpenPayload is the JSON shape of the `open` event sent at SSE stream
// connection establishment (Phase 1.5 amendment).
type OpenPayload struct {
	PhaseID string `json:"phase_id"`
}
