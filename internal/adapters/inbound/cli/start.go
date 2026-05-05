package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newStartCmd(d Deps) *cobra.Command {
	var reset bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the local Sophia stack via docker compose",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Provisioner == nil {
				return fmt.Errorf("start: provisioner not wired")
			}
			res, err := d.Provisioner.Up(cmd.Context(), application.UpInput{Reset: reset})
			if err != nil {
				return err
			}
			if res.Wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "compose materialized at %s\n", res.ComposePath)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sophia: stack started (project=sophia)")
			return nil
		},
	}
	cmd.Flags().BoolVar(&reset, "reset-compose", false, "overwrite a user-edited compose.yaml (saves a .previous backup)")
	return cmd
}
