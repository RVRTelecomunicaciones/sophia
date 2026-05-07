package tui_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
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

func TestProgramOpenBrowserMsgInvokesBrowser(t *testing.T) {
	fb := fakes.NewFakeBrowser()
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   io.Discard,
		Browser:  fb,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()

	// Wait for Run to start.
	time.Sleep(50 * time.Millisecond)

	p.SendForTest(tui.OpenBrowserMsg{URL: "https://gov.local/x"})

	// Wait for browser fake to record the call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fb.GetOpened()) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	opened := fb.GetOpened()
	if len(opened) != 1 {
		t.Fatalf("FakeBrowser.Opened = %v, want 1 entry", opened)
	}
	if opened[0] != "https://gov.local/x" {
		t.Errorf("Opened URL = %q", opened[0])
	}

	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	if err := <-doneCh; err != nil {
		t.Errorf("Run returned: %v", err)
	}
}

func TestProgramOpenBrowserErrorPropagatesToModel(t *testing.T) {
	fb := fakes.NewFakeBrowser()
	fb.OpenErr = errors.New("xdg-open: not found")

	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   io.Discard,
		Browser:  fb,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()
	time.Sleep(50 * time.Millisecond)

	p.SendForTest(tui.OpenBrowserMsg{URL: "https://x"})
	time.Sleep(100 * time.Millisecond)

	state := p.Snapshot()
	found := false
	for _, e := range state.Errors() {
		if strings.Contains(e, "xdg-open") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an error line mentioning xdg-open; got %v", state.Errors())
	}

	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	<-doneCh
}

func TestProgramWithoutBrowserOpenBrowserMsgIsLoggedAsError(t *testing.T) {
	// No Browser injected — Browser is nil.
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()
	time.Sleep(50 * time.Millisecond)

	p.SendForTest(tui.OpenBrowserMsg{URL: "https://x"})
	time.Sleep(50 * time.Millisecond)

	state := p.Snapshot()
	found := false
	for _, e := range state.Errors() {
		if strings.Contains(strings.ToLower(e), "browser") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error mentioning browser; got %v", state.Errors())
	}

	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	<-doneCh
}
