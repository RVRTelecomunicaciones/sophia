package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// InitializerDeps groups the ports the Initializer needs.
type InitializerDeps struct {
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// InitInput controls Run.
type InitInput struct {
	Project       string
	BaseRef       string                   // empty ⇒ "main"
	ArtifactStore domain.ArtifactStoreMode // empty ⇒ ArtifactStoreMemoryEngine
	Force         bool
}

// InitResult reports what Run did.
type InitResult struct {
	Path     string
	RepoRoot string
}

// Initializer orchestrates `sophia init`.
type Initializer struct {
	deps InitializerDeps
}

// NewInitializer constructs an Initializer.
func NewInitializer(d InitializerDeps) *Initializer { return &Initializer{deps: d} }

// Run resolves the repo root and writes .sophia.yaml. Fails with
// domain.ErrNotARepo when run outside a git repository. Refuses to
// overwrite an existing config unless Force=true.
func (i *Initializer) Run(ctx context.Context, in InitInput) (InitResult, error) {
	if in.Project == "" {
		return InitResult{}, errors.New("init: --project is required")
	}
	root, err := i.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return InitResult{}, fmt.Errorf("%w: %v", domain.ErrNotARepo, err)
	}
	path := filepath.Join(root, ".sophia.yaml")

	if !in.Force {
		_, err := i.deps.ProjectStore.Read(ctx, path)
		if err == nil {
			return InitResult{}, fmt.Errorf("init: %s already exists; pass --force to overwrite", path)
		}
		if !errors.Is(err, domain.ErrConfigMissing) {
			return InitResult{}, fmt.Errorf("init: %s exists but is unreadable (%v); pass --force to repair", path, err)
		}
	}

	baseRef := in.BaseRef
	if baseRef == "" {
		baseRef = "main"
	}
	store := in.ArtifactStore
	if store == "" {
		store = domain.ArtifactStoreMemoryEngine
	}

	cfg := &domain.ProjectConfig{
		Version:       1,
		Project:       in.Project,
		BaseRef:       baseRef,
		ArtifactStore: store,
	}
	if err := i.deps.ProjectStore.Write(ctx, path, cfg); err != nil {
		return InitResult{}, fmt.Errorf("write: %w", err)
	}
	return InitResult{Path: path, RepoRoot: root}, nil
}
