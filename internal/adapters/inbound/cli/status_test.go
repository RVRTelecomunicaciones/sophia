package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

//nolint:unparam // pc/git returned for future tests; helper shape is the M8 contract.
func newStatusDeps(t *testing.T) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeStateStore, *fakes.FakeProjectConfigStore, *fakes.FakeGitInspector) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch: orch, State: state, Git: git, ProjectStore: pc,
	}, application.StatusOptions{})
	return cli.Deps{StatusReader: r}, orch, state, pc, git
}

func TestStatusCommandEmptyExitsZero(t *testing.T) {
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})

	if err := c.Execute(); err != nil {
		t.Errorf("status with no resolution should NOT error (exit 0); got %v", err)
	}
	if !strings.Contains(out.String(), "No local change found") {
		t.Errorf("output missing empty-state message: %s", out.String())
	}
}

func TestStatusCommandPositionalArgFetches(t *testing.T) {
	deps, orch, _, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "ARG", Status: domain.ChangeStatusRunning, Project: "p"})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "ARG"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ARG") {
		t.Errorf("output missing change ID: %s", out.String())
	}
	if !strings.Contains(out.String(), "running") {
		t.Errorf("output missing status: %s", out.String())
	}
	if !strings.Contains(out.String(), "flag") {
		t.Errorf("output missing source=flag: %s", out.String())
	}
}

func TestStatusCommandFallsBackToGlobal(t *testing.T) {
	deps, orch, state, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone, Project: "p"})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "GLOB") {
		t.Errorf("output missing global change: %s", out.String())
	}
	if !strings.Contains(out.String(), "global") {
		t.Errorf("output missing source=global: %s", out.String())
	}
}

func TestStatusCommandJSONFlagEmitsObject(t *testing.T) {
	deps, orch, _, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "ARG", Status: domain.ChangeStatusRunning, Project: "p"})

	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "ARG", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if got["change_id"] != "ARG" {
		t.Errorf("change_id = %v", got["change_id"])
	}
	if got["status"] != "running" {
		t.Errorf("status = %v", got["status"])
	}
}

func TestStatusCommandJSONEmptyEmitsNull(t *testing.T) {
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimSpace(out.String())
	if trimmed != "null" {
		t.Errorf("empty --json output = %q, want null", trimmed)
	}
}

func TestStatusCommandPropagatesExitCode3OnStaleArg(t *testing.T) {
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"status", "MISSING"})

	err := c.Execute()
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}
