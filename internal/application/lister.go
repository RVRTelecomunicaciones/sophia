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

// ListInput controls List. All four fields are forwarded verbatim to
// OrchestratorClient.ListChanges. Lister does NOT resolve project defaults,
// does NOT impose limits, does NOT translate empty strings into wildcards.
// The CLI layer (cli/changes.go) is responsible for any project-default
// resolution from .sophia.yaml before invoking List.
type ListInput struct {
	Project string
	Status  string
	Limit   int
	Offset  int
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
