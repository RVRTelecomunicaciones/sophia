//go:build integration

package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

func gitAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	return true
}

func TestInitInRealRepo(t *testing.T) {
	if !gitAvailable(t) {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	git := gitcli.New(gitcli.Config{})
	store := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{})
	init := application.NewInitializer(application.InitializerDeps{
		Git:          git,
		ProjectStore: store,
	})
	res, err := init.Run(context.Background(), application.InitInput{
		Project:       "ms-cotizacion",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if filepath.Dir(res.Path) != tmp {
		// EvalSymlinks may resolve macOS /tmp → /private/tmp; allow either.
		want, _ := filepath.EvalSymlinks(tmp)
		if filepath.Dir(res.Path) != want {
			t.Errorf("written outside repo: %s (want %s or %s)", res.Path, tmp, want)
		}
	}

	cfg, err := store.Read(context.Background(), res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "ms-cotizacion" {
		t.Errorf("Project = %q", cfg.Project)
	}
}

func TestStateStoreAtomicWriteSurvivesRestart(t *testing.T) {
	tmp := t.TempDir()
	s := filestate.New(filestate.Config{StateRoot: tmp})
	fp := domain.Fingerprint("fp-int")

	if err := s.SetLast(context.Background(), fp, "01HX"); err != nil {
		t.Fatal(err)
	}

	// Re-open in a fresh Store to simulate process restart.
	s2 := filestate.New(filestate.Config{StateRoot: tmp})
	got, err := s2.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got != "01HX" {
		t.Errorf("got %q after restart", got)
	}
}
