package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

type FakeOrchestrator struct {
	mu                  sync.Mutex
	HealthzErr          error
	CreateErr           error
	GetBlockUntilCancel bool
	TickHook            func(*domain.Change)
	changes             map[domain.ChangeID]*domain.Change
	nextID              int
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
	if f.CreateErr != nil {
		return nil, f.CreateErr
	}
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

func (f *FakeOrchestrator) GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if f.GetBlockUntilCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[id]
	if !ok {
		return nil, domain.ErrChangeNotFound
	}
	if f.TickHook != nil {
		// TickHook fires inside the mutex by design: tests use it to mutate the
		// stored change so the next call observes the new state. Hooks must not
		// call back into the orchestrator (would deadlock) or block on shared
		// state — keep them pure and fast.
		f.TickHook(c)
	}
	out := *c
	if c.Phases != nil {
		out.Phases = append([]domain.Phase{}, c.Phases...)
	}
	return &out, nil
}

// SetTerminal mutates a stored change to the given terminal status. Used
// by runner tests to simulate "after the stream ended, GetChange returns
// terminal".
func (f *FakeOrchestrator) SetTerminal(id domain.ChangeID, st domain.ChangeStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.changes[id]; ok {
		c.Status = st
	}
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
