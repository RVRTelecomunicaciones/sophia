package domain

// ArtifactStoreMode names the SDD artifact backend the orch uses for
// proposals, specs, designs, and other phase outputs. The valid values
// MUST match the orch's domain enum verbatim (see
// sophia-orchestator/internal/domain/change/status.go) — wire mismatches
// here surface as HTTP 400 'invalid artifact_store' from the orch on
// `change new`.
type ArtifactStoreMode string

const (
	// ArtifactStoreMemoryEngine routes artifacts to the
	// sophia-memory-engine HTTP service. This is the production
	// default. The string literal is "memory-engine" (kebab-case)
	// matching the orch's enum — NOT the legacy "engram" string
	// (which referred to an unrelated local memory tool and caused
	// HTTP 400 'invalid artifact_store' from the orch when sent on
	// the wire). See the rename PR for the full migration trail.
	ArtifactStoreMemoryEngine ArtifactStoreMode = "memory-engine"
	ArtifactStoreOpenspec     ArtifactStoreMode = "openspec"
	ArtifactStoreHybrid       ArtifactStoreMode = "hybrid"
	ArtifactStoreNone         ArtifactStoreMode = "none"
)

func (m ArtifactStoreMode) IsValid() bool {
	switch m {
	case ArtifactStoreMemoryEngine, ArtifactStoreOpenspec, ArtifactStoreHybrid, ArtifactStoreNone:
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
