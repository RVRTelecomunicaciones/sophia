package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/jsonsink"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

func newRunCmd(d Deps) *cobra.Command {
	var (
		noTUI               bool
		jsonOut             bool
		baseRef             string
		artifactStore       string
		project             string
		approvalTimeoutStr  string
	)
	cmd := &cobra.Command{
		Use:   "run [message]",
		Short: "Create and observe a Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.RunnerFactory == nil || d.Resolver == nil {
				return fmt.Errorf("run: runner factory not wired")
			}
			if err := validateModeFlags(noTUI, jsonOut); err != nil {
				return err
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("run: message required (positional argument)")
			}

			resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
				Flags: application.ResolverFlags{
					Project:       project,
					BaseRef:       baseRef,
					ArtifactStore: artifactStore,
				},
				Env:            envSnapshot(),
				UserConfigPath: d.UserConfigPath,
				RequireProject: true,
			})
			if err != nil {
				return err
			}

			approvalTimeout, err := time.ParseDuration(approvalTimeoutStr)
			if err != nil {
				return fmt.Errorf("run: --approval-timeout: %w", err)
			}

			input := application.RunInput{
				Project:       resolved.Project,
				Message:       args[0],
				BaseRef:       resolved.BaseRef,
				ArtifactStore: resolved.ArtifactStore,
			}

			if noTUI {
				return runJSONL(cmd.Context(), d, input, approvalTimeout)
			}
			return runTUI(cmd.Context(), d, input)
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of opening the TUI")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required with --no-tui)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "override base_ref")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "", "override artifact_store mode")
	cmd.Flags().StringVar(&project, "project", "", "override project slug")
	cmd.Flags().StringVar(&approvalTimeoutStr, "approval-timeout", "30m",
		"max wait for an approval gate before exit code 5 (--no-tui only)")
	return cmd
}

func validateModeFlags(noTUI, jsonOut bool) error {
	if noTUI && !jsonOut {
		return fmt.Errorf("run: --no-tui requires --json (machine-readable output)")
	}
	if jsonOut && !noTUI {
		return fmt.Errorf("run: --json requires --no-tui (TUI is the default)")
	}
	return nil
}

func runJSONL(parentCtx context.Context, d Deps, input application.RunInput, approvalTimeout time.Duration) error {
	innerSink := chooseJSONSink(d)

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	wrapped := newApprovalTimeoutSink(innerSink, approvalTimeout, cancel)
	runner := d.RunnerFactory(wrapped)

	res, err := runner.Run(ctx, input)
	_ = res

	// If the timer fired, Wait returns errApprovalTimeout. Map to ExitError{5}.
	if waitErr := wrapped.Wait(); waitErr != nil {
		return &application.ExitError{Code: 5, Err: waitErr}
	}

	if err != nil {
		var exit *application.ExitError
		if errors.As(err, &exit) {
			return exit
		}
		return err
	}
	return nil
}

func runTUI(parentCtx context.Context, d Deps, input application.RunInput) error {
	output := chooseTUIOutput(d)

	prog, err := tui.NewProgram(tui.ProgramConfig{Output: output})
	if err != nil {
		return fmt.Errorf("run: tui init: %w", err)
	}
	defer prog.Close() //nolint:errcheck

	runner := d.RunnerFactory(prog.Bridge())

	runnerCtx, cancelRunner := context.WithCancel(parentCtx)
	defer cancelRunner()

	type runnerResult struct {
		res application.RunResult
		err error
	}
	resultCh := make(chan runnerResult, 1)
	go func() {
		res, err := runner.Run(runnerCtx, input)
		resultCh <- runnerResult{res: res, err: err}
	}()

	hint, runErr := prog.Run(parentCtx)

	cancelRunner()

	rr := <-resultCh

	if hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}

	if runErr != nil {
		return runErr
	}
	if rr.err != nil {
		var exit *application.ExitError
		if errors.As(rr.err, &exit) {
			return exit
		}
		return rr.err
	}
	return nil
}

func chooseJSONSink(d Deps) inbound.EventSink {
	if d.JSONSinkOverride != nil {
		return d.JSONSinkOverride
	}
	return jsonsink.New(jsonsink.Config{Writer: os.Stdout})
}

func chooseTUIOutput(d Deps) io.Writer {
	if d.TUIOutput != nil {
		return d.TUIOutput
	}
	return os.Stdout
}

// envSnapshot returns the SOPHIA_* env vars consulted by the resolver.
func envSnapshot() map[string]string {
	out := map[string]string{}
	for _, k := range []string{
		application.EnvOrchestratorURL,
		application.EnvProject,
		application.EnvBaseRef,
	} {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		}
	}
	return out
}
