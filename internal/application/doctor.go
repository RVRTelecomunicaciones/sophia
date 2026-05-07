// Package application contains the use cases of the CLI. Use cases consume
// outbound ports and never import adapters or third-party UI libraries.
package application

import (
	"context"
	"errors"
	"strings"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// CheckLevel categorizes a doctor check outcome.
type CheckLevel string

// Doctor check levels.
const (
	LevelOK   CheckLevel = "ok"
	LevelInfo CheckLevel = "info"
	LevelWarn CheckLevel = "warn"
	LevelFail CheckLevel = "fail"
)

// Check is one row in the doctor report.
type Check struct {
	ID     string
	Title  string
	Level  CheckLevel
	Detail string
}

// DiagnosticsSummary aggregates check counts.
type DiagnosticsSummary struct {
	OK   int
	Info int
	Warn int
	Fail int
}

// DiagnosticsReport is the output of doctor.
type DiagnosticsReport struct {
	Checks  []Check
	Summary DiagnosticsSummary
}

// ErrPathInvalid is the legacy XDGValidator sentinel kept for compatibility
// with adapters that still expose it. New code uses PathResolver.
var ErrPathInvalid = errors.New("xdg path invalid")

// DoctorDeps groups the outbound ports the doctor service uses. Optional
// dependencies (e.g. Orch, SSE, Paths) may be nil — the corresponding check
// is then reported as info ("not configured") rather than fail.
type DoctorDeps struct {
	Compose outbound.ComposeRunner
	Git     outbound.GitInspector
	Paths   outbound.PathResolver
	Orch    outbound.OrchestratorClient
	SSE     outbound.SSEProber
}

// DoctorService orchestrates the M2 subset of doctor checks: docker, compose,
// git, XDG paths, orchestrator, SSE. Later milestones add: repo, .sophia.yaml,
// worktree.
type DoctorService struct {
	deps DoctorDeps
}

// NewDoctorService constructs a DoctorService.
func NewDoctorService(d DoctorDeps) *DoctorService { return &DoctorService{deps: d} }

// Run executes all checks and returns the report.
func (d *DoctorService) Run(ctx context.Context) DiagnosticsReport {
	checks := []Check{
		d.checkDocker(ctx),
		d.checkCompose(ctx),
		d.checkGit(ctx),
		d.checkPaths(),
		d.checkOrchestrator(ctx),
		d.checkSSE(ctx),
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
	v, err := d.deps.Compose.Version(ctx)
	if err != nil {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: "docker not available"}
	}
	return Check{ID: "docker", Title: "Docker daemon", Level: LevelOK, Detail: "available"}
}

func (d *DoctorService) checkCompose(ctx context.Context) Check {
	v, err := d.deps.Compose.Version(ctx)
	if err != nil || v == "" {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "compose not available"}
	}
	if !isComposeV2(v) {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "v2 required, got: " + v}
	}
	return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkGit(ctx context.Context) Check {
	v, err := d.deps.Git.Version(ctx)
	if err != nil {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: "git not available"}
	}
	return Check{ID: "git", Title: "Git", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkPaths() Check {
	if d.deps.Paths == nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelInfo, Detail: "no resolver wired"}
	}
	p, err := d.deps.Paths.Resolve()
	if err != nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: err.Error()}
	}
	if err := d.deps.Paths.ValidateDirs(p); err != nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: err.Error()}
	}
	return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelOK, Detail: p.StateRoot}
}

func (d *DoctorService) checkOrchestrator(ctx context.Context) Check {
	if d.deps.Orch == nil {
		return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelInfo, Detail: "no client wired"}
	}
	if err := d.deps.Orch.Healthz(ctx); err != nil {
		return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelFail, Detail: err.Error()}
	}
	return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelOK, Detail: "200 OK"}
}

func (d *DoctorService) checkSSE(ctx context.Context) Check {
	if d.deps.SSE == nil {
		return Check{ID: "sse", Title: "SSE handshake", Level: LevelInfo, Detail: "no prober wired"}
	}
	if err := d.deps.SSE.Probe(ctx); err != nil {
		return Check{ID: "sse", Title: "SSE handshake", Level: LevelWarn, Detail: err.Error()}
	}
	return Check{ID: "sse", Title: "SSE handshake", Level: LevelOK, Detail: "event-stream OK"}
}

func isComposeV2(version string) bool {
	low := strings.ToLower(version)
	_, suffix, found := strings.Cut(low, "compose version")
	if !found {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(suffix), "v2")
}
