package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type UserConfigStore interface {
	Read(ctx context.Context, path string) (*domain.UserConfig, error)
	Write(ctx context.Context, path string, cfg *domain.UserConfig) error
}
