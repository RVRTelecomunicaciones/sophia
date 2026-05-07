package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

type doctorM2Deps struct {
	Compose *fakes.FakeComposeRunner
	Git     *fakes.FakeGitInspector
	Paths   *fakes.FakePathResolver
	Orch    *fakes.FakeOrchestrator
}

func newDoctorM2(opts ...func(*doctorM2Deps)) (*application.DoctorService, *doctorM2Deps) { //nolint:unparam // opts variadic matches the functional-options pattern used across all fake constructors
	d := &doctorM2Deps{
		Compose: fakes.NewFakeComposeRunner(),
		Git:     fakes.NewFakeGitInspector(),
		Paths:   fakes.NewFakePathResolver(),
		Orch:    fakes.NewFakeOrchestrator(),
	}
	for _, o := range opts {
		o(d)
	}
	return application.NewDoctorService(application.DoctorDeps{
		Compose: d.Compose, Git: d.Git, Paths: d.Paths, Orch: d.Orch,
	}), d
}

func TestDoctorAllGreen(t *testing.T) {
	d, _ := newDoctorM2()
	r := d.Run(context.Background())
	if r.Summary.Fail != 0 {
		t.Errorf("expected 0 fails, got %+v\n%v", r.Summary, r.Checks)
	}
	if len(r.Checks) < 6 {
		t.Errorf("expected ≥6 checks, got %d", len(r.Checks))
	}
}

func TestDoctorReportsDockerComposeMissing(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Compose.ComposeVersion = ""
	r := d.Run(context.Background())
	if r.Summary.Fail == 0 {
		t.Error("expected fail when compose version is empty")
	}
}

func TestDoctorReportsComposeV1AsFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Compose.ComposeVersion = "docker-compose version 1.29.2"
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "compose" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected compose v1 fail; got %+v", r.Checks)
}

func TestDoctorReportsGitMissing(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Git.GitVersion = ""
	r := d.Run(context.Background())
	if r.Summary.Fail == 0 {
		t.Error("expected git fail")
	}
}

func TestDoctorPathsFailWhenInvalid(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Paths.ValidateErr = fakes.ErrFakeValidate
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "xdg_paths" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected xdg_paths fail; got %+v", r.Checks)
}

func TestDoctorOrchestratorFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Orch.HealthzErr = errors.New("refused")
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "orchestrator" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected orchestrator fail; got %+v", r.Checks)
}

// Phase 4 Task 4.7 / D-M10-07: SSE handshake is no longer probed
// pre-run; the doctor reports it as info ("deferred to first
// run/attach"). The legacy "fail-warn-on-probe-error" behavior is gone
// because the probe endpoint never existed on the orchestrator.
func TestDoctorSSEDeferredAsInfo(t *testing.T) {
	d, _ := newDoctorM2()
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "sse" {
			if c.Level != application.LevelInfo {
				t.Errorf("expected sse info, got %q", c.Level)
			}
			return
		}
	}
	t.Error("sse check missing")
}
