//go:build contract

package contract_test

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
)

// recordingSink is a tiny inbound.EventSink for contract smoke tests.
// It captures Events without rendering anything; tests assert on the
// captured set rather than on UI output.
type recordingSink struct {
	mu     sync.Mutex
	events []domain.Event
	gates  []domain.ApprovalGate
	final  domain.ChangeStatus
	closed bool
}

func newRecordingSink() *recordingSink { return &recordingSink{} }

func (s *recordingSink) OnSnapshot(_ context.Context, _ *domain.Change) error { return nil }
func (s *recordingSink) OnEvent(_ context.Context, ev domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}
func (s *recordingSink) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gates = append(s.gates, g)
	return nil
}
func (s *recordingSink) OnError(_ context.Context, _ error) error { return nil }
func (s *recordingSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.final = st
	return nil
}
func (s *recordingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// SawEventType reports whether at least one event with the given type
// passed through OnEvent.
func (s *recordingSink) SawEventType(t string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type == t {
			return true
		}
	}
	return false
}
