package contract

// Stable error codes for sophia-wire-v1 §9.2. Both client and server
// MUST switch on these constants rather than the human-readable
// `error` field, which may vary across versions.
//
// Adding new codes is non-breaking provided clients use a default
// branch on unknown codes (e.g. exit 3 with the raw code printed).
// Removing or renaming codes IS breaking — must wait for v2.
const (
	// Auth.
	CodeUnauthorized = "unauthorized"

	// Validation / request shape.
	CodeValidationFailed = "validation_failed"
	CodeApproverRequired = "approver_required"
	CodeLimitTooLarge    = "limit_too_large"

	// Resource not found (per-resource specificity per spec §9.2).
	CodeChangeNotFound = "change_not_found"
	CodePhaseNotFound  = "phase_not_found"

	// Resource conflict / state.
	CodeChangeAlreadyExists   = "change_already_exists"
	CodeChangeAlreadyTerminal = "change_already_terminal"
	CodePhaseNotResumable     = "phase_not_resumable"
	CodePhaseNotGated         = "phase_not_gated"
	CodeGateAlreadyDecided    = "gate_already_decided"
	CodePhaseTerminalNoEvents = "phase_terminal_no_events"

	// Server.
	CodeInternalError = "internal_error"
)

// IsKnownCode reports whether the given code string is one of the v1
// stable codes. False for any code the server may emit beyond the
// documented set (forward-compat: clients fall through to a default
// "exit 3 + print code" handler).
func IsKnownCode(code string) bool {
	switch code {
	case CodeUnauthorized,
		CodeValidationFailed,
		CodeApproverRequired,
		CodeLimitTooLarge,
		CodeChangeNotFound,
		CodePhaseNotFound,
		CodeChangeAlreadyExists,
		CodeChangeAlreadyTerminal,
		CodePhaseNotResumable,
		CodePhaseNotGated,
		CodeGateAlreadyDecided,
		CodePhaseTerminalNoEvents,
		CodeInternalError:
		return true
	}
	return false
}
