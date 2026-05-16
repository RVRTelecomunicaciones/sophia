package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// Defaults applied when no other layer provides a value.
const (
	DefaultOrchestratorURL = "http://localhost:9080"
	DefaultBaseRef         = "main"
	DefaultTimeoutSeconds  = 30

	EnvOrchestratorURL = "SOPHIA_ORCHESTRATOR_URL"
	EnvProject         = "SOPHIA_PROJECT"
	EnvBaseRef         = "SOPHIA_BASE_REF"
)

// ResolverFlags carries CLI flag values. Empty strings/zero ints mean "not set".
type ResolverFlags struct {
	OrchestratorURL string
	Project         string
	BaseRef         string
	ArtifactStore   string
	TimeoutSeconds  int
}

// ResolverInput controls Resolve.
type ResolverInput struct {
	Flags          ResolverFlags
	Env            map[string]string
	UserConfigPath string // empty ⇒ skip user config
	RequireProject bool   // true ⇒ fail with ErrConfigMissing when no project resolved
}

// ResolvedConfig is the layered result.
type ResolvedConfig struct {
	OrchestratorURL string
	Project         string
	BaseRef         string
	ArtifactStore   domain.ArtifactStoreMode
	TimeoutSeconds  int
}

// ConfigResolverDeps groups the ports the resolver needs.
type ConfigResolverDeps struct {
	ProjectStore outbound.ProjectConfigStore
	UserStore    outbound.UserConfigStore
	Git          outbound.GitInspector
}

// ConfigResolver layers configuration per spec §3.4.
type ConfigResolver struct {
	deps ConfigResolverDeps
}

// NewConfigResolver constructs a ConfigResolver.
func NewConfigResolver(d ConfigResolverDeps) *ConfigResolver { return &ConfigResolver{deps: d} }

// Resolve applies the precedence flags > env > project > user > defaults.
func (r *ConfigResolver) Resolve(ctx context.Context, in ResolverInput) (ResolvedConfig, error) {
	out := ResolvedConfig{
		OrchestratorURL: DefaultOrchestratorURL,
		BaseRef:         DefaultBaseRef,
		ArtifactStore:   domain.ArtifactStoreMemoryEngine,
		TimeoutSeconds:  DefaultTimeoutSeconds,
	}

	// User config (lowest non-default).
	if in.UserConfigPath != "" {
		uc, err := r.deps.UserStore.Read(ctx, in.UserConfigPath)
		if err != nil && !errors.Is(err, domain.ErrConfigMissing) {
			return out, fmt.Errorf("user config: %w", err)
		}
		if uc != nil {
			if uc.OrchestratorURL != "" {
				out.OrchestratorURL = uc.OrchestratorURL
			}
			if uc.TimeoutSeconds > 0 {
				out.TimeoutSeconds = uc.TimeoutSeconds
			}
		}
	}

	// Project config (.sophia.yaml).
	pc, pcErr := r.readProjectConfig(ctx)
	if pcErr != nil && !errors.Is(pcErr, domain.ErrConfigMissing) && !errors.Is(pcErr, domain.ErrNotARepo) {
		return out, fmt.Errorf("project config: %w", pcErr)
	}
	if pc != nil {
		if pc.Project != "" {
			out.Project = pc.Project
		}
		if pc.BaseRef != "" {
			out.BaseRef = pc.BaseRef
		}
		if pc.ArtifactStore != "" {
			out.ArtifactStore = pc.ArtifactStore
		}
	}

	// Env vars.
	if v := in.Env[EnvOrchestratorURL]; v != "" {
		out.OrchestratorURL = v
	}
	if v := in.Env[EnvProject]; v != "" {
		out.Project = v
	}
	if v := in.Env[EnvBaseRef]; v != "" {
		out.BaseRef = v
	}

	// Flags (highest).
	if in.Flags.OrchestratorURL != "" {
		out.OrchestratorURL = in.Flags.OrchestratorURL
	}
	if in.Flags.Project != "" {
		out.Project = in.Flags.Project
	}
	if in.Flags.BaseRef != "" {
		out.BaseRef = in.Flags.BaseRef
	}
	if in.Flags.ArtifactStore != "" {
		mode := domain.ArtifactStoreMode(in.Flags.ArtifactStore)
		if !mode.IsValid() {
			return out, fmt.Errorf("%w: artifact-store %q is not one of memory-engine|openspec|hybrid|none",
				domain.ErrConfigMissing, in.Flags.ArtifactStore)
		}
		out.ArtifactStore = mode
	}
	if in.Flags.TimeoutSeconds > 0 {
		out.TimeoutSeconds = in.Flags.TimeoutSeconds
	}

	if in.RequireProject && out.Project == "" {
		return out, fmt.Errorf("%w: project not set (need .sophia.yaml or --project / SOPHIA_PROJECT)", domain.ErrConfigMissing)
	}
	return out, nil
}

func (r *ConfigResolver) readProjectConfig(ctx context.Context) (*domain.ProjectConfig, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNotARepo, err)
	}
	path := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
