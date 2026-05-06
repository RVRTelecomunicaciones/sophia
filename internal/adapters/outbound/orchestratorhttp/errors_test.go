package orchestratorhttp_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
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
