package application_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newDoctor() (*application.DoctorService, *fakes.FakeComposeRunner, *fakes.FakeGitInspector) {
	compose := fakes.NewFakeComposeRunner()
	git := fakes.NewFakeGitInspector()
	xdg := func(_ string) error { return nil }
	d := application.NewDoctorService(compose, git, xdg)
	return d, compose, git
}

func TestDoctorAllGreen(t *testing.T) {
	d, _, _ := newDoctor()
	report := d.Run(context.Background())
	if report.Summary.Fail != 0 {
		t.Errorf("expected 0 fails, got %+v\n%v", report.Summary, report.Checks)
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}

func TestDoctorReportsDockerComposeMissing(t *testing.T) {
	d, compose, _ := newDoctor()
	compose.ComposeVersion = ""
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected a fail when compose version is empty")
	}
}

func TestDoctorReportsComposeV1AsFail(t *testing.T) {
	d, compose, _ := newDoctor()
	compose.ComposeVersion = "docker-compose version 1.29.2"
	report := d.Run(context.Background())
	found := false
	for _, c := range report.Checks {
		if c.ID == "compose" && c.Level == application.LevelFail {
			found = true
		}
	}
	if !found {
		t.Errorf("expected compose v1 to fail; got %+v", report.Checks)
	}
}

func TestDoctorReportsGitMissing(t *testing.T) {
	d, _, git := newDoctor()
	git.GitVersion = ""
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected git fail when version empty")
	}
}

func TestDoctorXDGPathError(t *testing.T) {
	compose := fakes.NewFakeComposeRunner()
	git := fakes.NewFakeGitInspector()
	xdg := func(p string) error { return application.ErrPathInvalid }
	d := application.NewDoctorService(compose, git, xdg)
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected XDG path fail")
	}
}
