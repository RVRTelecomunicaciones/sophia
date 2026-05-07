package sseprobe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/sseprobe"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

func TestProberImplementsPort(t *testing.T) {
	var _ outbound.SSEProber = sseprobe.New(sseprobe.Config{BaseURL: "http://x"})
}

func TestProbeSucceedsOnEventStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": ping\n\n"))
	}))
	defer srv.Close()

	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err != nil {
		t.Errorf("Probe err: %v", err)
	}
}

func TestProbeFailsOnWrongContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error on non-event-stream response")
	}
}

func TestProbeFailsOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error on 404")
	}
}
