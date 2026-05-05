// Package orchestratorhttp implements outbound.OrchestratorClient via HTTP.
// In M2 only Healthz is implemented; the rest return ErrNotImplemented and
// land in M4.
package orchestratorhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ErrNotImplemented is returned by methods landing in later milestones.
var ErrNotImplemented = errors.New("orchestratorhttp: not implemented in M2")

// Config controls the client.
type Config struct {
	BaseURL string
	Timeout time.Duration
	HTTP    *http.Client
}

// Client implements outbound.OrchestratorClient.
type Client struct {
	base string
	http *http.Client
}

// New returns a Client.
func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTP
	if hc == nil {
		hc = httpclient.New(httpclient.Config{Timeout: cfg.Timeout})
	}
	return &Client{base: base, http: hc}
}

// CreateChange is not implemented in M2 (lands in M4).
func (c *Client) CreateChange(_ context.Context, _ outbound.CreateChangeInput) (*domain.Change, error) {
	return nil, ErrNotImplemented
}

// GetChange is not implemented in M2 (lands in M4).
func (c *Client) GetChange(_ context.Context, _ domain.ChangeID) (*domain.Change, error) {
	return nil, ErrNotImplemented
}

// ListChanges is not implemented in M2 (lands in M4).
func (c *Client) ListChanges(_ context.Context, _ outbound.ListChangesFilter) ([]*domain.Change, error) {
	return nil, ErrNotImplemented
}
