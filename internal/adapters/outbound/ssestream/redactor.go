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
	// JWT triplets: 3 base64url segments joined by dots. Third segment is
	// the signature, which is base64url HMAC/RSA output and is always
	// ≥32 chars. Word boundaries prevent matching when embedded inside
	// larger identifiers (e.g. compound trace IDs in a log message).
	regexp.MustCompile(`\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{32,}\b`),
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
	if payload == nil {
		return nil
	}
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
