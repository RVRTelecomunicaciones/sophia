package application

import (
	"context"
	"errors"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// AborterDeps groups the dependencies of `sophia abort`.
type AborterDeps struct {
	Orch outbound.OrchestratorClient
}

// Aborter implements `sophia abort <change-id>` per sophia-wire-v1 §4.2
// + Phase 4 Task 4.6.
type Aborter struct {
	deps AborterDeps
}

// NewAborter constructs an Aborter.
func NewAborter(d AborterDeps) *Aborter { return &Aborter{deps: d} }

// AbortInput controls Abort.
type AbortInput struct {
	ChangeID domain.ChangeID
	Reason   string
}

// Abort calls the orchestrator's abort endpoint. ErrChangeAlreadyTerminal
// is returned as-is so the CLI can surface it as idempotent success.
func (a *Aborter) Abort(ctx context.Context, in AbortInput) error {
	if in.ChangeID.IsZero() {
		return errors.New("change_id required")
	}
	return a.deps.Orch.AbortChange(ctx, in.ChangeID, outbound.AbortChangeInput{Reason: in.Reason})
}
