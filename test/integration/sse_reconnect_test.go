//go:build integration

package integration_test

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

// TestSSEReconnectRecoversFromBlip spins up an httptest server that emits
// 2 events, closes the connection, then on reconnect (verifying the
// Last-Event-ID header is set to "evt-2") emits 2 more events including a
// terminal phase.completed.
//
// Asserts: 4 events reach the channel in order, no duplicates, the second
// connection's Last-Event-ID header carries "evt-2".
func TestSSEReconnectRecoversFromBlip(t *testing.T) {
	var (
		mu          sync.Mutex
		connections int
		seenIDs     []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connections++
		seenIDs = append(seenIDs, r.Header.Get("Last-Event-ID"))
		conn := connections
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		switch conn {
		case 1:
			writeEvent(w, "phase.started", "evt-1", `{"payload":{"phase_id":"p-1"}}`)
			flusher.Flush()
			writeEvent(w, "phase.started", "evt-2", `{"payload":{"phase_id":"p-2"}}`)
			flusher.Flush()
			// Drop the connection — simulates a transient blip.
		case 2:
			writeEvent(w, "phase.completed", "evt-3", `{"payload":{"status":"running"}}`)
			flusher.Flush()
			writeEvent(w, "phase.completed", "evt-4", `{"payload":{"status":"done"}}`)
			flusher.Flush()
			// Server graceful close.
		}
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Path:       "/api/v1/changes/%s/events",
		Backoff:    ssestream.BackoffConfig{Min: 5 * time.Millisecond, Max: 20 * time.Millisecond},
		MaxRetries: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainAtLeast(t, ch, 4, 4*time.Second)

	wantIDs := []string{"evt-1", "evt-2", "evt-3", "evt-4"}
	if len(got) < len(wantIDs) {
		t.Fatalf("got %d events, want at least %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].EventID != want {
			t.Errorf("event %d EventID = %q, want %q", i, got[i].EventID, want)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if connections < 2 {
		t.Fatalf("expected >=2 connections, got %d", connections)
	}
	if len(seenIDs) < 2 || seenIDs[1] != "evt-2" {
		t.Errorf("reconnect Last-Event-ID = %v, want second to be %q", seenIDs, "evt-2")
	}
}

func TestSSESkipsHeartbeatsAndForwardsRest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		writeEvent(w, "heartbeat", "hb-1", `{}`)
		flusher.Flush()
		writeEvent(w, "phase.started", "evt-1", `{"payload":{}}`)
		flusher.Flush()
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL: srv.URL,
		Path:    "/api/v1/changes/%s/events",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASEXXXXXXXXXXXXXXXXXXX"}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainAtLeast(t, ch, 1, 2*time.Second)
	for _, ev := range got {
		if ev.Type == "heartbeat" {
			t.Errorf("heartbeat leaked to channel: %+v", ev)
		}
	}
	if got[0].Type != "phase.started" {
		t.Errorf("first non-heartbeat = %+v, want phase.started", got[0])
	}
}

func writeEvent(w http.ResponseWriter, eventType, id, data string) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func drainAtLeast(t *testing.T, ch <-chan domain.Event, n int, timeout time.Duration) []domain.Event {
	t.Helper()
	out := make([]domain.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				if len(out) >= n {
					return out
				}
				t.Fatalf("channel closed after %d events (wanted %d)", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for events (got %d, wanted %d)", len(out), n)
		}
	}
	return out
}
