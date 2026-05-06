package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// StatusSource indicates where the resolved last_change_id came from.
type StatusSource string

const (
	StatusSourceFlag    StatusSource = "flag"
	StatusSourceProject StatusSource = "project"
	StatusSourceGlobal  StatusSource = "global"
	StatusSourceNone    StatusSource = "none"
)

// StatusDeps groups the ports StatusReader needs.
type StatusDeps struct {
	Orch         outbound.OrchestratorClient
	State        outbound.StateStore
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// StatusOptions tunes StatusReader.
type StatusOptions struct {
	// FetchTimeout caps the GetChange call. Default 10s.
	FetchTimeout time.Duration
}

// ResolveInput controls Resolve.
type ResolveInput struct {
	ChangeID domain.ChangeID
}

// StatusReport is the shape returned by Resolve.
type StatusReport struct {
	IsEmpty bool
	Source  StatusSource
	Change  *domain.Change
}

// StatusReader implements `sophia status`.
type StatusReader struct {
	deps StatusDeps
	opts StatusOptions
}

// NewStatusReader constructs a StatusReader. Pass StatusOptions{} for defaults.
func NewStatusReader(d StatusDeps, opts StatusOptions) *StatusReader {
	if opts.FetchTimeout <= 0 {
		opts.FetchTimeout = 10 * time.Second
	}
	return &StatusReader{deps: d, opts: opts}
}

// Resolve walks: arg → project-scoped → global → empty. When a source is
// found, fetches the snapshot from the orchestrator and returns it on the
// StatusReport. Maps errors to spec §2.3 exit codes via *ExitError.
func (r *StatusReader) Resolve(ctx context.Context, in ResolveInput) (StatusReport, error) {
	id, src, err := r.locate(ctx, in)
	if err != nil {
		return StatusReport{}, err
	}
	if id.IsZero() {
		return StatusReport{IsEmpty: true, Source: StatusSourceNone}, nil
	}
	snap, err := r.fetch(ctx, id)
	if err != nil {
		return StatusReport{}, err
	}
	return StatusReport{Source: src, Change: snap}, nil
}

// locate runs the resolution order: arg → project-scoped → global. A
// malformed .sophia.yaml (ErrInvalidYAML) is fatal — `status` must not hide
// config corruption. Unexpected tryProject failures (state-store outage,
// etc.) also surface as exit 3 rather than silently falling through.
func (r *StatusReader) locate(ctx context.Context, in ResolveInput) (domain.ChangeID, StatusSource, error) {
	if !in.ChangeID.IsZero() {
		return in.ChangeID, StatusSourceFlag, nil
	}

	id, src, err := r.tryProject(ctx)
	switch {
	case err == nil && !id.IsZero():
		return id, src, nil
	case err == nil:
		// Project config parses cleanly but no scoped last_change_id yet —
		// fall through to global.
	case errors.Is(err, domain.ErrInvalidYAML):
		return "", "", &ExitError{Code: 3, Err: fmt.Errorf("project config invalid: %w", err)}
	case errors.Is(err, domain.ErrConfigMissing):
		// No .sophia.yaml or no git repo — fall through to global.
	default:
		// Unexpected error (state-store outage, etc.) — surface, don't hide.
		return "", "", &ExitError{Code: 3, Err: fmt.Errorf("project resolution: %w", err)}
	}

	gid, err := r.deps.State.GetGlobalLast(ctx)
	if err != nil {
		return "", "", fmt.Errorf("global last: %w", err)
	}
	if !gid.IsZero() {
		return gid, StatusSourceGlobal, nil
	}
	return "", StatusSourceNone, nil
}

// fetch GETs the snapshot from the orchestrator with the configured timeout
// and maps errors to exit codes: parent ctx cancel and internal FetchTimeout
// produce exit 4; ChangeNotFound / Unreachable produce exit 3.
func (r *StatusReader) fetch(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if r.deps.Orch == nil {
		return nil, &ExitError{Code: 3, Err: errors.New("status: orchestrator client not wired")}
	}
	fctx, cancel := context.WithTimeout(ctx, r.opts.FetchTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(fctx, id)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, &ExitError{Code: 4, Err: err}
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(fctx.Err(), context.DeadlineExceeded) {
			return nil, &ExitError{Code: 4, Err: fmt.Errorf("get change timed out after %s: %w", r.opts.FetchTimeout, err)}
		}
		return nil, &ExitError{Code: 3, Err: fmt.Errorf("get change: %w", err)}
	}
	return snap, nil
}

// tryProject surfaces ProjectConfigStore.Read errors verbatim so locate can
// distinguish ErrConfigMissing (fall through) from ErrInvalidYAML (fatal).
// "Outside a git repo" is normalized to ErrConfigMissing so locate only has
// to dispatch on the two domain sentinels.
func (r *StatusReader) tryProject(ctx context.Context) (domain.ChangeID, StatusSource, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Not in a git repo (or git not installed): treat as "no project
		// context" so locate falls through to global. The `doctor` command
		// is responsible for surfacing a missing git binary separately.
		return "", "", domain.ErrConfigMissing
	}
	cfgPath := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, cfgPath)
	if err != nil {
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
