package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newResolver() (*application.ConfigResolver, *fakes.FakeProjectConfigStore, *fakes.FakeUserConfigStore, *fakes.FakeGitInspector) {
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()
	git := fakes.NewFakeGitInspector()
	r := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc,
		UserStore:    uc,
		Git:          git,
	})
	return r, pc, uc, git
}

func TestResolverFlagBeatsEnv(t *testing.T) {
	r, _, _, _ := newResolver()
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		Flags: application.ResolverFlags{OrchestratorURL: "http://flag"},
		Env:   map[string]string{"SOPHIA_ORCHESTRATOR_URL": "http://env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != "http://flag" {
		t.Errorf("OrchestratorURL = %q", got.OrchestratorURL)
	}
}

func TestResolverEnvBeatsProject(t *testing.T) {
	r, pc, _, git := newResolver()
	git.Root = "/repo"
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "from-yaml", BaseRef: "develop",
	})
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		Env: map[string]string{"SOPHIA_PROJECT": "from-env", "SOPHIA_BASE_REF": "release"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "from-env" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.BaseRef != "release" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
}

func TestResolverProjectBeatsUser(t *testing.T) {
	r, pc, uc, git := newResolver()
	git.Root = "/repo"
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "from-yaml", BaseRef: "main", ArtifactStore: domain.ArtifactStoreOpenspec,
	})
	_ = uc.Write(context.Background(), "/cfg/config.yaml", &domain.UserConfig{
		OrchestratorURL: "http://from-user", TimeoutSeconds: 42,
	})
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		UserConfigPath: "/cfg/config.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "from-yaml" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.OrchestratorURL != "http://from-user" {
		t.Errorf("OrchestratorURL = %q", got.OrchestratorURL)
	}
	if got.ArtifactStore != domain.ArtifactStoreOpenspec {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
}

func TestResolverDefaultsApplyLast(t *testing.T) {
	r, _, _, git := newResolver()
	git.NotARepo = true
	got, err := r.Resolve(context.Background(), application.ResolverInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != application.DefaultOrchestratorURL {
		t.Errorf("OrchestratorURL = %q (want default)", got.OrchestratorURL)
	}
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
	if got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
	if got.TimeoutSeconds != application.DefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds = %d", got.TimeoutSeconds)
	}
}

func TestResolverFailsWhenProjectMissingForRun(t *testing.T) {
	r, _, _, git := newResolver()
	git.NotARepo = true
	_, err := r.Resolve(context.Background(), application.ResolverInput{RequireProject: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrConfigMissing) {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestResolverInvalidProjectYAMLBubblesUp(t *testing.T) {
	r, pc, _, git := newResolver()
	git.Root = "/repo"
	pc.ReadErr = domain.ErrInvalidYAML
	_, err := r.Resolve(context.Background(), application.ResolverInput{RequireProject: true})
	if !errors.Is(err, domain.ErrInvalidYAML) {
		t.Errorf("expected ErrInvalidYAML, got %v", err)
	}
}

func TestResolverInvalidUserYAMLBubblesUp(t *testing.T) {
	r, _, uc, _ := newResolver()
	uc.ReadErr = domain.ErrInvalidYAML
	_, err := r.Resolve(context.Background(), application.ResolverInput{
		UserConfigPath: "/cfg/config.yaml",
	})
	if !errors.Is(err, domain.ErrInvalidYAML) {
		t.Errorf("expected ErrInvalidYAML, got %v", err)
	}
}

func TestResolverRejectsInvalidArtifactStoreFlag(t *testing.T) {
	r, _, _, _ := newResolver()
	_, err := r.Resolve(context.Background(), application.ResolverInput{
		Flags: application.ResolverFlags{ArtifactStore: "weird"},
	})
	if err == nil {
		t.Fatal("expected error on invalid artifact-store")
	}
	if !errors.Is(err, domain.ErrConfigMissing) {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}
