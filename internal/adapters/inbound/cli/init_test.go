package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newInitDeps() (cli.Deps, *fakes.FakeProjectConfigStore) {
	store := fakes.NewFakeProjectConfigStore()
	return cli.Deps{
		Initializer: application.NewInitializer(application.InitializerDeps{
			Git:          fakes.NewFakeGitInspector(),
			ProjectStore: store,
		}),
	}, store
}

func TestInitCommandRequiresProjectFlag(t *testing.T) {
	deps, _ := newInitDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"init"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when --project missing")
	}
}

func TestInitCommandWritesConfig(t *testing.T) {
	deps, store := newInitDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"init", "--project", "ms-cotizacion"})
	if err := c.Execute(); err != nil {
		t.Fatalf("init err: %v", err)
	}
	if !strings.Contains(out.String(), "ms-cotizacion") {
		t.Errorf("output missing project name: %q", out.String())
	}
	got, err := store.Read(context.Background(), "/repo/.sophia.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Project != "ms-cotizacion" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
}

func TestInitForceOverwrite(t *testing.T) {
	deps, store := newInitDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"init", "--project", "old"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	c2 := cli.NewRoot(deps)
	c2.SetOut(&bytes.Buffer{})
	c2.SetArgs([]string{"init", "--project", "new"})
	if err := c2.Execute(); err == nil {
		t.Error("expected abort without --force")
	}

	c3 := cli.NewRoot(deps)
	c3.SetOut(&bytes.Buffer{})
	c3.SetArgs([]string{"init", "--project", "new", "--force"})
	if err := c3.Execute(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(context.Background(), "/repo/.sophia.yaml")
	if got.Project != "new" {
		t.Errorf("Project = %q after --force", got.Project)
	}
}
