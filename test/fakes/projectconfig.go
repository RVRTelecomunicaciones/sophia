package fakes

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

type FakeProjectConfigStore struct {
	mu      sync.Mutex
	store   map[string]*domain.ProjectConfig
	ReadErr error
}

func NewFakeProjectConfigStore() *FakeProjectConfigStore {
	return &FakeProjectConfigStore{store: map[string]*domain.ProjectConfig{}}
}

func (s *FakeProjectConfigStore) Read(_ context.Context, path string) (*domain.ProjectConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ReadErr != nil {
		return nil, s.ReadErr
	}
	cfg, ok := s.store[path]
	if !ok {
		return nil, domain.ErrConfigMissing
	}
	out := *cfg
	return &out, nil
}

func (s *FakeProjectConfigStore) Write(_ context.Context, path string, cfg *domain.ProjectConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *cfg
	s.store[path] = &out
	return nil
}

func (s *FakeProjectConfigStore) Find(_ context.Context, startDir string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := startDir
	for dir != "" && dir != "/" {
		candidate := dir + "/.sophia.yaml"
		if _, ok := s.store[candidate]; ok {
			return candidate, nil
		}
		idx := strings.LastIndex(dir, "/")
		if idx <= 0 {
			break
		}
		dir = dir[:idx]
	}
	if _, ok := s.store["/.sophia.yaml"]; ok {
		return "/.sophia.yaml", nil
	}
	return "", errors.New("no .sophia.yaml found")
}
