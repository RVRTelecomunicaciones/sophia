package cli

import (
	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Runner       *application.Runner
	Resolver     *application.ConfigResolver

	UserConfigPath string // optional; passed to ConfigResolver

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
	root.AddCommand(newInitCmd(d))
	root.AddCommand(newStartCmd(d))
	root.AddCommand(newStopCmd(d))
	root.AddCommand(newRunCmd(d))
	root.AddCommand(newStubCmd("attach", "Attach to an existing Change", "M8"))
	root.AddCommand(newStatusCmd(d))
	root.AddCommand(newStubCmd("changes", "List recent Changes", "M8"))

	return root
}
