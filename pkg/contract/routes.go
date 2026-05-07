package contract

// Route path constants for sophia-wire-v1 §4. Both client and server
// MUST use these constants instead of hard-coded strings.
//
// Path placeholders (`{change_id}`, `{phase_id}`, `{phase_type}`)
// follow chi's URL-param syntax. Clients build full paths via the
// helper functions below; servers route via chi.Route trees that
// match the same shapes.
const (
	// Health + readiness (un-authenticated, sophia-wire-v1 §4.1).
	RouteHealth = "/api/v1/health"
	RouteReady  = "/api/v1/ready"

	// Changes (authenticated unless anon-loopback, sophia-wire-v1 §4.2).
	RouteChanges          = "/api/v1/changes"
	RouteChangeByID       = "/api/v1/changes/{change_id}"
	RouteChangeAbort      = "/api/v1/changes/{change_id}/abort"
	RouteChangePhaseRun   = "/api/v1/changes/{change_id}/phases/{phase_type}/run"

	// Phases — phase-scoped per D-M10-13 Form A (sophia-wire-v1 §4.3).
	RoutePhaseByID    = "/api/v1/phases/{phase_id}"
	RoutePhaseResume  = "/api/v1/phases/{phase_id}/resume"
	RoutePhaseApprove = "/api/v1/phases/{phase_id}/approve"
	RoutePhaseReject  = "/api/v1/phases/{phase_id}/reject"
	RoutePhaseBoard   = "/api/v1/phases/{phase_id}/board"
	RoutePhaseEvents  = "/api/v1/phases/{phase_id}/events"

	// Reserved for v0.3.0+ (Form B from D-M10-13). NOT implemented in v1.
	// Listed so a future v2 client can detect server support without
	// hard-coding strings.
	RouteApprovalApproveReserved = "/api/v1/approvals/{gate_id}/approve"
	RouteApprovalRejectReserved  = "/api/v1/approvals/{gate_id}/reject"
)

// ChangePath returns the full URL path for `GET /api/v1/changes/{id}`.
// Uses string concatenation rather than fmt.Sprintf to avoid pulling
// fmt into the contract package's dependency closure.
func ChangePath(changeID string) string {
	return "/api/v1/changes/" + changeID
}

// ChangeAbortPath returns the path for `POST /changes/{id}/abort`.
func ChangeAbortPath(changeID string) string {
	return "/api/v1/changes/" + changeID + "/abort"
}

// ChangePhaseRunPath returns the path for `POST /changes/{id}/phases/{type}/run`.
func ChangePhaseRunPath(changeID, phaseType string) string {
	return "/api/v1/changes/" + changeID + "/phases/" + phaseType + "/run"
}

// PhasePath returns the path for `GET /api/v1/phases/{id}`.
func PhasePath(phaseID string) string {
	return "/api/v1/phases/" + phaseID
}

// PhaseResumePath returns the path for `POST /api/v1/phases/{id}/resume`.
func PhaseResumePath(phaseID string) string {
	return "/api/v1/phases/" + phaseID + "/resume"
}

// PhaseApprovePath returns the path for `POST /api/v1/phases/{id}/approve`.
func PhaseApprovePath(phaseID string) string {
	return "/api/v1/phases/" + phaseID + "/approve"
}

// PhaseRejectPath returns the path for `POST /api/v1/phases/{id}/reject`.
func PhaseRejectPath(phaseID string) string {
	return "/api/v1/phases/" + phaseID + "/reject"
}

// PhaseBoardPath returns the path for `GET /api/v1/phases/{id}/board`.
func PhaseBoardPath(phaseID string) string {
	return "/api/v1/phases/" + phaseID + "/board"
}

// PhaseEventsPath returns the path for `GET /api/v1/phases/{id}/events`.
func PhaseEventsPath(phaseID string) string {
	return "/api/v1/phases/" + phaseID + "/events"
}
