package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeEventStreamImplementsClient(t *testing.T) {
	var _ outbound.EventStreamClient = (*fakes.FakeEventStream)(nil)
}

func TestFakeEventStreamDelivers(t *testing.T) {
	s := fakes.NewFakeEventStream()
	target := outbound.StreamTarget{ChangeID: "c", PhaseID: "p"}
	ch, cancel, err := s.Subscribe(context.Background(), target, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	s.Push(target, domain.Event{Type: "phase.started"})
	got := <-ch
	if got.Type != "phase.started" {
		t.Errorf("got %q", got.Type)
	}
}

func TestFakeEventStreamCloseIsIdempotent(t *testing.T) {
	s := fakes.NewFakeEventStream()
	target := outbound.StreamTarget{ChangeID: "c"}
	_, cancel, err := s.Subscribe(context.Background(), target, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cancel(); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := cancel(); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
}
