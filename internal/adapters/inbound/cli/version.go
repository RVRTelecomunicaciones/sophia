package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print sophia version, commit, and build date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "sophia %s (commit %s, built %s)\n", d.Version, d.Commit, d.BuildDate)
			return nil
		},
	}
}
