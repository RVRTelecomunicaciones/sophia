package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// newAbortCmd registers `sophia abort <change-id>` (Phase 4 Task 4.6).
// Idempotency: change_already_terminal exits 0 with informational text.
func newAbortCmd(d Deps) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "abort <change-id>",
		Short: "Abort a Change that is in flight",
		Long: `Abort signals the orchestrator to terminate a Change. The
CLI exits 0 if the Change was running (and is now aborting) OR was
already terminal (idempotent).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Aborter == nil {
				return fmt.Errorf("abort: aborter not wired")
			}
			id := domain.ChangeID(args[0])
			err := d.Aborter.Abort(cmd.Context(), application.AbortInput{
				ChangeID: id,
				Reason:   reason,
			})
			switch {
			case err == nil:
				fmt.Fprintf(cmd.OutOrStdout(), "sophia: change %s aborting\n", id)
				return nil
			case errors.Is(err, domain.ErrChangeAlreadyTerminal):
				fmt.Fprintf(cmd.OutOrStdout(),
					"sophia: change %s already terminal (no action taken)\n", id)
				return nil
			default:
				return err
			}
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "optional human-readable reason")
	return cmd
}
