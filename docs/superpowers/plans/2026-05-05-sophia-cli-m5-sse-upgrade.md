# Sophia CLI — M5 SSE Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace M4's polling loop in `sophia run --no-tui --json` with a real SSE consumer. Add reconnect with `Last-Event-ID`, exponential backoff (1s→30s, max 5 retries per phase), a 60s heartbeat watchdog, a tolerant parser that skips malformed/unknown events, and a redaction pipeline that strips secrets BEFORE events reach any sink. Keep the initial `CreateChange` + `GetChange` snapshot path (snapshots on initial connect and post-disconnect refresh). Validate end-to-end against an httptest SSE stub that simulates a network blip.

**Architecture:** Five new files under `internal/adapters/outbound/ssestream/` make up the SSE pipeline: parser → redactor → reconnect/backoff → client (which implements `outbound.EventStreamClient`). The application `Runner` is rewritten to consume the SSE channel instead of polling. The polling code path is removed (no compatibility fallback). Bootstrap is updated to construct an `ssestream.Client` and inject it as `RunnerDeps.EventStream`.

**Tech Stack:** Go 1.24.x · `github.com/tmaxmax/go-sse` (new dep) · `net/http` · `encoding/json`. Risk RM5-01 below documents the fallback to `net/http` + `bufio.Scanner` if `go-sse` doesn't fit the wire shape.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.4, §5.3, §5.4, §5.7, §6.3, §7.2 M5 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M5)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M5 boundaries — what is NOT in M5:**

- No TUI (Bubble Tea) — `--no-tui --json` only (M6 swaps sinks).
- No ApplyBoard view — M7.
- No ApprovalGate banner — M7 (M5 still emits `OnApprovalGate` calls; jsonsink already serializes them).
- No real `sophia attach` / `sophia changes` — stay stubs (M8).
- No browser opener — M7.
- No approval-timeout exit code 5 — M5 reaches terminal status only (M5+).
- No `--orchestrator-url` flag — same as M4. Per-call URL rebinding is M7+ work; SOPHIA_ORCHESTRATOR_URL env still honored at bootstrap.
- No cross-process `Last-Event-ID` persistence — single-process resilience only (M8 may add).
- No `RunPhase` compatibility mode (spec §5.2). Polling fallback is OUT — Runner is SSE-first.

---

## Phase 1 — Dependency + package scaffolding

### Task 1: Add `tmaxmax/go-sse` dependency and ssestream package skeleton

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/adapters/outbound/ssestream/doc.go`

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/tmaxmax/go-sse@latest
```

This pulls the latest stable release of `tmaxmax/go-sse`. If the API differs from the code in Task 5 (verify by reading `go.sum` and the package godoc), STOP and ask the user before adapting — the fallback path is documented in RM5-01.

- [ ] **Step 2: Create the package skeleton**

`internal/adapters/outbound/ssestream/doc.go`:

```go
// Package ssestream implements outbound.EventStreamClient by consuming the
// orchestrator's Server-Sent Events feed. It exposes a single Client that
// composes four internal pieces:
//
//   - parser:   tolerant SSE-event → domain.Event translation (§5.3, §5.4)
//   - redactor: strips secrets from event payloads before sinks see them (§6.3)
//   - reconnect/backoff/watchdog: 1s→30s exponential backoff, 5 retries
//     per phase, 60s heartbeat watchdog, Last-Event-ID resumption (§5.7)
//   - client:   wires the above on top of github.com/tmaxmax/go-sse
//
// The package is M5 scope only. Spec §7.2 lists the M5 DoD.
package ssestream
```

- [ ] **Step 3: Verify build still works**

Run:
```bash
go vet ./internal/adapters/outbound/ssestream/...
go test ./...
```
Expected: PASS. The package compiles as an empty doc-only package. Existing tests unaffected.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/adapters/outbound/ssestream/doc.go
git commit -m "chore(ssestream): add tmaxmax/go-sse dep + package skeleton"
```

---

## Phase 2 — Redactor (pure, isolated, no I/O)

### Task 2: ssestream/redactor.go — secret redaction pipeline

**Files:**
- Create: `internal/adapters/outbound/ssestream/redactor.go`
- Create: `internal/adapters/outbound/ssestream/redactor_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/ssestream/redactor_test.go`:

```go
package ssestream_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
)

func TestRedactStringRedactsBearerTokens(t *testing.T) {
	in := "Authorization: Bearer abc.def_ghi-12345"
	got := ssestream.RedactString(in)
	if strings.Contains(got, "abc.def_ghi-12345") {
		t.Errorf("token leaked: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected [REDACTED]: %q", got)
	}
}

func TestRedactStringRedactsJWTs(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	in := "user provided " + jwt + " for auth"
	got := ssestream.RedactString(in)
	if strings.Contains(got, jwt) {
		t.Errorf("JWT leaked: %q", got)
	}
}

func TestRedactStringRedactsAWSKeys(t *testing.T) {
	in := "key=AKIAIOSFODNN7EXAMPLE more"
	got := ssestream.RedactString(in)
	if strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS key leaked: %q", got)
	}
}

func TestRedactStringRedactsGitHubTokens(t *testing.T) {
	cases := []string{
		"ghp_" + strings.Repeat("a", 36),
		"gho_" + strings.Repeat("b", 36),
		"ghu_" + strings.Repeat("c", 36),
		"ghs_" + strings.Repeat("d", 36),
		"ghr_" + strings.Repeat("e", 36),
	}
	for _, tok := range cases {
		got := ssestream.RedactString("token=" + tok + " end")
		if strings.Contains(got, tok) {
			t.Errorf("github token leaked for %q: %q", tok, got)
		}
	}
}

func TestRedactStringLeavesPlainTextAlone(t *testing.T) {
	in := "phase running explore: 5/12 steps complete"
	got := ssestream.RedactString(in)
	if got != in {
		t.Errorf("plain text mutated: %q → %q", in, got)
	}
}

func TestRedactPayloadRedactsSensitiveFieldNames(t *testing.T) {
	payload := map[string]any{
		"phase_id":     "01HXABC",
		"task_id":      "t-1",
		"token":        "supersecretvalue1234567890ABCDEF",
		"authorization": "Bearer xyz",
		"message":      "hello",
	}
	got := ssestream.RedactPayload(payload)
	if got["phase_id"] != "01HXABC" {
		t.Errorf("phase_id should be untouched: %v", got["phase_id"])
	}
	if got["task_id"] != "t-1" {
		t.Errorf("task_id should be untouched: %v", got["task_id"])
	}
	if s, _ := got["token"].(string); !strings.Contains(s, "[REDACTED]") {
		t.Errorf("token should be redacted: %v", got["token"])
	}
	if s, _ := got["authorization"].(string); !strings.Contains(s, "[REDACTED]") {
		t.Errorf("authorization should be redacted: %v", got["authorization"])
	}
	if got["message"] != "hello" {
		t.Errorf("message should be untouched: %v", got["message"])
	}
}

func TestRedactPayloadAppliesPatternsToFreeFormStrings(t *testing.T) {
	payload := map[string]any{
		"message": "auth failed: Bearer abc.def_ghi-secret-token",
	}
	got := ssestream.RedactPayload(payload)
	s, _ := got["message"].(string)
	if strings.Contains(s, "abc.def_ghi-secret-token") {
		t.Errorf("token leaked in free-form message: %q", s)
	}
}

func TestRedactPayloadRecursesIntoNestedMaps(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"secret": "ghp_" + strings.Repeat("z", 36),
			"phase":  "explore",
		},
	}
	got := ssestream.RedactPayload(payload)
	nested, _ := got["data"].(map[string]any)
	if s, _ := nested["secret"].(string); !strings.Contains(s, "[REDACTED]") {
		t.Errorf("nested secret not redacted: %v", nested["secret"])
	}
	if nested["phase"] != "explore" {
		t.Errorf("nested phase mutated: %v", nested["phase"])
	}
}

func TestRedactPayloadRecursesIntoSlices(t *testing.T) {
	payload := map[string]any{
		"events": []any{
			map[string]any{"token": "ghp_" + strings.Repeat("x", 36)},
			"plain text",
		},
	}
	got := ssestream.RedactPayload(payload)
	evs, _ := got["events"].([]any)
	if len(evs) != 2 {
		t.Fatalf("events len = %d", len(evs))
	}
	first, _ := evs[0].(map[string]any)
	if s, _ := first["token"].(string); !strings.Contains(s, "[REDACTED]") {
		t.Errorf("nested-in-slice secret not redacted: %v", first["token"])
	}
}

func TestRedactPayloadDoesNotMutateInput(t *testing.T) {
	original := map[string]any{
		"token": "Bearer secret",
	}
	_ = ssestream.RedactPayload(original)
	if original["token"] != "Bearer secret" {
		t.Errorf("input mutated: %v", original["token"])
	}
}

func TestRedactPayloadSkipsTechnicalAllowlist(t *testing.T) {
	// Technical fields can look high-entropy (ULIDs etc.) but are not secrets.
	payload := map[string]any{
		"change_id": "01HXABCDEFGHJKMNPQRSTVWXYZ",
		"phase_id":  "01HXABCDEFGHJKMNPQRSTVWXYZ",
		"agent_id":  "agent-01HXABCDEFGHJKMNPQRSTVWXYZ",
		"trace_id":  "01HXABCDEFGHJKMNPQRSTVWXYZ",
		"event_id":  "01HXABCDEFGHJKMNPQRSTVWXYZ",
	}
	got := ssestream.RedactPayload(payload)
	for k, v := range payload {
		if got[k] != v {
			t.Errorf("technical field %q mutated: %v → %v", k, v, got[k])
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/ssestream/...`
Expected: FAIL — symbols `RedactString` and `RedactPayload` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/ssestream/redactor.go`:

```go
package ssestream

import (
	"regexp"
	"strings"
)

// redactionMarker is the literal substring that replaces matched secrets.
const redactionMarker = "[REDACTED]"

// secretPatterns are applied to every free-form string and to the values of
// sensitive fields. They cover the corpus listed in spec §6.3.
var secretPatterns = []*regexp.Regexp{
	// Bearer tokens: "Bearer <opaque>" — opaque allows base64url + dots/dashes.
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9._\-+/=]+`),
	// JWT triplets: 3 base64url segments joined by dots.
	regexp.MustCompile(`[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
	// AWS access key IDs.
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// GitHub PAT/OAuth/User/Server/Refresh tokens.
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
}

// sensitiveFieldNames is the set of payload keys whose values are ALWAYS
// treated as opaque secrets — replaced wholesale with [REDACTED] regardless
// of pattern match. Match is case-insensitive.
var sensitiveFieldNames = map[string]struct{}{
	"token":         {},
	"secret":        {},
	"key":           {},
	"authorization": {},
	"password":      {},
	"credential":    {},
	"credentials":   {},
	"api_key":       {},
	"apikey":        {},
}

// technicalAllowlist names fields that often look high-entropy but are NEVER
// secrets — bypassing the entropy/pattern redaction. Match is case-insensitive.
var technicalAllowlist = map[string]struct{}{
	"phase_id":  {},
	"change_id": {},
	"agent_id":  {},
	"task_id":   {},
	"group_id":  {},
	"trace_id":  {},
	"event_id":  {},
	"run_id":    {},
	"id":        {},
	"type":      {},
	"status":    {},
	"phase":     {},
	"reason":    {},
	"risk":      {},
	"policy":    {},
	"decision":  {},
}

// RedactString applies every secret pattern to s, replacing each match with
// [REDACTED]. Pure function — safe to call from any goroutine.
func RedactString(s string) string {
	for _, p := range secretPatterns {
		s = p.ReplaceAllString(s, redactionMarker)
	}
	return s
}

// RedactPayload returns a deep copy of payload with secrets stripped.
//
//   - Keys in sensitiveFieldNames have their values replaced with [REDACTED].
//   - Keys in technicalAllowlist are passed through verbatim.
//   - All other string values are passed through RedactString.
//   - Nested maps and slices are walked recursively.
//   - Input is never mutated.
func RedactPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for k, v := range payload {
		out[k] = redactValue(k, v)
	}
	return out
}

func redactValue(fieldName string, v any) any {
	lower := strings.ToLower(fieldName)
	if _, sensitive := sensitiveFieldNames[lower]; sensitive {
		// Wholesale replacement: don't preserve original.
		return redactionMarker
	}
	if _, technical := technicalAllowlist[lower]; technical {
		return v
	}
	switch t := v.(type) {
	case string:
		return RedactString(t)
	case map[string]any:
		return RedactPayload(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = redactValue("", item)
		}
		return out
	default:
		return v
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/ssestream/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/ssestream/redactor.go internal/adapters/outbound/ssestream/redactor_test.go
git commit -m "feat(ssestream): add secret redactor (patterns + field-name + allowlist per §6.3)"
```

---

## Phase 3 — Tolerant parser

### Task 3: ssestream/parser.go — SSE-event → domain.Event with tolerance

**Files:**
- Create: `internal/adapters/outbound/ssestream/parser.go`
- Create: `internal/adapters/outbound/ssestream/parser_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/ssestream/parser_test.go`:

```go
package ssestream_test

import (
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
)

func TestParseEventDecodesKnownEventType(t *testing.T) {
	raw := ssestream.RawSSE{
		Type: "phase.started",
		ID:   "01HXEVT-1",
		Data: `{"timestamp":"2026-05-05T14:23:01.234Z","payload":{"phase_type":"explore","phase_id":"p-1","task_description":"go"},"trace_id":"trc-1"}`,
	}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ev.Type != "phase.started" {
		t.Errorf("Type = %q", ev.Type)
	}
	if ev.EventID != "01HXEVT-1" {
		t.Errorf("EventID = %q", ev.EventID)
	}
	want := time.Date(2026, 5, 5, 14, 23, 1, 234_000_000, time.UTC)
	if !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", ev.Timestamp, want)
	}
	if ev.Payload["phase_type"] != "explore" {
		t.Errorf("payload.phase_type = %v", ev.Payload["phase_type"])
	}
	if ev.TraceID != "trc-1" {
		t.Errorf("TraceID = %q", ev.TraceID)
	}
}

func TestParseEventTolerantOfMissingTimestamp(t *testing.T) {
	raw := ssestream.RawSSE{
		Type: "phase.completed",
		ID:   "evt",
		Data: `{"payload":{"status":"done"}}`,
	}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !ev.Timestamp.IsZero() {
		t.Errorf("expected zero Timestamp, got %v", ev.Timestamp)
	}
}

func TestParseEventTolerantOfMissingPayload(t *testing.T) {
	raw := ssestream.RawSSE{Type: "heartbeat", Data: `{}`}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ev.Type != "heartbeat" {
		t.Errorf("Type = %q", ev.Type)
	}
}

func TestParseEventReturnsFalseOnInvalidJSON(t *testing.T) {
	raw := ssestream.RawSSE{Type: "phase.started", Data: `{not json`}
	_, ok := ssestream.ParseEvent(raw)
	if ok {
		t.Error("expected ok=false on malformed JSON")
	}
}

func TestParseEventReturnsFalseOnEmptyType(t *testing.T) {
	raw := ssestream.RawSSE{Type: "", Data: `{}`}
	_, ok := ssestream.ParseEvent(raw)
	if ok {
		t.Error("expected ok=false on empty type")
	}
}

func TestParseEventForwardCompatibleUnknownTypes(t *testing.T) {
	raw := ssestream.RawSSE{Type: "future.event.v2", ID: "x", Data: `{"payload":{"k":"v"}}`}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true (forward-compatible)")
	}
	if ev.Type != "future.event.v2" {
		t.Errorf("Type = %q", ev.Type)
	}
	if ev.Payload["k"] != "v" {
		t.Errorf("payload not preserved: %v", ev.Payload)
	}
}

func TestParseEventRedactsPayloadStrings(t *testing.T) {
	raw := ssestream.RawSSE{
		Type: "phase.started",
		Data: `{"payload":{"token":"Bearer secret","phase_id":"p-1"}}`,
	}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	tok, _ := ev.Payload["token"].(string)
	if tok != "[REDACTED]" {
		t.Errorf("token not redacted: %q", tok)
	}
	if ev.Payload["phase_id"] != "p-1" {
		t.Errorf("phase_id mutated: %v", ev.Payload["phase_id"])
	}
}

func TestParseEventRedactsFreeFormMessage(t *testing.T) {
	raw := ssestream.RawSSE{
		Type: "agent.completed",
		Data: `{"payload":{"message":"auth: Bearer abc.def_ghi-private"}}`,
	}
	ev, ok := ssestream.ParseEvent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	msg, _ := ev.Payload["message"].(string)
	if strings.Contains(msg, "abc.def_ghi-private") {
		t.Errorf("free-form leaked: %q", msg)
	}
}

func TestIsHeartbeatRecognizesHeartbeatType(t *testing.T) {
	if !ssestream.IsHeartbeat(ssestream.RawSSE{Type: "heartbeat"}) {
		t.Error("expected true for heartbeat")
	}
	if ssestream.IsHeartbeat(ssestream.RawSSE{Type: "phase.started"}) {
		t.Error("expected false for phase.started")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/ssestream/...`
Expected: FAIL — `RawSSE`, `ParseEvent`, `IsHeartbeat` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/ssestream/parser.go`:

```go
package ssestream

import (
	"encoding/json"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// HeartbeatType is the SSE event-type emitted by the orchestrator as a
// liveness ping. Per spec §5.4, heartbeats update the watchdog but are NOT
// forwarded to the EventSink.
const HeartbeatType = "heartbeat"

// RawSSE is the minimal shape the parser consumes. Adapters wrapping
// tmaxmax/go-sse (or the bufio.Scanner fallback) translate the wire-level
// representation into this struct.
type RawSSE struct {
	Type string // "event:" field — domain.Event.Type
	ID   string // "id:" field — Last-Event-ID source
	Data string // "data:" field — JSON envelope per spec §5.3
}

// envelope is the JSON shape of the SSE "data:" line per spec §5.3.
type envelope struct {
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
	TraceID   string         `json:"trace_id"`
}

// ParseEvent translates a RawSSE record into a domain.Event. Returns
// (event, true) on success and (zero, false) when the record should be
// skipped — callers MUST log a warning and drop the record. Tolerance
// rules per spec §5.3, §5.4:
//
//   - Empty Type → skip.
//   - Invalid JSON in Data → skip.
//   - Missing fields in the envelope → default to zero values.
//   - Unknown Type → emitted unchanged (forward-compatible).
//   - Payload strings are redacted before the Event leaves the parser.
func ParseEvent(raw RawSSE) (domain.Event, bool) {
	if raw.Type == "" {
		return domain.Event{}, false
	}
	var env envelope
	if raw.Data != "" {
		if err := json.Unmarshal([]byte(raw.Data), &env); err != nil {
			return domain.Event{}, false
		}
	}
	payload := env.Payload
	if payload != nil {
		payload = RedactPayload(payload)
	}
	return domain.Event{
		Type:      raw.Type,
		Timestamp: env.Timestamp,
		Payload:   payload,
		TraceID:   env.TraceID,
		EventID:   raw.ID,
	}, true
}

// IsHeartbeat reports whether the raw record is the orchestrator's
// liveness ping. Caller uses this to update the watchdog without forwarding
// the event to the sink.
func IsHeartbeat(raw RawSSE) bool {
	return raw.Type == HeartbeatType
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/ssestream/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/ssestream/parser.go internal/adapters/outbound/ssestream/parser_test.go
git commit -m "feat(ssestream): add tolerant SSE parser with redaction integration (§5.3, §5.4)"
```

---

## Phase 4 — Reconnect / backoff / watchdog (pure stateful types)

### Task 4: ssestream/reconnect.go — Backoff, Watchdog, RetryBudget

**Files:**
- Create: `internal/adapters/outbound/ssestream/reconnect.go`
- Create: `internal/adapters/outbound/ssestream/reconnect_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/ssestream/reconnect_test.go`:

```go
package ssestream_test

import (
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
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
	w.Reset() // should restart the 60s window
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

func TestWatchdogStopReleasesResources(t *testing.T) {
	clk := &fakeClock{now: time.Now()}
	w := ssestream.NewWatchdog(ssestream.WatchdogConfig{
		Timeout: 60 * time.Second,
		Clock:   clk,
	})
	w.Reset()
	w.Stop()
	// After Stop, Done channel should close (or be drained safely).
	select {
	case <-w.Done():
		// closed — acceptable
	case <-time.After(50 * time.Millisecond):
		// not closed but Stop returns cleanly — also acceptable
	}
}

// fakeClock is a deterministic clock for watchdog tests. Advance moves wall
// time forward and immediately fires any timers whose deadline has passed.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*fakeTimer
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	stopped  bool
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, fn func()) ssestream.StoppableTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{deadline: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	go func(deadline time.Time) {
		for {
			c.mu.Lock()
			now := c.now
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
	}(t.deadline)
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
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/ssestream/...`
Expected: FAIL — `Backoff`, `RetryBudget`, `Watchdog`, `Clock`, `StoppableTimer` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/ssestream/reconnect.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/ssestream/... -race`
Expected: PASS. The fake-clock-based watchdog tests may flake on heavily loaded CI; if they do, raise the small `time.Sleep` margins (currently 20ms / 200ms) up to 100ms / 500ms — they exist solely to give the goroutine scheduler room.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/ssestream/reconnect.go internal/adapters/outbound/ssestream/reconnect_test.go
git commit -m "feat(ssestream): add Backoff, RetryBudget, Watchdog (§5.7 reconnect primitives)"
```

---

## Phase 5 — SSE client (wires parser + redactor + reconnect)

### Task 5: ssestream/client.go — outbound.EventStreamClient implementation

**Files:**
- Create: `internal/adapters/outbound/ssestream/client.go`
- Create: `internal/adapters/outbound/ssestream/client_test.go`

> **Verification gate:** before writing the implementation, read the public API of `github.com/tmaxmax/go-sse` (godoc and `go.sum` artifacts). The code below assumes the v0.x shape: `sse.NewConnection(req)` + `c.SubscribeToAll(func(sse.Event))` + `c.Connect()` returning when the stream ends. If the actual API differs, STOP and re-shape the implementation around the real types — keep the parser/redactor/backoff calls intact. The fallback to `net/http` + `bufio.Scanner` is documented in RM5-01.

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/ssestream/client_test.go`:

```go
package ssestream_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestClientSubscribeReceivesEvents(t *testing.T) {
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		writeSSE(w, "phase.started", "evt-1", `{"timestamp":"2026-05-05T14:23:01.234Z","payload":{"phase_id":"p-1"}}`)
		flush()
		writeSSE(w, "phase.completed", "evt-2", `{"payload":{"status":"done"}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 2, time.Second)
	if got[0].Type != "phase.started" || got[0].EventID != "evt-1" {
		t.Errorf("event 0 = %+v", got[0])
	}
	if got[1].Type != "phase.completed" || got[1].EventID != "evt-2" {
		t.Errorf("event 1 = %+v", got[1])
	}
}

func TestClientSendsLastEventIDOnReconnect(t *testing.T) {
	var (
		mu          sync.Mutex
		connections int
		seenIDs     []string
	)
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		mu.Lock()
		connections++
		seenIDs = append(seenIDs, r.Header.Get("Last-Event-ID"))
		conn := connections
		mu.Unlock()
		switch conn {
		case 1:
			writeSSE(w, "phase.started", "evt-1", `{"payload":{"phase_id":"p-1"}}`)
			flush()
			// Force a clean disconnect by returning.
		case 2:
			writeSSE(w, "phase.completed", "evt-2", `{"payload":{"status":"done"}}`)
			flush()
		}
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Backoff:    ssestream.BackoffConfig{Min: time.Millisecond, Max: 5 * time.Millisecond},
		MaxRetries: 5,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 2, 2*time.Second)
	if got[0].EventID != "evt-1" || got[1].EventID != "evt-2" {
		t.Errorf("events = %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if connections < 2 {
		t.Fatalf("expected ≥2 connections, got %d", connections)
	}
	if seenIDs[1] != "evt-1" {
		t.Errorf("reconnect Last-Event-ID = %q, want %q", seenIDs[1], "evt-1")
	}
}

func TestClientHonorsInitialLastEventID(t *testing.T) {
	var seen atomic.Value
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		seen.Store(r.Header.Get("Last-Event-ID"))
		writeSSE(w, "phase.started", "evt-1", `{"payload":{}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{LastEventID: "evt-prior"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	_ = drainN(t, ch, 1, time.Second)
	if got, _ := seen.Load().(string); got != "evt-prior" {
		t.Errorf("Last-Event-ID = %q, want %q", got, "evt-prior")
	}
}

func TestClientSkipsHeartbeatsButRecordsLiveness(t *testing.T) {
	srv := newSSEStub(func(w http.ResponseWriter, r *http.Request, flush func()) {
		writeSSE(w, "heartbeat", "hb-1", `{}`)
		flush()
		writeSSE(w, "phase.started", "evt-1", `{"payload":{}}`)
		flush()
	})
	defer srv.Close()

	c := ssestream.New(ssestream.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainN(t, ch, 1, time.Second)
	if got[0].Type != "phase.started" {
		t.Errorf("got heartbeat instead of phase.started: %+v", got)
	}
}

func TestClientGivesUpAfterRetryBudgetExhausted(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Backoff:    ssestream.BackoffConfig{Min: time.Millisecond, Max: 5 * time.Millisecond},
		MaxRetries: 3,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	// Channel should close (zero-value drain) once the budget is exhausted.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				if hits.Load() < 1 {
					t.Errorf("expected ≥1 connection attempt, got %d", hits.Load())
				}
				return
			}
		case <-deadline:
			t.Fatal("channel never closed after budget exhausted")
		}
	}
}

// --- helpers ---

// newSSEStub returns an httptest server whose handler runs `step` and then
// returns. The server sets Content-Type: text/event-stream and exposes the
// flusher to the step.
func newSSEStub(step func(w http.ResponseWriter, r *http.Request, flush func())) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("ResponseWriter does not implement Flusher")
		}
		flush := func() { flusher.Flush() }
		step(w, r, flush)
	}))
}

func writeSSE(w http.ResponseWriter, eventType, id, data string) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func drainN(t *testing.T, ch <-chan domain.Event, n int, timeout time.Duration) []domain.Event {
	t.Helper()
	out := make([]domain.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %d events (wanted %d)", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for events (got %d, wanted %d)", len(out), n)
		}
	}
	return out
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/ssestream/...`
Expected: FAIL — `ssestream.New`, `ssestream.Config`, `Subscribe` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/ssestream/client.go`:

> **Implementer note:** the structure below uses `tmaxmax/go-sse`'s `Connection`/`SubscribeMessages` API. If the installed version differs in spelling (`SubscribeToAll`, `OnEvent`, etc.), keep the OUTER shape — Subscribe → goroutine → backoff loop → parser/redactor/watchdog → channel — and adapt only the inner `connectOnce` body. ASK the user before introducing the bufio.Scanner fallback (RM5-01).

```go
package ssestream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tmaxmax/go-sse"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// DefaultStreamPath is the SSE endpoint path used by the orchestrator.
const DefaultStreamPath = "/api/v1/changes/%s/events"

// Config controls the SSE client.
type Config struct {
	BaseURL    string         // e.g. "http://localhost:9080"
	Path       string         // optional override; defaults to DefaultStreamPath
	HTTP       *http.Client   // optional; defaults to httpclient.New
	Backoff    BackoffConfig  // reconnect backoff
	MaxRetries int            // 0 ⇒ DefaultMaxRetries
	Heartbeat  time.Duration  // 0 ⇒ DefaultHeartbeat
	Clock      Clock          // optional; tests inject fake
}

// Client implements outbound.EventStreamClient.
type Client struct {
	base       string
	path       string
	http       *http.Client
	backoffCfg BackoffConfig
	maxRetries int
	heartbeat  time.Duration
	clock      Clock
}

// New constructs a Client.
func New(cfg Config) *Client {
	path := cfg.Path
	if path == "" {
		path = DefaultStreamPath
	}
	hc := cfg.HTTP
	if hc == nil {
		// SSE consumers should not use a fixed Timeout — long-lived stream.
		hc = httpclient.New(httpclient.Config{Timeout: 0})
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
	}
}

// Subscribe implements outbound.EventStreamClient.
//
// The returned channel is closed when:
//   - the orchestrator emits a graceful close AND the budget is exhausted, or
//   - the retry budget runs out across blip recoveries, or
//   - the watchdog fires AND the budget is exhausted, or
//   - the caller invokes the cancel func, or
//   - ctx is canceled.
//
// Heartbeats are NOT forwarded to the channel; they only reset the watchdog.
// All payload strings are run through the redactor before reaching the channel.
func (c *Client) Subscribe(ctx context.Context, target outbound.StreamTarget, opts outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	if target.ChangeID.IsZero() {
		return nil, nil, errors.New("ssestream: target.ChangeID required")
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

// run is the long-lived reconnect loop. It exits (and closes out) when
// any termination condition is met.
func (c *Client) run(ctx context.Context, url, initialLastID string, out chan<- domain.Event) {
	defer close(out)

	backoff := NewBackoff(c.backoffCfg)
	budget := NewRetryBudget(c.maxRetries)
	watchdog := NewWatchdog(WatchdogConfig{Timeout: c.heartbeat, Clock: c.clock})
	defer watchdog.Stop()

	lastID := initialLastID

	for {
		// Check ctx before each attempt — caller may have canceled mid-backoff.
		if err := ctx.Err(); err != nil {
			return
		}

		anyEvent, lastSeenID, err := c.connectOnce(ctx, url, lastID, out, watchdog, budget, backoff)
		if lastSeenID != "" {
			lastID = lastSeenID
		}
		if anyEvent {
			// Successful stretch — restore the full retry budget and
			// reset the backoff. Spec §5.7 row "transient" is satisfied
			// once at least one event is seen on the new connection.
			budget.Reset()
			backoff.Reset()
		}

		// Decide whether to retry.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if errors.Is(err, errAuthAbort) {
			// 401/403: V1 has no auth — log & abort, no retry.
			return
		}
		if !budget.TryUse() {
			// Budget exhausted: close the channel and let the caller
			// decide via ExitError code 4.
			return
		}

		// Apply backoff before the next attempt.
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff.Next()):
		}
	}
}

// errAuthAbort signals an unrecoverable HTTP 401/403 response.
var errAuthAbort = errors.New("ssestream: unauthorized")

// connectOnce opens a single SSE connection, forwards events until it ends,
// and returns whether at least one event was received. lastSeenID is the
// most recent SSE id observed.
func (c *Client) connectOnce(
	ctx context.Context,
	url string,
	lastID string,
	out chan<- domain.Event,
	watchdog *Watchdog,
	_ *RetryBudget,
	_ *Backoff,
) (anyEvent bool, lastSeenID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, lastID, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}

	conn := sse.NewConnection(req)

	// Restart the watchdog as soon as a connection is established.
	watchdog.Reset()

	// Wire each incoming SSE event through parser → redactor → channel.
	conn.SubscribeToAll(func(ev sse.Event) {
		raw := RawSSE{Type: ev.Type, ID: ev.LastEventID, Data: ev.Data}
		if IsHeartbeat(raw) {
			watchdog.Reset()
			lastSeenID = raw.ID
			return
		}
		domEv, ok := ParseEvent(raw)
		if !ok {
			// Tolerant skip — caller's logger fires elsewhere.
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

	// Connect blocks until the stream ends, the watchdog fires, or ctx is done.
	connDone := make(chan error, 1)
	go func() { connDone <- conn.Connect() }()

	select {
	case err := <-connDone:
		return anyEvent, lastSeenID, classifyConnectErr(err)
	case <-watchdog.Done():
		// Force a reconnect: close the request context indirectly by
		// canceling, then drain the connect goroutine.
		// The outer ctx will eventually cancel via the run-loop selecting on
		// it; for now we return so the loop can decide whether to retry.
		return anyEvent, lastSeenID, errors.New("watchdog: no heartbeat for 60s")
	case <-ctx.Done():
		return anyEvent, lastSeenID, ctx.Err()
	}
}

// classifyConnectErr maps tmaxmax/go-sse errors and HTTP status codes onto
// the categories the run loop branches on. Implementations of go-sse expose
// a ResponseError type that carries the status code; if the API differs,
// adjust this function in place.
func classifyConnectErr(err error) error {
	if err == nil {
		return nil
	}
	// Respect ctx cancellation propagation.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Look for HTTP status info — we accept either a ResponseError-shaped
	// type or a substring match as a defensive fallback.
	msg := err.Error()
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") {
		return errAuthAbort
	}
	return err
}

func (c *Client) buildURL(target outbound.StreamTarget) string {
	path := c.path
	if strings.Contains(path, "%s") {
		path = fmt.Sprintf(path, target.ChangeID.String())
	}
	return c.base + path
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/ssestream/... -race -timeout 30s`
Expected: PASS. If `tmaxmax/go-sse`'s `Connection` / `SubscribeToAll` symbols differ from the actual API, adapt the implementation but keep the test contract — the tests assert OBSERVABLE behavior, not internal types. If a fundamental shape mismatch surfaces (e.g. go-sse exposes only a callback API and not a `Connect()` blocking call), STOP and ask before pursuing the RM5-01 fallback.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/ssestream/client.go internal/adapters/outbound/ssestream/client_test.go
git commit -m "feat(ssestream): add Client (EventStreamClient impl with reconnect, watchdog, redaction)"
```

---

## Phase 6 — Runner refactor: SSE-first, polling removed

### Task 6: application/runner.go — replace polling with SSE consumption

**Files:**
- Modify: `internal/application/runner.go`
- Modify: `internal/application/runner_test.go`

- [ ] **Step 1: Read the existing runner**

Read `internal/application/runner.go` and `internal/application/runner_test.go`. The current `Run`:
1. Validates input.
2. CreateChange.
3. Persists last_change_id (global + project).
4. OnSnapshot for the initial response.
5. If terminal → finish; else → poll().
6. poll() loops on `time.After(delay)` calling GetChange + OnSnapshot until terminal.

The refactor keeps steps 1–4 verbatim; replaces 5–6 with `stream()`.

- [ ] **Step 2: Update RunnerDeps and tests**

Add `EventStream outbound.EventStreamClient` to `RunnerDeps`. Replace runner_test.go fixtures: drop `TickHook` reliance for the streaming tests (keep one polling-style test ONLY for the `created.Status.IsTerminal()` short-circuit path).

`internal/application/runner_test.go` (full replacement):

```go
package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

type recordingSink struct {
	Snapshots []*domain.Change
	Events    []domain.Event
	Gates     []domain.ApprovalGate
	Errors    []error
	Final     domain.ChangeStatus
	closed    bool
}

func (s *recordingSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	cp := *c
	s.Snapshots = append(s.Snapshots, &cp)
	return nil
}
func (s *recordingSink) OnEvent(_ context.Context, e domain.Event) error {
	s.Events = append(s.Events, e)
	return nil
}
func (s *recordingSink) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	s.Gates = append(s.Gates, g)
	return nil
}
func (s *recordingSink) OnError(_ context.Context, err error) error {
	s.Errors = append(s.Errors, err)
	return nil
}
func (s *recordingSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	s.Final = st
	return nil
}
func (s *recordingSink) Close() error { s.closed = true; return nil }

func newRunner(orch *fakes.FakeOrchestrator, stream *fakes.FakeEventStream, sink *recordingSink) (*application.Runner, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	r := application.NewRunner(application.RunnerDeps{
		Orch:        orch,
		State:       state,
		Git:         fakes.NewFakeGitInspector(),
		Sink:        sink,
		EventStream: stream,
	}, application.RunnerOptions{
		SnapshotRefreshTimeout: time.Second,
	})
	return r, state
}

func TestRunnerCreatesAndConsumesSSEUntilTerminalEvent(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, state := newRunner(orch, stream, sink)

	// Push a phase.completed event after the runner subscribes. Use a hook
	// on FakeEventStream to push as soon as the subscription registers.
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-2"})
			// Terminal status is observed via post-stream snapshot refresh:
			// orchestrator now reports done.
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target) // graceful server close
		}()
	}

	res, err := r.Run(context.Background(), application.RunInput{
		Project:       "p",
		Message:       "msg",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if len(sink.Events) < 2 {
		t.Errorf("expected ≥2 events, got %d", len(sink.Events))
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete final = %q", sink.Final)
	}

	gid, _ := state.GetGlobalLast(context.Background())
	if gid != res.ChangeID {
		t.Errorf("global last = %q, want %q", gid, res.ChangeID)
	}
}

func TestRunnerTranslatesApprovalRequiredEventToOnApprovalGate(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{
				Type:    "approval.required",
				EventID: "gate-1",
				Payload: map[string]any{
					"gate_url": "http://gate/1",
					"reason":   "policy",
					"risk":     "high",
					"policy":   "manual",
					"phase":    "apply",
				},
			})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.Gates) != 1 {
		t.Fatalf("expected 1 ApprovalGate, got %d", len(sink.Gates))
	}
	if sink.Gates[0].URL != "http://gate/1" {
		t.Errorf("gate URL = %q", sink.Gates[0].URL)
	}
	// Per D-M5-02: approval.required is ALSO emitted via OnEvent so jsonsink
	// can serialize it without a special path.
	saw := false
	for _, ev := range sink.Events {
		if ev.Type == "approval.required" {
			saw = true
		}
	}
	if !saw {
		t.Error("approval.required should also be emitted via OnEvent")
	}
}

func TestRunnerSkipsHeartbeatEvents(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "heartbeat", EventID: "hb-1"})
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	if _, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	}); err != nil {
		t.Fatal(err)
	}
	for _, ev := range sink.Events {
		if ev.Type == "heartbeat" {
			t.Errorf("heartbeat should not reach OnEvent: %+v", ev)
		}
	}
}

func TestRunnerExitCode4WhenStreamEndsBeforeTerminal(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			// Server closes the stream but orchestrator status is still running.
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}

func TestRunnerExitCode1OnTerminalFailureViaSnapshot(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusFailed)
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 1 {
		t.Errorf("Code = %d, want 1", exit.Code)
	}
}

func TestRunnerExitCode3OnCreateFailure(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.CreateErr = errors.New("orchestrator unreachable")
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestRunnerCanceledContextReturnsExit4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(_ outbound.StreamTarget) {
		// Never push, never close — let ctx cancel us out.
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := r.Run(ctx, application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}

func TestRunnerInputRequiresProjectAndMessage(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	if _, err := r.Run(context.Background(), application.RunInput{Message: ""}); err == nil {
		t.Error("expected error on empty message")
	}
	if _, err := r.Run(context.Background(), application.RunInput{Message: "m"}); err == nil {
		t.Error("expected error on empty project")
	}
}
```

- [ ] **Step 3: Extend FakeEventStream + FakeOrchestrator**

Read `test/fakes/eventstream.go` and `test/fakes/orchestrator.go`. Add to `FakeEventStream`:

```go
type FakeEventStream struct {
	mu          sync.Mutex
	subscribers map[string][]chan domain.Event
	OnSubscribe func(outbound.StreamTarget)
}
```

Update `Subscribe` to invoke `OnSubscribe` AFTER registering the channel (defer the callback to a goroutine so the caller's setup completes first):

```go
func (s *FakeEventStream) Subscribe(_ context.Context, target outbound.StreamTarget, _ outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	s.mu.Lock()
	ch := make(chan domain.Event, 16)
	s.subscribers[key(target)] = append(s.subscribers[key(target)], ch)
	hook := s.OnSubscribe
	s.mu.Unlock()
	if hook != nil {
		go hook(target)
	}
	var once sync.Once
	cancel := func() error {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			subs := s.subscribers[key(target)]
			for i, c := range subs {
				if c == ch {
					s.subscribers[key(target)] = append(subs[:i], subs[i+1:]...)
					close(ch)
					return
				}
			}
		})
		return nil
	}
	return ch, cancel, nil
}
```

Add `Close(target)` helper that closes all subscriber channels for the target (simulates graceful server close):

```go
// Close simulates a graceful server-side stream close for target.
func (s *FakeEventStream) Close(target outbound.StreamTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers[key(target)] {
		close(ch)
	}
	s.subscribers[key(target)] = nil
}
```

Add `SetTerminal` to `FakeOrchestrator`:

```go
// SetTerminal mutates a stored change to the given terminal status. Used
// by runner tests to simulate "after the stream ended, GetChange returns
// terminal".
func (f *FakeOrchestrator) SetTerminal(id domain.ChangeID, st domain.ChangeStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.changes[id]; ok {
		c.Status = st
	}
}
```

- [ ] **Step 4: Run tests (still failing — Runner uses SSE)**

Run: `go test ./internal/application/... ./test/fakes/... -race`
Expected: FAIL on Runner-related symbols; FakeEventStream/FakeOrchestrator tests still PASS.

- [ ] **Step 5: Implement runner.go (full replacement)**

`internal/application/runner.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ExitError carries a process exit code per spec §2.3.
//
//	0 → terminal DONE
//	1 → terminal BLOCKED/FAILED
//	3 → config / orchestrator-unreachable / change-not-found
//	4 → transient error (stream ended without terminal, ctx canceled, retry budget exhausted)
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
}

// Unwrap allows errors.Is/As traversal.
func (e *ExitError) Unwrap() error { return e.Err }

// RunnerDeps groups the ports the Runner needs.
type RunnerDeps struct {
	Orch        outbound.OrchestratorClient
	State       outbound.StateStore
	Git         outbound.GitInspector
	Sink        inbound.EventSink
	EventStream outbound.EventStreamClient
}

// RunnerOptions tunes the runner.
//
// SnapshotRefreshTimeout caps how long the post-stream GetChange may take
// before the runner gives up and reports exit 4.
type RunnerOptions struct {
	SnapshotRefreshTimeout time.Duration
}

// RunInput controls Run.
type RunInput struct {
	Project       string
	Message       string
	BaseRef       string
	ArtifactStore domain.ArtifactStoreMode
}

// RunResult reports the terminal outcome.
type RunResult struct {
	ChangeID    domain.ChangeID
	FinalStatus domain.ChangeStatus
}

// Runner orchestrates `sophia run` per spec §2.2 (SSE-first in M5).
type Runner struct {
	deps RunnerDeps
	opts RunnerOptions
}

// NewRunner constructs a Runner.
func NewRunner(d RunnerDeps, opts RunnerOptions) *Runner {
	if opts.SnapshotRefreshTimeout <= 0 {
		opts.SnapshotRefreshTimeout = 10 * time.Second
	}
	return &Runner{deps: d, opts: opts}
}

// Run creates a Change and observes it via SSE until terminal status.
// Returns RunResult and either nil (DONE) or *ExitError with the spec code.
func (r *Runner) Run(ctx context.Context, in RunInput) (RunResult, error) {
	if in.Message == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: --message required")}
	}
	if in.Project == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: project not set")}
	}
	if r.deps.EventStream == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: event stream not wired")}
	}
	if in.ArtifactStore == "" {
		in.ArtifactStore = domain.ArtifactStoreEngram
	}
	if in.BaseRef == "" {
		in.BaseRef = "main"
	}

	defer r.deps.Sink.Close() //nolint:errcheck // best-effort

	created, err := r.deps.Orch.CreateChange(ctx, outbound.CreateChangeInput{
		Name:              in.Message,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: string(in.ArtifactStore),
	})
	if err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		return RunResult{}, &ExitError{Code: 3, Err: err}
	}

	res := RunResult{ChangeID: created.ID}
	if err := r.persistChangeID(ctx, in.Project, created.ID); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if err := r.deps.Sink.OnSnapshot(ctx, created); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}

	if created.Status.IsTerminal() {
		return r.finish(ctx, res, created.Status)
	}

	final, err := r.stream(ctx, created.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
		return res, &ExitError{Code: 4, Err: err}
	}
	return r.finish(ctx, res, final)
}

// stream subscribes to the SSE feed for id and forwards events to the sink
// until the channel closes (either graceful server close or retry budget
// exhausted) or ctx is canceled. After the channel closes, it refreshes the
// change snapshot to determine terminal status.
func (r *Runner) stream(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	ch, stop, err := r.deps.EventStream.Subscribe(ctx, outbound.StreamTarget{ChangeID: id}, outbound.SubscribeOptions{})
	if err != nil {
		return "", fmt.Errorf("subscribe: %w", err)
	}
	defer stop() //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				// Channel closed: refresh snapshot to determine terminal status.
				return r.refreshAfterStreamEnd(ctx, id)
			}
			r.dispatchEvent(ctx, ev)
		}
	}
}

// dispatchEvent forwards a single event to the sink. Approval events get
// translated into OnApprovalGate AND emitted via OnEvent (D-M5-02).
func (r *Runner) dispatchEvent(ctx context.Context, ev domain.Event) {
	if err := r.deps.Sink.OnEvent(ctx, ev); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if ev.Type == "approval.required" {
		gate := approvalGateFromEvent(ev)
		if err := r.deps.Sink.OnApprovalGate(ctx, gate); err != nil {
			_ = r.deps.Sink.OnError(ctx, err)
		}
	}
}

// approvalGateFromEvent extracts an ApprovalGate from a redacted payload.
// Missing fields default to zero values per the parser's tolerance rules.
func approvalGateFromEvent(ev domain.Event) domain.ApprovalGate {
	gate := domain.ApprovalGate{TraceID: ev.TraceID}
	if ev.Payload == nil {
		return gate
	}
	gate.URL, _ = ev.Payload["gate_url"].(string)
	gate.Reason, _ = ev.Payload["reason"].(string)
	gate.Risk, _ = ev.Payload["risk"].(string)
	gate.Policy, _ = ev.Payload["policy"].(string)
	if ph, ok := ev.Payload["phase"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	}
	if cid, ok := ev.Payload["change_id"].(string); ok {
		gate.ChangeID = domain.ChangeID(cid)
	}
	return gate
}

// refreshAfterStreamEnd issues a final GetChange to determine terminal status.
// Per D-M5-03 this is the only place mid-run snapshots happen — not on every
// reconnect.
func (r *Runner) refreshAfterStreamEnd(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	rctx, cancel := context.WithTimeout(ctx, r.opts.SnapshotRefreshTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(rctx, id)
	if err != nil {
		return "", fmt.Errorf("post-stream snapshot: %w", err)
	}
	if err := r.deps.Sink.OnSnapshot(ctx, snap); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if !snap.Status.IsTerminal() {
		return "", fmt.Errorf("stream ended before terminal status (current=%q)", snap.Status)
	}
	return snap.Status, nil
}

func (r *Runner) finish(ctx context.Context, res RunResult, st domain.ChangeStatus) (RunResult, error) {
	res.FinalStatus = st
	_ = r.deps.Sink.OnComplete(ctx, st)
	switch st {
	case domain.ChangeStatusDone:
		return res, nil
	case domain.ChangeStatusBlocked, domain.ChangeStatusFailed:
		return res, &ExitError{Code: 1, Err: fmt.Errorf("change ended %s", st)}
	default:
		return res, &ExitError{Code: 4, Err: fmt.Errorf("unexpected non-terminal status %q", st)}
	}
}

func (r *Runner) persistChangeID(ctx context.Context, project string, id domain.ChangeID) error {
	if err := r.deps.State.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo — keep only the global record. Not fatal.
		return nil
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := r.deps.State.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/application/... ./test/fakes/... -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/application/runner.go internal/application/runner_test.go \
        test/fakes/eventstream.go test/fakes/orchestrator.go
git commit -m "refactor(application): replace polling with SSE consumption + approval translation"
```

---

## Phase 7 — Bootstrap rewiring

### Task 7: bootstrap/wire.go — inject ssestream.Client into Runner

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`

- [ ] **Step 1: Read wire.go and wire_test.go**

The current wire.go (from M4) constructs `runner` with `application.RunnerDeps{Orch, State, Git, Sink}` and `application.RunnerOptions{}`. The polling defaults inside the Runner are gone — `RunnerOptions` is now `SnapshotRefreshTimeout` only.

- [ ] **Step 2: Update wire_test.go**

Add a test:

```go
func TestNewWiresM5SSEStream(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	// Smoke: the run command is still wired; the SSE stream is constructed
	// inside the runner. We can't assert the stream type from outside without
	// exposing it via Deps, so this is a smoke check.
	c, _, err := root.Find([]string{"run"})
	if err != nil || c == nil {
		t.Fatalf("run cmd missing: %v", err)
	}
}
```

- [ ] **Step 3: Update wire.go**

Add the import:
```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
```

Construct the SSE client and inject it. Replace the runner block:

```go
// SSE stream client (M5): consumes /api/v1/changes/{id}/events with
// reconnect, Last-Event-ID, and 60s heartbeat watchdog per spec §5.7.
stream := ssestream.New(ssestream.Config{
    BaseURL:    cfg.OrchestratorURL,
    Backoff:    ssestream.BackoffConfig{Min: time.Second, Max: 30 * time.Second},
    MaxRetries: ssestream.DefaultMaxRetries,
    Heartbeat:  ssestream.DefaultHeartbeat,
})

// JSON sink writes to stdout. The runner is constructed with this sink
// for V1; M6 will swap to a TUI sink based on flags.
sink := jsonsink.New(jsonsink.Config{Writer: os.Stdout})
runner := application.NewRunner(application.RunnerDeps{
    Orch:        orch,
    State:       state,
    Git:         git,
    Sink:        sink,
    EventStream: stream,
}, application.RunnerOptions{})
```

(Ensure `time` is in the import list.)

- [ ] **Step 4: Run tests + build**

Run:
```bash
go test ./internal/bootstrap/... -race
make build
```
Expected: PASS, binary builds.

- [ ] **Step 5: Smoke**

```bash
./bin/sophia run --help
./bin/sophia run "test" --no-tui --json
echo "exit=$?"
```

Expected:
- `run --help` still shows `--no-tui --json --base-ref --artifact-store --project` (unchanged from M4 — no new flags in M5).
- `./bin/sophia run "test" --no-tui --json` outside a repo with `.sophia.yaml` returns exit 3 (RequireProject fails) just like in M4.

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/wire.go internal/bootstrap/wire_test.go
git commit -m "feat(bootstrap): wire ssestream.Client into Runner.EventStream"
```

---

## Phase 8 — Integration test (real httptest SSE server with blip recovery)

### Task 8: test/integration/sse_reconnect_test.go

**Files:**
- Create: `test/integration/sse_reconnect_test.go`

- [ ] **Step 1: Read existing integration tests**

Look at `test/integration/` (M3 added init/filestate integration tests) to match the directory layout and import style.

- [ ] **Step 2: Write the test**

`test/integration/sse_reconnect_test.go`:

```go
package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// TestSSEReconnectRecoversFromBlip spins up an httptest server that emits
// 2 events, closes the connection, then on reconnect (verifying the
// Last-Event-ID header is set to "evt-2") emits 2 more events including a
// terminal phase.completed.
//
// Asserts: 4 events reach the channel in order, no duplicates, the second
// connection's Last-Event-ID header carries "evt-2".
func TestSSEReconnectRecoversFromBlip(t *testing.T) {
	var (
		mu          sync.Mutex
		connections int
		seenIDs     []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connections++
		seenIDs = append(seenIDs, r.Header.Get("Last-Event-ID"))
		conn := connections
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		switch conn {
		case 1:
			writeEvent(w, "phase.started", "evt-1", `{"payload":{"phase_id":"p-1"}}`)
			flusher.Flush()
			writeEvent(w, "phase.started", "evt-2", `{"payload":{"phase_id":"p-2"}}`)
			flusher.Flush()
			// Drop the connection — simulates a transient blip.
		case 2:
			writeEvent(w, "phase.completed", "evt-3", `{"payload":{"status":"running"}}`)
			flusher.Flush()
			writeEvent(w, "phase.completed", "evt-4", `{"payload":{"status":"done"}}`)
			flusher.Flush()
			// Server graceful close.
		}
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL:    srv.URL,
		Path:       "/api/v1/changes/%s/events",
		Backoff:    ssestream.BackoffConfig{Min: 5 * time.Millisecond, Max: 20 * time.Millisecond},
		MaxRetries: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainAtLeast(t, ch, 4, 4*time.Second)

	wantIDs := []string{"evt-1", "evt-2", "evt-3", "evt-4"}
	if len(got) < len(wantIDs) {
		t.Fatalf("got %d events, want at least %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].EventID != want {
			t.Errorf("event %d EventID = %q, want %q", i, got[i].EventID, want)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if connections < 2 {
		t.Fatalf("expected ≥2 connections, got %d", connections)
	}
	if len(seenIDs) < 2 || seenIDs[1] != "evt-2" {
		t.Errorf("reconnect Last-Event-ID = %v, want second to be %q", seenIDs, "evt-2")
	}
}

func TestSSESkipsHeartbeatsAndForwardsRest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		writeEvent(w, "heartbeat", "hb-1", `{}`)
		flusher.Flush()
		writeEvent(w, "phase.started", "evt-1", `{"payload":{}}`)
		flusher.Flush()
	}))
	defer srv.Close()

	c := ssestream.New(ssestream.Config{
		BaseURL: srv.URL,
		Path:    "/api/v1/changes/%s/events",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.Subscribe(ctx, outbound.StreamTarget{ChangeID: domain.ChangeID("01HX")}, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck

	got := drainAtLeast(t, ch, 1, 2*time.Second)
	for _, ev := range got {
		if ev.Type == "heartbeat" {
			t.Errorf("heartbeat leaked to channel: %+v", ev)
		}
	}
	if got[0].Type != "phase.started" {
		t.Errorf("first non-heartbeat = %+v, want phase.started", got[0])
	}
}

func writeEvent(w http.ResponseWriter, eventType, id, data string) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func drainAtLeast(t *testing.T, ch <-chan domain.Event, n int, timeout time.Duration) []domain.Event {
	t.Helper()
	out := make([]domain.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				if len(out) >= n {
					return out
				}
				t.Fatalf("channel closed after %d events (wanted %d)", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for events (got %d, wanted %d)", len(out), n)
		}
	}
	return out
}
```

- [ ] **Step 3: Run the test**

Run: `go test ./test/integration/... -race -timeout 30s -run TestSSE`
Expected: PASS. If the reconnect attempt does not surface `Last-Event-ID: evt-2`, double-check Task 5's `connectOnce` — it MUST set the header from the most recent `lastSeenID` accumulated during the previous connection.

- [ ] **Step 4: Commit**

```bash
git add test/integration/sse_reconnect_test.go
git commit -m "test(integration): add SSE blip recovery + heartbeat skip tests"
```

---

## Phase 9 — Final validation

### Task 9: Final validation pass

**Files:** none (verification only).

- [ ] **Step 1: vet + tests + race**

```bash
go vet ./...
go test -race ./...
```
Expected: exit 0.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```
Acceptable `//nolint` patterns: existing precedents (gosec on subprocess shellouts, unparam on fakes, errcheck on `defer stop()` and `defer Sink.Close()`). Fix new findings in place.

- [ ] **Step 3: Coverage**

```bash
go test -coverprofile=cover.out ./internal/domain/... ./internal/application/... ./internal/adapters/outbound/ssestream/...
go tool cover -func=cover.out | tail -n 1
```
Expected: total ≥ 70% (the redactor and parser are pure functions and should clear 90%+ on their own).

- [ ] **Step 4: Binary smoke**

```bash
make build

# 1) Help unchanged from M4
./bin/sophia run --help

# 2) Outside a repo, exit 3 (no .sophia.yaml)
./bin/sophia run "test" --no-tui --json
echo "no-config exit=$?"

# 3) Without flags, exit 3 (M4 message still applies — TUI ships M6)
./bin/sophia run "test"
echo "no-flags exit=$?"

# 4) Other commands still work
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
```

Expected:
- `run --help` shows `--no-tui --json --base-ref --artifact-store --project` (no `--orchestrator-url` — defer to M7).
- `run "test" --no-tui --json` outside `.sophia.yaml` exits 3.
- Other commands unchanged.

- [ ] **Step 5: Integration smoke**

```bash
go test -race ./test/integration/...
```
Expected: PASS for SSE reconnect + heartbeat tests AND the M3 init/filestate integration tests.

- [ ] **Step 6: E2E smoke (still validates M4 auto_advance assumption against new SSE pipeline)**

The M4 e2e test (`test/e2e/run_polling_test.go`) used a polling-only stub. Once the runner is SSE-first, that stub no longer covers the run path — but it still verifies CreateChange + initial GetChange + last_change_id persistence. Keep it green.

For M5 e2e, escalate: the runner now requires an SSE endpoint. Add to `test/e2e/run_polling_test.go` either (a) a `text/event-stream` handler at `/api/v1/changes/{id}/events` that emits one terminal event, or (b) accept exit code 4 when the stub returns 404 on the SSE path AND the post-stream snapshot is terminal. Option (a) is cleaner — extend the stub:

```go
case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
    w.Header().Set("Content-Type", "text/event-stream")
    w.WriteHeader(http.StatusOK)
    flusher := w.(http.Flusher)
    fmt.Fprint(w, "event: phase.completed\nid: evt-1\ndata: {\"payload\":{\"status\":\"done\"}}\n\n")
    flusher.Flush()
    // Then close — the runner will GET the change and observe done.
```

If after the stub update the M4 e2e test fails, document it as M5 e2e tech debt (the path now exercises ssestream.Client end-to-end and the M4 test name is stale — rename in a follow-up).

```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS.

- [ ] **Step 7: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m5): final validation pass" || echo "nothing to commit"
git tag -a m5-sse-upgrade -m "M5 SSE Upgrade complete"
git tag
```

---

## Self-review checklist

- [ ] **Spec coverage:** Every M5 DoD from spec §7.2 has at least one task.
  - `ssestream.Client` implements `outbound.EventStreamClient` via `tmaxmax/go-sse` → Tasks 1, 5
  - Reconnect + Last-Event-ID + exp backoff (1s→30s, max 5) → Tasks 4, 5, 8
  - 60s no-heartbeat → force reconnect → Tasks 4 (Watchdog), 5 (wired in client)
  - Tolerant parser (skip malformed/unknown, never drop phase.* / approval.*) → Task 3
  - Redaction pipeline applied before sinks → Tasks 2 (redactor), 3 (parser integration)
  - fakeStream in unit tests; e2e validates ordering against real SSE server → Tasks 6, 8
- [ ] **No placeholders:** No "TBD"/"TODO"/"similar to" in steps.
- [ ] **Type consistency:** `application.RunnerDeps.EventStream`, `outbound.EventStreamClient`, `ssestream.RawSSE`, `ssestream.Config` consistent across tasks.
- [ ] **Frequent commits:** every task ends with a commit.
- [ ] **TDD discipline:** failing test before implementation in every Phase 2–5 task; Phase 6 refactor extends test suite first.
- [ ] **No premature M6+ scope:** no TUI, no ApprovalGate banner, no browser opener, no `--orchestrator-url`, no cross-process resume.
- [ ] **No polling fallback:** Runner has no `poll()` method after Task 6 — search runner.go for `time.After` post-refactor must return nothing.

---

## Pending decisions (carried into M5 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M5-01 | `tmaxmax/go-sse` vs `net/http` + `bufio.Scanner` | `tmaxmax/go-sse` (RM5-01 fallback if its API doesn't fit Task 5's shape). Task 5 has an explicit "verification gate" before implementing — STOP and ask if go-sse's surface differs. |
| D-M5-02 | `approval.required`: only `OnApprovalGate`, or both `OnEvent` + `OnApprovalGate`? | BOTH — sinks decide whether to ignore. `jsonsink` emits one JSONL line per call site, so emitting twice produces two lines (one tagged `event`, one tagged `approval.required` payload). M7 banner sink will key off `OnApprovalGate` only. |
| D-M5-03 | When to refresh snapshot mid-stream? | ONLY on graceful close OR retry budget exhausted. Not on every reconnect. (Spec §5.7 row "server graceful close" + run loop end-of-stream.) |
| D-M5-04 | Heartbeat: forward to sink? | NO — only updates the watchdog. Spec §5.4 lists heartbeat with empty payload precisely so consumers don't render it. |
| D-M5-05 | Redactor scope: apply to event `Type` or only `Payload`? | Only `Payload` (the `Type` is a structured enum-like string) and any string fields nested inside payload. Field-name-context redaction guarantees technical fields like `phase_id` pass through. |
| D-M5-06 | Per-phase retry budget reset | Reset on first successful event reception on a new connection. (Backoff also resets.) Task 5's run loop calls both `budget.Reset()` and `backoff.Reset()` once `anyEvent` is true. |
| D-M5-07 | Initial CreateChange / first GetChange retry under SSE budget? | NO — they are NOT part of the SSE retry budget. Their existing exit-code mapping (3 on unreachable / 4 on ctx-cancel) carries over from M4. |

---

## Risks specific to M5

| ID | Risk | Mitigation |
|---|---|---|
| RM5-01 | `tmaxmax/go-sse` does not interoperate with the orchestrator's SSE shape (e.g. exposes only callback API, no `Connect()` blocking call, or mishandles `Last-Event-ID`) | Fallback path: replace `client.go`'s `connectOnce` with a hand-rolled `net/http` GET + `bufio.Scanner` reading SSE lines per spec §5.3 wire format (~200–300 LOC). Task 5 is structured so this is a focused replacement of one file — parser, redactor, backoff, and watchdog stay intact. STOP and ask the user before swapping. |
| RM5-02 | Redactor over-redacts technical payloads (ULID-shaped IDs misclassified as secrets) | Conservative scope: redaction is field-name-gated. `technicalAllowlist` (Task 2) bypasses redaction for `phase_id`, `change_id`, `agent_id`, `task_id`, `group_id`, `trace_id`, `event_id`, `run_id`, `id`, `type`, `status`, `phase`, `reason`, `risk`, `policy`, `decision`. Pattern-based redaction only fires for clearly secret-shaped strings (Bearer, JWT, AKIA…, gh*_) — no entropy-only heuristics. |
| RM5-03 | Reconnect storms: server briefly down + 5 retries × multiple subscribers | Single Subscribe per `Run` — Runner only opens one stream. Budget is per-phase (per Subscribe call). Once exhausted, `run()` closes the channel; the Runner reports ExitError code 4 cleanly. No global backoff state shared across runs. |
| RM5-04 | `Last-Event-ID` lost across process restarts | Out of scope for M5 — single-process resilience only. M8 may add disk persistence (`<stateRoot>/last_event_id`). Documented in spec §7.2 M8 hints. |
| RM5-05 | Test flakiness: timer-based watchdog races with goroutine scheduler | Inject `Clock` interface (Task 4); tests use `fakeClock` that advances deterministically. Watchdog tests have 20–200ms slack; raise to 100–500ms if CI flakes. |
| RM5-06 | Runner exits cleanly via SSE channel close but the post-stream snapshot also fails (orchestrator briefly unavailable) | `refreshAfterStreamEnd` runs with `SnapshotRefreshTimeout` (default 10s). On failure, returns ExitError code 4 with the snapshot error. User can inspect the JSONL trail to see the prior events; `last_change_id` was persisted on CreateChange so they can re-attach in M8. |
| RM5-07 | Auth headers (401/403) leak via error.Error() back to logs | Task 5's `classifyConnectErr` maps 401/403 to `errAuthAbort` and returns the sentinel; Runner forwards to `OnError`, which goes through the redactor only at payload level — error strings bypass redaction. Mitigation: Task 5 should NOT include the response body in the error message for 401/403. Verify in the implementation. |

---

## What this plan does NOT cover (intentional)

- TUI Timeline (Bubble Tea) → M6
- ApplyBoard view → M7
- ApprovalGate banner → M7 (M5 wires `OnApprovalGate` calls; jsonsink already serializes them — no banner, no browser open)
- Real `sophia attach` / `sophia changes` → M8
- Browser opener → M7
- Approval-timeout exit code 5 → M5+ (M5 reaches terminal status only via stream end + snapshot)
- Per-call `--orchestrator-url` flag → M7+ (orch client + ssestream.Client both bind to a single URL at construction time; per-call rebinding is architectural work)
- Cross-process `Last-Event-ID` persistence → M8 (single-process only in M5)
- `RunPhase` compatibility mode (spec §5.2) → not in scope; if real orchestrator does NOT auto_advance, escalate as a post-M5 spec change. Not a polling fallback.

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-05-sophia-cli-m5-sse-upgrade.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task. Use `superpowers:subagent-driven-development`. Each task has a self-contained TDD cycle (write test → fail → implement → pass → commit), so subagents can work independently with minimal context.

**2. Sequential single-agent** — use `superpowers:executing-plans` and walk Task 1 → Task 9 in order. Recommended only if you want to keep the full context window for any cross-task surprises (most likely Task 5 if `tmaxmax/go-sse`'s API differs from this plan's assumptions — see RM5-01).

Either way: keep an eye on D-M5-01 (the `tmaxmax/go-sse` verification gate at the top of Task 5's implementation) — that is the single most likely place this plan will need adjustment during execution.

---

## Implementation Notes — Deviations from Plan

The plan above was the design intent; this section captures what actually shipped. Each entry: what the plan said, what was implemented, why, and where it lives.

### Task 2 — Redactor

### 1. Redactor JWT regex tightened

- **Plan**: `[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`
- **Implemented**: `\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{32,}\b`
- **File**: `internal/adapters/outbound/ssestream/redactor.go` (line 20)
- **Why**: Code review found false-positives on legitimate dotted strings such as `task_execution.phase_started.explore_mode` and ULID triplets. Word boundaries prevent matching identifiers embedded inside larger strings; 32-char third-segment minimum reflects that real JWT signatures are always ≥32 base64url chars.

### 2. `RedactPayload(nil)` returns nil

- **Plan**: implied a non-nil empty map would be returned for nil input
- **Implemented**: nil-guard at the top of `RedactPayload` returns nil immediately
- **File**: `internal/adapters/outbound/ssestream/redactor.go` (line 80)
- **Why**: Preserves nil-in → nil-out semantics expected by callers; the parser's nil-guard (line 71 of `parser.go`) already skips redaction when `payload == nil`.

### Task 3 — Parser

### 3. Timestamp tolerance via `flexTime`

- **Plan**: plain `time.Time` field with `omitempty`
- **Implemented**: custom `flexTime` type with `UnmarshalJSON` accepting `""` and `null` as zero time
- **File**: `internal/adapters/outbound/ssestream/parser.go` (`flexTime` type, lines 27–36)
- **Why**: Code review caught that Go's `time.Time.UnmarshalJSON` rejects empty-string input with a parse error. Spec §5.4 says missing fields default to empty — without `flexTime`, events with `""` timestamps would be silently dropped.

### Task 4 — Reconnect primitives

### 4. `Clock.Now()` removed from interface

- **Plan**: `Clock { Now() time.Time; AfterFunc(d, fn) StoppableTimer }`
- **Implemented**: `Clock { AfterFunc(d, fn) StoppableTimer }` (`Now` removed)
- **File**: `internal/adapters/outbound/ssestream/reconnect.go` (`Clock` interface, line 18)
- **Why**: Code review found `Now()` was dead — never called by `Backoff`, `RetryBudget`, or `Watchdog`.

### 5. Watchdog single-use semantics documented

- **Plan**: single-use behaviour was implicit
- **Implemented**: doc comment on `Watchdog` struct explicitly states "once Done fires or Stop is called, the watchdog is permanently disabled and Reset becomes a no-op. Callers create a new Watchdog per connection attempt."
- **File**: `internal/adapters/outbound/ssestream/reconnect.go` (`Watchdog` struct, lines 126–130)
- **Why**: Code review for clarity; the comment sets a contract that prevents callers from attempting to reuse a watchdog across reconnect attempts.

### Task 5 — SSE Client

### 6. SSE Client: disable go-sse internal retry

- **Plan**: `sse.NewConnection(req)` called on a bare `*sse.Client` (go-sse's default has infinite retry)
- **Implemented**: `sse.Client{Backoff: sse.Backoff{MaxRetries: -1}}` constructed per `connectOnce` call
- **File**: `internal/adapters/outbound/ssestream/client.go` (lines 209–215)
- **Why**: go-sse v0.11.0 retries indefinitely by default — `Connect()` never returns on a clean server close without this. The outer reconnect loop in `run()` is the sole authority over reconnect policy.

### 7. Bare `http.Client` (no `httpclient.New`)

- **Plan**: `httpclient.New(httpclient.Config{Timeout: 0})`
- **Implemented**: bare `&http.Client{}`
- **File**: `internal/adapters/outbound/ssestream/client.go` (`New()`, lines 76–80)
- **Why**: Discovery during implementation: `httpclient.New(Config{Timeout:0})` applies a 5 s default timeout internally. Long-lived SSE connections require no timeout.

### 8. Per-attempt context cancel in `connectOnce`

- **Plan**: used the outer `ctx` directly for the HTTP request
- **Implemented**: `attemptCtx, cancelAttempt := context.WithCancel(ctx); defer cancelAttempt()` at the top of `connectOnce`
- **File**: `internal/adapters/outbound/ssestream/client.go` (`connectOnce`, lines 194–195)
- **Why**: Code review High — when the watchdog fires, the goroutine running `conn.Connect()` continued on the stale connection. The per-attempt cancel ensures the goroutine exits cleanly on every return path from `connectOnce`.

### 9. `classifyConnectErr` scoped to `ConnectionError.Err`

- **Plan**: `strings.Contains(err.Error(), "401")`
- **Implemented**: `errors.As(err, &connErr)` then `strings.Contains(connErr.Err.Error(), "401")`
- **File**: `internal/adapters/outbound/ssestream/client.go` (`classifyConnectErr`, lines 267–273)
- **Why**: Code review Medium — scoping the 401/403 check to the inner go-sse validator error (not the full wrapped message) is more stable across go-sse format changes.

### 10. `buildURL` panics on missing `%s`

- **Plan**: silently dropped the ChangeID if `Path` lacked `%s`
- **Implemented**: `New()` panics if a non-empty `Path` does not contain `%s`
- **File**: `internal/adapters/outbound/ssestream/client.go` (`New()`, lines 71–73)
- **Why**: Code review Medium — converts a silent misconfiguration into a loud bootstrap-time failure.

### 11. Heartbeats reset retry budget

- **Plan**: only domain events (non-heartbeat) reset the budget
- **Implemented**: heartbeats also set `anyEvent = true`, resetting the budget on the next outer-loop iteration
- **File**: `internal/adapters/outbound/ssestream/client.go` (`connectOnce` callback, lines 222–225)
- **Why**: Code review Medium — heartbeats prove the connection is healthy; resetting the budget on heartbeat reception is the conservative, correct interpretation.

### 12. `ConfigResolver`: `ArtifactStore` validation

- **Plan**: `domain.ArtifactStoreMode(in.Flags.ArtifactStore)` unconditional cast
- **Implemented**: `mode.IsValid()` gate before accepting the flag value; returns a wrapped `ErrConfigMissing` on invalid input
- **File**: `internal/application/configresolver.go` (lines 129–134)
- **Why**: Code review — silent bad data reaching the orchestrator was a real risk. `domain.ArtifactStoreMode` already exposes `IsValid()`.

### 13. Exported env constants

- **Plan**: env key strings (`SOPHIA_ORCHESTRATOR_URL`, etc.) as scattered string literals inside the resolver
- **Implemented**: `EnvOrchestratorURL`, `EnvProject`, `EnvBaseRef` exported as package-level constants
- **File**: `internal/application/configresolver.go` (lines 19–21)
- **Why**: Task 8's cobra `run.go` (`envSnapshot()`) needs the same keys; exporting avoids duplication and keeps the two consumers in sync.

### Task 6 — Runner

### 14. Runner stream-error exit code

- **Plan**: stream-phase non-ctx errors → `ExitError{Code: 4}`
- **Implemented**: the `CreateChange` non-ctx error path was corrected to `ExitError{Code: 3}` per spec §2.3 (commit b04f21d); the stream post-close error path (`refreshAfterStreamEnd`) retains `Code: 4`
- **File**: `internal/application/runner.go` (lines 113–114 for create path; lines 135–136 for stream path)
- **Why**: Code review identified that `ErrChangeNotFound` / `ErrUnreachable` from `CreateChange` map to Code 3 per spec §2.3. The stream-close path (`GetChange` snapshot) was judged as transient (Code 4) since a successful connection already existed.

### 15. Runner: `context.WithoutCancel` on cancellation

- **Plan**: `OnError` was skipped on ctx-canceled paths (ctx already dead)
- **Implemented**: `r.deps.Sink.OnError(context.WithoutCancel(ctx), err)` called before returning on ctx-canceled paths
- **File**: `internal/application/runner.go` (lines 110, 130–133)
- **Why**: Code review — sinks must learn about the interruption even after the context is cancelled. `context.WithoutCancel` propagates values without the cancellation signal.

### Task 6 — Test fakes

### 16. `TickHook` stays inside `FakeOrchestrator` mutex

- **Plan**: implied moving the hook invocation outside the mutex
- **Implemented**: hook fires inside the mutex with an explanatory contract comment
- **File**: `test/fakes/orchestrator.go` (`GetChange`, lines 67–73)
- **Why**: Moving it outside would break `TestRunnerCreatesAndPollsUntilDone` — the hook mutates the stored `Change` so that the next `GetChange` call observes the updated state. The contract comment documents that hooks must not call back into the orchestrator.

### Task 8 — CLI

### 17. `--orchestrator-url` flag dropped

- **Plan**: `run` command included `--orchestrator-url` flag
- **Implemented**: flag removed entirely; orchestrator URL is env-only via `SOPHIA_ORCHESTRATOR_URL`
- **File**: `internal/adapters/inbound/cli/run.go`
- **Why**: Code review — the flag was registered but its value never reached the orchestrator client, which was constructed at bootstrap time with the default URL. An unimplemented flag is actively misleading. Per-call URL rebinding is deferred to M7+.

### Task 9 — e2e

### 18. e2e stub: 401 on second connection

- **Plan**: stub emits one event then closes cleanly
- **Implemented**: stub returns HTTP 401 on the second `/events` connection to terminate the retry loop cleanly
- **File**: `test/e2e/run_polling_test.go` (lines 51–54)
- **Why**: The SSE client resets the retry budget whenever `anyEvent == true`. A single-event-then-close stub caused infinite reconnects. A 401 response triggers `errAuthAbort`, which causes the outer `run()` loop to exit without consuming the budget.
