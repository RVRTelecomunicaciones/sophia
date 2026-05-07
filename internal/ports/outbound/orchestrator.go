package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type CreateChangeInput struct {
	Name              string
	Project           string
	BaseRef           string
	ArtifactStoreMode string
}

type ListChangesFilter struct {
	Project string
	Status  string
	Limit   int
	Offset  int
}

// ApprovalDecisionInput is the body of POST /phases/{id}/approve and
// POST /phases/{id}/reject (sophia-wire-v1 §4.3, §6.5).
type ApprovalDecisionInput struct {
	Approver string
	Reason   string
}

// AbortChangeInput is the body of POST /changes/{id}/abort.
type AbortChangeInput struct {
	Reason string
}

type OrchestratorClient interface {
	Healthz(ctx context.Context) error
	CreateChange(ctx context.Context, in CreateChangeInput) (*domain.Change, error)
	GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error)
	ListChanges(ctx context.Context, filter ListChangesFilter) ([]*domain.Change, error)

	// AbortChange terminates a Change in-flight (sophia-wire-v1 §4.2).
	// 409 change_already_terminal is permissible — caller MUST surface
	// it as idempotent success.
	AbortChange(ctx context.Context, id domain.ChangeID, in AbortChangeInput) error

	// ApprovePhase resolves an approval gate as approved
	// (sophia-wire-v1 §4.3 / §8). 409 gate_already_decided is
	// permissible — caller MUST surface it as idempotent.
	ApprovePhase(ctx context.Context, phaseID string, in ApprovalDecisionInput) error

	// RejectPhase resolves an approval gate as rejected
	// (sophia-wire-v1 §4.3 / §8). 409 gate_already_decided is
	// permissible — caller MUST surface it as idempotent.
	RejectPhase(ctx context.Context, phaseID string, in ApprovalDecisionInput) error
}
