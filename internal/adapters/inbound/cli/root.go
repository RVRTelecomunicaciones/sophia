package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// RunnerFactory builds a *application.Runner with the caller-provided sink.
type RunnerFactory func(sink inbound.EventSink) *application.Runner

type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Lister       *application.Lister
	Resolver     *application.ConfigResolver

	// Orch is the raw OrchestratorClient. Required by Task 6's
	// `attachJSONL` eager-arm (declared here so Task 6 wiring is trivial).
	Orch outbound.OrchestratorClient

	// RunnerFactory is the M6 way of constructing a Runner — sink-injected
	// at command time. Mandatory for `sophia run`.
	RunnerFactory RunnerFactory

	// JSONSinkOverride lets tests inject a recording sink instead of
	// jsonsink.New(os.Stdout). Production leaves this nil.
	JSONSinkOverride inbound.EventSink

	// TUIOutput is the writer the TUI program renders to. Defaults to
	// os.Stdout. Tests inject a buffer.
	TUIOutput io.Writer

	// Browser is the outbound.Browser passed to the TUI program for [O].
	// Bootstrap injects an osbrowser instance; tests inject FakeBrowser.
	Browser outbound.Browser

	UserConfigPath string

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
	root.AddCommand(newChangesCmd(d))

	return root
}
