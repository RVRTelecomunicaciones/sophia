package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestAborter_AbortSendsReason(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.AbortChangeInput
	orch.OnAbort = func(_ domain.ChangeID, in outbound.AbortChangeInput) { captured = in }
	a := application.NewAborter(application.AborterDeps{Orch: orch})
	if err := a.Abort(context.Background(), application.AbortInput{
		ChangeID: domain.ChangeID("01CH"), Reason: "user requested",
	}); err != nil {
		t.Fatal(err)
	}
	if captured.Reason != "user requested" {
		t.Errorf("reason = %q", captured.Reason)
	}
}

func TestAborter_RequiresChangeID(t *testing.T) {
	a := application.NewAborter(application.AborterDeps{Orch: fakes.NewFakeOrchestrator()})
	err := a.Abort(context.Background(), application.AbortInput{})
	if err == nil {
		t.Error("expected error on empty change_id")
	}
}

func TestAborter_PropagatesAlreadyTerminal(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.AbortErr = domain.ErrChangeAlreadyTerminal
	a := application.NewAborter(application.AborterDeps{Orch: orch})
	err := a.Abort(context.Background(), application.AbortInput{ChangeID: "01CH"})
	if !errors.Is(err, domain.ErrChangeAlreadyTerminal) {
		t.Errorf("expected ErrChangeAlreadyTerminal, got %v", err)
	}
}
