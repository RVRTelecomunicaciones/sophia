package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

type provDeps struct {
	Compose *fakes.FakeComposeRunner
	Paths   *fakes.FakePathResolver
}

func newProvisioner() (*application.Provisioner, *provDeps) {
	d := &provDeps{
		Compose: fakes.NewFakeComposeRunner(),
		Paths:   fakes.NewFakePathResolver(),
	}
	d.Paths.Paths = domain.XDGPaths{DataRoot: "/x/data"}
	mat := func(dataRoot string, _ []byte, _ bool) (string, bool, error) {
		return dataRoot + "/compose/compose.yaml", true, nil
	}
	p := application.NewProvisioner(application.ProvisionerDeps{
		Compose:     d.Compose,
		Paths:       d.Paths,
		Materialize: mat,
		Embedded:    []byte("services: {}\n"),
	})
	return p, d
}

func TestProvisionerUpEnsuresPathsAndCallsCompose(t *testing.T) {
	p, deps := newProvisioner()
	res, err := p.Up(context.Background(), application.UpInput{Reset: false})
	if err != nil {
		t.Fatal(err)
	}
	if deps.Paths.EnsureDirsCalls != 1 {
		t.Errorf("EnsureDirsCalls = %d", deps.Paths.EnsureDirsCalls)
	}
	if deps.Compose.UpCalls != 1 {
		t.Errorf("Compose.UpCalls = %d", deps.Compose.UpCalls)
	}
	if res.ComposePath != "/x/data/compose/compose.yaml" {
		t.Errorf("ComposePath = %q", res.ComposePath)
	}
	if !res.Wrote {
		t.Error("expected res.Wrote=true")
	}
}

func TestProvisionerUpProjectNameIsSophia(t *testing.T) {
	p, deps := newProvisioner()
	if _, err := p.Up(context.Background(), application.UpInput{}); err != nil {
		t.Fatal(err)
	}
	if got := deps.Compose.LastProjectName; got != "sophia" {
		t.Errorf("project name = %q, want sophia", got)
	}
}

func TestProvisionerUpFailsWhenComposeFails(t *testing.T) {
	p, deps := newProvisioner()
	deps.Compose.UpErr = errors.New("daemon down")
	if _, err := p.Up(context.Background(), application.UpInput{}); err == nil {
		t.Error("expected error")
	}
}

func TestProvisionerDownInvokesCompose(t *testing.T) {
	p, deps := newProvisioner()
	if err := p.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if deps.Compose.DownCalls != 1 {
		t.Errorf("DownCalls = %d", deps.Compose.DownCalls)
	}
	if got := deps.Compose.LastProjectName; got != "sophia" {
		t.Errorf("project name = %q", got)
	}
}
