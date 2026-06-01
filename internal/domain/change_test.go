package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestChangeIDIsZero(t *testing.T) {
	var id domain.ChangeID
	if !id.IsZero() {
		t.Error("zero-value ChangeID should be zero")
	}
	id = "01HX..."
	if id.IsZero() {
		t.Error("non-empty ChangeID should not be zero")
	}
}

func TestChangeIDString(t *testing.T) {
	id := domain.ChangeID("abc")
	if id.String() != "abc" {
		t.Errorf("got %q, want %q", id.String(), "abc")
	}
}

func TestChangeStatusIsTerminal(t *testing.T) {
	cases := map[domain.ChangeStatus]bool{
		domain.ChangeStatusPending: false,
		domain.ChangeStatusRunning: false,
		domain.ChangeStatusDone:    true,
		domain.ChangeStatusBlocked: true,
		domain.ChangeStatusFailed:  true,
		domain.ChangeStatusAborted: true,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestChangeStatusAbortedValue(t *testing.T) {
	// Spec §505: ChangeStatus aborted must equal the wire value "aborted"
	// (pkg/contract/events.go ChangeStatusAborted = "aborted").
	const wantWireValue = "aborted"
	if got := string(domain.ChangeStatusAborted); got != wantWireValue {
		t.Errorf("ChangeStatusAborted = %q, want wire value %q", got, wantWireValue)
	}
}

func TestChangeZeroValue(t *testing.T) {
	var c domain.Change
	if !c.ID.IsZero() {
		t.Error("zero Change should have zero ID")
	}
	if c.Phases != nil {
		t.Error("zero Change should have nil Phases")
	}
}
