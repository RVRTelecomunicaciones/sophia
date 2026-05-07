package orchestratorhttp

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// StatusError is returned by orchestratorhttp methods on non-2xx
// responses. It carries the HTTP status, the canonical error envelope
// fields (sophia-wire-v1 §9.1), and the raw body for diagnostics.
//
// errors.Is(err, domain.Err*) maps the wire `code` field to the matching
// domain sentinel. Two fallback paths cover orchestrators that pre-date
// or mis-implement the envelope: 5xx without a code maps to
// domain.ErrUnreachable; non-2xx without parseable JSON maps the HTTP
// status to the closest sentinel.
type StatusError struct {
	StatusCode int
	Code       string         // sophia-wire-v1 §9.2 stable code, "" when body unparseable
	Message    string         // envelope `error` field
	Details    map[string]any // envelope `details` (may be nil)
	Body       []byte         // raw body — capped at maxBodySize upstream
}

// NewStatusError parses body as a contract.ErrorResponse and returns a
// populated StatusError. When body is empty or unparseable, only
// StatusCode + Body are set; the upstream errors.Is fallback uses
// StatusCode for mapping.
func NewStatusError(code int, body []byte) *StatusError {
	se := &StatusError{StatusCode: code, Body: body}
	if len(body) == 0 {
		return se
	}
	var env contract.ErrorResponse
	if err := json.Unmarshal(body, &env); err == nil {
		se.Code = env.Code
		se.Message = env.Error
		se.Details = env.Details
	}
	return se
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("orchestrator HTTP %d %s: %s", e.StatusCode, e.Code, e.Message)
	case e.Code != "":
		return fmt.Sprintf("orchestrator HTTP %d %s", e.StatusCode, e.Code)
	case len(e.Body) == 0:
		return fmt.Sprintf("orchestrator HTTP %d", e.StatusCode)
	}
	body := e.Body
	if len(body) > 256 {
		body = body[:256]
	}
	return fmt.Sprintf("orchestrator HTTP %d: %s", e.StatusCode, body)
}

// Is enables errors.Is(err, domain.Err*) for the 13 stable codes.
// Falls back to HTTP-status-based mapping when the envelope is missing
// or unparseable, preserving compatibility with pre-v0.2.0 servers.
func (e *StatusError) Is(target error) bool {
	if c := codeToSentinel(e.Code); c != nil && errors.Is(target, c) {
		return true
	}
	// Status-only fallbacks (no code parsed).
	switch {
	case e.StatusCode == 401 && errors.Is(target, domain.ErrUnauthorized):
		return true
	case e.StatusCode == 404 && errors.Is(target, domain.ErrChangeNotFound):
		return true
	case e.StatusCode >= 500 && e.StatusCode < 600 && errors.Is(target, domain.ErrUnreachable):
		return true
	}
	return false
}

// codeToSentinel maps a sophia-wire-v1 §9.2 code to its domain sentinel.
// Returns nil when the code is unknown — clients SHOULD fall through to
// generic exit 3 with the raw code printed.
func codeToSentinel(code string) error {
	switch code {
	case contract.CodeUnauthorized:
		return domain.ErrUnauthorized
	case contract.CodeValidationFailed:
		return domain.ErrValidationFailed
	case contract.CodeApproverRequired:
		return domain.ErrApproverRequired
	case contract.CodeLimitTooLarge:
		return domain.ErrLimitTooLarge
	case contract.CodeChangeNotFound:
		return domain.ErrChangeNotFound
	case contract.CodePhaseNotFound:
		return domain.ErrPhaseNotFound
	case contract.CodeChangeAlreadyExists:
		return domain.ErrChangeAlreadyExists
	case contract.CodeChangeAlreadyTerminal:
		return domain.ErrChangeAlreadyTerminal
	case contract.CodePhaseNotResumable:
		return domain.ErrPhaseNotResumable
	case contract.CodePhaseNotGated:
		return domain.ErrPhaseNotGated
	case contract.CodeGateAlreadyDecided:
		return domain.ErrGateAlreadyDecided
	case contract.CodePhaseTerminalNoEvents:
		return domain.ErrPhaseTerminalNoEvents
	case contract.CodeInternalError:
		return domain.ErrInternal
	}
	return nil
}
