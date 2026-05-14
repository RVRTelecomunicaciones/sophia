// Package httpclient builds the *http.Client used by every outbound HTTP
// adapter. Centralizing here ensures consistent timeouts, redirect policy,
// and ADR-0005 P2.2b W3C Traceparent emission.
package httpclient

import (
	"crypto/rand"
	"io"
	"net/http"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
)

// DefaultTimeout is applied when Config.Timeout is zero.
const DefaultTimeout = 5 * time.Second

// Config controls the constructed client.
type Config struct {
	// Timeout caps the request lifetime. Defaults to DefaultTimeout when zero.
	// Use a zero value with NewNoTimeout for long-lived streams.
	Timeout time.Duration
	// Trace is the top-level Trace minted once per CLI invocation. When set,
	// every outbound request carries a Traceparent header whose trace_id
	// equals Trace.TraceID and whose span_id is freshly rotated per request
	// (ADR-0005 P2.2b). When the zero value, no header is added — the binary
	// can still function in environments where trace generation failed.
	Trace trace.Trace
	// RandSource is the random source used to mint per-request span_ids.
	// Defaults to crypto/rand.Reader. Tests inject a deterministic reader.
	RandSource io.Reader
}

// New returns a configured *http.Client. When cfg.Trace is non-zero, the
// returned client wraps its Transport with a RoundTripper that adds a
// Traceparent header on every request, rotating span_id per call so each
// HTTP exchange is a fresh child span under the same CLI-invocation trace.
func New(cfg Config) *http.Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: wrapTransport(http.DefaultTransport, cfg.Trace, cfg.RandSource),
	}
}

// NewNoTimeout returns an *http.Client with no request timeout — required
// for long-lived SSE streams. Traceparent propagation works the same way:
// the initial GET carries Traceparent with a single span_id; the long-lived
// stream reuses that span for its entire lifetime (no rotation per event).
func NewNoTimeout(cfg Config) *http.Client {
	return &http.Client{
		Transport: wrapTransport(http.DefaultTransport, cfg.Trace, cfg.RandSource),
	}
}

// WrapTransport is the public alias for wrapTransport. Adapters that build
// their own *http.Client (e.g. SSE streams that need no timeout) call this
// to attach the W3C Traceparent RoundTripper.
func WrapTransport(base http.RoundTripper, t trace.Trace, randSrc io.Reader) http.RoundTripper {
	return wrapTransport(base, t, randSrc)
}

// wrapTransport wraps base with a traceparent-emitting RoundTripper iff a
// non-zero Trace is supplied. Returning base unchanged when Trace is zero
// keeps the production path zero-cost when the bootstrap fails to mint a
// trace (defensive coding for V1; in normal operation Trace is always set).
func wrapTransport(base http.RoundTripper, t trace.Trace, randSrc io.Reader) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if t.TraceID == "" {
		return base
	}
	if randSrc == nil {
		randSrc = rand.Reader
	}
	return &traceRoundTripper{base: base, parent: t, rand: randSrc}
}

// traceRoundTripper is an http.RoundTripper that mints a fresh span_id for
// every outbound request (child span of the CLI-invocation trace) and sets
// it as the Traceparent header on a cloned request. The original request
// must not be mutated — RoundTripper contract §RoundTrip.
type traceRoundTripper struct {
	base   http.RoundTripper
	parent trace.Trace
	rand   io.Reader
}

// RoundTrip implements http.RoundTripper. It chooses the Trace for the
// request in priority order:
//
//  1. trace.FromContext(req.Context()) — caller-supplied per-call override.
//     Used by SSE: it pins a single span across the long-lived stream by
//     placing a pre-rotated Trace in the context.
//  2. parent.WithNewSpan(rand) — the default: rotate a fresh span_id under
//     the CLI-invocation trace_id.
//
// If span minting fails (rand.Reader exhausted in tests, etc.) the request
// proceeds without a Traceparent rather than failing the request.
func (rt *traceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	if cloned.Header == nil {
		cloned.Header = http.Header{}
	}
	t, ok := trace.FromContext(req.Context())
	if !ok {
		child, err := rt.parent.WithNewSpan(rt.rand)
		if err != nil {
			return rt.base.RoundTrip(cloned) //nolint:wrapcheck // surface upstream error verbatim
		}
		t = child
	}
	cloned.Header.Set("Traceparent", t.String())
	return rt.base.RoundTrip(cloned) //nolint:wrapcheck // surface upstream error verbatim
}
