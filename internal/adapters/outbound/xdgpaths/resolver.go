// Package xdgpaths implements outbound.PathResolver using XDG Base Directory
// conventions, with a Linux-style fallback on all platforms when XDG_* vars
// are unset (per spec §3.1).
package xdgpaths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the resolver. Env is injected for testability; in
// production the bootstrap layer passes os-derived values.
type Config struct {
	// Env, when non-nil, is consulted instead of os.Getenv. Tests override
	// this to control HOME and XDG vars deterministically.
	Env map[string]string
}

// Resolver implements outbound.PathResolver.
type Resolver struct {
	cfg Config
}

// New returns a Resolver. Pass an empty Config for production defaults.
func New(cfg Config) *Resolver { return &Resolver{cfg: cfg} }

func (r *Resolver) getenv(k string) string {
	if r.cfg.Env != nil {
		return r.cfg.Env[k]
	}
	return os.Getenv(k)
}

// Resolve returns the four roots, suffixing each with "sophia".
func (r *Resolver) Resolve() (domain.XDGPaths, error) {
	home := r.getenv("HOME")
	root := func(envKey, fallback string) (string, error) {
		if v := r.getenv(envKey); v != "" {
			return filepath.Join(v, "sophia"), nil
		}
		if home == "" {
			return "", fmt.Errorf("HOME unset and %s unset", envKey)
		}
		return filepath.Join(home, fallback, "sophia"), nil
	}
	cfg, err := root("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return domain.XDGPaths{}, err
	}
	st, err := root("XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return domain.XDGPaths{}, err
	}
	data, err := root("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return domain.XDGPaths{}, err
	}
	cache, err := root("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return domain.XDGPaths{}, err
	}
	return domain.XDGPaths{
		ConfigRoot: cfg,
		StateRoot:  st,
		DataRoot:   data,
		CacheRoot:  cache,
	}, nil
}

// EnsureDirs creates each root at 0700 if missing.
func (r *Resolver) EnsureDirs(p domain.XDGPaths) error {
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if d == "" {
			return errors.New("empty path in XDGPaths")
		}
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("mkdir %q: %w", d, err)
		}
	}
	return nil
}

// ValidateDirs returns nil iff every root exists with mode 0700.
func (r *Resolver) ValidateDirs(p domain.XDGPaths) error {
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if d == "" {
			return errors.New("empty path in XDGPaths")
		}
		fi, err := os.Stat(d)
		if err != nil {
			return fmt.Errorf("stat %q: %w", d, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("%q is not a directory", d)
		}
		if fi.Mode().Perm() != 0o700 {
			return fmt.Errorf("%q perm %o, want 0700", d, fi.Mode().Perm())
		}
	}
	return nil
}
