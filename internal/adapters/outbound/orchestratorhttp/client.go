// Package orchestratorhttp implements outbound.OrchestratorClient via HTTP.
package orchestratorhttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
)

// Config controls the client.
type Config struct {
	BaseURL string
	Timeout time.Duration
	HTTP    *http.Client
	// APIKey is the X-Sophia-API-Key header value (sophia-wire-v1 §3.1).
	// Empty key = anonymous; bootstrap MUST only allow this when the
	// orchestrator URL is loopback (D-M10-02).
	APIKey string
}

// Client implements outbound.OrchestratorClient.
type Client struct {
	base   string
	http   *http.Client
	apiKey string
}

// New returns a Client.
func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTP
	if hc == nil {
		hc = httpclient.New(httpclient.Config{Timeout: cfg.Timeout})
	}
	return &Client{base: base, http: hc, apiKey: cfg.APIKey}
}

// applyAuth attaches the X-Sophia-API-Key header to req when an API key
// is configured. The key value MUST NOT be logged anywhere (§3.1).
func (c *Client) applyAuth(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	req.Header.Set(contract.HeaderAPIKey, c.apiKey)
}
