package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

type FakeStateStore struct {
	mu       sync.Mutex
	projects map[domain.Fingerprint]domain.ChangeID
	global   domain.ChangeID
}

func NewFakeStateStore() *FakeStateStore {
	return &FakeStateStore{projects: map[domain.Fingerprint]domain.ChangeID{}}
}

func (s *FakeStateStore) GetLast(_ context.Context, fp domain.Fingerprint) (domain.ChangeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projects[fp], nil
}

func (s *FakeStateStore) SetLast(_ context.Context, fp domain.Fingerprint, id domain.ChangeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[fp] = id
	return nil
}

func (s *FakeStateStore) GetGlobalLast(_ context.Context) (domain.ChangeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.global, nil
}

func (s *FakeStateStore) SetGlobalLast(_ context.Context, id domain.ChangeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = id
	return nil
}
