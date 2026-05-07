package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// DefaultChangesLimit is the default value for `sophia changes --limit`
// (D-M8-01: spec §2.5 / §5.1). Higher values pass through unchanged; the
// orchestrator decides the upper bound.
const DefaultChangesLimit = 10

func newChangesCmd(d Deps) *cobra.Command {
	var (
		limit   int
		status  string
		project string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List recent Changes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Lister == nil {
				return fmt.Errorf("changes: lister not wired")
			}

			// Project resolution per D-M8-07 (CLI-only — Lister is a pure
			// pass-through):
			//
			//   --project not passed     → resolve default from .sophia.yaml
			//                              (best-effort; missing config or
			//                              malformed YAML logs a warning and
			//                              falls through to no filter)
			//   --project="anything"      → use as-is
			//   --project=""              → no filter (list all)
			var effectiveProject string
			projectFlagSet := cmd.Flags().Changed("project")
			if projectFlagSet {
				effectiveProject = project
			} else if d.Resolver != nil {
				resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
					UserConfigPath: d.UserConfigPath,
					RequireProject: false,
				})
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: project default unavailable (%v); listing all projects\n", err)
				} else {
					effectiveProject = resolved.Project
				}
			}

			out, err := d.Lister.List(cmd.Context(), application.ListInput{
				Project: effectiveProject,
				Status:  status,
				Limit:   limit,
				Offset:  0,
			})
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if jsonOut {
				return printChangesJSON(w, out)
			}
			return printChangesTable(w, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", DefaultChangesLimit, "max number of Changes to return")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (pending|running|done|blocked|failed)")
	cmd.Flags().StringVar(&project, "project", "", "filter by project; empty value disables the project filter")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON array")
	return cmd
}

// printChangesTable renders a column-aligned table.
func printChangesTable(w io.Writer, items []*domain.Change) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tSTATUS\tPROJECT\tBASE_REF\tCREATED_AT"); err != nil {
		return err
	}
	for _, c := range items {
		created := ""
		if !c.CreatedAt.IsZero() {
			created = c.CreatedAt.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.ID, c.Status, c.Project, c.BaseRef, created); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// printChangesJSON renders a JSON array using the orchestrator's wire shape
// (D-M8-08).
func printChangesJSON(w io.Writer, items []*domain.Change) error {
	out := make([]orchestratorhttp.ChangeResponse, 0, len(items))
	for _, c := range items {
		out = append(out, changeResponseFromDomain(c))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
