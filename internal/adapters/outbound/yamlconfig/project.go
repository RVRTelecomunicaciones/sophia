package yamlconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// MaxProjectYAMLSize is the spec §6.3 cap (100 KB) for `.sophia.yaml`.
const MaxProjectYAMLSize = 100 * 1024

// ProjectConfig configures the ProjectStore.
type ProjectConfig struct {
	FS afero.Fs
}

// ProjectStore implements outbound.ProjectConfigStore.
type ProjectStore struct {
	fs afero.Fs
}

// NewProjectStore returns a ProjectStore.
func NewProjectStore(cfg ProjectConfig) *ProjectStore {
	fs := cfg.FS
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &ProjectStore{fs: fs}
}

// Read returns the parsed `.sophia.yaml`. Maps not-found to ErrConfigMissing
// and parse errors to ErrInvalidYAML.
func (s *ProjectStore) Read(_ context.Context, path string) (*domain.ProjectConfig, error) {
	exists, err := afero.Exists(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !exists {
		return nil, domain.ErrConfigMissing
	}
	f, err := s.fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	limited := io.LimitReader(f, MaxProjectYAMLSize+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	if len(b) > MaxProjectYAMLSize {
		return nil, fmt.Errorf("file %q exceeds %d bytes", path, MaxProjectYAMLSize)
	}

	var dto ProjectDTO
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&dto); err != nil {
		return nil, domain.ErrInvalidYAML
	}
	return FromProjectDTO(&dto), nil
}

// Write persists cfg to path with mode 0644 (non-sensitive). Parent dirs
// are created with 0700 if missing.
func (s *ProjectStore) Write(_ context.Context, path string, cfg *domain.ProjectConfig) error {
	dto := ToProjectDTO(cfg)
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
	return afero.WriteFile(s.fs, path, b, 0o644)
}

// Find walks ancestors of startDir looking for a `.sophia.yaml`.
func (s *ProjectStore) Find(_ context.Context, startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		candidate := filepath.Join(dir, ".sophia.yaml")
		exists, err := afero.Exists(s.fs, candidate)
		if err == nil && exists {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .sophia.yaml found above %q", startDir)
}
