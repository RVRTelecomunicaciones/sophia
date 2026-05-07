package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func newStatusCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [<change-id>]",
		Short: "Show status of a Change (resolution: arg → project-scoped → global → empty)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.StatusReader == nil {
				return fmt.Errorf("status: reader not wired")
			}
			in := application.ResolveInput{}
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				in.ChangeID = domain.ChangeID(strings.TrimSpace(args[0]))
			}
			report, err := d.StatusReader.Resolve(cmd.Context(), in)
			if err != nil {
				var exit *application.ExitError
				if errors.As(err, &exit) {
					return exit
				}
				return err
			}
			w := cmd.OutOrStdout()
			if jsonOut {
				return printStatusJSON(w, report)
			}
			return printStatusHuman(w, report)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// printStatusHuman renders a multi-line human-readable summary.
func printStatusHuman(w io.Writer, r application.StatusReport) error {
	if r.IsEmpty {
		if _, err := fmt.Fprintln(w, "No local change found."); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w, "Use sophia changes or pass <change-id> explicitly.")
		return err
	}
	c := r.Change
	if _, err := fmt.Fprintf(w, "Change:  %s\n", c.ID); err != nil {
		return err
	}
	statusLine := string(c.Status)
	if c.CurrentPhaseID != "" {
		statusLine += fmt.Sprintf(" (current_phase=%s)", c.CurrentPhaseID)
	}
	if _, err := fmt.Fprintf(w, "Status:  %s\n", statusLine); err != nil {
		return err
	}
	if c.Project != "" {
		if _, err := fmt.Fprintf(w, "Project: %s\n", c.Project); err != nil {
			return err
		}
	}
	if c.BaseRef != "" {
		if _, err := fmt.Fprintf(w, "BaseRef: %s\n", c.BaseRef); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Source:  %s\n", r.Source); err != nil {
		return err
	}
	if !c.UpdatedAt.IsZero() {
		if _, err := fmt.Fprintf(w, "Updated: %s\n", c.UpdatedAt.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// printStatusJSON renders a single ChangeResponse object (or null when empty).
func printStatusJSON(w io.Writer, r application.StatusReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if r.IsEmpty || r.Change == nil {
		return enc.Encode(nil)
	}
	resp := changeResponseFromDomain(r.Change)
	return enc.Encode(resp)
}
