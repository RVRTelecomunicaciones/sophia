package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// StatusSource indicates where the resolved last_change_id came from.
type StatusSource string

// Status sources.
const (
	StatusSourceProject StatusSource = "project"
	StatusSourceGlobal  StatusSource = "global"
)

// StatusDeps groups the ports StatusReader needs.
type StatusDeps struct {
	State        outbound.StateStore
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// StatusOutput is the shape returned by Resolve.
type StatusOutput struct {
	IsEmpty  bool
	ChangeID domain.ChangeID
	Source   StatusSource
}

// StatusReader is the M3 placeholder `sophia status` use case. It resolves
// the local last_change_id only — no orchestrator call. Real status (with
// HTTP fetch) ships in M8.
type StatusReader struct {
	deps StatusDeps
}

// NewStatusReader constructs a StatusReader.
func NewStatusReader(d StatusDeps) *StatusReader { return &StatusReader{deps: d} }

// Resolve walks: project-scoped → global → empty.
func (r *StatusReader) Resolve(ctx context.Context) (StatusOutput, error) {
	if id, src, err := r.tryProject(ctx); err == nil && !id.IsZero() {
		return StatusOutput{ChangeID: id, Source: src}, nil
	}
	id, err := r.deps.State.GetGlobalLast(ctx)
	if err != nil {
		return StatusOutput{}, fmt.Errorf("global: %w", err)
	}
	if !id.IsZero() {
		return StatusOutput{ChangeID: id, Source: StatusSourceGlobal}, nil
	}
	return StatusOutput{IsEmpty: true}, nil
}

func (r *StatusReader) tryProject(ctx context.Context) (domain.ChangeID, StatusSource, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return "", "", err
	}
	cfgPath := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, cfgPath)
	if err != nil {
		if errors.Is(err, domain.ErrConfigMissing) {
			return "", "", err
		}
		return "", "", err
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(cfg.Project, root, remote)
	id, err := r.deps.State.GetLast(ctx, fp)
	if err != nil {
		return "", "", err
	}
	return id, StatusSourceProject, nil
}
