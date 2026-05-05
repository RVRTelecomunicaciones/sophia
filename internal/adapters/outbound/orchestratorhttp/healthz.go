package orchestratorhttp

import (
	"context"
	"fmt"
	"net/http"
)

// Healthz pings GET <BaseURL>/api/v1/healthz; returns nil on 200.
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/healthz", nil)
	if err != nil {
		return fmt.Errorf("healthz: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("healthz: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("healthz: status %d", resp.StatusCode)
	}
	return nil
}
