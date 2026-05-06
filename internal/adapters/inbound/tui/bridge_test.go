package tui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// fakeSender records every tea.Msg the bridge forwards. It is goroutine-safe
// because the bridge is allowed to call Send from any goroutine.
type fakeSender struct {
	mu      sync.Mutex
	msgs    []any
	release chan struct{} // nil = unblocked; non-nil unbuffered = blocked until close
}

func newFakeSender() *fakeSender {
	return &fakeSender{}
}

func (s *fakeSender) Send(m any) {
	s.mu.Lock()
	r := s.release
	s.mu.Unlock()
	if r != nil {
		<-r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
}

func (s *fakeSender) Messages() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]any, len(s.msgs))
	copy(out, s.msgs)
	return out
}

func (s *fakeSender) Block() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.release == nil {
		s.release = make(chan struct{})
	}
}

func (s *fakeSender) Release() {
	s.mu.Lock()
	r := s.release
	s.release = nil
	s.mu.Unlock()
	if r != nil {
		close(r)
	}
}

func TestBridgeForwardsSnapshotAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	change := &domain.Change{ID: domain.ChangeID("01HX"), Status: domain.ChangeStatusRunning}
	if err := b.OnSnapshot(context.Background(), change); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	if _, ok := got[0].(tui.SnapshotMsg); !ok {
		t.Errorf("expected SnapshotMsg, got %T (%+v)", got[0], got[0])
	}
}

func TestBridgeForwardsEventAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnEvent(context.Background(), domain.Event{Type: "phase.started", EventID: "evt-1"}); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	em, ok := got[0].(tui.EventMsg)
	if !ok {
		t.Fatalf("expected EventMsg, got %T", got[0])
	}
	if em.Event.Type != "phase.started" {
		t.Errorf("Event.Type = %q", em.Event.Type)
	}
}

func TestBridgeForwardsApprovalGateAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	gate := domain.ApprovalGate{URL: "http://gate", Phase: domain.PhaseApply}
	if err := b.OnApprovalGate(context.Background(), gate); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	am, ok := got[0].(tui.ApprovalGateMsg)
	if !ok {
		t.Fatalf("expected ApprovalGateMsg, got %T", got[0])
	}
	if am.Gate.URL != "http://gate" {
		t.Errorf("Gate.URL = %q", am.Gate.URL)
	}
}

func TestBridgeForwardsCompletionAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnComplete(context.Background(), domain.ChangeStatusDone); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	cm, ok := got[0].(tui.CompleteMsg)
	if !ok {
		t.Fatalf("expected CompleteMsg, got %T", got[0])
	}
	if cm.Status != domain.ChangeStatusDone {
		t.Errorf("Status = %q", cm.Status)
	}
}

func TestBridgeForwardsErrorAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnError(context.Background(), errSentinel); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	if _, ok := got[0].(tui.ErrorMsg); !ok {
		t.Errorf("expected ErrorMsg, got %T", got[0])
	}
}

func TestBridgeDropsHeartbeatFirstUnderPressure(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	// Fill the buffer (cap 256). The bridge runs Send on a worker goroutine,
	// so the first event is consumed immediately and gets stuck in Send;
	// subsequent events queue up in the channel until cap.
	// Push 257 to guarantee the buffer (256 slots) is full regardless of
	// whether the worker has dequeued one item yet.
	for i := 0; i < 257; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.started", EventID: ""})
	}

	// Now push a heartbeat — buffer is full, sender wedged. Drop expected.
	_ = b.OnEvent(context.Background(), domain.Event{Type: "heartbeat", EventID: "hb-1"})

	if got := b.DropsByCategory()[tui.DropCategoryHeartbeat]; got != 1 {
		t.Errorf("heartbeat drops = %d, want 1", got)
	}
}

func TestBridgeNeverDropsPhaseEvents(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	// Fill buffer with low-priority events so phase.* must displace them.
	for i := 0; i < 257; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}

	// 10 phase.* events — all 10 must enqueue (kicking out non-priority).
	for i := 0; i < 10; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "phase.started", EventID: "p-evt"})
	}

	// Release the sender; let it drain.
	s.Release()
	waitDrain(t, b, 2*time.Second)

	// Count phase.started messages received.
	gotPhase := 0
	for _, m := range s.Messages() {
		if em, ok := m.(tui.EventMsg); ok && em.Event.Type == "phase.started" {
			gotPhase++
		}
	}
	if gotPhase != 10 {
		t.Errorf("phase.* events forwarded = %d, want 10", gotPhase)
	}
	// Sanity: at least 10 non-priority events were dropped to make room.
	if got := b.DropsByCategory()[tui.DropCategoryAgentTask]; got < 10 {
		t.Errorf("agent.* drops = %d, want ≥10", got)
	}
}

func TestBridgeNeverDropsApprovalEvents(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	for i := 0; i < 257; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}
	if err := b.OnApprovalGate(context.Background(), domain.ApprovalGate{URL: "http://gate"}); err != nil {
		t.Fatal(err)
	}

	s.Release()
	waitDrain(t, b, 2*time.Second)

	saw := false
	for _, m := range s.Messages() {
		if _, ok := m.(tui.ApprovalGateMsg); ok {
			saw = true
			break
		}
	}
	if !saw {
		t.Error("ApprovalGateMsg must never be dropped")
	}
}

func TestBridgeDropsCounterIncrementsWhenAtCapacity(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	for i := 0; i < 257; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}
	_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})

	if got := b.Drops(); got != 1 {
		t.Errorf("Drops() = %d, want 1", got)
	}
	if got := b.DropsByCategory()[tui.DropCategoryAgentTask]; got != 1 {
		t.Errorf("agent.* drops = %d, want 1", got)
	}
}

func TestBridgeCloseStopsForwarding(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})

	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	_ = b.OnEvent(context.Background(), domain.Event{Type: "phase.started"})

	time.Sleep(50 * time.Millisecond)
	if got := len(s.Messages()); got > 0 {
		t.Errorf("messages forwarded after Close: %d", got)
	}
}

var errSentinel = sentinelError("sentinel")

type sentinelError string

func (s sentinelError) Error() string { return string(s) }

func waitMessages(t *testing.T, s *fakeSender, n int, timeout time.Duration) []any { //nolint:unparam // n is a general-purpose threshold; callers may pass values other than 1
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := s.Messages(); len(got) >= n {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d messages (got %d)", n, len(s.Messages()))
	return nil
}

func waitDrain(t *testing.T, b *tui.Bridge, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b.Pending() == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("bridge did not drain in %s (pending=%d)", timeout, b.Pending())
}

// Regression for High/Medium reviewer concern: pending CompleteMsg must be
// delivered even if Close fires before the worker has drained the queue.
func TestBridgeDrainsPriorityItemsOnClose(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})

	// Enqueue a non-priority event AND a priority OnComplete in quick succession.
	_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	_ = b.OnComplete(context.Background(), domain.ChangeStatusDone)

	// Immediately close. The worker may or may not have drained either yet.
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	// Wait for the worker to wind down (it must finish the priority send).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		gotComplete := false
		for _, m := range s.Messages() {
			if _, ok := m.(tui.CompleteMsg); ok {
				gotComplete = true
				break
			}
		}
		if gotComplete {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("CompleteMsg never delivered — queued msgs after Close: %v", s.Messages())
}

// Regression: OnSnapshot must deep-copy the Phases slice. If the caller
// mutates c.Phases[i] after returning from OnSnapshot, the queued
// SnapshotMsg must NOT see the mutation.
func TestBridgeOnSnapshotDeepCopiesPhases(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	change := &domain.Change{
		ID:     domain.ChangeID("01HX"),
		Status: domain.ChangeStatusRunning,
		Phases: []domain.Phase{
			{ID: "p-init", Type: domain.PhaseInit, Status: domain.PhaseStatusRunning},
		},
	}
	if err := b.OnSnapshot(context.Background(), change); err != nil {
		t.Fatal(err)
	}

	// Mutate the caller's slice immediately after return.
	change.Phases[0].Status = domain.PhaseStatusDone

	got := waitMessages(t, s, 1, time.Second)
	sm := got[0].(tui.SnapshotMsg)
	if sm.Change.Phases[0].Status != domain.PhaseStatusRunning {
		t.Errorf("queued snapshot mutated by caller: phase[0].Status = %q, want running",
			sm.Change.Phases[0].Status)
	}
}
