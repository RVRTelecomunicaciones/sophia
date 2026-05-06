package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

// errApprovalTimeout is the sentinel for approval-timeout expiry.
var errApprovalTimeout = errors.New("approval timeout exceeded")

// approvalTimeoutSink decorates an inbound.EventSink with an approval timer
// (spec §5.8). The timer starts on the first OnApprovalGate call. It is
// canceled by:
//
//   - approval.resolved event (via OnEvent)
//   - a phase.started event for a phase strictly after the gated phase
//   - an OnSnapshot whose CurrentPhase is strictly after the gated phase
//
// On expiry, the wrapper calls cancel() to stop the runner; the runner
// surfaces ctx.Err which the cli then translates into ExitError{Code: 5}.
//
// Wait() returns nil on natural completion and errApprovalTimeout when the
// timer fired before any cancel trigger.
type approvalTimeoutSink struct {
	inner   inbound.EventSink
	timeout time.Duration
	cancel  context.CancelFunc

	mu     sync.Mutex
	gate   *domain.ApprovalGate
	timer  *time.Timer
	fired  bool
	closed bool
}

func newApprovalTimeoutSink(inner inbound.EventSink, timeout time.Duration, cancel context.CancelFunc) *approvalTimeoutSink {
	return &approvalTimeoutSink{
		inner:   inner,
		timeout: timeout,
		cancel:  cancel,
	}
}

func (s *approvalTimeoutSink) OnSnapshot(ctx context.Context, c *domain.Change) error {
	s.maybeCancelOnSnapshot(c)
	return s.inner.OnSnapshot(ctx, c)
}

func (s *approvalTimeoutSink) OnEvent(ctx context.Context, ev domain.Event) error {
	s.observe(ev)
	return s.inner.OnEvent(ctx, ev)
}

func (s *approvalTimeoutSink) OnApprovalGate(ctx context.Context, g domain.ApprovalGate) error {
	s.startTimer(g)
	return s.inner.OnApprovalGate(ctx, g)
}

func (s *approvalTimeoutSink) OnError(ctx context.Context, err error) error {
	return s.inner.OnError(ctx, err)
}

func (s *approvalTimeoutSink) OnComplete(ctx context.Context, st domain.ChangeStatus) error {
	s.stopTimer()
	return s.inner.OnComplete(ctx, st)
}

func (s *approvalTimeoutSink) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	return s.inner.Close()
}

// Wait reports whether the timer fired (returns errApprovalTimeout) or
// completed naturally (returns nil).
func (s *approvalTimeoutSink) Wait() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired {
		return errApprovalTimeout
	}
	return nil
}

func (s *approvalTimeoutSink) startTimer(g domain.ApprovalGate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	cp := g
	s.gate = &cp
	if s.timeout <= 0 {
		return
	}
	s.timer = time.AfterFunc(s.timeout, func() {
		s.mu.Lock()
		s.fired = true
		s.mu.Unlock()
		// Call cancel outside the lock to avoid potential deadlock if cancel
		// triggers OnX callbacks that re-acquire s.mu.
		if s.cancel != nil {
			s.cancel()
		}
	})
}

func (s *approvalTimeoutSink) stopTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.gate = nil
}

func (s *approvalTimeoutSink) observe(ev domain.Event) {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate == nil {
		return
	}
	if ev.Type == "approval.resolved" {
		s.stopTimer()
		return
	}
	if ev.Type == "phase.started" {
		pt := phaseTypeFromEventPayload(ev.Payload)
		if pt != "" && phaseOrdinalAfter(pt, gate.Phase) {
			s.stopTimer()
		}
	}
}

func (s *approvalTimeoutSink) maybeCancelOnSnapshot(c *domain.Change) {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate == nil || c == nil {
		return
	}
	for _, p := range c.Phases {
		if p.ID != c.CurrentPhaseID {
			continue
		}
		if phaseOrdinalAfter(p.Type, gate.Phase) {
			s.stopTimer()
		}
		return
	}
}

// phaseTypeFromEventPayload extracts phase_type / phase from a payload.
func phaseTypeFromEventPayload(payload map[string]any) domain.PhaseType {
	if payload == nil {
		return ""
	}
	if s, ok := payload["phase_type"].(string); ok {
		return domain.PhaseType(s)
	}
	if s, ok := payload["phase"].(string); ok {
		return domain.PhaseType(s)
	}
	return ""
}

// phaseOrdinalAfter reports whether a's index in AllPhases() is strictly
// greater than b's.
func phaseOrdinalAfter(a, b domain.PhaseType) bool {
	idxA, idxB := -1, -1
	for i, pt := range domain.AllPhases() {
		if pt == a {
			idxA = i
		}
		if pt == b {
			idxB = i
		}
	}
	if idxA < 0 || idxB < 0 {
		return false
	}
	return idxA > idxB
}
