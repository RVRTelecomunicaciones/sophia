package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

type FakeEventStream struct {
	mu          sync.Mutex
	subscribers map[string][]chan domain.Event
	// OnSubscribe is called (in a goroutine) each time Subscribe is invoked.
	// Tests use it to push events or close the stream after setup completes.
	OnSubscribe func(outbound.StreamTarget)
}

func NewFakeEventStream() *FakeEventStream {
	return &FakeEventStream{subscribers: map[string][]chan domain.Event{}}
}

func key(t outbound.StreamTarget) string {
	return string(t.ChangeID) + "|" + t.PhaseID
}

func (s *FakeEventStream) Subscribe(_ context.Context, target outbound.StreamTarget, _ outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	s.mu.Lock()
	ch := make(chan domain.Event, 16)
	s.subscribers[key(target)] = append(s.subscribers[key(target)], ch)
	hook := s.OnSubscribe
	s.mu.Unlock()
	if hook != nil {
		go hook(target)
	}
	var once sync.Once
	cancel := func() error { //nolint:unparam // returns error to satisfy outbound.CancelFunc contract; always nil in fake
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
	// Snapshot subscribers under the lock; send outside so a full-buffer
	// receiver can't deadlock the rest of the fake (Close/Subscribe/cancel
	// also acquire mu).
	s.mu.Lock()
	subs := make([]chan domain.Event, len(s.subscribers[key(target)]))
	copy(subs, s.subscribers[key(target)])
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- ev
	}
}

// Close simulates a graceful server-side stream close for target.
func (s *FakeEventStream) Close(target outbound.StreamTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers[key(target)] {
		close(ch)
	}
	s.subscribers[key(target)] = nil
}
