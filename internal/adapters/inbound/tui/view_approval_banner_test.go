package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestApprovalBannerHiddenByDefault(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out := tui.View(m)
	for _, banner := range []string{"Approval required", "Gate:", "[O]"} {
		if strings.Contains(out, banner) {
			t.Errorf("banner element %q should be hidden by default; got:\n%s", banner, out)
		}
	}
}

func TestApprovalBannerVisibleWhenGateSet(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://gov.local/approvals/abc",
			Reason: "NO APPLY WITHOUT TASKS APPROVED",
			Risk:   "medium",
			Policy: "require_approval",
			Phase:  domain.PhaseApply,
		})
	out := tui.View(m)

	for _, want := range []string{
		"Approval required",
		"apply",
		"medium",
		"NO APPLY WITHOUT TASKS APPROVED",
		"require_approval",
		"https://gov.local/approvals/abc",
		"[O]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestApprovalBannerOverlayInTimelineView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewTimeline).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://x",
			Phase:  domain.PhaseApply,
			Reason: "policy",
		})
	out := tui.View(m)
	if i := strings.Index(out, "Approval required"); i < 0 {
		t.Fatalf("banner header missing:\n%s", out)
	}
	if j := strings.Index(out, "explore"); j > 0 {
		i := strings.Index(out, "Approval required")
		if i > j {
			t.Errorf("banner should appear BEFORE Timeline body (banner=%d, explore=%d)", i, j)
		}
	}
}

func TestApprovalBannerOverlayInApplyBoardView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		WithBannerGate(&domain.ApprovalGate{URL: "https://x", Phase: domain.PhaseApply})
	out := tui.View(m)
	if !strings.Contains(out, "Approval required") {
		t.Errorf("banner missing in ApplyBoard view:\n%s", out)
	}
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing under banner:\n%s", out)
	}
}

func TestApprovalBannerHandlesEmptyFields(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL: "https://x",
		})
	out := tui.View(m)
	if !strings.Contains(out, "Approval required") {
		t.Errorf("banner header missing on empty gate:\n%s", out)
	}
}

func TestApprovalBannerANSISafetyOnReason(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://x",
			Reason: "\x1b[2J\x1b[H attacker tried to clear",
		})
	out := tui.View(m)
	if !strings.Contains(out, "attacker tried to clear") {
		t.Error("Reason text not present in output")
	}
	if strings.HasPrefix(out, "\x1b[2J") {
		t.Error("output must not begin with raw clear-screen from user input")
	}
}
