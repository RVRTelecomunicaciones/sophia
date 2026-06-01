package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestAllPhasesAreNine(t *testing.T) {
	got := domain.AllPhases()
	if len(got) != 9 {
		t.Fatalf("expected 9 phases, got %d", len(got))
	}
	want := []domain.PhaseType{
		domain.PhaseInit, domain.PhaseExplore, domain.PhaseProposal,
		domain.PhaseSpec, domain.PhaseDesign, domain.PhaseTasks,
		domain.PhaseApply, domain.PhaseVerify, domain.PhaseArchive,
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("phase[%d] = %q, want %q", i, got[i], p)
		}
	}
}

func TestPhaseTypeIsValid(t *testing.T) {
	for _, p := range domain.AllPhases() {
		if !p.IsValid() {
			t.Errorf("%q should be valid", p)
		}
	}
	if domain.PhaseType("bogus").IsValid() {
		t.Error("bogus phase should be invalid")
	}
	if domain.PhaseType("").IsValid() {
		t.Error("empty phase should be invalid")
	}
}

func TestPhaseStatusIsTerminal(t *testing.T) {
	// Canonical 7-value set per sophia-wire-v1 §6.1.
	// Terminal: done, done_with_concerns, blocked.
	// Non-terminal: pending, running, needs_context, interrupted.
	// Note: "failed" is NOT a phase status — it is the phase.failed SSE event (§5.3).
	cases := map[domain.PhaseStatus]bool{
		domain.PhaseStatusPending:          false,
		domain.PhaseStatusRunning:          false,
		domain.PhaseStatusDone:             true,
		domain.PhaseStatusDoneWithConcerns: true,
		domain.PhaseStatusBlocked:          true,
		domain.PhaseStatusNeedsContext:     false,
		domain.PhaseStatusInterrupted:      false,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestPhaseZeroValue(t *testing.T) {
	var p domain.Phase
	if p.Type != "" || p.Status != "" {
		t.Errorf("zero Phase should have empty Type and Status")
	}
	if !p.StartedAt.IsZero() {
		t.Error("zero Phase StartedAt must be zero time")
	}
}
