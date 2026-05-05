package yamlconfig_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/spf13/afero"
)

func TestProjectStoreImplementsPort(t *testing.T) {
	var _ outbound.ProjectConfigStore = yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: afero.NewMemMapFs()})
}

func TestProjectStoreWriteThenRead(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})

	cfg := &domain.ProjectConfig{
		Version:       1,
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	}
	path := "/repo/.sophia.yaml"
	if err := s.Write(context.Background(), path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "ms-cotizacion" || got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestProjectStoreReadMissingReturnsErrConfigMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != domain.ErrConfigMissing {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestProjectStoreReadInvalidYAMLReturnsErrInvalidYAML(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/repo/.sophia.yaml", []byte("::not yaml::"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != domain.ErrInvalidYAML {
		t.Errorf("expected ErrInvalidYAML, got %v", err)
	}
}

func TestProjectStoreReadRejectsOversizedFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	big := make([]byte, 101*1024)
	for i := range big {
		big[i] = 'a'
	}
	if err := afero.WriteFile(fs, "/repo/.sophia.yaml", big, 0o644); err != nil {
		t.Fatal(err)
	}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	if _, err := s.Read(context.Background(), "/repo/.sophia.yaml"); err == nil {
		t.Error("expected size cap error")
	}
}

func TestProjectStoreFindWalksAncestors(t *testing.T) {
	fs := afero.NewMemMapFs()
	cfg := &domain.ProjectConfig{Version: 1, Project: "p", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	if err := s.Write(context.Background(), "/repo/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	startDir := filepath.Join("/repo", "sub", "deeper")
	if err := fs.MkdirAll(startDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find(context.Background(), startDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/repo/.sophia.yaml" {
		t.Errorf("Find = %q", got)
	}
}

func TestProjectStoreFindReturnsErrNotFound(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Find(context.Background(), "/nowhere/sub")
	if err == nil {
		t.Error("expected error")
	}
}

func TestProjectStoreWriteCreatesParentDirs(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	if err := s.Write(context.Background(), "/new/path/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	exists, _ := afero.Exists(fs, "/new/path/.sophia.yaml")
	if !exists {
		t.Error("expected file written under /new/path")
	}
}
