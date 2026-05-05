package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStopCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Sophia stack",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Provisioner == nil {
				return fmt.Errorf("stop: provisioner not wired")
			}
			if err := d.Provisioner.Down(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sophia: stack stopped (project=sophia)")
			return nil
		},
	}
}
