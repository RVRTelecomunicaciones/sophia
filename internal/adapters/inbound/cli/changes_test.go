package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newChangesDeps(t *testing.T) (cli.Deps, *fakes.FakeOrchestrator) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "default-proj", BaseRef: "main", ArtifactStore: domain.ArtifactStoreMemoryEngine,
	})
	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})
	lister := application.NewLister(application.ListerDeps{Orch: orch})
	return cli.Deps{Resolver: resolver, Lister: lister}, orch
}

func TestChangesCommandPrintsTableByDefault(t *testing.T) {
	deps, orch := newChangesDeps(t)
	orch.SeedChange(&domain.Change{
		ID: "01H1", Status: domain.ChangeStatusDone, Project: "default-proj", BaseRef: "main",
	})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"changes"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "01H1") {
		t.Errorf("output missing change ID: %q", got)
	}
	if !strings.Contains(got, "done") {
		t.Errorf("output missing status: %q", got)
	}
	if !strings.Contains(got, "ID") || !strings.Contains(got, "STATUS") {
		t.Errorf("output missing header: %q", got)
	}
}

func TestChangesCommandJSONFlagEmitsArray(t *testing.T) {
	deps, orch := newChangesDeps(t)
	orch.SeedChange(&domain.Change{
		ID: "01H1", Status: domain.ChangeStatusRunning, Project: "default-proj",
	})
	orch.SeedChange(&domain.Change{
		ID: "01H2", Status: domain.ChangeStatusDone, Project: "default-proj",
	})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"changes", "--json"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	var arr []map[string]any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("output not valid JSON array: %v\n%s", err, out.String())
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
	for _, item := range arr {
		if _, ok := item["change_id"]; !ok {
			t.Errorf("item missing change_id: %+v", item)
		}
	}
}

func TestChangesCommandDefaultLimitIs10(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenLimit int
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenLimit = f.Limit }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenLimit != 10 {
		t.Errorf("default --limit forwarded as %d, want 10", seenLimit)
	}
}

func TestChangesCommandLimitFlagOverrides(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenLimit int
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenLimit = f.Limit }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--limit", "5"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenLimit != 5 {
		t.Errorf("--limit=5 forwarded as %d", seenLimit)
	}
}

func TestChangesCommandStatusFilterPassesThrough(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenStatus string
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenStatus = f.Status }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--status", "done"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenStatus != "done" {
		t.Errorf("--status forwarded as %q, want done", seenStatus)
	}
}

func TestChangesCommandProjectDefaultFromConfig(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "default-proj" {
		t.Errorf("default project = %q, want default-proj", seenProject)
	}
}

func TestChangesCommandProjectFlagOverrides(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--project", "other"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "other" {
		t.Errorf("--project = %q, want other", seenProject)
	}
}

func TestChangesCommandEmptyProjectFlagMeansNoFilter(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound.ListChangesFilter) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--project", ""})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "" {
		t.Errorf("--project=\"\" should disable project filter, got %q", seenProject)
	}
}
