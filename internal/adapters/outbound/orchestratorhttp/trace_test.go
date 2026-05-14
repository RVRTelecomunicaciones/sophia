package orchestratorhttp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
)

// Outbound integration: the orchestratorhttp.Client built on top of
// httpclient.New(...Trace...) emits Traceparent on every call AND
// rotates the span_id between calls while keeping trace_id stable.
// This is the acceptance-criterion test for ADR-0005 P2.2b.
func TestOrchestratorClient_EmitsTraceparent_StableTraceID(t *testing.T) {
	var observed []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = append(observed, r.Header.Get("Traceparent"))
		// Echo Traceparent back per Phase A contract.
		if tp := r.Header.Get("Traceparent"); tp != "" {
			w.Header().Set("Traceparent", tp)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	parent, err := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if err != nil {
		t.Fatalf("parse parent: %v", err)
	}
	hc := httpclient.New(httpclient.Config{Trace: parent})
	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, HTTP: hc})

	// Two calls — Healthz is the simplest method available on Client.
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz 1: %v", err)
	}
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz 2: %v", err)
	}

	if len(observed) != 2 {
		t.Fatalf("expected 2 inbound requests, got %d", len(observed))
	}
	for i, tp := range observed {
		if tp == "" {
			t.Fatalf("call %d: missing Traceparent header", i+1)
		}
		parts := strings.Split(tp, "-")
		if len(parts) != 4 {
			t.Fatalf("call %d: malformed Traceparent %q", i+1, tp)
		}
		if parts[1] != parent.TraceID {
			t.Errorf("call %d: trace_id = %q, want %q", i+1, parts[1], parent.TraceID)
		}
	}
	// span_id must differ between the two outbound calls (per-request child span).
	p1, _ := trace.Parse(observed[0])
	p2, _ := trace.Parse(observed[1])
	if p1.SpanID == p2.SpanID {
		t.Errorf("span_id not rotated between calls: %q", p1.SpanID)
	}
}
