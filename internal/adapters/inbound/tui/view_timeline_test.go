package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestViewRendersAllNinePhaseRows(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HXABC")})
	out := tui.View(m)

	for _, pt := range domain.AllPhases() {
		if !strings.Contains(out, string(pt)) {
			t.Errorf("View output missing phase %q:\n%s", pt, out)
		}
	}
}

func TestViewIncludesChangeID(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HXABCDEF")})
	out := tui.View(m)
	if !strings.Contains(out, "01HXABCDEF") {
		t.Errorf("View should display change ID; got:\n%s", out)
	}
}

func TestViewMarksRunningPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplySnapshot(&domain.Change{
			ID:             domain.ChangeID("01HX"),
			Status:         domain.ChangeStatusRunning,
			CurrentPhaseID: "p-1",
			Phases: []domain.Phase{
				{ID: "p-1", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
			},
		})
	out := tui.View(m)
	lines := strings.Split(out, "\n")
	exploreLine := ""
	for _, line := range lines {
		if strings.Contains(line, "explore") {
			exploreLine = line
			break
		}
	}
	if exploreLine == "" {
		t.Fatal("explore phase line not found")
	}
	if !strings.ContainsAny(exploreLine, "▶>*") && !strings.Contains(exploreLine, "running") {
		t.Errorf("running marker missing in explore line: %q", exploreLine)
	}
}

func TestViewMarksApprovalRequiredPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplyEvent(domain.Event{
			Type: "approval.required",
			Payload: map[string]any{
				"phase": string(domain.PhaseApply),
			},
		})
	out := tui.View(m)

	lines := strings.Split(out, "\n")
	applyLine := ""
	for _, line := range lines {
		if strings.Contains(line, "apply") {
			applyLine = line
			break
		}
	}
	if applyLine == "" {
		t.Fatal("apply phase line not found")
	}
	if !strings.Contains(applyLine, "!") {
		t.Errorf("approval marker (!) missing in apply line: %q", applyLine)
	}
}

func TestViewShowsConfirmDialogWhenConfirming(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithConfirmingDetach(true)
	out := tui.View(m)
	if !strings.Contains(out, "Detach?") {
		t.Errorf("confirm dialog missing; got:\n%s", out)
	}
}

func TestViewShowsKeybindingHints(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out := tui.View(m)
	if !strings.Contains(strings.ToLower(out), "q") {
		t.Errorf("View should hint at the Q keybinding; got:\n%s", out)
	}
}

func TestViewIsPure(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Errorf("View must be pure (same input → same output)")
	}
}

func TestViewDoesNotInterpretANSIInPayload(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithError("\x1b[2J\x1b[H attacker tried to clear screen")
	out := tui.View(m)
	if !strings.Contains(out, "clear screen") {
		t.Error("error text payload missing from View")
	}
	if strings.HasPrefix(out, "\x1b[2J") {
		t.Error("View must not begin with raw ANSI clear-screen from user input")
	}
}
