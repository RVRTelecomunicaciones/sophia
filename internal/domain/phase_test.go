package domain_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
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
	cases := map[domain.PhaseStatus]bool{
		domain.PhaseStatusPending: false,
		domain.PhaseStatusRunning: false,
		domain.PhaseStatusDone:    true,
		domain.PhaseStatusBlocked: true,
		domain.PhaseStatusFailed:  true,
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
	_ = time.Time{}
}
