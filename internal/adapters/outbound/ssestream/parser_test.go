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
