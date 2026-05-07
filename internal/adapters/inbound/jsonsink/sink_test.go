package jsonsink_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/jsonsink"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
)

func TestSinkImplementsEventSink(t *testing.T) {
	var _ inbound.EventSink = jsonsink.New(jsonsink.Config{Writer: &bytes.Buffer{}})
}

func TestSnapshotEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	change := &domain.Change{
		ID:             "01HX",
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "phase-1",
		Phases: []domain.Phase{
			{ID: "phase-1", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}
	if err := s.OnSnapshot(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, line)
	}
	if got["type"] != "snapshot" {
		t.Errorf("type = %v", got["type"])
	}
	if got["change_id"] != "01HX" {
		t.Errorf("change_id = %v", got["change_id"])
	}
	if got["status"] != "running" {
		t.Errorf("status = %v", got["status"])
	}
}

func TestEventEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	ev := domain.Event{
		Type:      "phase.started",
		Timestamp: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"phase": "explore"},
	}
	if err := s.OnEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "event" {
		t.Errorf("type = %v", got["type"])
	}
	if got["ev"] != "phase.started" {
		t.Errorf("ev = %v", got["ev"])
	}
}

func TestApprovalGateEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	gate := domain.ApprovalGate{
		URL: "https://gov/approvals/x", Reason: "needs review", Risk: "high", Policy: "require_approval",
		ChangeID: "01HX", Phase: domain.PhaseApply,
	}
	if err := s.OnApprovalGate(context.Background(), gate); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "event" {
		t.Errorf("type = %v", got["type"])
	}
	if got["ev"] != "approval.required" {
		t.Errorf("ev = %v", got["ev"])
	}
	payload, _ := got["payload"].(map[string]any)
	if payload["gate_url"] != "https://gov/approvals/x" {
		t.Errorf("gate_url = %v", payload["gate_url"])
	}
}

func TestErrorEmitsAndDoesNotReturn(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	if err := s.OnError(context.Background(), errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"type":"error"`) {
		t.Errorf("missing error line: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"message":"boom"`) {
		t.Errorf("missing message: %s", buf.String())
	}
}

func TestCompleteEmitsTerminalStatus(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	if err := s.OnComplete(context.Background(), domain.ChangeStatusDone); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"final_status":"done"`) {
		t.Errorf("missing final_status: %s", buf.String())
	}
}

func TestSinkIsConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				_ = s.OnEvent(context.Background(), domain.Event{Type: "x"})
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	if got := strings.Count(buf.String(), "\n"); got != 200 {
		t.Errorf("line count = %d, want 200", got)
	}
}

// failingWriter always errors on Write — used to verify OnError suppresses
// write failures.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestOnErrorSuppressesWriteFailure(t *testing.T) {
	s := jsonsink.New(jsonsink.Config{Writer: failingWriter{}})
	if err := s.OnError(context.Background(), errors.New("boom")); err != nil {
		t.Errorf("OnError must not bubble write errors, got: %v", err)
	}
}

func TestSinkCloseIsNoOp(t *testing.T) {
	s := jsonsink.New(jsonsink.Config{Writer: &bytes.Buffer{}})
	if err := s.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
