package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func newAttachCmd(d Deps) *cobra.Command {
	var (
		noTUI              bool
		jsonOut            bool
		approvalTimeoutStr string
	)
	cmd := &cobra.Command{
		Use:   "attach <change-id>",
		Short: "Attach to an existing Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.AttacherFactory == nil {
				return fmt.Errorf("attach: attacher factory not wired")
			}
			if err := validateModeFlags(noTUI, jsonOut); err != nil {
				return err
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("attach: change-id required (positional argument)")
			}

			// Resolve project for persisting last_change_id (best-effort).
			// Missing .sophia.yaml is fine — Attacher falls back to global-only.
			var project string
			if d.Resolver != nil {
				resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
					Env:            envSnapshot(),
					UserConfigPath: d.UserConfigPath,
					RequireProject: false,
				})
				if err == nil {
					project = resolved.Project
				}
			}

			approvalTimeout, err := time.ParseDuration(approvalTimeoutStr)
			if err != nil {
				return fmt.Errorf("attach: --approval-timeout: %w", err)
			}

			input := application.AttachInput{
				ChangeID: domain.ChangeID(args[0]),
				Project:  project,
			}

			if noTUI {
				return attachJSONL(cmd.Context(), d, input, approvalTimeout)
			}
			return attachTUI(cmd.Context(), d, input)
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of opening the TUI")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required with --no-tui)")
	cmd.Flags().StringVar(&approvalTimeoutStr, "approval-timeout", "30m",
		"max wait for an approval gate before exit code 5 (--no-tui only)")
	return cmd
}

func attachJSONL(parentCtx context.Context, d Deps, input application.AttachInput, approvalTimeout time.Duration) error {
	innerSink := chooseJSONSink(d)

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	wrapped := newApprovalTimeoutSink(innerSink, approvalTimeout, cancel)
	attacher := d.AttacherFactory(wrapped)

	// D-M8-13: eager-arm path. The CLI fetches the snapshot itself so it can
	// scan for an in-flight approval gate and start the timeout BEFORE
	// handing the snapshot to the Attacher. No double GetChange.
	snap, err := d.Orch.GetChange(ctx, input.ChangeID)
	if err != nil {
		_ = wrapped.Close()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return &application.ExitError{Code: 4, Err: err}
		}
		return &application.ExitError{Code: 3, Err: fmt.Errorf("attach: get change: %w", err)}
	}

	// Eager-arm the timer if the snapshot shows a phase already blocked on
	// approval. The synthetic gate carries phase + change_id so the JSONL
	// sink prints a meaningful row. If a real approval.required arrives via
	// SSE later, approvalTimeoutSink.startTimer is a no-op (cambio 3) so the
	// original eager-arm timestamp is preserved.
	if blocked := firstBlockedApprovalPhase(snap); blocked != nil {
		gate := domain.ApprovalGate{
			Phase:    blocked.Type,
			ChangeID: snap.ID,
		}
		_ = wrapped.OnApprovalGate(ctx, gate)
	}

	_, err = attacher.AttachFromSnapshot(ctx, snap, input.Project, wrapped)

	if waitErr := wrapped.Wait(); waitErr != nil {
		return &application.ExitError{Code: 5, Err: waitErr}
	}
	if err != nil {
		var exit *application.ExitError
		if errors.As(err, &exit) {
			return exit
		}
		return err
	}
	return nil
}

// firstBlockedApprovalPhase returns the first phase in snap whose status is
// PhaseStatusBlocked, or nil if none. Used by attachJSONL for D-M8-13's
// eager-arm of approvalTimeoutSink.
func firstBlockedApprovalPhase(snap *domain.Change) *domain.Phase {
	if snap == nil {
		return nil
	}
	for i := range snap.Phases {
		if snap.Phases[i].Status == domain.PhaseStatusBlocked {
			return &snap.Phases[i]
		}
	}
	return nil
}

func attachTUI(parentCtx context.Context, d Deps, input application.AttachInput) error {
	output := chooseTUIOutput(d)

	prog, err := tui.NewProgram(tui.ProgramConfig{
		Output:  output,
		Browser: d.Browser,
	})
	if err != nil {
		return fmt.Errorf("attach: tui init: %w", err)
	}
	defer prog.Close() //nolint:errcheck

	attacher := d.AttacherFactory(prog.Bridge())

	attacherCtx, cancelAttacher := context.WithCancel(parentCtx)
	defer cancelAttacher()

	type attacherResult struct {
		res application.RunResult
		err error
	}
	resultCh := make(chan attacherResult, 1)
	go func() {
		res, err := attacher.Attach(attacherCtx, input, prog.Bridge())
		resultCh <- attacherResult{res: res, err: err}
	}()

	hint, runErr := prog.Run(parentCtx)

	cancelAttacher()

	rr := <-resultCh

	if hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}

	if runErr != nil {
		return runErr
	}
	if rr.err != nil {
		var exit *application.ExitError
		if errors.As(rr.err, &exit) {
			return exit
		}
		return rr.err
	}
	return nil
}
