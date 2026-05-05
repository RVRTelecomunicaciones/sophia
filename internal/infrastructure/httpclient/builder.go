// Package httpclient builds the *http.Client used by every outbound HTTP
// adapter. Centralizing here ensures consistent timeouts, redirect policy,
// and (in V1.1+) telemetry/proxies.
package httpclient

import (
	"net/http"
	"time"
)

// DefaultTimeout is applied when Config.Timeout is zero.
const DefaultTimeout = 5 * time.Second

// Config controls the constructed client.
type Config struct {
	Timeout time.Duration
}

// New returns a configured *http.Client.
func New(cfg Config) *http.Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &http.Client{Timeout: cfg.Timeout}
}
