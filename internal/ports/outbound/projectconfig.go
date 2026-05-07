package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type ProjectConfigStore interface {
	Read(ctx context.Context, path string) (*domain.ProjectConfig, error)
	Write(ctx context.Context, path string, cfg *domain.ProjectConfig) error
	Find(ctx context.Context, startDir string) (path string, err error)
}
