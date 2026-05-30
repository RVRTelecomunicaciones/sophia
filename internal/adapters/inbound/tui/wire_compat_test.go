package tui_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// TestModel_AcceptsCanonicalAgentDispatched — Phase 4 scope item 9:
// agent.dispatched is the canonical sophia-wire-v1 Timeline event. After
// cli-tui-applyboard-realign it is no longer routed to ApplyBoard (it feeds
// Timeline only; the model.go routing update lands in PR-2). The TUI MUST
// still accept the event without panicking and MUST NOT populate ApplyBoard.
func TestModel_AcceptsCanonicalAgentDispatched(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	m = m.ApplyEvent(domain.Event{
		Type: "agent.dispatched",
		Payload: map[string]any{
			"phase_id":   "p1",
			"session_id": "s1",
			"role":       "team_lead",
			"provider":   "anthropic",
		},
	})
	board := m.ApplyBoard()
	// agent.dispatched is NOT an apply.* event and must not populate ApplyBoard.
	if len(board.Groups()) != 0 {
		t.Errorf("agent.dispatched must not populate ApplyBoard; got %d groups", len(board.Groups()))
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
