package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

type FakeOrchestrator struct {
	mu         sync.Mutex
	HealthzErr error
	changes    map[domain.ChangeID]*domain.Change
	nextID     int
}

func NewFakeOrchestrator() *FakeOrchestrator {
	return &FakeOrchestrator{changes: map[domain.ChangeID]*domain.Change{}}
}

func (f *FakeOrchestrator) SeedChange(c *domain.Change) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes[c.ID] = c
}

func (f *FakeOrchestrator) Healthz(_ context.Context) error {
	return f.HealthzErr
}

func (f *FakeOrchestrator) CreateChange(_ context.Context, in outbound.CreateChangeInput) (*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := domain.ChangeID(fmt.Sprintf("fake-change-%d", f.nextID))
	c := &domain.Change{
		ID:                id,
		Name:              in.Name,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: in.ArtifactStoreMode,
		Status:            domain.ChangeStatusPending,
	}
	f.changes[id] = c
	return c, nil
}

func (f *FakeOrchestrator) GetChange(_ context.Context, id domain.ChangeID) (*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[id]
	if !ok {
		return nil, domain.ErrChangeNotFound
	}
	return c, nil
}

func (f *FakeOrchestrator) ListChanges(_ context.Context, filter outbound.ListChangesFilter) ([]*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.Change, 0, len(f.changes))
	for _, c := range f.changes {
		if filter.Project != "" && c.Project != filter.Project {
			continue
		}
		if filter.Status != "" && string(c.Status) != filter.Status {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
