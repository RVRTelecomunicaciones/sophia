package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStubCmd(use, short, milestone string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: not implemented yet (planned for %s)\n", use, milestone)
			return nil
		},
	}
}
