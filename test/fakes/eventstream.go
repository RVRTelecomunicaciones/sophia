package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

type FakeEventStream struct {
	mu          sync.Mutex
	subscribers map[string][]chan domain.Event
}

func NewFakeEventStream() *FakeEventStream {
	return &FakeEventStream{subscribers: map[string][]chan domain.Event{}}
}

func key(t outbound.StreamTarget) string {
	return string(t.ChangeID) + "|" + t.PhaseID
}

func (s *FakeEventStream) Subscribe(_ context.Context, target outbound.StreamTarget, _ outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan domain.Event, 16)
	s.subscribers[key(target)] = append(s.subscribers[key(target)], ch)
	var once sync.Once
	cancel := func() error {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			subs := s.subscribers[key(target)]
			for i, c := range subs {
				if c == ch {
					s.subscribers[key(target)] = append(subs[:i], subs[i+1:]...)
					close(ch)
					return
				}
			}
		})
		return nil
	}
	return ch, cancel, nil
}

func (s *FakeEventStream) Push(target outbound.StreamTarget, ev domain.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers[key(target)] {
		ch <- ev
	}
}
