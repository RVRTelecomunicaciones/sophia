package fakes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeSSEProberImplementsPort(t *testing.T) {
	var _ outbound.SSEProber = (*fakes.FakeSSEProber)(nil)
}

func TestFakeSSEProberReturnsConfiguredError(t *testing.T) {
	p := fakes.NewFakeSSEProber()
	if err := p.Probe(context.Background()); err != nil {
		t.Fatalf("default Probe should be nil, got %v", err)
	}
	p.ProbeErr = errors.New("handshake failed")
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error")
	}
}
