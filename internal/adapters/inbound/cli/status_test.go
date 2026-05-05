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

func newStatusDeps() (cli.Deps, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	return cli.Deps{
		StatusReader: application.NewStatusReader(application.StatusDeps{
			State:        state,
			Git:          fakes.NewFakeGitInspector(),
			ProjectStore: fakes.NewFakeProjectConfigStore(),
		}),
	}, state
}

func TestStatusEmptyMessage(t *testing.T) {
	deps, _ := newStatusDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No local change found") {
		t.Errorf("output missing empty message: %q", out.String())
	}
}

func TestStatusPrintsChangeID(t *testing.T) {
	deps, state := newStatusDeps()
	_ = state.SetGlobalLast(context.Background(), "01HX-test")
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "01HX-test") {
		t.Errorf("output missing change id: %q", out.String())
	}
}
