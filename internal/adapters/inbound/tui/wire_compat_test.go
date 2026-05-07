package tui_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// TestModel_AcceptsCanonicalAgentDispatched — Phase 4 scope item 9:
// the TUI MUST accept the canonical sophia-wire-v1 event name
// `agent.dispatched` (the orchestrator emits this name post-Phase-3.7)
// without breaking legacy `agent.spawned` consumers.
func TestModel_AcceptsCanonicalAgentDispatched(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	m = m.ApplyEvent(domain.Event{
		Type: "agent.dispatched",
		Payload: map[string]any{
			"agent_id":   "a1",
			"agent_role": "team_lead",
			"group_id":   "g1",
			"task_id":    "t1",
		},
	})
	board := m.ApplyBoard()
	if len(board.Groups()) == 0 {
		t.Fatal("expected agent.dispatched to populate ApplyBoard")
	}
}

// TestModel_TolerantOfApplyDiagnostics — Phase 1.5 amendment: `apply.*`
// events are Optional/diagnostic. The TUI MUST not break when the
// orchestrator emits them; they are silently ignored on the timeline.
func TestModel_TolerantOfApplyDiagnostics(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	for _, t := range []string{
		"apply.tx.committed",
		"apply.lock.acquired",
		"apply.governor.released",
	} {
		m = m.ApplyEvent(domain.Event{Type: t})
	}
	if m.BannerGate() != nil {
		t.Error("apply.* must not raise a banner")
	}
}

// TestModel_TolerantOfOpenEvent — `open` (sophia-wire-v1 §5.3, Phase
// 1.5 amendment) is sent at SSE connection establishment. The TUI
// treats it as a no-op.
func TestModel_TolerantOfOpenEvent(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	m = m.ApplyEvent(domain.Event{Type: "open", Payload: map[string]any{
		"phase_id": "01PHASE",
	}})
	// Should not panic or alter state observably.
	if m.BannerGate() != nil {
		t.Error("open event must not raise a banner")
	}
}
