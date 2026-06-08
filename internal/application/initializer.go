package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// InitializerDeps groups the ports the Initializer needs.
type InitializerDeps struct {
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
	// Prober detects the Graphify toolchain after .sophia.yaml is written.
	// Optional: when nil, graphify detection is skipped silently.
	Prober outbound.GraphifyProber
	// Logger is used for graphify probe log messages. When nil, slog.Default() is used.
	Logger *slog.Logger
}

// InitInput controls Run.
type InitInput struct {
	Project       string
	BaseRef       string                   // empty ⇒ "main"
	ArtifactStore domain.ArtifactStoreMode // empty ⇒ ArtifactStoreMemoryEngine
	Force         bool
	// AutoBootstrap mirrors the --auto-bootstrap-graphify CLI flag. When true
	// and Probe returns Available=false, Bootstrap is called once.
	// Default: false (degraded-first per V4.1 §7-ter.7).
	AutoBootstrap bool
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

	// Graphify toolchain probe — runs AFTER .sophia.yaml is written (degraded-first).
	// When Prober is nil the feature is simply absent; no error.
	if i.deps.Prober != nil {
		i.probeGraphify(ctx, in.AutoBootstrap)
	}

	return InitResult{Path: path, RepoRoot: root}, nil
}

// probeGraphify runs the Graphify probe and optionally bootstraps the toolchain.
// It NEVER returns an error — degraded mode is non-fatal per V4.1 §7-ter.7.
func (i *Initializer) probeGraphify(ctx context.Context, autoBootstrap bool) {
	log := i.deps.Logger
	if log == nil {
		log = slog.Default()
	}

	result, err := i.deps.Prober.Probe(ctx)
	if err != nil {
		log.Warn("graphify probe failed; INIT will run in degraded mode", "err", err)
		return
	}

	if result.Available {
		log.Info("graphify detected", "version", result.Version)
		return
	}

	// Graphify not available.
	log.Warn("graphify not detected; INIT will run in degraded mode",
		"missing_deps", result.MissingDeps)

	if autoBootstrap {
		if bErr := i.deps.Prober.Bootstrap(ctx); bErr != nil {
			log.Warn("graphify auto-bootstrap failed", "err", bErr)
		}
	}
}
