package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newRunCmd(d Deps) *cobra.Command {
	var (
		noTUI         bool
		jsonOut       bool
		baseRef       string
		artifactStore string
		project       string
	)
	cmd := &cobra.Command{
		Use:   "run [message]",
		Short: "Create and observe a Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Runner == nil || d.Resolver == nil {
				return fmt.Errorf("run: runner not wired")
			}
			if !noTUI || !jsonOut {
				return fmt.Errorf("run: --no-tui and --json are both required in M4 (TUI ships in M6)")
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
			res, err := d.Runner.Run(cmd.Context(), application.RunInput{
				Project:       resolved.Project,
				Message:       args[0],
				BaseRef:       resolved.BaseRef,
				ArtifactStore: resolved.ArtifactStore,
			})
			if err != nil {
				var exit *application.ExitError
				if errors.As(err, &exit) {
					return exit
				}
				return err
			}
			_ = res // success path — output already streamed to the sink
			return nil
		},
	}
	// --orchestrator-url ships in M5 (orch client needs per-call URL support).
	// Set SOPHIA_ORCHESTRATOR_URL at process start instead.
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of a TUI (required in M4)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required in M4 with --no-tui)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "override base_ref")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "", "override artifact_store mode")
	cmd.Flags().StringVar(&project, "project", "", "override project slug")
	return cmd
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
