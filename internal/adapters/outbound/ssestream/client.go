package ssestream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sse "github.com/tmaxmax/go-sse"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// DefaultStreamPath is the URL template for the SSE event stream. The single
// %s is replaced with the PhaseID string. sophia-wire-v1 §4.3 + D-M10-05:
// canonical streams are phase-scoped; the change-scoped path was removed
// in v0.2.0 in favor of the multiplexer wrapping per-phase subscriptions.
const DefaultStreamPath = "/api/v1/phases/%s/events"

// Config is the constructor input for Client.
type Config struct {
	// BaseURL is the scheme+host (no trailing slash), e.g. "https://api.example.com".
	BaseURL string
	// Path overrides DefaultStreamPath. Use %s where the ChangeID should go.
	Path string
	// HTTP is an optional pre-built client. Defaults to a client with no
	// timeout (required for long-lived SSE streams).
	HTTP *http.Client
	// Backoff configures the outer reconnect delay. Zero-value defaults to
	// DefaultBackoffMin/Max.
	Backoff BackoffConfig
	// MaxRetries caps the number of consecutive failed reconnects before the
	// goroutine gives up and closes the channel. Zero/negative defaults to
	// DefaultMaxRetries.
	MaxRetries int
	// Heartbeat is the watchdog timeout — how long without any event (including
	// heartbeats) before we force a reconnect. Defaults to DefaultHeartbeat.
	Heartbeat time.Duration
	// Clock injects a time source; nil defaults to real wall-clock. Only
	// useful in tests that control the watchdog.
	Clock Clock
	// APIKey is the X-Sophia-API-Key header value (sophia-wire-v1 §3.1).
	// Empty key = anonymous; bootstrap MUST only allow this when the
	// orchestrator URL is loopback (D-M10-02). MUST NOT be logged.
	APIKey string
}

// Client implements outbound.EventStreamClient by connecting to the
// orchestrator's SSE endpoint and translating wire events into domain.Events.
// It handles:
//   - Automatic reconnect with exponential backoff (outer loop we own)
//   - Last-Event-ID resumption on reconnect
//   - Heartbeat watchdog (force reconnect when server goes silent)
//   - Retry budget (give up after N consecutive failures)
//   - go-sse is used for transport only — with its internal retry disabled
//     (MaxRetries: -1) so our outer loop owns the reconnect policy.
type Client struct {
	base       string
	path       string
	http       *http.Client
	backoffCfg BackoffConfig
	maxRetries int
	heartbeat  time.Duration
	clock      Clock
	apiKey     string
}

// New constructs a Client from cfg. All zero fields get sensible defaults.
func New(cfg Config) *Client {
	path := cfg.Path
	if path == "" {
		path = DefaultStreamPath
	}
	if !strings.Contains(path, "%s") {
		panic("ssestream: Config.Path must contain %s for ChangeID substitution")
	}
	hc := cfg.HTTP
	if hc == nil {
		// Zero timeout = no timeout, which is required for long-lived SSE
		// streams. We cannot use httpclient.New because that applies a 5 s
		// default when Timeout is 0.
		hc = &http.Client{}
	}
	heartbeat := cfg.Heartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeat
	}
	clk := cfg.Clock
	if clk == nil {
		clk = realClock{}
	}
	return &Client{
		base:       strings.TrimRight(cfg.BaseURL, "/"),
		path:       path,
		http:       hc,
		backoffCfg: cfg.Backoff,
		maxRetries: cfg.MaxRetries,
		heartbeat:  heartbeat,
		clock:      clk,
		apiKey:     cfg.APIKey,
	}
}

// Subscribe implements outbound.EventStreamClient.
// It starts a background goroutine that connects to the per-phase SSE
// stream identified by target.PhaseID (sophia-wire-v1 §4.3 / D-M10-05),
// translates events, and emits them on the returned channel.
// The channel is closed when:
//   - The caller cancels ctx
//   - The retry budget is exhausted
//   - An auth error (401/403) is received
//   - The orchestrator returns 410 phase_terminal_no_events — the
//     phase is already terminal and no further events will be emitted;
//     the caller MUST fall back to GET /api/v1/phases/{id} for state.
//
// The returned stop function cancels the stream and returns nil.
func (c *Client) Subscribe(ctx context.Context, target outbound.StreamTarget, opts outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	if target.PhaseID == "" {
		return nil, nil, errors.New("ssestream: target.PhaseID required")
	}
	url := c.buildURL(target)

	out := make(chan domain.Event, 16)
	ctx, cancel := context.WithCancel(ctx)
	var once sync.Once
	stop := func() error {
		once.Do(func() { cancel() })
		return nil
	}

	go c.run(ctx, url, opts.LastEventID, out)

	return out, stop, nil
}

// run is the long-lived goroutine. It owns the outer reconnect loop and
// closes out when it terminates.
func (c *Client) run(ctx context.Context, url, initialLastID string, out chan<- domain.Event) {
	defer close(out)

	backoff := NewBackoff(c.backoffCfg)
	budget := NewRetryBudget(c.maxRetries)
	watchdog := NewWatchdog(WatchdogConfig{Timeout: c.heartbeat, Clock: c.clock})
	defer watchdog.Stop()

	lastID := initialLastID

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		anyEvent, lastSeenID, err := c.connectOnce(ctx, url, lastID, out, watchdog)
		if lastSeenID != "" {
			lastID = lastSeenID
		}
		if anyEvent {
			budget.Reset()
			backoff.Reset()
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if errors.Is(err, errAuthAbort) {
			return
		}
		// 410 phase_terminal_no_events: stop immediately. Reconnecting
		// would just 410 again until the retry budget exhausted; the
		// caller's stream-end-then-snapshot path handles terminal phase
		// state via GET /api/v1/phases/{id}.
		if errors.Is(err, ErrPhaseTerminal) {
			return
		}
		if !budget.TryUse() {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff.Next()):
		}
	}
}

// errAuthAbort is returned by connectOnce when the server replies 401/403.
// The outer loop treats this as a permanent failure.
var errAuthAbort = errors.New("ssestream: unauthorized")

// ErrPhaseTerminal is surfaced by Subscribe (via the stream-end path)
// when the orchestrator returns 410 phase_terminal_no_events
// (sophia-wire-v1 §9.2). The caller MUST fetch the phase snapshot via
// GET /api/v1/phases/{id} for terminal state — no further events are
// coming.
var ErrPhaseTerminal = errors.New("ssestream: phase terminal — no further events")

// connectOnce performs a single connection attempt using go-sse, with its
// internal retry disabled (Backoff.MaxRetries: -1). The outer run loop owns
// reconnect policy.
//
// Race note: anyEvent and lastSeenID are written by the go-sse callback
// goroutine (inside conn.Connect) and read after Connect returns. This is
// safe because Connect serialises all callback invocations and returns only
// after the last one completes, establishing a happens-before edge.
func (c *Client) connectOnce(
	ctx context.Context,
	url string,
	lastID string,
	out chan<- domain.Event,
	watchdog *Watchdog,
) (anyEvent bool, lastSeenID string, err error) {
	// Per-attempt cancel: when watchdog or ctx fires we return from this function.
	// Cancelling attemptCtx unblocks the conn.Connect() goroutine so it exits
	// cleanly instead of leaking on the next reconnect attempt.
	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	defer cancelAttempt()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return false, lastID, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	// X-Sophia-API-Key per sophia-wire-v1 §3.1. Empty key omits the
	// header (anonymous; orchestrator MUST be loopback per D-M10-02).
	// The key value is NEVER logged.
	if c.apiKey != "" {
		req.Header.Set(contract.HeaderAPIKey, c.apiKey)
	}

	// Use a go-sse Client with MaxRetries: -1 so Connect() returns after a
	// single stream attempt. Our outer loop handles reconnect policy.
	sseClient := &sse.Client{
		HTTPClient: c.http,
		Backoff: sse.Backoff{
			MaxRetries: -1,
		},
	}
	conn := sseClient.NewConnection(req)
	watchdog.Reset()

	conn.SubscribeToAll(func(ev sse.Event) {
		raw := RawSSE{Type: ev.Type, ID: ev.LastEventID, Data: ev.Data}
		if IsHeartbeat(raw) {
			watchdog.Reset()
			anyEvent = true // heartbeats prove the connection is healthy → reset budget
			if raw.ID != "" {
				lastSeenID = raw.ID
			}
			return
		}
		domEv, ok := ParseEvent(raw)
		if !ok {
			return
		}
		watchdog.Reset()
		anyEvent = true
		if raw.ID != "" {
			lastSeenID = raw.ID
		}
		select {
		case out <- domEv:
		case <-ctx.Done():
		}
	})

	connDone := make(chan error, 1)
	go func() { connDone <- conn.Connect() }()

	select {
	case cerr := <-connDone:
		return anyEvent, lastSeenID, classifyConnectErr(cerr)
	case <-watchdog.Done():
		return anyEvent, lastSeenID, errors.New("watchdog: no heartbeat")
	case <-ctx.Done():
		return anyEvent, lastSeenID, ctx.Err()
	}
}

// classifyConnectErr normalises errors from conn.Connect().
func classifyConnectErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// go-sse wraps validator errors in *sse.ConnectionError. Scope the
	// substring check to the inner validator error for stability if
	// go-sse changes its outer wrapping format. Status 410 is the
	// phase_terminal_no_events signal (sophia-wire-v1 §9.2) — surface
	// it as a typed error so the runner can fall back to a snapshot.
	var connErr *sse.ConnectionError
	if errors.As(err, &connErr) && connErr.Err != nil {
		msg := connErr.Err.Error()
		switch {
		case strings.Contains(msg, "401"), strings.Contains(msg, "403"):
			return errAuthAbort
		case strings.Contains(msg, "410"):
			return ErrPhaseTerminal
		}
	}
	return err
}

// buildURL constructs the full URL for the given target. The path
// template's single %s is substituted with target.PhaseID per the
// per-phase stream model (sophia-wire-v1 §4.3 / D-M10-05).
func (c *Client) buildURL(target outbound.StreamTarget) string {
	path := c.path
	if strings.Contains(path, "%s") {
		path = fmt.Sprintf(path, target.PhaseID)
	}
	return c.base + path
}
