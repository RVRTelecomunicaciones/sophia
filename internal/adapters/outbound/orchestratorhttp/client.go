// Package orchestratorhttp implements outbound.OrchestratorClient via HTTP.
package orchestratorhttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
)

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
