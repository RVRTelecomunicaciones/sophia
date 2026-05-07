package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func newStartDeps() (cli.Deps, *fakes.FakeComposeRunner) {
	compose := fakes.NewFakeComposeRunner()
	mat := func(_ string, _ []byte, _ bool) (string, bool, error) {
		return "/x/compose.yaml", true, nil
	}
	return cli.Deps{
		Provisioner: application.NewProvisioner(application.ProvisionerDeps{
			Compose:     compose,
			Paths:       fakes.NewFakePathResolver(),
			Materialize: mat,
			Embedded:    []byte("services: {}\n"),
		}),
	}, compose
}

func TestStartCommandSucceeds(t *testing.T) {
	deps, compose := newStartDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"start"})
	if err := c.Execute(); err != nil {
		t.Fatalf("start err: %v", err)
	}
	if compose.UpCalls != 1 {
		t.Errorf("UpCalls = %d", compose.UpCalls)
	}
	if !strings.Contains(out.String(), "started") {
		t.Errorf("output missing 'started': %q", out.String())
	}
}

func TestStartResetComposeFlag(t *testing.T) {
	deps, _ := newStartDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"start", "--reset-compose"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
