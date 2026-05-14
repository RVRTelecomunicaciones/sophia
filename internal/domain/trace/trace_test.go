package trace_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain/trace"
)

// deterministicReader returns a reader that cycles through the supplied bytes.
// Useful for making New / WithNewSpan outputs predictable in tests.
func deterministicReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}

// Test 1: New generates a well-formed, parseable Trace.
func TestNew_GeneratesValidTrace(t *testing.T) {
	seed := make([]byte, 24)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	tr, err := trace.New(deterministicReader(seed))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(tr.TraceID) != 32 {
		t.Errorf("TraceID len = %d, want 32", len(tr.TraceID))
	}
	if len(tr.SpanID) != 16 {
		t.Errorf("SpanID len = %d, want 16", len(tr.SpanID))
	}
	if !tr.Sampled {
		t.Error("Sampled = false, want true")
	}
}

// Test 2: New → String → Parse round-trip.
func TestNew_ParseRoundTrip(t *testing.T) {
	seed := make([]byte, 24)
	for i := range seed {
		seed[i] = byte(i + 0xAA)
	}
	tr, err := trace.New(deterministicReader(seed))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	header := tr.String()
	if !strings.HasPrefix(header, "00-") {
		t.Fatalf("header %q must begin with version 00", header)
	}
	parsed, err := trace.Parse(header)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.TraceID != tr.TraceID {
		t.Errorf("TraceID = %q, want %q", parsed.TraceID, tr.TraceID)
	}
	if parsed.SpanID != tr.SpanID {
		t.Errorf("SpanID = %q, want %q", parsed.SpanID, tr.SpanID)
	}
	if parsed.Sampled != tr.Sampled {
		t.Errorf("Sampled = %v, want %v", parsed.Sampled, tr.Sampled)
	}
}

// Test 3: WithNewSpan preserves TraceID and rotates SpanID.
func TestWithNewSpan_PreservesTraceIDRotatesSpanID(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	parent, err := trace.New(deterministicReader(seed[:24]))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	child, err := parent.WithNewSpan(deterministicReader(seed[24:]))
	if err != nil {
		t.Fatalf("WithNewSpan: %v", err)
	}
	if child.TraceID != parent.TraceID {
		t.Errorf("TraceID changed: parent=%q child=%q", parent.TraceID, child.TraceID)
	}
	if child.SpanID == parent.SpanID {
		t.Errorf("SpanID not rotated: %q", child.SpanID)
	}
	if !child.Sampled {
		t.Error("child.Sampled = false, want true (inherits from parent)")
	}
}

// Test 4: Parse rejects malformed input.
func TestParse_Rejects(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"too-few-segments", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7"},
		{"wrong-version", "01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"short-trace-id", "00-4bf92f35-00f067aa0ba902b7-01"},
		{"all-zero-trace-id", "00-00000000000000000000000000000000-00f067aa0ba902b7-01"},
		{"all-zero-span-id", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"},
		{"non-hex-trace-id", "00-ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ-00f067aa0ba902b7-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := trace.Parse(tc.in); err == nil {
				t.Errorf("Parse(%q) = nil error, want ErrInvalidTraceparent", tc.in)
			}
		})
	}
}

// Test 5: String formats both sampled and unsampled flags.
func TestString_SampledFlags(t *testing.T) {
	tr := trace.Trace{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", Sampled: true}
	if got, want := tr.String(), "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	tr.Sampled = false
	if got, want := tr.String(), "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// Test 6: Context round-trip — store and retrieve.
func TestContext_RoundTrip(t *testing.T) {
	seed := make([]byte, 24)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	tr, err := trace.New(deterministicReader(seed))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := trace.NewContext(context.Background(), tr)
	got, ok := trace.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext: not found")
	}
	if got.TraceID != tr.TraceID || got.SpanID != tr.SpanID {
		t.Errorf("FromContext = %+v, want %+v", got, tr)
	}

	if _, ok := trace.FromContext(context.Background()); ok {
		t.Error("FromContext on empty context = ok, want !ok")
	}
}
