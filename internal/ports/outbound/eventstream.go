package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

type StreamTarget struct {
	ChangeID domain.ChangeID
	PhaseID  string
}

type SubscribeOptions struct {
	LastEventID string
}

type EventStreamClient interface {
	Subscribe(ctx context.Context, target StreamTarget, opts SubscribeOptions) (<-chan domain.Event, func() error, error)
}
