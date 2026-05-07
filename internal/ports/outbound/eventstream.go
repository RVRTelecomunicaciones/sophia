package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// StreamTarget identifies a stream subscription. sophia-wire-v1 §4.3 +
// D-M10-05 (per-phase canonical model): PhaseID is the authoritative
// stream identifier. ChangeID is retained for caller-side bookkeeping
// (the runner persists last_change_id and refreshes via the change
// endpoint between phase switches).
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
