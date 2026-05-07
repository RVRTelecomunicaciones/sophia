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

func TestApprover_ApproveSendsContractFields(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.ApprovalDecisionInput
	var phaseID string
	orch.OnApprove = func(pid string, in outbound.ApprovalDecisionInput) {
		phaseID = pid
		captured = in
	}
	a := application.NewApprover(application.ApproverDeps{Orch: orch})
	err := a.Approve(context.Background(), application.ApprovalInput{
		PhaseID: "01PHASEX", Approver: "alice", Reason: "lgtm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if phaseID != "01PHASEX" {
		t.Errorf("phase id = %q", phaseID)
	}
	if captured.Approver != "alice" || captured.Reason != "lgtm" {
		t.Errorf("captured = %+v", captured)
	}
}

func TestApprover_ApproverRequiredOnEmpty(t *testing.T) {
	a := application.NewApprover(application.ApproverDeps{Orch: fakes.NewFakeOrchestrator()})
	err := a.Approve(context.Background(), application.ApprovalInput{
		PhaseID: "01P", Approver: "",
	})
	if !errors.Is(err, domain.ErrApproverRequired) {
		t.Errorf("expected ErrApproverRequired, got %v", err)
	}
}

func TestApprover_RejectSendsContractFields(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.ApprovalDecisionInput
	orch.OnReject = func(_ string, in outbound.ApprovalDecisionInput) { captured = in }
	a := application.NewApprover(application.ApproverDeps{Orch: orch})
	if err := a.Reject(context.Background(), application.ApprovalInput{
		PhaseID: "01P", Approver: "alice", Reason: "bad",
	}); err != nil {
		t.Fatal(err)
	}
	if captured.Approver != "alice" || captured.Reason != "bad" {
		t.Errorf("captured = %+v", captured)
	}
}

func TestApprover_PropagatesGateAlreadyDecided(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.ApproveErr = domain.ErrGateAlreadyDecided
	a := application.NewApprover(application.ApproverDeps{Orch: orch})
	err := a.Approve(context.Background(), application.ApprovalInput{
		PhaseID: "01P", Approver: "alice",
	})
	if !errors.Is(err, domain.ErrGateAlreadyDecided) {
		t.Errorf("expected ErrGateAlreadyDecided propagated, got %v", err)
	}
}
