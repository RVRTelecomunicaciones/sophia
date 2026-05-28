package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Smoke tests for the contract package. The point is NOT to exercise
// business logic (this package has none) but to lock the wire shape
// against silent JSON tag drift.

func TestVersionConstant(t *testing.T) {
	if Version != "v1" {
		t.Fatalf("Version = %q, want v1", Version)
	}
}

func TestHeaderConstants(t *testing.T) {
	if HeaderAPIKey != "X-Sophia-API-Key" {
		t.Errorf("HeaderAPIKey = %q, want X-Sophia-API-Key", HeaderAPIKey)
	}
	if HeaderAPIKeyLegacy != "X-API-Key" {
		t.Errorf("HeaderAPIKeyLegacy = %q, want X-API-Key", HeaderAPIKeyLegacy)
	}
}

func TestRequiredAuthHeaders(t *testing.T) {
	h := RequiredAuthHeaders("k")
	if got := h.Get(HeaderAPIKey); got != "k" {
		t.Errorf("X-Sophia-API-Key = %q, want k", got)
	}
	empty := RequiredAuthHeaders("")
	if got := empty.Get(HeaderAPIKey); got != "" {
		t.Errorf("empty key set header = %q, want empty", got)
	}
}

func TestRouteConstants(t *testing.T) {
	cases := map[string]string{
		"RouteHealth":         RouteHealth,
		"RouteReady":          RouteReady,
		"RouteChanges":        RouteChanges,
		"RouteChangeByID":     RouteChangeByID,
		"RouteChangeAbort":    RouteChangeAbort,
		"RouteChangePhaseRun": RouteChangePhaseRun,
		"RoutePhaseByID":      RoutePhaseByID,
		"RoutePhaseResume":    RoutePhaseResume,
		"RoutePhaseApprove":   RoutePhaseApprove,
		"RoutePhaseReject":    RoutePhaseReject,
		"RoutePhaseBoard":     RoutePhaseBoard,
		"RoutePhaseEvents":    RoutePhaseEvents,
	}
	for name, val := range cases {
		if !strings.HasPrefix(val, "/api/v1/") {
			t.Errorf("%s = %q, expected /api/v1/ prefix", name, val)
		}
	}
}

func TestPathHelpers(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{ChangePath("01HX"), "/api/v1/changes/01HX"},
		{ChangeAbortPath("01HX"), "/api/v1/changes/01HX/abort"},
		{ChangePhaseRunPath("01HX", "implement"), "/api/v1/changes/01HX/phases/implement/run"},
		{PhasePath("01HY"), "/api/v1/phases/01HY"},
		{PhaseResumePath("01HY"), "/api/v1/phases/01HY/resume"},
		{PhaseApprovePath("01HY"), "/api/v1/phases/01HY/approve"},
		{PhaseRejectPath("01HY"), "/api/v1/phases/01HY/reject"},
		{PhaseBoardPath("01HY"), "/api/v1/phases/01HY/board"},
		{PhaseEventsPath("01HY"), "/api/v1/phases/01HY/events"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("path = %q, want %q", c.got, c.want)
		}
	}
}

// TestEventConstants_Required covers scenario B3: IsRequiredEvent remains a
// switch over the 6 required events (unchanged).
func TestEventConstants_Required(t *testing.T) {
	required := []string{
		EventHeartbeat,
		EventPhaseStarted,
		EventPhaseCompleted,
		EventPhaseFailed,
		EventApprovalRequired,
		EventApprovalResolved,
	}
	for _, e := range required {
		if !IsRequiredEvent(e) {
			t.Errorf("IsRequiredEvent(%q) = false, want true", e)
		}
		if !IsKnownEvent(e) {
			t.Errorf("IsKnownEvent(%q) = false, want true", e)
		}
	}
}

// TestEventConstants_Optional covers backward compat for the existing
// optional-known events that are still in knownEvents post-refactor.
// EventTaskCreated/Started/Completed/Failed and EventAgentCompleted are
// intentionally removed from this list (they are now aspirational/aliased).
func TestEventConstants_Optional(t *testing.T) {
	optional := []string{
		EventAgentDispatched,
		EventOpen,
		EventPhaseCompletedWithConcerns, EventPhaseNeedsContext, EventAgentEnvelopeReceived,
		EventApplyBoardCreated, EventApplyGroupCompleted, EventApplyGroupFailed,
		EventApplyBoardSaveFailed, EventApplyWorktreeError,
	}
	for _, e := range optional {
		if IsRequiredEvent(e) {
			t.Errorf("IsRequiredEvent(%q) = true, want false (optional)", e)
		}
		if !IsKnownEvent(e) {
			t.Errorf("IsKnownEvent(%q) = false, want true (documented)", e)
		}
	}
}

func TestEventConstants_Unknown(t *testing.T) {
	if IsKnownEvent("nonexistent.foo") {
		t.Error("IsKnownEvent returned true for unknown event type")
	}
	if IsRequiredEvent("nonexistent.foo") {
		t.Error("IsRequiredEvent returned true for unknown event type")
	}
}

// TestEventConstants_NewOrchEvents covers scenarios A1-A5: the 5 new
// orchestrator runtime events are recognized by IsKnownEvent.
func TestEventConstants_NewOrchEvents(t *testing.T) {
	cases := []struct {
		name  string
		event string
	}{
		{"A1: apply.group.degraded", EventApplyGroupDegraded},
		{"A2: apply.materialize.started", EventApplyMaterializeStarted},
		{"A3: apply.materialize.completed", EventApplyMaterializeCompleted},
		{"A4: apply.materialize.error", EventApplyMaterializeError},
		{"A5: memory.artifact_persist_failed", EventMemoryArtifactPersistFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !IsKnownEvent(tc.event) {
				t.Errorf("IsKnownEvent(%q) = false, want true", tc.event)
			}
		})
	}
}

// TestEventConstants_BackwardCompat covers scenarios C1-C4: previously-known
// events still return true from IsKnownEvent after the map refactor.
func TestEventConstants_BackwardCompat(t *testing.T) {
	cases := []struct {
		name  string
		event string
	}{
		{"C1: phase.started", "phase.started"},
		{"C2: heartbeat (CLI-only SSE protocol)", "heartbeat"},
		{"C3: open (CLI-only SSE protocol)", "open"},
		{"C4: agent.envelope.received (canonical orch)", "agent.envelope.received"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !IsKnownEvent(tc.event) {
				t.Errorf("IsKnownEvent(%q) = false, want true", tc.event)
			}
		})
	}
}

// TestEventConstants_AspirationalNotRecognized covers scenarios D1-D4 and D7:
// aspirational EventTask* constants and the legacy "agent.completed" literal
// are NOT in knownEvents.
func TestEventConstants_AspirationalNotRecognized(t *testing.T) {
	cases := []struct {
		name  string
		event string
	}{
		{"D1: task.created", "task.created"},
		{"D2: task.started", "task.started"},
		{"D3: task.completed", "task.completed"},
		{"D4: task.failed", "task.failed"},
		{"D7: agent.completed (legacy literal)", "agent.completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsKnownEvent(tc.event) {
				t.Errorf("IsKnownEvent(%q) = true, want false (aspirational/legacy)", tc.event)
			}
		})
	}
}

// TestEventAgentCompleted_IsDeprecatedAlias covers scenario D6: EventAgentCompleted
// is a deprecated alias with value equal to the canonical orch event string.
func TestEventAgentCompleted_IsDeprecatedAlias(t *testing.T) {
	if EventAgentCompleted != "agent.envelope.received" {
		t.Errorf("EventAgentCompleted = %q, want %q", EventAgentCompleted, "agent.envelope.received")
	}
	if EventAgentCompleted != EventAgentEnvelopeReceived {
		t.Errorf("EventAgentCompleted (%q) != EventAgentEnvelopeReceived (%q)",
			EventAgentCompleted, EventAgentEnvelopeReceived)
	}
}

// TestEventConstants_AspirationalConstantsStillDeclared covers scenario D5:
// the aspirational EventTask* constants still compile (constant exists).
func TestEventConstants_AspirationalConstantsStillDeclared(t *testing.T) {
	// Compile-time guarantee: these references confirm the constants are declared.
	_ = EventTaskCreated
	_ = EventTaskStarted
	_ = EventTaskCompleted
	_ = EventTaskFailed
}

func TestErrorCodes_Stable(t *testing.T) {
	codes := []string{
		CodeUnauthorized,
		CodeValidationFailed,
		CodeApproverRequired,
		CodeLimitTooLarge,
		CodeChangeNotFound,
		CodePhaseNotFound,
		CodeChangeAlreadyExists,
		CodeChangeAlreadyTerminal,
		CodePhaseNotResumable,
		CodePhaseNotGated,
		CodeGateAlreadyDecided,
		CodePhaseTerminalNoEvents,
		CodeInternalError,
	}
	if len(codes) != 13 {
		t.Fatalf("expected 13 stable codes per sophia-wire-v1 §9.2, got %d", len(codes))
	}
	for _, c := range codes {
		if !IsKnownCode(c) {
			t.Errorf("IsKnownCode(%q) = false, want true", c)
		}
	}
	if IsKnownCode("foo_bar") {
		t.Error("IsKnownCode returned true for unknown code")
	}
}

func TestChangeResponse_JSONShape(t *testing.T) {
	r := ChangeResponse{
		ChangeID: "01HX",
		Name:     "implement /healthz",
		Project:  "ms-x",
		Status:   ChangeStatusRunning,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"change_id":"01HX"`) {
		t.Errorf("missing change_id: %s", s)
	}
	if !strings.Contains(s, `"status":"running"`) {
		t.Errorf("missing status: %s", s)
	}
	if strings.Contains(s, `"phases"`) {
		t.Errorf("empty Phases must be omitted: %s", s)
	}
}

func TestApprovalRequiredPayload_OptionalFields(t *testing.T) {
	// Phase 1.5 amendment: risk + policy are Optional.
	p := ApprovalRequiredPayload{
		PhaseID: "01HY",
		GateURL: "https://x/g",
		Reason:  "high-risk diff",
	}
	b, _ := json.Marshal(p)
	s := string(b)
	if strings.Contains(s, `"risk"`) {
		t.Errorf("empty Risk should be omitted: %s", s)
	}
	if strings.Contains(s, `"policy"`) {
		t.Errorf("empty Policy should be omitted: %s", s)
	}

	// With risk + policy populated.
	p2 := ApprovalRequiredPayload{
		PhaseID: "01HY", GateURL: "https://x/g", Reason: "r", Risk: "high", Policy: "rule-3",
	}
	b2, _ := json.Marshal(p2)
	if !strings.Contains(string(b2), `"risk":"high"`) || !strings.Contains(string(b2), `"policy":"rule-3"`) {
		t.Errorf("populated risk/policy missing: %s", b2)
	}
}

func TestApprovalResolvedPayload_DecisionValues(t *testing.T) {
	p := ApprovalResolvedPayload{
		PhaseID: "01HY", Decision: DecisionApproved, Approver: "russell",
		DecidedAt: time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC),
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"decision":"approved"`) {
		t.Errorf("decision: %s", b)
	}
	if !strings.Contains(string(b), `"approver":"russell"`) {
		t.Errorf("approver: %s", b)
	}
}

func TestErrorResponse_Envelope(t *testing.T) {
	e := ErrorResponse{
		Code:    CodeValidationFailed,
		Error:   "missing field 'name'",
		Details: map[string]any{"field": "name"},
	}
	b, _ := json.Marshal(e)
	s := string(b)
	if !strings.Contains(s, `"code":"validation_failed"`) {
		t.Errorf("code: %s", s)
	}
	if !strings.Contains(s, `"details":{"field":"name"}`) {
		t.Errorf("details (object shape): %s", s)
	}
}
