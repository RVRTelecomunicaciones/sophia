package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

type FakeOrchestrator struct {
	mu                  sync.Mutex
	HealthzErr          error
	CreateErr           error
	ListErr             error
	AbortErr            error
	ApproveErr          error
	RejectErr           error
	RunPhaseErr         error
	GetBlockUntilCancel bool
	TickHook            func(*domain.Change)
	OnListChanges       func(outbound.ListChangesFilter)
	OnGetChange         func(domain.ChangeID)
	OnAbort             func(domain.ChangeID, outbound.AbortChangeInput)
	OnApprove           func(string, outbound.ApprovalDecisionInput)
	OnReject            func(string, outbound.ApprovalDecisionInput)
	OnRunPhase          func(domain.ChangeID, string, outbound.RunPhaseInput)
	changes             map[domain.ChangeID]*domain.Change
	nextID              int
}

func NewFakeOrchestrator() *FakeOrchestrator {
	return &FakeOrchestrator{changes: map[domain.ChangeID]*domain.Change{}}
}

func (f *FakeOrchestrator) SeedChange(c *domain.Change) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Backfill a default current_phase_id for non-terminal seeds so
	// runner/attacher tests that haven't been Phase-4-aware still
	// produce a usable target for the multiplexer (D-M10-05).
	if c.CurrentPhaseID == "" && !c.Status.IsTerminal() {
		c.CurrentPhaseID = string(c.ID) + "-phase"
	}
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
	// Per-Phase-4 multiplexer (D-M10-05) requires a current_phase_id
	// for non-terminal changes. The fake mirrors the orchestrator's
	// "init phase auto-start" behavior so the runner can subscribe
	// immediately. Tests that need a phase-less change can clear the
	// field via SeedChange after Create.
	c := &domain.Change{
		ID:                id,
		Name:              in.Name,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: in.ArtifactStoreMode,
		Status:            domain.ChangeStatusPending,
		CurrentPhaseID:    fmt.Sprintf("fake-phase-%d", f.nextID),
	}
	f.changes[id] = c
	return c, nil
}

func (f *FakeOrchestrator) GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if f.OnGetChange != nil {
		f.OnGetChange(id)
	}
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

// MutateChange applies fn to the stored Change atomically. Phase 4
// multiplexer tests use it to flip current_phase_id between
// subscriptions to simulate phase advancement.
func (f *FakeOrchestrator) MutateChange(id domain.ChangeID, fn func(*domain.Change)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.changes[id]; ok {
		fn(c)
	}
}

func (f *FakeOrchestrator) AbortChange(_ context.Context, id domain.ChangeID, in outbound.AbortChangeInput) error {
	if f.OnAbort != nil {
		f.OnAbort(id, in)
	}
	return f.AbortErr
}

func (f *FakeOrchestrator) ApprovePhase(_ context.Context, phaseID string, in outbound.ApprovalDecisionInput) error {
	if f.OnApprove != nil {
		f.OnApprove(phaseID, in)
	}
	return f.ApproveErr
}

func (f *FakeOrchestrator) RejectPhase(_ context.Context, phaseID string, in outbound.ApprovalDecisionInput) error {
	if f.OnReject != nil {
		f.OnReject(phaseID, in)
	}
	return f.RejectErr
}

// RunPhase satisfies the OrchestratorClient.RunPhase method. The fake
// returns the change's current_phase_id (already populated by
// CreateChange to mirror the orch's init-phase auto-start). Tests that
// need a specific phase_id mutate the change via MutateChange before
// invoking RunPhase. Errors are surfaced via RunPhaseErr; the OnRunPhase
// hook lets tests assert what task description was passed.
func (f *FakeOrchestrator) RunPhase(_ context.Context, id domain.ChangeID, phaseType string, in outbound.RunPhaseInput) (*outbound.RunPhaseResult, error) {
	if f.OnRunPhase != nil {
		f.OnRunPhase(id, phaseType, in)
	}
	if f.RunPhaseErr != nil {
		return nil, f.RunPhaseErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[id]
	if !ok {
		return nil, domain.ErrChangeNotFound
	}
	return &outbound.RunPhaseResult{
		PhaseID:   c.CurrentPhaseID,
		Status:    "running",
		EventsURL: "/api/v1/phases/" + c.CurrentPhaseID + "/events",
	}, nil
}

func (f *FakeOrchestrator) ListChanges(_ context.Context, filter outbound.ListChangesFilter) ([]*domain.Change, error) {
	if f.OnListChanges != nil {
		f.OnListChanges(filter)
	}
	if f.ListErr != nil {
		return nil, f.ListErr
	}
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
