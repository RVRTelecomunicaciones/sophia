package yamlconfig

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// UserConfig configures the UserStore.
type UserConfig struct {
	FS afero.Fs
}

// UserStore implements outbound.UserConfigStore.
//
// Per spec §3.1: user config is sensitive and is written 0600.
type UserStore struct {
	fs afero.Fs
}

// NewUserStore returns a UserStore.
func NewUserStore(cfg UserConfig) *UserStore {
	fs := cfg.FS
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &UserStore{fs: fs}
}

// Read returns the parsed user config or ErrConfigMissing.
func (s *UserStore) Read(_ context.Context, path string) (*domain.UserConfig, error) {
	exists, err := afero.Exists(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !exists {
		return nil, domain.ErrConfigMissing
	}
	b, err := afero.ReadFile(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var dto UserDTO
	if err := yaml.Unmarshal(b, &dto); err != nil {
		return nil, domain.ErrInvalidYAML
	}
	return FromUserDTO(&dto), nil
}

// Write persists cfg to path with mode 0600.
func (s *UserStore) Write(_ context.Context, path string, cfg *domain.UserConfig) error {
	dto := ToUserDTO(cfg)
	b, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." && dir != "/" {
		if err := s.fs.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("mkdir %q: %w", dir, err)
		}
	}
	return afero.WriteFile(s.fs, path, b, 0o600)
}
