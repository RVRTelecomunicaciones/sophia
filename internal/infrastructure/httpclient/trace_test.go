package httpclient_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
)

// captureHeaders runs srv against a single GET from the supplied client and
// returns the headers it observed.
func captureHeaders(t *testing.T, c *http.Client, ctx context.Context) http.Header {
	t.Helper()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return got
}

// Test: when a non-zero Trace is configured, outbound request carries
// Traceparent with the SAME trace_id and a fresh span_id (child span).
func TestNew_EmitsTraceparent_RotatesSpan(t *testing.T) {
	parent, err := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if err != nil {
		t.Fatalf("parent parse: %v", err)
	}

	c := httpclient.New(httpclient.Config{Trace: parent})

	hdrs := captureHeaders(t, c, context.Background())
	tp := hdrs.Get("Traceparent")
	if tp == "" {
		t.Fatal("missing Traceparent header")
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("Traceparent malformed: %q", tp)
	}
	if parts[1] != parent.TraceID {
		t.Errorf("trace_id = %q, want %q (must be preserved across HTTP calls)", parts[1], parent.TraceID)
	}
	if parts[2] == parent.SpanID {
		t.Errorf("span_id not rotated: %q (must be fresh per outbound request)", parts[2])
	}
	if len(parts[2]) != 16 {
		t.Errorf("span_id length = %d, want 16", len(parts[2]))
	}
	if parts[0] != "00" {
		t.Errorf("version = %q, want 00", parts[0])
	}
}

// Test: when no Trace is configured, NO Traceparent header is added.
// Production never hits this path (bootstrap always mints a Trace) but the
// defensive zero-Trace branch must be a strict no-op so tests and edge
// failures don't emit malformed headers.
func TestNew_NoTrace_NoHeader(t *testing.T) {
	c := httpclient.New(httpclient.Config{})
	hdrs := captureHeaders(t, c, context.Background())
	if got := hdrs.Get("Traceparent"); got != "" {
		t.Errorf("Traceparent = %q, want empty", got)
	}
}

// Test: span_id rotates between two separate outbound calls under the
// SAME invocation trace_id.
func TestNew_MultipleRequests_ShareTraceIDRotateSpan(t *testing.T) {
	parent, _ := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	c := httpclient.New(httpclient.Config{Trace: parent})

	h1 := captureHeaders(t, c, context.Background())
	h2 := captureHeaders(t, c, context.Background())

	tp1, tp2 := h1.Get("Traceparent"), h2.Get("Traceparent")
	if tp1 == "" || tp2 == "" {
		t.Fatalf("missing Traceparent: %q / %q", tp1, tp2)
	}
	p1, _ := trace.Parse(tp1)
	p2, _ := trace.Parse(tp2)
	if p1.TraceID != p2.TraceID {
		t.Errorf("trace_id drift: %q vs %q", p1.TraceID, p2.TraceID)
	}
	if p1.TraceID != parent.TraceID {
		t.Errorf("trace_id != parent: got %q want %q", p1.TraceID, parent.TraceID)
	}
	if p1.SpanID == p2.SpanID {
		t.Errorf("span_id reused across calls: %q", p1.SpanID)
	}
}

// Test: when a Trace is placed in the request context (the SSE pin path),
// the RoundTripper MUST use it verbatim and NOT rotate span_id.
func TestNew_ContextTrace_PinnedSpan(t *testing.T) {
	parent, _ := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	c := httpclient.New(httpclient.Config{Trace: parent})

	// Pin a known child span on the context.
	pinned := trace.Trace{
		TraceID: parent.TraceID,
		SpanID:  "abcdef0123456789",
		Sampled: true,
	}
	ctx := trace.NewContext(context.Background(), pinned)

	h1 := captureHeaders(t, c, ctx)
	h2 := captureHeaders(t, c, ctx)
	if h1.Get("Traceparent") != pinned.String() {
		t.Errorf("Traceparent[1] = %q, want pinned %q", h1.Get("Traceparent"), pinned.String())
	}
	if h2.Get("Traceparent") != pinned.String() {
		t.Errorf("Traceparent[2] = %q, want pinned (no rotation)", h2.Get("Traceparent"))
	}
}

// Test: explicit deterministic rand source yields predictable span_ids.
// This confirms RandSource injection works (used by the package's own
// tests; production passes nil to get crypto/rand.Reader).
func TestNew_DeterministicRandSource(t *testing.T) {
	parent, _ := trace.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	// 8 bytes per RoundTrip; supply two distinct deterministic spans.
	seed := []byte{
		// span 1: 11 22 33 44 55 66 77 88
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		// span 2: aa bb cc dd ee ff 00 11
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11,
	}
	c := httpclient.New(httpclient.Config{Trace: parent, RandSource: bytes.NewReader(seed)})

	h1 := captureHeaders(t, c, context.Background())
	tp1, err := trace.Parse(h1.Get("Traceparent"))
	if err != nil {
		t.Fatalf("parse h1: %v", err)
	}
	if tp1.SpanID != "1122334455667788" {
		t.Errorf("span_id1 = %q, want 1122334455667788", tp1.SpanID)
	}

	h2 := captureHeaders(t, c, context.Background())
	tp2, err := trace.Parse(h2.Get("Traceparent"))
	if err != nil {
		t.Fatalf("parse h2: %v", err)
	}
	if tp2.SpanID != "aabbccddeeff0011" {
		t.Errorf("span_id2 = %q, want aabbccddeeff0011", tp2.SpanID)
	}
}
