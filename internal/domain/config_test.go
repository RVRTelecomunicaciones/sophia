package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func TestArtifactStoreModeIsValid(t *testing.T) {
	valid := []domain.ArtifactStoreMode{
		domain.ArtifactStoreEngram,
		domain.ArtifactStoreOpenspec,
		domain.ArtifactStoreHybrid,
		domain.ArtifactStoreNone,
	}
	for _, m := range valid {
		if !m.IsValid() {
			t.Errorf("%q should be valid", m)
		}
	}
	if domain.ArtifactStoreMode("bogus").IsValid() {
		t.Error("bogus mode should not be valid")
	}
	if domain.ArtifactStoreMode("").IsValid() {
		t.Error("empty mode should not be valid")
	}
}

func TestProjectConfigZeroValue(t *testing.T) {
	var c domain.ProjectConfig
	if c.Project != "" || c.BaseRef != "" {
		t.Error("zero ProjectConfig should have empty fields")
	}
}

func TestUserConfigZeroValue(t *testing.T) {
	var c domain.UserConfig
	if c.OrchestratorURL != "" || c.TimeoutSeconds != 0 {
		t.Error("zero UserConfig should have empty fields")
	}
}
