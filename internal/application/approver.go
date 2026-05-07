package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// ApproverDeps groups the dependencies of the approve/reject use cases.
type ApproverDeps struct {
	Orch outbound.OrchestratorClient
}

// Approver implements `sophia approve` and `sophia reject` per
// sophia-wire-v1 §4.3 + Phase 4 Task 4.4/4.5. The CLI verb is
// complementary to the M7 browser `[O]` keybinding (D-M10-03);
// either channel resolves the gate.
type Approver struct {
	deps ApproverDeps
}

// NewApprover constructs an Approver.
func NewApprover(d ApproverDeps) *Approver { return &Approver{deps: d} }

// ApprovalInput controls Approve / Reject.
type ApprovalInput struct {
	PhaseID  string
	Approver string
	Reason   string
}

// Approve resolves the gate as approved. ErrGateAlreadyDecided is
// returned as-is so the CLI can map it to an idempotent visible state
// per the user's directive (Phase 4 scope item 8).
func (a *Approver) Approve(ctx context.Context, in ApprovalInput) error {
	if err := validateDecisionInput(in); err != nil {
		return err
	}
	return a.deps.Orch.ApprovePhase(ctx, in.PhaseID, outbound.ApprovalDecisionInput{
		Approver: in.Approver, Reason: in.Reason,
	})
}

// Reject resolves the gate as rejected. Same idempotency contract as
// Approve — the CLI handles ErrGateAlreadyDecided as informational.
func (a *Approver) Reject(ctx context.Context, in ApprovalInput) error {
	if err := validateDecisionInput(in); err != nil {
		return err
	}
	return a.deps.Orch.RejectPhase(ctx, in.PhaseID, outbound.ApprovalDecisionInput{
		Approver: in.Approver, Reason: in.Reason,
	})
}

func validateDecisionInput(in ApprovalInput) error {
	if in.PhaseID == "" {
		return errors.New("phase_id required")
	}
	if in.Approver == "" {
		return fmt.Errorf("%w: --approver or $USER must be set", domain.ErrApproverRequired)
	}
	return nil
}
