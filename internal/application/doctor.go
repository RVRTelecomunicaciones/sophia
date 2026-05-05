package application

import (
	"context"
	"errors"
	"strings"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

type CheckLevel string

const (
	LevelOK   CheckLevel = "ok"
	LevelInfo CheckLevel = "info"
	LevelWarn CheckLevel = "warn"
	LevelFail CheckLevel = "fail"
)

type Check struct {
	ID     string
	Title  string
	Level  CheckLevel
	Detail string
}

type DiagnosticsSummary struct {
	OK   int
	Info int
	Warn int
	Fail int
}

type DiagnosticsReport struct {
	Checks  []Check
	Summary DiagnosticsSummary
}

var ErrPathInvalid = errors.New("xdg path invalid")

type XDGValidator func(path string) error

type DoctorService struct {
	compose outbound.ComposeRunner
	git     outbound.GitInspector
	xdg     XDGValidator
}

func NewDoctorService(compose outbound.ComposeRunner, git outbound.GitInspector, xdg XDGValidator) *DoctorService {
	return &DoctorService{compose: compose, git: git, xdg: xdg}
}

func (d *DoctorService) Run(ctx context.Context) DiagnosticsReport {
	checks := []Check{
		d.checkDocker(ctx),
		d.checkCompose(ctx),
		d.checkGit(ctx),
		d.checkXDG(),
	}
	summary := DiagnosticsSummary{}
	for _, c := range checks {
		switch c.Level {
		case LevelOK:
			summary.OK++
		case LevelInfo:
			summary.Info++
		case LevelWarn:
			summary.Warn++
		case LevelFail:
			summary.Fail++
		}
	}
	return DiagnosticsReport{Checks: checks, Summary: summary}
}

func (d *DoctorService) checkDocker(ctx context.Context) Check {
	v, err := d.compose.Version(ctx)
	if err != nil {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: "docker not available"}
	}
	return Check{ID: "docker", Title: "Docker daemon", Level: LevelOK, Detail: "available"}
}

func (d *DoctorService) checkCompose(ctx context.Context) Check {
	v, err := d.compose.Version(ctx)
	if err != nil || v == "" {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "compose not available"}
	}
	if !isComposeV2(v) {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "v2 required, got: " + v}
	}
	return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkGit(ctx context.Context) Check {
	v, err := d.git.Version(ctx)
	if err != nil {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: "git not available"}
	}
	return Check{ID: "git", Title: "Git", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkXDG() Check {
	for _, p := range []string{"configRoot", "stateRoot", "dataRoot"} {
		if err := d.xdg(p); err != nil {
			return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: p + ": " + err.Error()}
		}
	}
	return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelOK, Detail: "all paths valid"}
}

func isComposeV2(version string) bool {
	low := strings.ToLower(version)
	_, suffix, found := strings.Cut(low, "compose version")
	if !found {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(suffix), "v2")
}
