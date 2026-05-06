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

// flexTime tolerates empty-string and null JSON timestamps, both of which
// represent "missing" per spec §5.4. Real RFC3339 strings parse normally
// via the embedded time.Time.
type flexTime struct{ time.Time }

func (f *flexTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == `""` || s == "null" {
		f.Time = time.Time{}
		return nil
	}
	return f.Time.UnmarshalJSON(b)
}

// envelope is the JSON shape of the SSE "data:" line per spec §5.3.
type envelope struct {
	Timestamp flexTime       `json:"timestamp"`
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
//   - Missing or empty-string Timestamp → zero time.Time (per §5.4
//     "missing fields default to empty"; we treat "" and null the same).
//   - Missing Payload → nil map (preserved by the redactor's nil-guard).
//   - Unknown Type → emitted unchanged (forward-compatible). Per §5.4
//     tolerance policy; §5.3 rule 4's "skip unknown" is intentionally
//     superseded so the TUI can render future orchestrator event types
//     without a CLI update.
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
		Timestamp: env.Timestamp.Time,
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
