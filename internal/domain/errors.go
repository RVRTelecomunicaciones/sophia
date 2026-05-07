package domain

import "errors"

// Local CLI sentinels. Pre-existing; kept stable.
var (
	ErrConfigMissing  = errors.New("config missing")
	ErrChangeNotFound = errors.New("change not found")
	ErrUnreachable    = errors.New("orchestrator unreachable")
	ErrInvalidYAML    = errors.New("invalid yaml")
	ErrNotARepo       = errors.New("not a git repository")
	ErrInvalidURL     = errors.New("invalid url")
)

// Wire-protocol sentinels covering the 13 stable error codes of
// sophia-wire-v1 §9.2. The orchestratorhttp adapter maps the canonical
// `code` field of each ErrorResponse into these sentinels so callers can
// match via errors.Is without parsing strings.
//
// ErrChangeNotFound (above) is the domain sentinel for code
// `change_not_found`; ErrUnreachable covers the network/5xx case and is
// distinct from `internal_error`.
var (
	ErrUnauthorized          = errors.New("unauthorized")
	ErrValidationFailed      = errors.New("validation failed")
	ErrApproverRequired      = errors.New("approver required")
	ErrLimitTooLarge         = errors.New("limit too large")
	ErrPhaseNotFound         = errors.New("phase not found")
	ErrChangeAlreadyExists   = errors.New("change already exists")
	ErrChangeAlreadyTerminal = errors.New("change already terminal")
	ErrPhaseNotResumable     = errors.New("phase not resumable")
	ErrPhaseNotGated         = errors.New("phase not gated")
	ErrGateAlreadyDecided    = errors.New("gate already decided")
	ErrPhaseTerminalNoEvents = errors.New("phase terminal — no further events")
	ErrInternal              = errors.New("orchestrator internal error")
	ErrAuthRequired          = errors.New("auth required for remote orchestrator (set SOPHIA_API_KEY or --api-key)")
)
