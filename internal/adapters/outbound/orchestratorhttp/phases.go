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

// RunPhase POSTs /api/v1/changes/{id}/phases/{type}/run with the
// task description. Implements outbound.OrchestratorClient.RunPhase.
// Returns the phase_id + events_url so the caller (Runner.Run) can
// subscribe to SSE immediately.
func (c *Client) RunPhase(ctx context.Context, id domain.ChangeID, phaseType string, in outbound.RunPhaseInput) (*outbound.RunPhaseResult, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("run phase: empty change id")
	}
	if phaseType == "" {
		return nil, fmt.Errorf("run phase: empty phase type")
	}
	body, err := json.Marshal(contract.RunPhaseRequest{TaskDescription: in.TaskDescription})
	if err != nil {
		return nil, fmt.Errorf("run phase: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+contract.ChangePhaseRunPath(string(id), phaseType), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("run phase: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	var resp contract.RunPhaseResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("run phase: %w", err)
	}
	return &outbound.RunPhaseResult{
		PhaseID:   resp.PhaseID,
		Status:    resp.Status,
		EventsURL: resp.EventsURL,
	}, nil
}

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
