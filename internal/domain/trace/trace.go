// Package trace provides W3C traceparent value objects and context helpers for
// E2E correlation-ID propagation (ADR-0005 P2.2b). The design is intentionally
// kept free of any OTEL SDK dependency — it uses only stdlib crypto/rand,
// encoding/hex, and context so it can be imported by every layer including
// domain code.
//
// Wire contract (mirrors sophia-orchestator P2.2a exactly):
//
//	Header:  Traceparent  (W3C trace-context Level 1)
//	Format:  00-<trace_id_32_hex>-<span_id_16_hex>-<flags_2_hex>
//	Sampling: always-on (flags=01) for V1; probabilistic sampling is Sprint 3.
//
// CLI semantics: trace_id is generated ONCE per CLI invocation (top-level Trace
// minted at bootstrap time). Each outbound HTTP/SSE call rotates the span_id
// via WithNewSpan so the server can correlate per-request spans inside the
// same CLI-invocation trace.
package trace

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInvalidTraceparent is returned by Parse when the supplied header value
// does not conform to the W3C trace-context spec (version 00).
var ErrInvalidTraceparent = errors.New("trace: invalid traceparent header")

// Trace is an immutable value object that carries the three fields of a W3C
// traceparent: TraceID (128-bit / 32 hex), SpanID (64-bit / 16 hex), and
// Sampled (flags bit 0). All fields are lower-case hex per the spec.
type Trace struct {
	TraceID string // 32 lower-case hex chars
	SpanID  string // 16 lower-case hex chars
	Sampled bool
}

// New generates a fresh Trace with a random 128-bit TraceID, a random 64-bit
// SpanID, and Sampled=true (always-on sampling for V1 local dev).
//
// rand should be crypto/rand.Reader in production. Tests may supply a
// deterministic io.Reader (e.g. bytes.NewReader) for reproducibility.
func New(rand io.Reader) (Trace, error) {
	traceBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand, traceBytes); err != nil {
		return Trace{}, fmt.Errorf("trace.New: read trace_id: %w", err)
	}
	spanBytes := make([]byte, 8)
	if _, err := io.ReadFull(rand, spanBytes); err != nil {
		return Trace{}, fmt.Errorf("trace.New: read span_id: %w", err)
	}
	return Trace{
		TraceID: hex.EncodeToString(traceBytes),
		SpanID:  hex.EncodeToString(spanBytes),
		Sampled: true,
	}, nil
}

// Parse decodes a W3C traceparent header value into a Trace. It enforces:
//   - exactly 4 dash-separated segments
//   - version field must be "00"
//   - trace_id must be 32 lower-case hex chars and not all zeros
//   - span_id must be 16 lower-case hex chars and not all zeros
//   - flags field must be exactly 2 hex chars
//
// Returns ErrInvalidTraceparent on any violation. The CLI uses Parse only to
// confirm the orchestator echoed back the Traceparent it received (optional
// debug-level log line).
func Parse(traceparent string) (Trace, error) {
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return Trace{}, fmt.Errorf("%w: expected 4 segments, got %d", ErrInvalidTraceparent, len(parts))
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]

	if version != "00" {
		return Trace{}, fmt.Errorf("%w: unsupported version %q", ErrInvalidTraceparent, version)
	}
	if err := validateHex(traceID, 32, "trace_id"); err != nil {
		return Trace{}, err
	}
	if allZeros(traceID) {
		return Trace{}, fmt.Errorf("%w: trace_id must not be all zeros", ErrInvalidTraceparent)
	}
	if err := validateHex(spanID, 16, "span_id"); err != nil {
		return Trace{}, err
	}
	if allZeros(spanID) {
		return Trace{}, fmt.Errorf("%w: span_id must not be all zeros", ErrInvalidTraceparent)
	}
	if len(flags) != 2 {
		return Trace{}, fmt.Errorf("%w: flags must be 2 hex chars", ErrInvalidTraceparent)
	}
	flagBytes, err := hex.DecodeString(flags)
	if err != nil {
		return Trace{}, fmt.Errorf("%w: flags not valid hex: %s", ErrInvalidTraceparent, err)
	}
	return Trace{
		TraceID: strings.ToLower(traceID),
		SpanID:  strings.ToLower(spanID),
		Sampled: flagBytes[0]&0x01 != 0,
	}, nil
}

// String formats the Trace as a W3C traceparent header value.
// Sampled=true → flags "01"; Sampled=false → flags "00".
func (t Trace) String() string {
	flags := "00"
	if t.Sampled {
		flags = "01"
	}
	return "00-" + t.TraceID + "-" + t.SpanID + "-" + flags
}

// WithNewSpan returns a new Trace that shares the same TraceID (so the
// correlation chain is preserved) but has a fresh random SpanID.  This is
// used by outbound HTTP/SSE adapters to represent a child span without
// running a full OTEL SDK.
func (t Trace) WithNewSpan(rand io.Reader) (Trace, error) {
	spanBytes := make([]byte, 8)
	if _, err := io.ReadFull(rand, spanBytes); err != nil {
		return Trace{}, fmt.Errorf("trace.WithNewSpan: read span_id: %w", err)
	}
	return Trace{
		TraceID: t.TraceID,
		SpanID:  hex.EncodeToString(spanBytes),
		Sampled: t.Sampled,
	}, nil
}

// validateHex returns ErrInvalidTraceparent if s is not valid lower-case hex
// of the expected length.
func validateHex(s string, expectedLen int, field string) error {
	if len(s) != expectedLen {
		return fmt.Errorf("%w: %s must be %d hex chars, got %d", ErrInvalidTraceparent, field, expectedLen, len(s))
	}
	if !isHex(s) {
		return fmt.Errorf("%w: %s contains non-hex characters", ErrInvalidTraceparent, field)
	}
	return nil
}

// isHex reports whether s consists entirely of [0-9a-fA-F].
func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return len(s) > 0
}

// allZeros reports whether a hex string encodes all-zero bytes.
func allZeros(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
