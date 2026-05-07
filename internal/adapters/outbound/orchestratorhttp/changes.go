package orchestratorhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

// maxBodySize caps response body reads to 1 MiB to bound memory.
const maxBodySize = 1 << 20

// CreateChange POSTs /api/v1/changes.
func (c *Client) CreateChange(ctx context.Context, in outbound.CreateChangeInput) (*domain.Change, error) {
	body, err := json.Marshal(CreateChangeRequest{
		Name:              in.Name,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: in.ArtifactStoreMode,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/changes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	var resp ChangeResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.ToDomain(), nil
}

// GetChange GETs /api/v1/changes/{id}.
func (c *Client) GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("empty change id")
	}
	u := c.base + "/api/v1/changes/" + url.PathEscape(string(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	var resp ChangeResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.ToDomain(), nil
}

// ListChanges GETs /api/v1/changes with optional filters.
func (c *Client) ListChanges(ctx context.Context, f outbound.ListChangesFilter) ([]*domain.Change, error) {
	q := url.Values{}
	if f.Project != "" {
		q.Set("project", f.Project)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}
	u := c.base + "/api/v1/changes"
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	var resp ListChangesResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := make([]*domain.Change, len(resp.Items))
	for i, item := range resp.Items {
		out[i] = item.ToDomain()
	}
	return out, nil
}

// doJSON executes req, reads up to maxBodySize bytes, returns a *StatusError
// on non-2xx, or json-decodes a 2xx body into out.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBodySize {
		return fmt.Errorf("response body exceeds %d bytes", maxBodySize)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewStatusError(resp.StatusCode, body)
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode body: %w", err)
		}
	}
	return nil
}

// doNoContent executes req and discards the response body. Returns
// a *StatusError on non-2xx (carrying the parsed envelope) and nil on
// any 2xx, regardless of body. Used by abort/approve/reject which the
// CLI fires-and-forgets.
func (c *Client) doNoContent(req *http.Request) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBodySize {
		return fmt.Errorf("response body exceeds %d bytes", maxBodySize)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewStatusError(resp.StatusCode, body)
	}
	return nil
}
