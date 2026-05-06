package application

import (
	"context"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ListerDeps groups the ports the Lister needs.
type ListerDeps struct {
	Orch outbound.OrchestratorClient
}

// ListInput controls List.
//
// Project resolution rules:
//
//   - Project != ""                              → forward as filter.
//   - Project == "" AND !IgnoreProjectDefault    → forward "" (caller didn't
//     pass a project; orchestrator decides — typically returns all).
//   - Project == "" AND IgnoreProjectDefault     → forward "" explicitly,
//     same wire shape but documents that the CLI layer DELIBERATELY chose
//     "all projects" (e.g. user typed --project="").
//
// The two empty-string cases produce identical wire calls; IgnoreProjectDefault
// is a narrative aid for the CLI's flag semantics, NOT a behavior switch in
// the Lister.
type ListInput struct {
	Project              string
	IgnoreProjectDefault bool
	Status               string
	Limit                int
	Offset               int
}

// Lister implements `sophia changes`.
type Lister struct {
	deps ListerDeps
}

// NewLister constructs a Lister.
func NewLister(d ListerDeps) *Lister { return &Lister{deps: d} }

// List queries the orchestrator and returns the matching Changes.
func (l *Lister) List(ctx context.Context, in ListInput) ([]*domain.Change, error) {
	if l.deps.Orch == nil {
		return nil, fmt.Errorf("lister: orchestrator client not wired")
	}
	filter := outbound.ListChangesFilter{
		Project: in.Project,
		Status:  in.Status,
		Limit:   in.Limit,
		Offset:  in.Offset,
	}
	out, err := l.deps.Orch.ListChanges(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	return out, nil
}
