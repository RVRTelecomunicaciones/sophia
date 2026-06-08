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
		autoBootstrap bool
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
				AutoBootstrap: autoBootstrap,
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
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "memory-engine", "artifact store: memory-engine | openspec | hybrid | none")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing or invalid .sophia.yaml")
	cmd.Flags().BoolVar(&autoBootstrap, "auto-bootstrap-graphify", false,
		"If graphify is not detected, attempt `uv tool install graphifyy[mcp]==0.8.35`. Default OFF.")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func baseRefDefault(s string) string {
	if s == "" {
		return "main"
	}
	return s
}
