package orchestratorhttp_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestStatusErrorPreservesCode(t *testing.T) {
	e := orchestratorhttp.NewStatusError(404, []byte(`{"error":"not found"}`))
	if e.StatusCode != 404 {
		t.Errorf("StatusCode = %d", e.StatusCode)
	}
	if e.Error() == "" {
		t.Error("empty message")
	}
}

func TestStatusErrorMapsNotFoundToErrChangeNotFound(t *testing.T) {
	e := orchestratorhttp.NewStatusError(404, nil)
	if !errors.Is(e, domain.ErrChangeNotFound) {
		t.Errorf("404 should map to ErrChangeNotFound")
	}
}

func TestStatusErrorMapsServerErrorsToErrUnreachable(t *testing.T) {
	e := orchestratorhttp.NewStatusError(503, nil)
	if !errors.Is(e, domain.ErrUnreachable) {
		t.Errorf("5xx should map to ErrUnreachable")
	}
}

func TestStatusErrorOtherCodesAreNotMapped(t *testing.T) {
	e := orchestratorhttp.NewStatusError(400, []byte("bad input"))
	if errors.Is(e, domain.ErrChangeNotFound) || errors.Is(e, domain.ErrUnreachable) {
		t.Errorf("400 should not map to known sentinels")
	}
}

// TestStatusError_MapsAllStableCodes asserts every code in
// sophia-wire-v1 §9.2 round-trips to its domain sentinel via errors.Is.
// The orchestrator wire-v1 envelope is {code, error, details?}.
func TestStatusError_MapsAllStableCodes(t *testing.T) {
	cases := []struct {
		code     string
		status   int
		sentinel error
	}{
		{"unauthorized", 401, domain.ErrUnauthorized},
		{"validation_failed", 400, domain.ErrValidationFailed},
		{"approver_required", 400, domain.ErrApproverRequired},
		{"limit_too_large", 400, domain.ErrLimitTooLarge},
		{"change_not_found", 404, domain.ErrChangeNotFound},
		{"phase_not_found", 404, domain.ErrPhaseNotFound},
		{"change_already_exists", 409, domain.ErrChangeAlreadyExists},
		{"change_already_terminal", 409, domain.ErrChangeAlreadyTerminal},
		{"phase_not_resumable", 409, domain.ErrPhaseNotResumable},
		{"phase_not_gated", 409, domain.ErrPhaseNotGated},
		{"gate_already_decided", 409, domain.ErrGateAlreadyDecided},
		{"phase_terminal_no_events", 410, domain.ErrPhaseTerminalNoEvents},
		{"internal_error", 500, domain.ErrInternal},
	}
	for _, tc := range cases {
		body := []byte(`{"code":"` + tc.code + `","error":"x"}`)
		e := orchestratorhttp.NewStatusError(tc.status, body)
		if !errors.Is(e, tc.sentinel) {
			t.Errorf("code %q (HTTP %d) should map to %v, got %v",
				tc.code, tc.status, tc.sentinel, e)
		}
		if e.Code != tc.code {
			t.Errorf("Code field = %q, want %q", e.Code, tc.code)
		}
	}
}

// TestStatusError_DetailsParsed asserts the envelope's details map is
// preserved for callers that want to surface field-level info
// (e.g. limit_too_large carries `limit` + `max_limit`).
func TestStatusError_DetailsParsed(t *testing.T) {
	body := []byte(`{"code":"limit_too_large","error":"too big","details":{"limit":500,"max_limit":100}}`)
	e := orchestratorhttp.NewStatusError(400, body)
	if e.Details == nil {
		t.Fatal("Details nil")
	}
	if got, _ := e.Details["max_limit"].(float64); got != 100 {
		t.Errorf("max_limit = %v, want 100", got)
	}
}
