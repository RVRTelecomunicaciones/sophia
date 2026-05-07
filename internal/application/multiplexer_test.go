package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

// TestMultiplexer_ResubscribesOnPhaseSwitch — Phase 4 Task 4.3 / D-M10-05.
// The multiplexer MUST close the per-phase stream when current_phase_id
// changes on the Change snapshot and re-subscribe to the new phase.
func TestMultiplexer_ResubscribesOnPhaseSwitch(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	var (
		mu              sync.Mutex
		seenSubscribes  []string
		closedFirstOnce bool
	)
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		mu.Lock()
		seenSubscribes = append(seenSubscribes, target.PhaseID)
		mu.Unlock()
		// First subscribe = phase A. Push one event then close to
		// simulate the phase ending. Before close, swap the change's
		// current_phase_id to B so the multiplexer's snapshot decides
		// to re-subscribe.
		if target.PhaseID != "phase-B" && !closedFirstOnce {
			closedFirstOnce = true
			go func() {
				stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-A"})
				orch.MutateChange(target.ChangeID, func(c *domain.Change) {
					c.CurrentPhaseID = "phase-B"
				})
				stream.Close(target)
			}()
			return
		}
		// Second subscribe = phase B. Push one event, mark change
		// terminal, close.
		go func() {
			stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-B"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	res, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q, want done", res.FinalStatus)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenSubscribes) < 2 {
		t.Fatalf("expected ≥2 subscriptions, got %d: %v", len(seenSubscribes), seenSubscribes)
	}
	if seenSubscribes[0] == seenSubscribes[1] {
		t.Errorf("expected different phase ids across subscribes, got %v", seenSubscribes)
	}
	if seenSubscribes[len(seenSubscribes)-1] != "phase-B" {
		t.Errorf("expected last subscribe to be phase-B, got %v", seenSubscribes)
	}
}

// TestMultiplexer_ShortCircuitsOnTerminalSnapshot asserts the runner
// does not subscribe at all when the initial snapshot is already
// terminal. Real flow: a `sophia attach <id>` on a finished change
// resolves to the cached final state without burning an SSE budget.
func TestMultiplexer_ShortCircuitsOnTerminalSnapshot(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	subscribed := make(chan struct{}, 1)
	stream.OnSubscribe = func(_ outbound.StreamTarget) { subscribed <- struct{}{} }

	// Seed a Change in terminal state.
	orch.SeedChange(&domain.Change{
		ID: "01CH", Status: domain.ChangeStatusDone,
	})
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	final, err := r.Observe(context.Background(),
		application.RunResult{ChangeID: "01CH"}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if final.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", final.FinalStatus)
	}
	select {
	case <-subscribed:
		t.Error("multiplexer must NOT subscribe when initial snapshot is terminal")
	case <-time.After(50 * time.Millisecond):
	}
}
