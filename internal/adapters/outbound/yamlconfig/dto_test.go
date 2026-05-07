package yamlconfig_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestProjectDTORoundTrip(t *testing.T) {
	in := &domain.ProjectConfig{
		Version:       1,
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	}
	dto := yamlconfig.ToProjectDTO(in)
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back yamlconfig.ProjectDTO
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	got := yamlconfig.FromProjectDTO(&back)
	if got.Project != in.Project || got.BaseRef != in.BaseRef || got.ArtifactStore != in.ArtifactStore {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestUserDTORoundTrip(t *testing.T) {
	in := &domain.UserConfig{
		Version:         1,
		OrchestratorURL: "http://localhost:9080",
		TimeoutSeconds:  30,
	}
	dto := yamlconfig.ToUserDTO(in)
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back yamlconfig.UserDTO
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	got := yamlconfig.FromUserDTO(&back)
	if got.OrchestratorURL != in.OrchestratorURL || got.TimeoutSeconds != in.TimeoutSeconds {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestProjectDTOArtifactStoreYAMLKey(t *testing.T) {
	dto := yamlconfig.ProjectDTO{
		Version:       1,
		Project:       "p",
		BaseRef:       "main",
		ArtifactStore: "engram",
	}
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) == "" {
		t.Fatal("empty yaml")
	}
	if !contains(string(out), "artifact_store: engram") {
		t.Errorf("yaml does not use artifact_store key: %s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
