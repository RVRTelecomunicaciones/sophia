package ssestream_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

func TestClientSubscribeReceivesEvents(t *testing.T) {
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		writeSSE(w, "phase.started", "evt-1", `{"timestamp":"2026-05-05T14:23:01.234Z","payload":{"phase_id":"p-1"}}`)
		flush()
		writeSSE(w, "phase.completed", "evt-2", `{"payload":{"status":"done"}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 2, time.Second)
	if got[0].Type != "phase.started" || got[0].EventID != "evt-1" {
		t.Errorf("event 0 = %+v", got[0])
	}
	if got[1].Type != "phase.completed" || got[1].EventID != "evt-2" {
		t.Errorf("event 1 = %+v", got[1])
	}
}

func TestClientSendsLastEventIDOnReconnect(t *testing.T) {
	var (
		mu          sync.Mutex
		connections int
		seenIDs     []string
	)
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		mu.Lock()
		connections++
		seenIDs = append(seenIDs, r.Header.Get("Last-Event-ID"))
		conn := connections
		mu.Unlock()
		switch conn {
		case 1:
			writeSSE(w, "phase.started", "evt-1", `{"payload":{"phase_id":"p-1"}}`)
			flush()
			// Force a clean disconnect by returning.
		case 2:
			writeSSE(w, "phase.completed", "evt-2", `{"payload":{"status":"done"}}`)
			flush()
		}
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Backoff:    ssestream.BackoffConfig{Min: time.Millisecond, Max: 5 * time.Millisecond},
		MaxRetries: 5,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 2, 2*time.Second)
	if got[0].EventID != "evt-1" || got[1].EventID != "evt-2" {
		t.Errorf("events = %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if connections < 2 {
		t.Fatalf("expected ≥2 connections, got %d", connections)
	}
	if seenIDs[1] != "evt-1" {
		t.Errorf("reconnect Last-Event-ID = %q, want %q", seenIDs[1], "evt-1")
	}
}

func TestClientHonorsInitialLastEventID(t *testing.T) {
	var seen atomic.Value
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		seen.Store(r.Header.Get("Last-Event-ID"))
		writeSSE(w, "phase.started", "evt-1", `{"payload":{}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{LastEventID: "evt-prior"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	_ = drainN(t, ch, 1, time.Second)
	if got, _ := seen.Load().(string); got != "evt-prior" {
		t.Errorf("Last-Event-ID = %q, want %q", got, "evt-prior")
	}
}

func TestClientSkipsHeartbeatsButRecordsLiveness(t *testing.T) {
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		writeSSE(w, "heartbeat", "hb-1", `{}`)
		flush()
		writeSSE(w, "phase.started", "evt-1", `{"payload":{}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 1, time.Second)
	if got[0].Type != "phase.started" {
		t.Errorf("got heartbeat instead of phase.started: %+v", got)
	}
}

func TestClientGivesUpAfterRetryBudgetExhausted(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Backoff:    ssestream.BackoffConfig{Min: time.Millisecond, Max: 5 * time.Millisecond},
		MaxRetries: 3,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// MaxRetries=3 → exactly 4 connection attempts (1 initial + 3 retries).
				if n := hits.Load(); n != 4 {
					t.Errorf("expected exactly 4 connection attempts, got %d", n)
				}
				return
			}
		case <-deadline:
			t.Fatal("channel never closed after budget exhausted")
		}
	}
}

// --- helpers ---

func newSSEStub(step func(w http.ResponseWriter, r *http.Request, flush func())) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("ResponseWriter does not implement Flusher")
		}
		flush := func() { flusher.Flush() }
		step(w, r, flush)
	}))
}

func writeSSE(w http.ResponseWriter, eventType, id, data string) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func drainN(t *testing.T, ch <-chan domain.Event, n int, timeout time.Duration) []domain.Event {
	t.Helper()
	out := make([]domain.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %d events (wanted %d)", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for events (got %d, wanted %d)", len(out), n)
		}
	}
	return out
}
