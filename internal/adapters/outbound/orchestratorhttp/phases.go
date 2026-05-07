package orchestratorhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// AbortChange POSTs /api/v1/changes/{id}/abort with {reason?}.
// Implements outbound.OrchestratorClient.AbortChange.
func (c *Client) AbortChange(ctx context.Context, id domain.ChangeID, in outbound.AbortChangeInput) error {
	if id.IsZero() {
		return fmt.Errorf("abort: empty change id")
	}
	body, err := json.Marshal(contract.AbortChangeRequest{Reason: in.Reason})
	if err != nil {
		return fmt.Errorf("abort: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+contract.ChangeAbortPath(string(id)), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("abort: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)
	return c.doNoContent(req)
}

// ApprovePhase POSTs /api/v1/phases/{id}/approve with {approver, reason?}.
// Implements outbound.OrchestratorClient.ApprovePhase.
func (c *Client) ApprovePhase(ctx context.Context, phaseID string, in outbound.ApprovalDecisionInput) error {
	return c.decidePhase(ctx, contract.PhaseApprovePath(phaseID), in)
}

// RejectPhase POSTs /api/v1/phases/{id}/reject with {approver, reason?}.
// Implements outbound.OrchestratorClient.RejectPhase.
func (c *Client) RejectPhase(ctx context.Context, phaseID string, in outbound.ApprovalDecisionInput) error {
	return c.decidePhase(ctx, contract.PhaseRejectPath(phaseID), in)
}

// decidePhase is the shared POST body builder for approve / reject.
func (c *Client) decidePhase(ctx context.Context, path string, in outbound.ApprovalDecisionInput) error {
	if in.Approver == "" {
		return fmt.Errorf("approver required")
	}
	body, err := json.Marshal(contract.ApprovalDecisionRequest{
		Approver: in.Approver,
		Reason:   in.Reason,
	})
	if err != nil {
		return fmt.Errorf("decide: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("decide: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)
	return c.doNoContent(req)
}
