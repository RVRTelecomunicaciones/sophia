package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

type FakeUserConfigStore struct {
	mu      sync.Mutex
	store   map[string]*domain.UserConfig
	ReadErr error
}

func NewFakeUserConfigStore() *FakeUserConfigStore {
	return &FakeUserConfigStore{store: map[string]*domain.UserConfig{}}
}

func (s *FakeUserConfigStore) Read(_ context.Context, path string) (*domain.UserConfig, error) {
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

func (s *FakeUserConfigStore) Write(_ context.Context, path string, cfg *domain.UserConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *cfg
	s.store[path] = &out
	return nil
}
