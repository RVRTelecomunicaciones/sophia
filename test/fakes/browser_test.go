package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeBrowserImplementsPort(t *testing.T) {
	var _ outbound.Browser = (*fakes.FakeBrowser)(nil)
}

func TestFakeBrowserRecordsURLs(t *testing.T) {
	b := fakes.NewFakeBrowser()
	_ = b.Open(context.Background(), "https://example.com/a")
	_ = b.Open(context.Background(), "https://example.com/b")
	if got := len(b.Opened); got != 2 {
		t.Fatalf("Opened len = %d", got)
	}
	if b.Opened[0] != "https://example.com/a" || b.Opened[1] != "https://example.com/b" {
		t.Errorf("Opened = %v", b.Opened)
	}
}
