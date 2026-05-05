package cli

import (
	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

type Deps struct {
	Doctor      *application.DoctorService
	Provisioner *application.Provisioner

	Version   string
	Commit    string
	BuildDate string
}

func NewRoot(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "sophia",
		Short: "Sophia CLI — create and observe SDD Changes",
		Long: `sophia is the human entry point of the Sophia ecosystem.

It creates and observes Changes executed by sophia-orchestator. The CLI
itself does not coordinate phases, evaluate policy, or store canonical
state.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd(d))
	root.AddCommand(newDoctorCmd(d))
	root.AddCommand(newStubCmd("init", "Initialize .sophia.yaml at the resolved repo root", "M3"))
	root.AddCommand(newStartCmd(d))
	root.AddCommand(newStopCmd(d))
	root.AddCommand(newStubCmd("run", "Create and observe a Change", "M4"))
	root.AddCommand(newStubCmd("attach", "Attach to an existing Change", "M8"))
	root.AddCommand(newStubCmd("status", "Show status of a Change", "M3"))
	root.AddCommand(newStubCmd("changes", "List recent Changes", "M8"))

	return root
}
