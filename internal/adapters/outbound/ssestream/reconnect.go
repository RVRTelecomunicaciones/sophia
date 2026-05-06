package ssestream

import (
	"sync"
	"time"
)

// Default backoff bounds per spec §5.7.
const (
	DefaultBackoffMin = time.Second
	DefaultBackoffMax = 30 * time.Second
	DefaultMaxRetries = 5
	DefaultHeartbeat  = 60 * time.Second
)

// Clock abstracts time for deterministic tests. Production code uses
// realClock{}; tests inject a fake.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, fn func()) StoppableTimer
}

// StoppableTimer is the subset of *time.Timer the watchdog needs.
type StoppableTimer interface {
	Stop() bool
	Reset(d time.Duration) bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) AfterFunc(d time.Duration, fn func()) StoppableTimer {
	return time.AfterFunc(d, fn)
}

// BackoffConfig configures Backoff.
type BackoffConfig struct {
	Min time.Duration // default 1s
	Max time.Duration // default 30s
}

// Backoff implements spec §5.7 exponential reconnect backoff:
// 1s → 2s → 4s → 8s → 16s, capped at Max.
type Backoff struct {
	mu  sync.Mutex
	min time.Duration
	max time.Duration
	cur time.Duration
}

// NewBackoff constructs a Backoff. Zero-value config defaults to 1s/30s.
func NewBackoff(cfg BackoffConfig) *Backoff {
	if cfg.Min <= 0 {
		cfg.Min = DefaultBackoffMin
	}
	if cfg.Max <= 0 {
		cfg.Max = DefaultBackoffMax
	}
	return &Backoff{min: cfg.Min, max: cfg.Max}
}

// Next returns the next backoff delay and advances internal state.
func (b *Backoff) Next() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur == 0 {
		b.cur = b.min
		return b.cur
	}
	b.cur *= 2
	if b.cur > b.max {
		b.cur = b.max
	}
	return b.cur
}

// Reset resets the backoff so the next Next() returns Min.
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cur = 0
}

// RetryBudget enforces spec §5.7's "max 5 retries per phase" cap.
type RetryBudget struct {
	mu       sync.Mutex
	max      int
	consumed int
}

// NewRetryBudget constructs a budget with the given cap. Zero or negative
// max defaults to DefaultMaxRetries.
func NewRetryBudget(max int) *RetryBudget {
	if max <= 0 {
		max = DefaultMaxRetries
	}
	return &RetryBudget{max: max}
}

// TryUse decrements the budget. Returns false when exhausted.
func (b *RetryBudget) TryUse() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consumed >= b.max {
		return false
	}
	b.consumed++
	return true
}

// Reset clears the consumed counter — called on successful event reception
// so a clean stretch of streaming restores the full retry budget.
func (b *RetryBudget) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consumed = 0
}

// WatchdogConfig configures Watchdog.
type WatchdogConfig struct {
	Timeout time.Duration // default 60s
	Clock   Clock         // default realClock{}
}

// Watchdog fires its Done channel when no Reset() has been called within
// Timeout. Used to force a reconnect when the orchestrator stops sending
// heartbeats — spec §5.7 row 5.
type Watchdog struct {
	mu      sync.Mutex
	timeout time.Duration
	clock   Clock
	timer   StoppableTimer
	done    chan struct{}
	closed  bool
}

// NewWatchdog constructs a Watchdog. The watchdog is INACTIVE until Reset()
// is called for the first time.
func NewWatchdog(cfg WatchdogConfig) *Watchdog {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultHeartbeat
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	return &Watchdog{
		timeout: cfg.Timeout,
		clock:   cfg.Clock,
		done:    make(chan struct{}),
	}
}

// Reset (re)starts the timeout window. Idempotent — safe to call from any
// goroutine on every received event.
func (w *Watchdog) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if w.timer == nil {
		w.timer = w.clock.AfterFunc(w.timeout, w.fire)
		return
	}
	w.timer.Reset(w.timeout)
}

// Done returns a channel that closes when the watchdog fires.
func (w *Watchdog) Done() <-chan struct{} {
	return w.done
}

// Stop disables the watchdog. After Stop, Reset is a no-op.
func (w *Watchdog) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
}

func (w *Watchdog) fire() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	close(w.done)
}
