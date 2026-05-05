package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ComposeProjectName is the fixed compose project name used by sophia
// regardless of CWD (spec §3.6 hard invariant).
const ComposeProjectName = "sophia"

// MaterializeFunc lets the application layer invoke the composeexec
// materializer without importing the adapter package directly.
//
// Returned tuple: (path, wrote, err).
type MaterializeFunc func(dataRoot string, embedded []byte, reset bool) (string, bool, error)

// ProvisionerDeps groups the ports the Provisioner needs.
type ProvisionerDeps struct {
	Compose     outbound.ComposeRunner
	Paths       outbound.PathResolver
	Materialize MaterializeFunc
	Embedded    []byte
}

// UpInput controls Up.
type UpInput struct {
	Reset bool // map of `sophia start --reset-compose`
}

// UpResult reports what Up did.
type UpResult struct {
	ComposePath string
	Wrote       bool
}

// Provisioner orchestrates `sophia start` / `sophia stop`.
type Provisioner struct {
	deps ProvisionerDeps
}

// NewProvisioner constructs a Provisioner.
func NewProvisioner(d ProvisionerDeps) *Provisioner { return &Provisioner{deps: d} }

// Up resolves XDG paths, ensures dirs, materializes compose, and runs
// `docker compose -p sophia -f <path> up -d`.
func (p *Provisioner) Up(ctx context.Context, in UpInput) (UpResult, error) {
	paths, err := p.deps.Paths.Resolve()
	if err != nil {
		return UpResult{}, fmt.Errorf("resolve paths: %w", err)
	}
	if err := p.deps.Paths.EnsureDirs(paths); err != nil {
		return UpResult{}, fmt.Errorf("ensure dirs: %w", err)
	}
	if p.deps.Materialize == nil {
		return UpResult{}, errors.New("materialize func not wired")
	}
	path, wrote, err := p.deps.Materialize(paths.DataRoot, p.deps.Embedded, in.Reset)
	if err != nil {
		return UpResult{}, fmt.Errorf("materialize: %w", err)
	}
	if err := p.deps.Compose.Up(ctx, path, ComposeProjectName); err != nil {
		return UpResult{ComposePath: path, Wrote: wrote}, fmt.Errorf("compose up: %w", err)
	}
	return UpResult{ComposePath: path, Wrote: wrote}, nil
}

// Down resolves XDG paths and runs `docker compose -p sophia -f <path> down`.
func (p *Provisioner) Down(ctx context.Context) error {
	paths, err := p.deps.Paths.Resolve()
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	composePath := paths.DataRoot + "/compose/compose.yaml"
	if err := p.deps.Compose.Down(ctx, composePath, ComposeProjectName); err != nil {
		return fmt.Errorf("compose down: %w", err)
	}
	return nil
}
