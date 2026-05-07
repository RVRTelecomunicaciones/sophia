package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeComposeImplementsComposeRunner(t *testing.T) {
	var _ outbound.ComposeRunner = (*fakes.FakeComposeRunner)(nil)
}

func TestFakeComposeVersion(t *testing.T) {
	c := fakes.NewFakeComposeRunner()
	c.ComposeVersion = "Docker Compose version v2.32.1"
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != "Docker Compose version v2.32.1" {
		t.Errorf("Version = %q", v)
	}
}

func TestFakeComposeUpDownRecorded(t *testing.T) {
	c := fakes.NewFakeComposeRunner()
	_ = c.Up(context.Background(), "/p/compose.yaml", "sophia")
	_ = c.Down(context.Background(), "/p/compose.yaml", "sophia")
	if got := c.UpCalls; got != 1 {
		t.Errorf("UpCalls = %d, want 1", got)
	}
	if got := c.DownCalls; got != 1 {
		t.Errorf("DownCalls = %d, want 1", got)
	}
}
