package outbound

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// PathResolver resolves and validates the four XDG-style roots used by the CLI.
// Real adapter (M2): internal/adapters/outbound/xdgpaths.
type PathResolver interface {
	// Resolve returns the four roots based on env vars and HOME, applying
	// the macOS Linux-style fallback documented in the spec.
	Resolve() (domain.XDGPaths, error)

	// EnsureDirs creates each root directory at 0700 if missing.
	// It is idempotent and only used by `sophia start` (and future write paths);
	// `sophia doctor` does NOT create directories.
	EnsureDirs(p domain.XDGPaths) error

	// ValidateDirs returns nil iff each root exists with mode 0700.
	// Used by `sophia doctor` (which is read-only).
	ValidateDirs(p domain.XDGPaths) error
}
