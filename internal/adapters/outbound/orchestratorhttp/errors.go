package orchestratorhttp

import (
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// StatusError is returned by orchestratorhttp methods on non-2xx responses.
// It maps 404 to domain.ErrChangeNotFound and 5xx to domain.ErrUnreachable
// via errors.Is.
type StatusError struct {
	StatusCode int
	Body       []byte
}

// NewStatusError constructs a StatusError.
func NewStatusError(code int, body []byte) *StatusError {
	return &StatusError{StatusCode: code, Body: body}
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("orchestrator HTTP %d", e.StatusCode)
	}
	// Cap body in error message to prevent log bloat.
	body := e.Body
	if len(body) > 256 {
		body = body[:256]
	}
	return fmt.Sprintf("orchestrator HTTP %d: %s", e.StatusCode, body)
}

// Is enables errors.Is(err, domain.ErrChangeNotFound) and similar.
func (e *StatusError) Is(target error) bool {
	switch {
	case e.StatusCode == 404 && errors.Is(target, domain.ErrChangeNotFound):
		return true
	case e.StatusCode >= 500 && e.StatusCode < 600 && errors.Is(target, domain.ErrUnreachable):
		return true
	}
	return false
}
