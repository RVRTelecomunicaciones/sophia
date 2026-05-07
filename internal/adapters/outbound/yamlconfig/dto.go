// Package yamlconfig implements outbound.ProjectConfigStore and
// outbound.UserConfigStore using gopkg.in/yaml.v3 over an afero filesystem.
package yamlconfig

import (
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// ProjectDTO is the on-disk shape of `.sophia.yaml`.
type ProjectDTO struct {
	Version       int    `yaml:"version"`
	Project       string `yaml:"project"`
	BaseRef       string `yaml:"base_ref"`
	ArtifactStore string `yaml:"artifact_store"`
}

// UserDTO is the on-disk shape of `<configRoot>/config.yaml`.
type UserDTO struct {
	Version      int               `yaml:"version"`
	Orchestrator OrchestratorBlock `yaml:"orchestrator"`
}

// OrchestratorBlock is the orchestrator subtree of UserDTO.
type OrchestratorBlock struct {
	URL            string `yaml:"url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// ToProjectDTO converts a domain config to its DTO.
func ToProjectDTO(c *domain.ProjectConfig) ProjectDTO {
	return ProjectDTO{
		Version:       c.Version,
		Project:       c.Project,
		BaseRef:       c.BaseRef,
		ArtifactStore: string(c.ArtifactStore),
	}
}

// FromProjectDTO converts a DTO to the domain type.
func FromProjectDTO(d *ProjectDTO) *domain.ProjectConfig {
	return &domain.ProjectConfig{
		Version:       d.Version,
		Project:       d.Project,
		BaseRef:       d.BaseRef,
		ArtifactStore: domain.ArtifactStoreMode(d.ArtifactStore),
	}
}

// ToUserDTO converts a domain UserConfig to DTO.
func ToUserDTO(c *domain.UserConfig) UserDTO {
	return UserDTO{
		Version: c.Version,
		Orchestrator: OrchestratorBlock{
			URL:            c.OrchestratorURL,
			TimeoutSeconds: c.TimeoutSeconds,
		},
	}
}

// FromUserDTO converts a DTO to UserConfig.
func FromUserDTO(d *UserDTO) *domain.UserConfig {
	return &domain.UserConfig{
		Version:         d.Version,
		OrchestratorURL: d.Orchestrator.URL,
		TimeoutSeconds:  d.Orchestrator.TimeoutSeconds,
	}
}
