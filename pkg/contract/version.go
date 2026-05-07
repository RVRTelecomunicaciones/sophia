// Package contract is the canonical wire shape between sophia-cli and
// sophia-orchestator. Both repos import this package to avoid drift on
// route paths, SSE event names, error codes, and request/response DTOs.
//
// STRICT scope:
//   - DTOs (request / response shapes)
//   - constants (route paths, event names, error codes, header names, version)
//   - test helpers (under pkg/contract/test/)
//
// Out of scope (NEVER import in this package):
//   - internal/* of sophia-cli
//   - application/adapters/cli of sophia-cli
//   - bubbletea / lipgloss / cobra
//   - any business logic
//
// Every change to this package's exported surface is a coordinated
// cross-repo change: PRs in sophia-cli and sophia-orchestator must
// land in lockstep, and `docs/specs/sophia-wire-v1.md` MUST be
// updated in the same commit pair.
package contract

import "net/http"

// Version is the wire spec version this package implements.
// See: docs/specs/sophia-wire-v1.md §1.
const Version = "v1"

// HeaderAPIKey is the canonical auth header name (sophia-wire-v1 §3.1).
const HeaderAPIKey = "X-Sophia-API-Key"

// HeaderAPIKeyLegacy is the historical fallback header. Servers MUST
// accept both; clients SHOULD use HeaderAPIKey only.
const HeaderAPIKeyLegacy = "X-API-Key"

// HeaderAPIVersion is reserved for v2 (multi-version servers per §1).
// Servers in v1 ignore this header. Reserved here so v2 clients have a
// stable constant to write into.
const HeaderAPIVersion = "API-Version"

// RequiredAuthHeaders returns the headers a client MUST send on every
// authenticated request. Returns an empty http.Header when apiKey is "".
func RequiredAuthHeaders(apiKey string) http.Header {
	h := http.Header{}
	if apiKey != "" {
		h.Set(HeaderAPIKey, apiKey)
	}
	return h
}
