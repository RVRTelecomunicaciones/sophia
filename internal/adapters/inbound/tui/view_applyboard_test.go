package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// applyModel builds a Model in ViewApplyBoard mode and feeds it the given
// events so tests can construct arbitrary ApplyBoard state via the public API.
func applyModel(events ...domain.Event) tui.Model {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	for _, ev := range events {
		m = m.ApplyEvent(ev)
	}
	return m
}

// TestView_EmptyStatePlaceholder — D1: empty board renders the placeholder.
func TestView_EmptyStatePlaceholder(t *testing.T) {
	m := applyModel() // no events → no groups, no materialize
	out := tui.View(m)

	if !strings.Contains(out, "No tasks yet") {
		t.Errorf("D1: empty state hint missing:\n%s", out)
	}
	if strings.Contains(out, "▼") {
		t.Errorf("D1: empty state must not contain group markers:\n%s", out)
	}
	// Header must still name the view.
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("D1: ApplyBoard header missing:\n%s", out)
	}
}

// TestView_PopulatedBoardRendersTree — D2: board renders group → task →
// session tree with running-status marker.
func TestView_PopulatedBoardRendersTree(t *testing.T) {
	m := applyModel(
		// Create board with 1 group.
		domain.Event{
			Type:    contract.EventApplyBoardCreated,
			Payload: map[string]any{"board_id": "b1", "groups": 1},
		},
		// Spawn team lead on g1.
		domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "s-lead-1", "group_id": "g1"},
		},
		// Claim task t1 (no group_id in payload — defensive attach).
		domain.Event{
			Type:    contract.EventApplyTaskClaimed,
			Payload: map[string]any{"task_id": "t1", "session_id": "s-impl-1"},
		},
	)
	out := tui.View(m)

	// Group header.
	if !strings.Contains(out, "g1") {
		t.Errorf("D2: group id g1 missing:\n%s", out)
	}
	// Team-lead session.
	if !strings.Contains(out, "s-lead-1") {
		t.Errorf("D2: team-lead session id s-lead-1 missing:\n%s", out)
	}
	// Task row with implement session.
	if !strings.Contains(out, "t1") {
		t.Errorf("D2: task id t1 missing:\n%s", out)
	}
	if !strings.Contains(out, "s-impl-1") {
		t.Errorf("D2: implement session id s-impl-1 missing:\n%s", out)
	}
	// Running status marker present.
	if !strings.Contains(out, "running") {
		t.Errorf("D2: running status marker missing:\n%s", out)
	}
}

// TestView_EscalatedFailedDegradedDistinctMarkers — D3: each terminal state
// renders with a visually distinct icon/style; degraded groups show FailedDep.
func TestView_EscalatedFailedDegradedDistinctMarkers(t *testing.T) {
	m := applyModel(
		// Group g-fail: mark failed.
		domain.Event{
			Type:    contract.EventApplyGroupFailed,
			Payload: map[string]any{"group_id": "g-fail", "reason": "dispatch timeout"},
		},
		// Group g-deg: mark degraded with failed_dep.
		domain.Event{
			Type:    contract.EventApplyGroupDegraded,
			Payload: map[string]any{"group_id": "g-deg", "failed_dep": "g0", "failed_dep_err": "envelope blocked", "continued_run": true},
		},
		// Group g-esc: task t-esc escalated.
		domain.Event{
			Type:    contract.EventApplyTeamLeadSpawned,
			Payload: map[string]any{"session_id": "sl-esc", "group_id": "g-esc"},
		},
		domain.Event{
			Type:    contract.EventApplyTaskEscalated,
			Payload: map[string]any{"task_id": "t-esc", "attempts": 3, "reason": "blocked", "final_envelope_summary": "blocked context", "blocking_requirements": []any{"R1"}},
		},
	)
	out := tui.View(m)

	// Failed group present.
	if !strings.Contains(out, "g-fail") {
		t.Errorf("D3: group g-fail missing:\n%s", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("D3: 'failed' status missing from output:\n%s", out)
	}

	// Degraded group present with failed_dep.
	if !strings.Contains(out, "g-deg") {
		t.Errorf("D3: group g-deg missing:\n%s", out)
	}
	if !strings.Contains(out, "degraded") {
		t.Errorf("D3: 'degraded' status missing from output:\n%s", out)
	}
	if !strings.Contains(out, "failed_dep=g0") {
		t.Errorf("D3: degraded group must display failed_dep=g0:\n%s", out)
	}

	// Escalated task present.
	if !strings.Contains(out, "t-esc") {
		t.Errorf("D3: escalated task t-esc missing:\n%s", out)
	}
	if !strings.Contains(out, "escalated") {
		t.Errorf("D3: 'escalated' status missing from output:\n%s", out)
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("D3: escalation reason 'blocked' missing from output:\n%s", out)
	}
}

// TestView_MaterializeProgressRendered — D4: materialize running/completed
// renders with target path; completed shows groups count.
func TestView_MaterializeProgressRendered(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		m := applyModel(
			domain.Event{
				Type:    contract.EventApplyMaterializeStarted,
				Payload: map[string]any{"target_path": "/repo/x"},
			},
		)
		out := tui.View(m)
		if !strings.Contains(out, "/repo/x") {
			t.Errorf("D4/running: target path /repo/x missing:\n%s", out)
		}
		if !strings.Contains(out, "materialize") {
			t.Errorf("D4/running: 'materialize' label missing:\n%s", out)
		}
	})

	t.Run("completed", func(t *testing.T) {
		m := applyModel(
			domain.Event{
				Type:    contract.EventApplyMaterializeStarted,
				Payload: map[string]any{"target_path": "/repo/x"},
			},
			domain.Event{
				Type:    contract.EventApplyMaterializeCompleted,
				Payload: map[string]any{"target_path": "/repo/x", "groups_materialized": 3},
			},
		)
		out := tui.View(m)
		if !strings.Contains(out, "/repo/x") {
			t.Errorf("D4/completed: target path /repo/x missing:\n%s", out)
		}
		if !strings.Contains(out, "3") {
			t.Errorf("D4/completed: groups count 3 missing:\n%s", out)
		}
	})
}

// TestView_ApplyBoardHeaderIncludesLabel — header always contains "ApplyBoard".
func TestView_ApplyBoardHeaderIncludesLabel(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing:\n%s", out)
	}
}

// TestView_KeybindingHintsIncludeTab — hint line mentions Tab (to switch to Timeline).
func TestView_KeybindingHintsIncludeTab(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(strings.ToLower(out), "tab") {
		t.Errorf("hint should mention Tab; got:\n%s", out)
	}
}

// TestView_ApplyBoardIsPure — same Model produces same output (referential transparency).
func TestView_ApplyBoardIsPure(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Error("ApplyBoard View must be pure (same input → same output)")
	}
}
