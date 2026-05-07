package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// RunnerFactory builds a *application.Runner with the caller-provided sink.
type RunnerFactory func(sink inbound.EventSink) *application.Runner

// AttacherFactory builds a *application.Attacher with the caller-provided sink.
type AttacherFactory func(sink inbound.EventSink) *application.Attacher

type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Lister       *application.Lister
	Approver     *application.Approver
	Aborter      *application.Aborter
	Resolver     *application.ConfigResolver

	// Orch is the raw OrchestratorClient. Required by Task 6's
	// `attachJSONL` eager-arm (declared here so Task 6 wiring is trivial).
	Orch outbound.OrchestratorClient

	// RunnerFactory is the M6 way of constructing a Runner — sink-injected
	// at command time. Mandatory for `sophia run`.
	RunnerFactory RunnerFactory

	// AttacherFactory is the M8 way of constructing an Attacher — sink-injected
	// at command time. Mandatory for `sophia attach`.
	AttacherFactory AttacherFactory

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
	root.AddCommand(newAttachCmd(d))
	root.AddCommand(newStatusCmd(d))
	root.AddCommand(newChangesCmd(d))
	// v0.2.0 (Phase 4 Task 4.4-4.6): in-band approval / rejection /
	// abort commands. The browser flow is unchanged; these commands are
	// additive (D-M10-03).
	root.AddCommand(newApproveCmd(d))
	root.AddCommand(newRejectCmd(d))
	root.AddCommand(newAbortCmd(d))

	return root
}
