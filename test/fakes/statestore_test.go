package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeStateStoreImplementsPort(t *testing.T) {
	var _ outbound.StateStore = (*fakes.FakeStateStore)(nil)
}

func TestFakeStateStoreProjectScoped(t *testing.T) {
	s := fakes.NewFakeStateStore()
	fp := domain.Fingerprint("abc")
	if id, _ := s.GetLast(context.Background(), fp); !id.IsZero() {
		t.Error("empty store should return zero")
	}
	if err := s.SetLast(context.Background(), fp, "01H..."); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetLast(context.Background(), fp)
	if got != "01H..." {
		t.Errorf("got %q", got)
	}
}

func TestFakeStateStoreGlobal(t *testing.T) {
	s := fakes.NewFakeStateStore()
	if id, _ := s.GetGlobalLast(context.Background()); !id.IsZero() {
		t.Error("expected zero")
	}
	_ = s.SetGlobalLast(context.Background(), "g1")
	got, _ := s.GetGlobalLast(context.Background())
	if got != "g1" {
		t.Errorf("got %q", got)
	}
}
