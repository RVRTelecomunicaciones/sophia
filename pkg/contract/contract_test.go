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

func TestEventConstants_Optional(t *testing.T) {
	optional := []string{
		EventTaskCreated, EventTaskStarted, EventTaskCompleted, EventTaskFailed,
		EventAgentDispatched, EventAgentCompleted,
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
