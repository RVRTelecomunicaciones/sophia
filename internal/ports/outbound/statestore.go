package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type StateStore interface {
	GetLast(ctx context.Context, fp domain.Fingerprint) (domain.ChangeID, error)
	SetLast(ctx context.Context, fp domain.Fingerprint, id domain.ChangeID) error
	GetGlobalLast(ctx context.Context) (domain.ChangeID, error)
	SetGlobalLast(ctx context.Context, id domain.ChangeID) error
}
