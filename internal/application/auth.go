package application

import (
	"os"
	"strings"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// EnvAPIKey is the env var name for the orchestrator API key.
// sophia-wire-v1 §3.1 + Phase 4 Task 4.2.
const EnvAPIKey = "SOPHIA_API_KEY" // #nosec G101 -- env var NAME, not a credential value

// APIKeyResolver resolves the orchestrator API key from one of:
//  1. an explicit Flag value (e.g. `--api-key`),
//  2. the SOPHIA_API_KEY env var,
//  3. nothing (anonymous; only valid against a loopback orchestrator).
//
// Resolve returns the canonical key for outbound HTTP/SSE requests.
// Authorize checks whether anonymous access is permitted given the
// orchestrator URL and returns domain.ErrAuthRequired when a remote
// orchestrator is targeted without a key.
//
// The resolver MUST NOT log or print the key value anywhere — only
// "set" / "missing" booleans are safe to surface (§3.1 / D-M10-02).
type APIKeyResolver struct {
	Flag string // value of --api-key (empty when not provided)
	Env  func(string) string
}

// NewAPIKeyResolver constructs a resolver. Pass nil for env to read from
// os.Getenv; tests inject a programmable function.
func NewAPIKeyResolver(flag string, env func(string) string) *APIKeyResolver {
	if env == nil {
		env = os.Getenv
	}
	return &APIKeyResolver{Flag: flag, Env: env}
}

// Resolve returns the API key (flag wins over env). Returns "" when no
// key is configured.
func (r *APIKeyResolver) Resolve() string {
	if v := strings.TrimSpace(r.Flag); v != "" {
		return v
	}
	return strings.TrimSpace(r.Env(EnvAPIKey))
}

// HasKey reports whether a non-empty key was resolved. Safe to log.
func (r *APIKeyResolver) HasKey() bool { return r.Resolve() != "" }

// Authorize validates the key against the orchestrator URL. Localhost
// orchestrators (loopback per IsLoopbackURL) accept anonymous; remote
// orchestrators MUST have a key. Returns domain.ErrAuthRequired with a
// friendly message when remote+missing, nil otherwise.
//
// On URL-parse failure, fails closed (auth required) — never assume the
// orchestrator is loopback when we can't verify.
func (r *APIKeyResolver) Authorize(orchestratorURL string) error {
	if r.HasKey() {
		return nil
	}
	loopback, err := IsLoopbackURL(orchestratorURL)
	if err != nil || !loopback {
		return domain.ErrAuthRequired
	}
	return nil
}
