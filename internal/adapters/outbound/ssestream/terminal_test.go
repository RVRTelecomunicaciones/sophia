package ssestream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// TestSSE_PhaseTerminalNoEvents asserts the client closes the channel
// promptly when the orchestrator returns 410 phase_terminal_no_events
// (sophia-wire-v1 §9.2). The channel close — not a return error — is
// the ABI; the runner's stream-end-then-snapshot loop handles the
// follow-up state probe.
func TestSSE_PhaseTerminalNoEvents(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"code":"phase_terminal_no_events","error":"x"}`))
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		MaxRetries: 5, // would normally exhaust on repeated failures
		Heartbeat:  100 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{
		ChangeID: domain.ChangeID("01HX"), PhaseID: "01PHASE",
	}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	// 410 should close the channel promptly without burning the retry
	// budget — the loop must NOT reconnect on terminal phases.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel yielded an event; expected immediate close on 410")
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("channel did not close within budget after 410")
	}
	if hits > 2 {
		t.Errorf("server hit %d times; expected ≤2 (no retry storm on 410)", hits)
	}
}
