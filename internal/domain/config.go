package domain

type ArtifactStoreMode string

const (
	ArtifactStoreEngram   ArtifactStoreMode = "engram"
	ArtifactStoreOpenspec ArtifactStoreMode = "openspec"
	ArtifactStoreHybrid   ArtifactStoreMode = "hybrid"
	ArtifactStoreNone     ArtifactStoreMode = "none"
)

func (m ArtifactStoreMode) IsValid() bool {
	switch m {
	case ArtifactStoreEngram, ArtifactStoreOpenspec, ArtifactStoreHybrid, ArtifactStoreNone:
		return true
	}
	return false
}

type ProjectConfig struct {
	Version       int
	Project       string
	BaseRef       string
	ArtifactStore ArtifactStoreMode
}

type UserConfig struct {
	Version         int
	OrchestratorURL string
	TimeoutSeconds  int
}
