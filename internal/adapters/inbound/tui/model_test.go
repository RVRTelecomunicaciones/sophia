package tui_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestNewModelInitializesNinePhaseRows(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})

	got := m.PhaseRows()
	if len(got) != 9 {
		t.Fatalf("phase rows = %d, want 9", len(got))
	}
	want := domain.AllPhases()
	for i, row := range got {
		if row.Type != want[i] {
			t.Errorf("row %d type = %q, want %q", i, row.Type, want[i])
		}
		if row.Status != domain.PhaseStatusPending {
			t.Errorf("row %d default status = %q, want pending", i, row.Status)
		}
	}
}

func TestNewModelDefaultDimensionsAreSafe(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	if m.Width() <= 0 || m.Height() <= 0 {
		t.Errorf("default dimensions must be positive: w=%d h=%d", m.Width(), m.Height())
	}
}

func TestModelApplySnapshotPopulatesPhases(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	change := &domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-init", Type: domain.PhaseInit, Status: domain.PhaseStatusDone, StartedAt: time.Unix(100, 0).UTC(), EndedAt: time.Unix(110, 0).UTC()},
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning, StartedAt: time.Unix(110, 0).UTC()},
		},
	}

	m2 := m.ApplySnapshot(change)

	rows := m2.PhaseRows()
	if rows[0].Status != domain.PhaseStatusDone {
		t.Errorf("init row status = %q, want done", rows[0].Status)
	}
	if rows[1].Status != domain.PhaseStatusRunning {
		t.Errorf("explore row status = %q, want running", rows[1].Status)
	}
	for i := 2; i < 9; i++ {
		if rows[i].Status != domain.PhaseStatusPending {
			t.Errorf("row %d status = %q, want pending", i, rows[i].Status)
		}
	}
	if m2.CurrentPhaseID() != "p-explore" {
		t.Errorf("CurrentPhaseID = %q, want p-explore", m2.CurrentPhaseID())
	}
	if m2.ChangeStatus() != domain.ChangeStatusRunning {
		t.Errorf("ChangeStatus = %q", m2.ChangeStatus())
	}
}

func TestModelApplyEventUpdatesPhaseStatus(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "phase.started",
		Payload: map[string]any{
			"phase_type": string(domain.PhaseExplore),
			"phase_id":   "p-1",
		},
	})

	rows := m.PhaseRows()
	if rows[1].Type != domain.PhaseExplore {
		t.Fatalf("row 1 should be explore: %q", rows[1].Type)
	}
	if rows[1].Status != domain.PhaseStatusRunning {
		t.Errorf("explore status after phase.started = %q, want running", rows[1].Status)
	}
	if m.CurrentPhaseID() != "p-1" {
		t.Errorf("CurrentPhaseID = %q, want p-1", m.CurrentPhaseID())
	}
}

func TestModelApplyEventCompletedTransitions(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "phase.completed",
		Payload: map[string]any{
			"phase_type": string(domain.PhaseProposal),
			"status":     string(domain.PhaseStatusDone),
		},
	})

	rows := m.PhaseRows()
	if rows[2].Status != domain.PhaseStatusDone {
		t.Errorf("proposal status = %q, want done", rows[2].Status)
	}
}

func TestModelApplyEventIgnoresUnknownPhaseType(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	before := m.PhaseRows()
	m = m.ApplyEvent(domain.Event{
		Type: "phase.started",
		Payload: map[string]any{
			"phase_type": "nonexistent",
		},
	})
	after := m.PhaseRows()
	for i := range before {
		if before[i].Status != after[i].Status {
			t.Errorf("row %d mutated despite unknown phase type", i)
		}
	}
}

func TestModelApplyEventApprovalRequiredMarksPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"phase": string(domain.PhaseApply),
		},
	})

	rows := m.PhaseRows()
	if !rows[6].HasApproval { // apply is index 6 in AllPhases()
		t.Error("apply row should be marked HasApproval after approval.required")
	}
}

func TestModelDetachState(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	if m.Detached() {
		t.Error("fresh model should not be detached")
	}
	if m.ConfirmingDetach() {
		t.Error("fresh model should not be confirming detach")
	}
	m2 := m.WithConfirmingDetach(true)
	if !m2.ConfirmingDetach() {
		t.Error("WithConfirmingDetach(true) should set the flag")
	}
	m3 := m2.WithDetached(true)
	if !m3.Detached() {
		t.Error("WithDetached(true) should set the flag")
	}
}

func TestModelResize(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.Resize(120, 40)
	if m2.Width() != 120 || m2.Height() != 40 {
		t.Errorf("after Resize(120, 40): w=%d h=%d", m2.Width(), m2.Height())
	}
}

func TestModelImmutability(t *testing.T) {
	// Methods that "update" the model MUST return a new value, not mutate
	// the receiver. This guards against accidental shared-state bugs.
	m1 := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m1.WithDetached(true)
	if m1.Detached() {
		t.Error("WithDetached mutated the receiver")
	}
	_ = m2
}

func TestModelDefaultViewIsTimeline(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	if m.CurrentView() != tui.ViewTimeline {
		t.Errorf("default view = %v, want ViewTimeline", m.CurrentView())
	}
}

func TestModelWithCurrentView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.WithCurrentView(tui.ViewApplyBoard)
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Errorf("after WithCurrentView(ApplyBoard): %v", m2.CurrentView())
	}
	if m.CurrentView() != tui.ViewTimeline {
		t.Error("WithCurrentView mutated the receiver")
	}
}

func TestModelApprovalGateMsgSetsBannerGate(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	gate := domain.ApprovalGate{
		URL:    "https://gov.local/approvals/abc",
		Reason: "policy",
		Risk:   "medium",
		Phase:  domain.PhaseApply,
	}
	m2 := m.WithBannerGate(&gate)

	got := m2.BannerGate()
	if got == nil {
		t.Fatal("BannerGate is nil after WithBannerGate")
	}
	if got.URL != "https://gov.local/approvals/abc" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestModelApplyEventApprovalRequiredSetsBannerGate(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.ApplyEvent(domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"phase":     string(domain.PhaseApply),
			"gate_url":  "https://gov.local/x",
			"reason":    "policy says no apply without tasks approved",
			"risk":      "medium",
			"policy":    "require_approval",
			"change_id": "01HX",
		},
		TraceID: "trace-1",
	})
	gate := m2.BannerGate()
	if gate == nil {
		t.Fatal("approval.required should set BannerGate")
	}
	if gate.URL != "https://gov.local/x" {
		t.Errorf("URL = %q", gate.URL)
	}
	if gate.Risk != "medium" {
		t.Errorf("Risk = %q", gate.Risk)
	}
	if gate.Phase != domain.PhaseApply {
		t.Errorf("Phase = %q", gate.Phase)
	}
	if gate.TraceID != "trace-1" {
		t.Errorf("TraceID = %q", gate.TraceID)
	}
}

func TestModelApplyEventApprovalResolvedClearsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{URL: "https://x", Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type: "approval.resolved",
		Payload: map[string]any{
			"decision":    "approved",
			"resolved_by": "alice",
		},
	})
	if m2.BannerGate() != nil {
		t.Errorf("approval.resolved should clear banner; got %+v", m2.BannerGate())
	}
}

func TestModelApplyEventForwardProgressClearsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseVerify)},
	})
	if m2.BannerGate() != nil {
		t.Error("phase.started for verify should clear apply-banner (forward progress)")
	}
}

func TestModelApplyEventSamePhaseDoesNotClearBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseApply)},
	})
	if m2.BannerGate() == nil {
		t.Error("phase.started for the SAME phase must not clear banner")
	}
}

func TestModelApplyEventEarlierPhaseDoesNotClearBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseExplore)},
	})
	if m2.BannerGate() == nil {
		t.Error("phase.started for an earlier phase must not clear banner")
	}
}

func TestModelApplySnapshotPastPhaseClearsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplySnapshot(&domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-verify",
		Phases: []domain.Phase{
			{ID: "p-verify", Type: domain.PhaseVerify, Status: domain.PhaseStatusRunning},
		},
	})
	if m2.BannerGate() != nil {
		t.Error("snapshot showing forward-progress phase should clear banner")
	}
}

func TestModelApplySnapshotSamePhaseKeepsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplySnapshot(&domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-apply",
		Phases: []domain.Phase{
			{ID: "p-apply", Type: domain.PhaseApply, Status: domain.PhaseStatusRunning},
		},
	})
	if m2.BannerGate() == nil {
		t.Error("snapshot still on the gated phase should keep banner")
	}
}

func TestModelApplyEventTaskStartedFeedsApplyBoard(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**",
		},
	})
	if m2.ApplyBoard().GroupCount() != 1 {
		t.Error("task.started should feed ApplyBoard")
	}
}

func TestModelApplyEventAgentSpawnedFeedsApplyBoard(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "team_lead", "group_id": "g1", "task_id": "t1"}})

	board := m.ApplyBoard()
	if len(board.Groups()) != 1 {
		t.Fatal("groups missing")
	}
	task := board.Groups()[0].Tasks[0]
	if len(task.Agents) != 1 || task.Agents[0].Role != "team_lead" {
		t.Errorf("agent missing or wrong: %+v", task.Agents)
	}
}
