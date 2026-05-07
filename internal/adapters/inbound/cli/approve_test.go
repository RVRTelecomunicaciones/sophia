package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newDecisionDeps(orch *fakes.FakeOrchestrator) cli.Deps {
	return cli.Deps{
		Approver: application.NewApprover(application.ApproverDeps{Orch: orch}),
		Aborter:  application.NewAborter(application.AborterDeps{Orch: orch}),
	}
}

func TestApproveCommand_HappyPath(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.ApprovalDecisionInput
	orch.OnApprove = func(_ string, in outbound.ApprovalDecisionInput) { captured = in }

	c := cli.NewRoot(newDecisionDeps(orch))
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"approve", "01PHASE", "--approver", "alice", "-r", "lgtm"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if captured.Approver != "alice" || captured.Reason != "lgtm" {
		t.Errorf("captured = %+v", captured)
	}
	if !strings.Contains(out.String(), "approved") {
		t.Errorf("output missing 'approved': %q", out.String())
	}
}

// TestApproveCommand_GateAlreadyDecidedIsIdempotent — Phase 4 scope item
// 8: gate_already_decided is informational, exit 0.
func TestApproveCommand_GateAlreadyDecidedIsIdempotent(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.ApproveErr = domain.ErrGateAlreadyDecided

	c := cli.NewRoot(newDecisionDeps(orch))
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"approve", "01PHASE", "--approver", "alice"})
	err := c.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("gate_already_decided should exit 0, got %v", err)
	}
	if !strings.Contains(out.String(), "already decided") {
		t.Errorf("output missing idempotency message: %q", out.String())
	}
}

func TestApproveCommand_RequiresApprover(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	c := cli.NewRoot(newDecisionDeps(orch))
	var errOut bytes.Buffer
	c.SetErr(&errOut)
	c.SetOut(&errOut)
	// Inject a USER-less env via t.Setenv so $USER fallback yields empty.
	t.Setenv("USER", "")
	c.SetArgs([]string{"approve", "01PHASE"})
	err := c.ExecuteContext(context.Background())
	if err == nil {
		t.Error("expected error when approver is empty")
	}
}

func TestRejectCommand_HappyPath(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.ApprovalDecisionInput
	orch.OnReject = func(_ string, in outbound.ApprovalDecisionInput) { captured = in }

	c := cli.NewRoot(newDecisionDeps(orch))
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"reject", "01PHASE", "--approver", "alice", "-r", "bad"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if captured.Approver != "alice" || captured.Reason != "bad" {
		t.Errorf("captured = %+v", captured)
	}
	if !strings.Contains(out.String(), "rejected") {
		t.Errorf("output missing 'rejected': %q", out.String())
	}
}

func TestAbortCommand_HappyPath(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	var captured outbound.AbortChangeInput
	orch.OnAbort = func(_ domain.ChangeID, in outbound.AbortChangeInput) { captured = in }

	c := cli.NewRoot(newDecisionDeps(orch))
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"abort", "01CHANGE", "-r", "user requested"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if captured.Reason != "user requested" {
		t.Errorf("reason = %q", captured.Reason)
	}
	if !strings.Contains(out.String(), "aborting") {
		t.Errorf("output missing 'aborting': %q", out.String())
	}
}

// TestAbortCommand_AlreadyTerminalIsIdempotent — Phase 4 scope item 8:
// change_already_terminal exits 0 with informational text.
func TestAbortCommand_AlreadyTerminalIsIdempotent(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.AbortErr = domain.ErrChangeAlreadyTerminal

	c := cli.NewRoot(newDecisionDeps(orch))
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"abort", "01CHANGE"})
	err := c.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("change_already_terminal should exit 0, got %v", err)
	}
	if !strings.Contains(out.String(), "already terminal") {
		t.Errorf("output missing idempotency message: %q", out.String())
	}
}
