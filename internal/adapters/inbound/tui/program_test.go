package tui_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestProgramReturnsBridgeImplementingEventSink(t *testing.T) {
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close() //nolint:errcheck

	if p.Bridge() == nil {
		t.Fatal("Bridge() returned nil")
	}
	// Bridge implements EventSink — a snapshot call must not error.
	if err := p.Bridge().OnSnapshot(context.Background(), &domain.Change{ID: domain.ChangeID("01HX")}); err != nil {
		t.Errorf("OnSnapshot: %v", err)
	}
}

func TestProgramRunReturnsEmptyHintOnNaturalExit(t *testing.T) {
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HXABC"),
		Output:   io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Ask the bridge to deliver a CompleteMsg — Update returns tea.Quit.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hint, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Per §2.2, when the user detaches we print the reattach hint. When
	// the program ends naturally (CompleteMsg), the hint is empty.
	if hint != "" {
		t.Errorf("hint after natural exit = %q, want empty", hint)
	}
}

func TestProgramRunReturnsHintOnCtxCancel(t *testing.T) {
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Run blocks until ctx cancels → the program quits.
	// Hint may be empty (no detach key was pressed) — we just verify
	// Run returns within timeout without panic.
	doneCh := make(chan struct{})
	go func() {
		_, _ = p.Run(ctx)
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of ctx cancel")
	}
}
