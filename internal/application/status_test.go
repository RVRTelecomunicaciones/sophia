package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newStatus() (*application.StatusReader, *fakes.FakeOrchestrator, *fakes.FakeStateStore, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch:         orch,
		State:        state,
		Git:          git,
		ProjectStore: store,
	}, application.StatusOptions{FetchTimeout: time.Second})
	return r, orch, state, git, store
}

func TestStatusEmptyWhenNoArgNoProjectNoGlobal(t *testing.T) {
	r, _, _, _, _ := newStatus()
	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsEmpty {
		t.Errorf("expected empty, got %+v", out)
	}
	if out.Source != application.StatusSourceNone {
		t.Errorf("Source = %q, want none", out.Source)
	}
	if out.Change != nil {
		t.Errorf("Change should be nil on empty result, got %+v", out.Change)
	}
}

func TestStatusFlagArgWinsAndFetchesSnapshot(t *testing.T) {
	r, orch, _, _, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "FROM-ARG", Status: domain.ChangeStatusRunning})

	out, err := r.Resolve(context.Background(), application.ResolveInput{
		ChangeID: "FROM-ARG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.Source != application.StatusSourceFlag {
		t.Errorf("Source = %q, want flag", out.Source)
	}
	if out.Change == nil || out.Change.ID != "FROM-ARG" {
		t.Errorf("Change = %+v", out.Change)
	}
	if out.Change.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", out.Change.Status)
	}
}

func TestStatusPrefersProjectScopedOverGlobal(t *testing.T) {
	r, orch, state, git, store := newStatus()
	orch.SeedChange(&domain.Change{ID: "PROJ", Status: domain.ChangeStatusDone})
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "PROJ")
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceProject {
		t.Errorf("Source = %q, want project", out.Source)
	}
	if out.Change.ID != "PROJ" {
		t.Errorf("ID = %q, want PROJ", out.Change.ID)
	}
}

func TestStatusFallsBackToGlobalWhenNoProjectScoped(t *testing.T) {
	r, orch, state, _, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusBlocked})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q, want global", out.Source)
	}
	if out.Change.ID != "GLOB" {
		t.Errorf("ID = %q", out.Change.ID)
	}
	if out.Change.Status != domain.ChangeStatusBlocked {
		t.Errorf("Status = %q", out.Change.Status)
	}
}

// Project config parses cleanly and produces a fingerprint, but no run has
// ever been recorded for this project. status falls through to global.
func TestStatusProjectConfigOKNoLastFallsThroughToGlobal(t *testing.T) {
	r, orch, state, git, store := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	// NOTE: state.SetLast is intentionally not called for this fingerprint.
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q, want global", out.Source)
	}
	if out.Change == nil || out.Change.ID != "GLOB" {
		t.Errorf("Change = %+v", out.Change)
	}
}

func TestStatusFlagArgChangeNotFoundExitCode3(t *testing.T) {
	r, _, _, _, _ := newStatus()
	_, err := r.Resolve(context.Background(), application.ResolveInput{
		ChangeID: "MISSING",
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
	if !errors.Is(err, domain.ErrChangeNotFound) {
		t.Errorf("expected wrapped ErrChangeNotFound; got %v", err)
	}
}

func TestStatusStaleProjectIDChangeNotFoundExitCode3(t *testing.T) {
	r, _, state, git, store := newStatus()
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "STALE")

	_, err := r.Resolve(context.Background(), application.ResolveInput{})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestStatusOutsideRepoFallsBackToGlobalAndFetches(t *testing.T) {
	r, orch, state, git, _ := newStatus()
	git.NotARepo = true
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q", out.Source)
	}
	if out.Change == nil || out.Change.ID != "GLOB" {
		t.Errorf("Change = %+v", out.Change)
	}
}

func TestStatusCtxCanceledDuringFetchExitCode4(t *testing.T) {
	r, orch, _, _, _ := newStatus()
	orch.GetBlockUntilCancel = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — fake blocks on ctx.Done() and returns immediately.

	_, err := r.Resolve(ctx, application.ResolveInput{ChangeID: "X"})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4 (ctx cancel during fetch)", exit.Code)
	}
}

// An internal FetchTimeout (parent ctx still alive) is exit 4.
func TestStatusInternalFetchTimeoutExitCode4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch:         orch,
		State:        state,
		Git:          git,
		ProjectStore: store,
	}, application.StatusOptions{FetchTimeout: 20 * time.Millisecond})

	orch.SeedChange(&domain.Change{ID: "X"})
	orch.GetBlockUntilCancel = true // hold until fctx times out

	_, err := r.Resolve(context.Background(), application.ResolveInput{ChangeID: "X"})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4 (internal fetch timeout)", exit.Code)
	}
}

// A malformed .sophia.yaml is fatal — status MUST NOT silently fall through
// to the global last_change_id.
func TestStatusInvalidProjectYAMLExitCode3(t *testing.T) {
	r, orch, state, git, store := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")
	git.Root = "/repo"
	store.ReadErr = fmt.Errorf("yaml: line 3: %w", domain.ErrInvalidYAML)

	_, err := r.Resolve(context.Background(), application.ResolveInput{})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3 (invalid .sophia.yaml)", exit.Code)
	}
	if !errors.Is(err, domain.ErrInvalidYAML) {
		t.Errorf("expected wrapped ErrInvalidYAML; got %v", err)
	}
}

// Missing .sophia.yaml falls through to global (negative pair to the
// invalid-yaml case above).
func TestStatusMissingProjectYAMLFallsThroughToGlobal(t *testing.T) {
	r, orch, state, git, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")
	git.Root = "/repo"
	// store has no .sophia.yaml at /repo/.sophia.yaml; Read returns ErrConfigMissing.

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatalf("expected fall-through to global; got %v", err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q, want global", out.Source)
	}
}
