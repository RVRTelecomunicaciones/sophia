// Package sseprobe performs a one-shot handshake against the orchestrator's
// SSE endpoint. It is used by `sophia doctor` and intentionally does NOT
// consume the stream — that is the M5 responsibility of EventStreamClient.
package sseprobe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
)

// DefaultPath is the SSE endpoint path probed by V1.
const DefaultPath = "/api/v1/events"

// Config controls the prober.
type Config struct {
	BaseURL string
	Path    string
	Timeout time.Duration
	HTTP    *http.Client
}

// Prober implements outbound.SSEProber.
type Prober struct {
	url  string
	http *http.Client
}

// New returns a Prober.
func New(cfg Config) *Prober {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = httpclient.New(httpclient.Config{Timeout: cfg.Timeout})
	}
	return &Prober{url: strings.TrimRight(cfg.BaseURL, "/") + path, http: hc}
}

// Probe sends GET <BaseURL><Path> with Accept: text/event-stream.
func (p *Prober) Probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return fmt.Errorf("sseprobe: build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("sseprobe: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sseprobe: status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		return fmt.Errorf("sseprobe: content-type %q (want text/event-stream)", ct)
	}
	return nil
}
