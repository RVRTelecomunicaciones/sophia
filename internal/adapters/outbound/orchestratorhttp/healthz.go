package orchestratorhttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// Healthz pings GET <BaseURL>/api/v1/health (sophia-wire-v1 §4.1) and
// returns nil on 2xx. The method is named Healthz for backward
// compatibility with the outbound port surface; the wire path is the
// canonical /api/v1/health (no /healthz alias on the orchestrator side
// per D-M10-06).
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+contract.RouteHealth, nil)
	if err != nil {
		return fmt.Errorf("health: build request: %w", err)
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("health: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health: status %d", resp.StatusCode)
	}
	return nil
}
