package ssestream_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// SSE acceptance test for ADR-0005 P2.2b: the initial GET to the per-phase
// stream MUST carry a Traceparent header whose trace_id equals the
// CLI-invocation trace_id. The span_id is freshly minted by Subscribe and
// pinned for the lifetime of the stream (no rotation per event).
func TestSSEClient_EmitsTraceparent_OnInitialGET(t *testing.T) {
	var (
		mu      sync.Mutex
		seenTPs []string
	)

	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		mu.Lock()
		seenTPs = append(seenTPs, r.Header.Get("Traceparent"))
		mu.Unlock()
		writeSSE(w, "phase.started", "evt-1", `{"payload":{"phase_id":"p-1"}}`)
		flush()
	})
	defer srv.Close()

	parent, err := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if err != nil {
		t.Fatalf("parse parent: %v", err)
	}

	c := ssestream.New(ssestream.Config{
		BaseURL: srv.URL,
		Trace:   parent,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx,
		outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"},
		outbound.SubscribeOptions{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// Drain one event so we know the server saw the request.
	_ = drainN(t, ch, 1, time.Second)
	_ = stop()

	mu.Lock()
	defer mu.Unlock()
	if len(seenTPs) == 0 {
		t.Fatal("server saw zero requests")
	}
	tp := seenTPs[0]
	if tp == "" {
		t.Fatal("missing Traceparent on initial GET")
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("malformed Traceparent: %q", tp)
	}
	if parts[1] != parent.TraceID {
		t.Errorf("trace_id = %q, want %q (must match invocation trace)", parts[1], parent.TraceID)
	}
	if parts[2] == parent.SpanID {
		t.Errorf("span_id reused parent: %q (Subscribe must mint a fresh child span)", parts[2])
	}
	if len(parts[2]) != 16 {
		t.Errorf("span_id length = %d, want 16", len(parts[2]))
	}
}
