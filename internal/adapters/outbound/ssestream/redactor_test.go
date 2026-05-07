package ssestream_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
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
		"phase_id":      "01HXABC",
		"task_id":       "t-1",
		"token":         "supersecretvalue1234567890ABCDEF",
		"authorization": "Bearer xyz",
		"message":       "hello",
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

// Regression: legitimate dotted strings (task descriptions, ULID-style IDs
// joined by dots) must NOT be eaten by the JWT pattern.
func TestRedactStringDoesNotMatchLegitimateDottedStrings(t *testing.T) {
	cases := []string{
		"task_execution.phase_started.explore_mode",
		"01HXABCDEFGHJKMNPQ.01HXABCDEFGHJKMNPQ.01HXABCDEFGHJKMNPQ",
		"compound.trace.identifier_short",
		"phase_one.phase_two.phase_three",
	}
	for _, in := range cases {
		got := ssestream.RedactString(in)
		if got != in {
			t.Errorf("legitimate string was redacted: %q -> %q", in, got)
		}
	}
}

// Regression: a real JWT (third segment ≥ 32 chars) IS still redacted.
func TestRedactStringStillRedactsRealJWTs(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	in := "Authorization=" + jwt + " "
	got := ssestream.RedactString(in)
	if strings.Contains(got, jwt) {
		t.Errorf("JWT leaked: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected [REDACTED]: %q", got)
	}
}

// Regression: nil payload returns nil (preserves caller's nil semantics).
func TestRedactPayloadNilReturnsNil(t *testing.T) {
	if got := ssestream.RedactPayload(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// Regression: scalar non-string values pass through verbatim.
func TestRedactPayloadPassesThroughScalars(t *testing.T) {
	payload := map[string]any{
		"count":      42,
		"confidence": 0.87,
		"active":     true,
	}
	got := ssestream.RedactPayload(payload)
	if got["count"] != 42 {
		t.Errorf("count = %v, want 42", got["count"])
	}
	if got["confidence"] != 0.87 {
		t.Errorf("confidence = %v, want 0.87", got["confidence"])
	}
	if got["active"] != true {
		t.Errorf("active = %v, want true", got["active"])
	}
}
