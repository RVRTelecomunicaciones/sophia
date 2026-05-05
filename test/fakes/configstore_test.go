package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeProjectConfigImplementsPort(t *testing.T) {
	var _ outbound.ProjectConfigStore = (*fakes.FakeProjectConfigStore)(nil)
}

func TestFakeUserConfigImplementsPort(t *testing.T) {
	var _ outbound.UserConfigStore = (*fakes.FakeUserConfigStore)(nil)
}

func TestFakeProjectConfigRoundTrip(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	cfg := &domain.ProjectConfig{Version: 1, Project: "p", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram}
	if err := s.Write(context.Background(), "/repo/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "p" || got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestFakeProjectConfigFind(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	cfg := &domain.ProjectConfig{Project: "p"}
	_ = s.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	path, err := s.Find(context.Background(), "/repo/sub/dir")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/repo/.sophia.yaml" {
		t.Errorf("Find = %q", path)
	}
}

func TestFakeProjectConfigFindMissing(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	_, err := s.Find(context.Background(), "/somewhere")
	if err == nil {
		t.Error("expected error when no .sophia.yaml is registered")
	}
}
