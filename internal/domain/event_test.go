package domain_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestEventZeroValue(t *testing.T) {
	var e domain.Event
	if e.Type != "" {
		t.Error("zero Event should have empty Type")
	}
	if e.Payload != nil {
		t.Error("zero Event should have nil Payload")
	}
}

func TestEventCarriesTimestamp(t *testing.T) {
	now := time.Now()
	e := domain.Event{Type: "phase.started", Timestamp: now}
	if !e.Timestamp.Equal(now) {
		t.Errorf("Timestamp lost: got %v, want %v", e.Timestamp, now)
	}
}

func TestEventPayloadAllowsArbitraryKeys(t *testing.T) {
	e := domain.Event{
		Type:    "agent.spawned",
		Payload: map[string]any{"agent_role": "team-lead", "n": 3},
	}
	if e.Payload["agent_role"] != "team-lead" {
		t.Error("Payload string lost")
	}
	if e.Payload["n"] != 3 {
		t.Error("Payload int lost")
	}
}
