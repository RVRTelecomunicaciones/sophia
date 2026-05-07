package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// newApproveCmd registers `sophia approve <phase-id>` (Phase 4 Task
// 4.4 / D-M10-13 Form A). Phase IDs are globally unique on the
// orchestrator so the change-id is not required in the URL.
//
// Idempotency (Phase 4 scope item 8): gate_already_decided is reported
// as an informational message and the CLI exits 0, mirroring the M7
// browser `[O]` flow that doesn't surface "I already clicked this".
func newApproveCmd(d Deps) *cobra.Command {
	var (
		approver string
		reason   string
	)
	cmd := &cobra.Command{
		Use:   "approve <phase-id>",
		Short: "Approve a phase that is awaiting an approval gate",
		Long: `Approve resolves an approval gate raised by the orchestrator
(approval.required SSE event). It is complementary to the M7
[O]pen-browser flow — either channel resolves the gate.

PHASE_ID is the globally-unique phase identifier surfaced in the SSE
event payload and printed by 'sophia status'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Approver == nil {
				return fmt.Errorf("approve: approver not wired")
			}
			actor := resolveApprover(approver)
			err := d.Approver.Approve(cmd.Context(), application.ApprovalInput{
				PhaseID:  args[0],
				Approver: actor,
				Reason:   reason,
			})
			return handleDecisionResult(cmd, "approved", args[0], err)
		},
	}
	cmd.Flags().StringVar(&approver, "approver", "", "approver username (defaults to $USER)")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "optional human-readable reason")
	return cmd
}

// newRejectCmd registers `sophia reject <phase-id>` (Phase 4 Task 4.5).
// Symmetric to approve.
func newRejectCmd(d Deps) *cobra.Command {
	var (
		approver string
		reason   string
	)
	cmd := &cobra.Command{
		Use:   "reject <phase-id>",
		Short: "Reject a phase that is awaiting an approval gate",
		Long: `Reject resolves an approval gate as denied. Like 'approve' it
is complementary to the M7 [O]pen-browser flow — either channel
resolves the gate.

PHASE_ID is the globally-unique phase identifier surfaced in the SSE
event payload.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Approver == nil {
				return fmt.Errorf("reject: approver not wired")
			}
			actor := resolveApprover(approver)
			err := d.Approver.Reject(cmd.Context(), application.ApprovalInput{
				PhaseID:  args[0],
				Approver: actor,
				Reason:   reason,
			})
			return handleDecisionResult(cmd, "rejected", args[0], err)
		},
	}
	cmd.Flags().StringVar(&approver, "approver", "", "approver username (defaults to $USER)")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "optional human-readable reason")
	return cmd
}

// resolveApprover returns the explicit --approver flag value or falls
// back to $USER. Empty string is propagated through to the application
// layer which rejects with domain.ErrApproverRequired.
func resolveApprover(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("USER")
}

// handleDecisionResult renders Approve/Reject outcomes. Idempotency
// rule: gate_already_decided exits 0 with an informational message
// (Phase 4 scope item 8). Other errors propagate to cobra → exit ≠ 0.
func handleDecisionResult(cmd *cobra.Command, verb, phaseID string, err error) error {
	if err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "sophia: phase %s %s\n", phaseID, verb)
		return nil
	}
	if errors.Is(err, domain.ErrGateAlreadyDecided) {
		fmt.Fprintf(cmd.OutOrStdout(),
			"sophia: phase %s gate already decided (no action taken)\n", phaseID)
		return nil
	}
	return err
}
