package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of a Change (M3: local resolution only; orchestrator call ships in M8)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.StatusReader == nil {
				return fmt.Errorf("status: reader not wired")
			}
			out, err := d.StatusReader.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if out.IsEmpty {
				fmt.Fprintln(w, "No local change found.")
				fmt.Fprintln(w, "Use sophia changes or pass <change-id> explicitly.")
				return nil
			}
			fmt.Fprintf(w, "last change: %s (source=%s)\n", out.ChangeID, out.Source)
			return nil
		},
	}
}
