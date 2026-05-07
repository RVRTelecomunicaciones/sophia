package inbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type EventSink interface {
	OnSnapshot(ctx context.Context, change *domain.Change) error
	OnEvent(ctx context.Context, ev domain.Event) error
	OnApprovalGate(ctx context.Context, gate domain.ApprovalGate) error
	OnError(ctx context.Context, err error) error
	OnComplete(ctx context.Context, finalStatus domain.ChangeStatus) error
	Close() error
}
