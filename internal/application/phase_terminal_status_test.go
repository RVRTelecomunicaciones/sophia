package application

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// TestPhaseTerminalStatusFromEvent covers the SSE-event → envelope_status
// extractor used by streamWithSink to remember the phase outcome that
// rode in on the wire. The fixture mirrors the orch's actual event
// payloads (sophia-wire-v1 §5.3): a `phase.completed` / `.failed` /
// `.needs_context` event carries an `envelope_status` string with the
// underlying envelope.Status value.
func TestPhaseTerminalStatusFromEvent(t *testing.T) {
	cases := map[string]struct {
		ev   domain.Event
		want string
	}{
		"phase.completed DONE": {
			ev:   domain.Event{Type: "phase.completed", Payload: map[string]any{"envelope_status": "DONE"}},
			want: "DONE",
		},
		"phase.completed_with_concerns": {
			ev:   domain.Event{Type: "phase.completed_with_concerns", Payload: map[string]any{"envelope_status": "DONE_WITH_CONCERNS"}},
			want: "DONE_WITH_CONCERNS",
		},
		"phase.failed FAILED": {
			ev:   domain.Event{Type: "phase.failed", Payload: map[string]any{"envelope_status": "FAILED"}},
			want: "FAILED",
		},
		"phase.needs_context": {
			ev:   domain.Event{Type: "phase.needs_context", Payload: map[string]any{"envelope_status": "NEEDS_CONTEXT"}},
			want: "NEEDS_CONTEXT",
		},
		"non-terminal event ignored": {
			ev:   domain.Event{Type: "agent.dispatched", Payload: map[string]any{"envelope_status": "RUNNING"}},
			want: "",
		},
		"terminal event with nil payload returns empty": {
			ev:   domain.Event{Type: "phase.completed", Payload: nil},
			want: "",
		},
		"terminal event with missing envelope_status returns empty": {
			ev:   domain.Event{Type: "phase.completed", Payload: map[string]any{}},
			want: "",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := phaseTerminalStatusFromEvent(tc.ev)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMapPhaseTerminalToChange covers the envelope.Status → ChangeStatus
// mapping used by streamWithSink as a fallback when the change stays
// "active" but the phase clearly terminated. The mapping decides the
// exit code via finishWithSink (DONE → 0, BLOCKED → 1, FAILED → 1).
func TestMapPhaseTerminalToChange(t *testing.T) {
	cases := map[string]domain.ChangeStatus{
		"DONE":                 domain.ChangeStatusDone,
		"DONE_WITH_CONCERNS":   domain.ChangeStatusDone,
		"DONE_WITH_REJECTIONS": domain.ChangeStatusDone,
		"BLOCKED":              domain.ChangeStatusBlocked,
		"NEEDS_CONTEXT":        domain.ChangeStatusBlocked,
		"FAILED":               domain.ChangeStatusFailed,
		"TIMED_OUT":            domain.ChangeStatusFailed,
		"":                     domain.ChangeStatus(""),
		"UNKNOWN_FUTURE_VALUE": domain.ChangeStatus(""),
	}
	for input, want := range cases {
		t.Run("envelope_status="+input, func(t *testing.T) {
			got := mapPhaseTerminalToChange(input)
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestIsPhaseInFlight locks in which orch phase statuses the runner
// treats as "still running, re-subscribe rather than bail" vs which
// it treats as "really terminal, decide outcome now". The 3 in-flight
// (pending/running/interrupted) come from the orch's PhaseStatus enum;
// keeping the table explicit prevents regressions if anyone narrows
// or widens the re-subscribe trigger.
func TestIsPhaseInFlight(t *testing.T) {
	inFlight := []string{"pending", "running", "interrupted"}
	for _, s := range inFlight {
		if !isPhaseInFlight(s) {
			t.Errorf("status %q must be in-flight (re-subscribe target)", s)
		}
	}

	terminal := []string{
		"done", "done_with_concerns", "done_with_rejections",
		"blocked", "needs_context", "failed", "aborted", "timed_out",
		"", "unknown-future-value",
	}
	for _, s := range terminal {
		if isPhaseInFlight(s) {
			t.Errorf("status %q must NOT be in-flight (terminal or unrecognized)", s)
		}
	}
}
