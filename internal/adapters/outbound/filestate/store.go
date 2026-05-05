// Package filestate implements outbound.StateStore using `<stateRoot>/`
// with atomic writes (temp file + os.Rename) per spec §3.5.
package filestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the Store.
type Config struct {
	// StateRoot is the resolved <stateRoot> path (typically XDGPaths.StateRoot).
	StateRoot string
}

// Store implements outbound.StateStore.
type Store struct {
	root string
}

// New returns a Store rooted at cfg.StateRoot.
func New(cfg Config) *Store { return &Store{root: cfg.StateRoot} }

// ProjectMeta is the JSON payload at <stateRoot>/projects/<fp>/meta.json.
type ProjectMeta struct {
	Project   string    `json:"project"`
	RepoRoot  string    `json:"repo_root"`
	RemoteURL string    `json:"remote_url"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) projectDir(fp domain.Fingerprint) string {
	return filepath.Join(s.root, "projects", string(fp))
}

func (s *Store) projectFile(fp domain.Fingerprint) string {
	return filepath.Join(s.projectDir(fp), "last_change_id")
}

func (s *Store) globalFile() string {
	return filepath.Join(s.root, "last_change_id")
}

func (s *Store) metaFile(fp domain.Fingerprint) string {
	return filepath.Join(s.projectDir(fp), "meta.json")
}

// GetLast returns the project-scoped last change id (zero if absent).
func (s *Store) GetLast(_ context.Context, fp domain.Fingerprint) (domain.ChangeID, error) {
	return readChangeID(s.projectFile(fp))
}

// SetLast atomically persists id under the project scope.
func (s *Store) SetLast(_ context.Context, fp domain.Fingerprint, id domain.ChangeID) error {
	if err := os.MkdirAll(s.projectDir(fp), 0o700); err != nil {
		return fmt.Errorf("mkdir project: %w", err)
	}
	return atomicWrite(s.projectFile(fp), []byte(id), 0o644)
}

// GetGlobalLast returns the global fallback last change id.
func (s *Store) GetGlobalLast(_ context.Context) (domain.ChangeID, error) {
	return readChangeID(s.globalFile())
}

// SetGlobalLast atomically persists id at the global scope.
func (s *Store) SetGlobalLast(_ context.Context, id domain.ChangeID) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("mkdir state root: %w", err)
	}
	return atomicWrite(s.globalFile(), []byte(id), 0o644)
}

// WriteProjectMeta persists meta.json under projects/<fp>/.
func (s *Store) WriteProjectMeta(_ context.Context, fp domain.Fingerprint, m ProjectMeta) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(s.projectDir(fp), 0o700); err != nil {
		return fmt.Errorf("mkdir project: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return atomicWrite(s.metaFile(fp), b, 0o644)
}

// ReadProjectMeta loads meta.json under projects/<fp>/.
func (s *Store) ReadProjectMeta(_ context.Context, fp domain.Fingerprint) (ProjectMeta, error) {
	b, err := os.ReadFile(s.metaFile(fp))
	if err != nil {
		return ProjectMeta{}, fmt.Errorf("read meta: %w", err)
	}
	var m ProjectMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return ProjectMeta{}, fmt.Errorf("unmarshal meta: %w", err)
	}
	return m, nil
}

func readChangeID(path string) (domain.ChangeID, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return domain.ChangeID(strings.TrimSpace(string(b))), nil
}

// atomicWrite writes b to path via a temp file in the same directory plus
// os.Rename. This is durable on POSIX FS for power loss within rename.
func atomicWrite(path string, b []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("temp create: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("temp write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("temp sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("temp close: %w", err)
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return fmt.Errorf("temp chmod: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
