package ssestream_test

import (
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
)

func TestBackoffStartsAt1s(t *testing.T) {
	b := ssestream.NewBackoff(ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second})
	if d := b.Next(); d != time.Second {
		t.Errorf("first delay = %v, want 1s", d)
	}
}

func TestBackoffDoublesUntilMax(t *testing.T) {
	b := ssestream.NewBackoff(ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second})
	wants := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
		30 * time.Second,
	}
	for i, want := range wants {
		if d := b.Next(); d != want {
			t.Errorf("Next #%d = %v, want %v", i, d, want)
		}
	}
}

func TestBackoffResetReturnsToMin(t *testing.T) {
	b := ssestream.NewBackoff(ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second})
	b.Next()
	b.Next()
	b.Reset()
	if d := b.Next(); d != time.Second {
		t.Errorf("after reset Next = %v, want 1s", d)
	}
}

func TestBackoffZeroConfigUsesDefaults(t *testing.T) {
	b := ssestream.NewBackoff(ssestream.BackoffConfig{})
	if d := b.Next(); d != time.Second {
		t.Errorf("default first = %v, want 1s", d)
	}
}

func TestRetryBudgetAllowsExactlyMax(t *testing.T) {
	b := ssestream.NewRetryBudget(5)
	for i := 0; i < 5; i++ {
		if !b.TryUse() {
			t.Errorf("attempt %d should be allowed", i+1)
		}
	}
	if b.TryUse() {
		t.Error("6th attempt should be rejected")
	}
}

func TestRetryBudgetResetClearsCount(t *testing.T) {
	b := ssestream.NewRetryBudget(2)
	b.TryUse()
	b.TryUse()
	if b.TryUse() {
		t.Fatal("3rd should fail")
	}
	b.Reset()
	if !b.TryUse() {
		t.Error("after reset 1st should succeed")
	}
}

func TestWatchdogFiresAfterTimeout(t *testing.T) {
	clk := &fakeClock{now: time.Now()}
	w := ssestream.NewWatchdog(ssestream.WatchdogConfig{
		Timeout: 60 * time.Second,
		Clock:   clk,
	})
	w.Reset()

	done := w.Done()

	clk.Advance(30 * time.Second) // not yet
	select {
	case <-done:
		t.Fatal("watchdog fired too early")
	case <-time.After(20 * time.Millisecond):
	}

	clk.Advance(31 * time.Second) // crosses 60s threshold
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not fire within 200ms after timeout")
	}
}

func TestWatchdogResetExtendsDeadline(t *testing.T) {
	clk := &fakeClock{now: time.Now()}
	w := ssestream.NewWatchdog(ssestream.WatchdogConfig{
		Timeout: 60 * time.Second,
		Clock:   clk,
	})
	w.Reset()
	done := w.Done()

	clk.Advance(45 * time.Second)
	w.Reset()                     // should restart the 60s window
	clk.Advance(45 * time.Second) // 90s total but only 45s since last Reset

	select {
	case <-done:
		t.Fatal("watchdog fired despite Reset")
	case <-time.After(20 * time.Millisecond):
	}

	clk.Advance(20 * time.Second)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not fire after fresh deadline")
	}
}

// Stop must prevent the watchdog from firing — even if the timeout window
// is later crossed. This is the contract Task 5's SSE client relies on
// when shutting down a connection cleanly.
func TestWatchdogStopPreventsLaterFire(t *testing.T) {
	clk := &fakeClock{now: time.Now()}
	w := ssestream.NewWatchdog(ssestream.WatchdogConfig{
		Timeout: 60 * time.Second,
		Clock:   clk,
	})
	w.Reset()
	w.Stop()

	clk.Advance(90 * time.Second) // well past the 60s timeout

	select {
	case <-w.Done():
		t.Fatal("watchdog fired after Stop()")
	case <-time.After(50 * time.Millisecond):
		// correct: Stop() prevented firing
	}
}

// fakeClock is a deterministic clock for watchdog tests. Advance moves wall
// time forward and immediately fires any timers whose deadline has passed.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	stopped  bool
}

func (c *fakeClock) AfterFunc(d time.Duration, fn func()) ssestream.StoppableTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{deadline: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	go func() {
		for {
			c.mu.Lock()
			now := c.now
			deadline := t.deadline
			stopped := t.stopped
			c.mu.Unlock()
			if stopped {
				return
			}
			if !now.Before(deadline) {
				fn()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return &fakeStoppableTimer{timer: t, clock: c}
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	// Give scheduled goroutines a tick to observe the new time.
	time.Sleep(20 * time.Millisecond)
}

type fakeStoppableTimer struct {
	timer *fakeTimer
	clock *fakeClock
}

func (s *fakeStoppableTimer) Stop() bool {
	s.clock.mu.Lock()
	defer s.clock.mu.Unlock()
	already := s.timer.stopped
	s.timer.stopped = true
	return !already
}

func (s *fakeStoppableTimer) Reset(d time.Duration) bool {
	s.clock.mu.Lock()
	defer s.clock.mu.Unlock()
	already := s.timer.stopped
	s.timer.stopped = false
	s.timer.deadline = s.clock.now.Add(d)
	return !already
}

func TestRetryBudgetZeroMaxUsesDefault(t *testing.T) {
	b := ssestream.NewRetryBudget(0)
	for i := 0; i < ssestream.DefaultMaxRetries; i++ {
		if !b.TryUse() {
			t.Fatalf("attempt %d should succeed with default max", i+1)
		}
	}
	if b.TryUse() {
		t.Error("attempt past default max should fail")
	}
}
