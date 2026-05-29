package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// PR-1 minimal smoke tests — keep the view compiling and rendering without
// panicking. Full view behavior tests (D1-D4) land in PR-2 alongside the
// view redesign.

func TestViewApplyBoardEmptyShowsHint(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing:\n%s", out)
	}
	if !strings.Contains(out, "No tasks yet") {
		t.Errorf("empty state hint missing:\n%s", out)
	}
}

func TestViewApplyBoardHeaderIncludesApplyBoardLabel(t *testing.T) {
	// PR-1 smoke: model.go routing for apply.* events is updated in PR-2.
	// This test confirms the view header always includes "ApplyBoard".
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)

	out := tui.View(m)
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing:\n%s", out)
	}
}

func TestViewApplyBoardKeybindingHintsIncludeTab(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(strings.ToLower(out), "tab") {
		t.Errorf("ApplyBoard hint should mention Tab; got:\n%s", out)
	}
}

func TestViewApplyBoardIsPure(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Error("ApplyBoard View must be pure")
	}
}
