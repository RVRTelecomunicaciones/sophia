package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newInit() (*application.Initializer, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	init := application.NewInitializer(application.InitializerDeps{
		Git:          git,
		ProjectStore: store,
	})
	return init, git, store
}

func TestInitWritesAtRepoRoot(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/home/u/myrepo"

	res, err := init.Run(context.Background(), application.InitInput{
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreMemoryEngine,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "/home/u/myrepo/.sophia.yaml" {
		t.Errorf("Path = %q", res.Path)
	}
	got, err := store.Read(context.Background(), res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "ms-cotizacion" {
		t.Errorf("project lost: %+v", got)
	}
}

func TestInitFailsOutsideGitRepo(t *testing.T) {
	init, git, _ := newInit()
	git.NotARepo = true
	_, err := init.Run(context.Background(), application.InitInput{Project: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotARepo) {
		t.Errorf("expected ErrNotARepo, got %v", err)
	}
}

func TestInitAbortsWhenConfigExistsWithoutForce(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/r"
	_ = store.Write(context.Background(), "/r/.sophia.yaml", &domain.ProjectConfig{Project: "old"})
	_, err := init.Run(context.Background(), application.InitInput{Project: "new"})
	if err == nil {
		t.Error("expected abort")
	}
}

func TestInitOverwritesWithForce(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/r"
	_ = store.Write(context.Background(), "/r/.sophia.yaml", &domain.ProjectConfig{Project: "old"})
	_, err := init.Run(context.Background(), application.InitInput{Project: "new", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(context.Background(), "/r/.sophia.yaml")
	if got.Project != "new" {
		t.Errorf("project = %q, want new", got.Project)
	}
}

func TestInitDefaultsBaseRefAndArtifactStore(t *testing.T) {
	init, _, store := newInit()
	res, err := init.Run(context.Background(), application.InitInput{Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(context.Background(), res.Path)
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
	if got.ArtifactStore != domain.ArtifactStoreMemoryEngine {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d", got.Version)
	}
}

func TestInitOverwriteWithForceWhenExistingIsInvalid(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/r"
	store.ReadErr = domain.ErrInvalidYAML
	_, err := init.Run(context.Background(), application.InitInput{Project: "p", Force: true})
	if err != nil {
		t.Fatal(err)
	}
}
