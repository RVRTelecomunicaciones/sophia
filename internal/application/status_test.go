package application_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newStatus() (*application.StatusReader, *fakes.FakeStateStore, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		State:        state,
		Git:          git,
		ProjectStore: store,
	})
	return r, state, git, store
}

func TestStatusEmptyWhenNoProjectAndNoGlobal(t *testing.T) {
	r, _, _, _ := newStatus()
	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsEmpty {
		t.Errorf("expected empty, got %+v", out)
	}
}

func TestStatusPrefersProjectScoped(t *testing.T) {
	r, state, git, store := newStatus()
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "01HX-project")
	_ = state.SetGlobalLast(context.Background(), "01HX-global")

	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.ChangeID != "01HX-project" {
		t.Errorf("ChangeID = %q (want project-scoped)", out.ChangeID)
	}
	if out.Source != application.StatusSourceProject {
		t.Errorf("Source = %q", out.Source)
	}
}

func TestStatusFallsBackToGlobal(t *testing.T) {
	r, state, _, _ := newStatus()
	_ = state.SetGlobalLast(context.Background(), "01HX-global")

	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.ChangeID != "01HX-global" {
		t.Errorf("ChangeID = %q", out.ChangeID)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q", out.Source)
	}
}

func TestStatusUsesNoConfigOutsideRepo(t *testing.T) {
	r, state, git, _ := newStatus()
	git.NotARepo = true
	_ = state.SetGlobalLast(context.Background(), "01HX-global")
	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.ChangeID != "01HX-global" || out.Source != application.StatusSourceGlobal {
		t.Errorf("expected global fallback, got %+v", out)
	}
}
