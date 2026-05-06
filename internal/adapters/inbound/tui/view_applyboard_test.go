package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestViewApplyBoardEmptyShowsHint(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing:\n%s", out)
	}
	if strings.Contains(out, "explore") && strings.Contains(out, "proposal") {
		t.Errorf("ApplyBoard view should not render the 9 phases:\n%s", out)
	}
}

func TestViewApplyBoardShowsGroupsTasksAgents(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1", "files_pattern": "internal/**/*.go"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "team_lead", "group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a2", "agent_role": "worker", "group_id": "g1", "task_id": "t1"}})

	out := tui.View(m)
	for _, want := range []string{"g1", "t1", "a1", "team_lead", "a2", "worker", "internal/**"} {
		if !strings.Contains(out, want) {
			t.Errorf("ApplyBoard output missing %q:\n%s", want, out)
		}
	}
}

func TestViewApplyBoardMarksRunningTask(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})

	out := tui.View(m)
	lines := strings.Split(out, "\n")
	t1Line := ""
	for _, line := range lines {
		if strings.Contains(line, "t1") {
			t1Line = line
			break
		}
	}
	if t1Line == "" {
		t.Fatal("t1 line not found")
	}
	if !strings.ContainsAny(t1Line, "▶>*") && !strings.Contains(t1Line, "running") {
		t.Errorf("running marker missing in t1 line: %q", t1Line)
	}
}

func TestViewApplyBoardMarksDoneTask(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "task.completed", Payload: map[string]any{"group_id": "g1", "task_id": "t1", "status": "done"}})

	out := tui.View(m)
	if !strings.Contains(out, "t1") {
		t.Fatal("t1 line missing")
	}
	if !strings.ContainsAny(out, "✓") && !strings.Contains(out, "done") {
		t.Errorf("done marker missing:\n%s", out)
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
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Error("ApplyBoard View must be pure")
	}
}
