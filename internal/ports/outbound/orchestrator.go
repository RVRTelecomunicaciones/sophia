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

type OrchestratorClient interface {
	Healthz(ctx context.Context) error
	CreateChange(ctx context.Context, in CreateChangeInput) (*domain.Change, error)
	GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error)
	ListChanges(ctx context.Context, filter ListChangesFilter) ([]*domain.Change, error)
}
