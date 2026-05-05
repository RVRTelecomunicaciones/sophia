package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestApprovalGateAllFieldsPresent(t *testing.T) {
	g := domain.ApprovalGate{
		URL:      "https://gov.local/approvals/abc",
		Reason:   "NO APPLY WITHOUT TASKS APPROVED",
		Risk:     "medium",
		Policy:   "require_approval",
		ChangeID: domain.ChangeID("01HX..."),
		Phase:    domain.PhaseApply,
		TraceID:  "tr-1",
	}
	if g.URL == "" || g.Reason == "" || g.Risk == "" || g.Policy == "" {
		t.Error("ApprovalGate fields lost")
	}
	if g.ChangeID.IsZero() {
		t.Error("ApprovalGate.ChangeID lost")
	}
	if g.Phase != domain.PhaseApply {
		t.Errorf("ApprovalGate.Phase = %q", g.Phase)
	}
}
