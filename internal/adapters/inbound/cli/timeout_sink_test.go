package cli

import (
	"context"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// noopSink lets the tests inspect the wrapper without driving a full sink.
type noopSink struct{}

func (noopSink) OnSnapshot(context.Context, *domain.Change) error          { return nil }
func (noopSink) OnEvent(context.Context, domain.Event) error               { return nil }
func (noopSink) OnApprovalGate(context.Context, domain.ApprovalGate) error { return nil }
func (noopSink) OnError(context.Context, error) error                      { return nil }
func (noopSink) OnComplete(context.Context, domain.ChangeStatus) error     { return nil }
func (noopSink) Close() error                                              { return nil }

// Cambio 3: a second OnApprovalGate while the timer is already running must
// NOT reset the timer. The eager-arm timestamp from the FIRST call wins.
func TestApprovalTimeoutSinkDoesNotResetOnReGate(t *testing.T) {
	timeout := 60 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	g1 := domain.ApprovalGate{Phase: "implement", ChangeID: "X"}
	if err := sink.OnApprovalGate(context.Background(), g1); err != nil {
		t.Fatal(err)
	}

	// Burn ~half the budget, then re-arm with a richer gate. With the fix
	// the timer fires ~30ms from now (60ms from t=0). With the M7 bug it
	// would fire ~60ms from now (90ms from t=0). 45ms is the discriminator.
	time.Sleep(30 * time.Millisecond)
	g2 := domain.ApprovalGate{Phase: "implement", ChangeID: "X", URL: "https://x.test/g"}
	if err := sink.OnApprovalGate(context.Background(), g2); err != nil {
		t.Fatal(err)
	}

	select {
	case <-canceled:
		// fired close to original 60ms — pass.
	case <-time.After(45 * time.Millisecond):
		t.Fatal("timer was reset by the second OnApprovalGate; cambio 3 is not applied")
	}
	if err := sink.Wait(); err == nil {
		t.Error("expected errApprovalTimeout from Wait after timer fired")
	}
}

// Cambio 3: approval.resolved clears the gate; a SUBSEQUENT approval.required
// must start a fresh timer.
func TestApprovalTimeoutSinkResolvedThenNewGateStartsFresh(t *testing.T) {
	timeout := 50 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	if err := sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X"}); err != nil {
		t.Fatal(err)
	}
	// Resolve before timer fires.
	time.Sleep(10 * time.Millisecond)
	if err := sink.OnEvent(context.Background(), domain.Event{Type: "approval.resolved", EventID: "evt-r"}); err != nil {
		t.Fatal(err)
	}

	// Wait long enough that the OLD timer would have fired if it weren't stopped.
	time.Sleep(60 * time.Millisecond)
	select {
	case <-canceled:
		t.Fatal("approval.resolved did NOT stop the timer")
	default:
	}

	// Now arm a brand-new gate. The fresh timer must use the FULL timeout.
	if err := sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "verify", ChangeID: "X"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
		// fires after a fresh ~50ms — pass.
	case <-time.After(120 * time.Millisecond):
		t.Fatal("post-resolved gate did NOT start a fresh timer")
	}
}

// Cambio 3 (positive control): a re-armed timer from a SUBSEQUENT real event
// preserves the eager-arm timestamp — the caller observes the same fired-at
// point.
func TestApprovalTimeoutSinkSecondGatePreservesArmTime(t *testing.T) {
	timeout := 40 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	start := time.Now()
	_ = sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X"})
	time.Sleep(20 * time.Millisecond)
	_ = sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X", URL: "https://x.test/g"})

	select {
	case <-canceled:
		elapsed := time.Since(start)
		if elapsed >= 60*time.Millisecond {
			t.Errorf("timer was reset (fired after %s, expected ~%s)", elapsed, timeout)
		}
	case <-time.After(120 * time.Millisecond):
		t.Fatal("timer never fired")
	}
}
