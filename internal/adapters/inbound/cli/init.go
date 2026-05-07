package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func newInitCmd(d Deps) *cobra.Command {
	var (
		project       string
		baseRef       string
		artifactStore string
		force         bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .sophia.yaml at the resolved repo root",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Initializer == nil {
				return fmt.Errorf("init: initializer not wired")
			}
			res, err := d.Initializer.Run(cmd.Context(), application.InitInput{
				Project:       project,
				BaseRef:       baseRef,
				ArtifactStore: domain.ArtifactStoreMode(artifactStore),
				Force:         force,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (project=%s, base_ref=%s)\n",
				res.Path, project, baseRefDefault(baseRef))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project slug (required)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "main", "default git ref for new Changes")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "engram", "artifact store: engram | openspec | hybrid | none")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing or invalid .sophia.yaml")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func baseRefDefault(s string) string {
	if s == "" {
		return "main"
	}
	return s
}
