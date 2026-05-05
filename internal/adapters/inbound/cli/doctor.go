package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newDoctorCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run environment diagnostics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := d.Doctor.Run(cmd.Context())
			if jsonOut {
				return printDoctorJSON(cmd.OutOrStdout(), report)
			}
			printDoctorTable(cmd.OutOrStdout(), report)
			if report.Summary.Fail > 0 {
				return fmt.Errorf("doctor: %d check(s) failed", report.Summary.Fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit a JSON report instead of the human table")
	return cmd
}

func printDoctorTable(w io.Writer, r application.DiagnosticsReport) {
	fmt.Fprintln(w, "sophia doctor — checking environment")
	fmt.Fprintln(w)
	for _, c := range r.Checks {
		icon := "✓"
		switch c.Level {
		case application.LevelInfo:
			icon = "ℹ"
		case application.LevelWarn:
			icon = "⚠"
		case application.LevelFail:
			icon = "✗"
		}
		fmt.Fprintf(w, "  %s %-20s %s\n", icon, c.Title, c.Detail)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d ok · %d info · %d warn · %d fail\n", r.Summary.OK, r.Summary.Info, r.Summary.Warn, r.Summary.Fail)
}

func printDoctorJSON(w io.Writer, r application.DiagnosticsReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
