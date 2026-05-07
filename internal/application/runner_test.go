package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

type recordingSink struct {
	Snapshots []*domain.Change
	Events    []domain.Event
	Gates     []domain.ApprovalGate
	Errors    []error
	Final     domain.ChangeStatus
	closed    bool
}

func (s *recordingSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	cp := *c
	s.Snapshots = append(s.Snapshots, &cp)
	return nil
}
func (s *recordingSink) OnEvent(_ context.Context, e domain.Event) error {
	s.Events = append(s.Events, e)
	return nil
}
func (s *recordingSink) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	s.Gates = append(s.Gates, g)
	return nil
}
func (s *recordingSink) OnError(_ context.Context, err error) error {
	s.Errors = append(s.Errors, err)
	return nil
}
func (s *recordingSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	s.Final = st
	return nil
}
func (s *recordingSink) Close() error { s.closed = true; return nil }

func newRunner(orch *fakes.FakeOrchestrator, stream *fakes.FakeEventStream, sink *recordingSink) (*application.Runner, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	r := application.NewRunner(application.RunnerDeps{
		Orch:        orch,
		State:       state,
		Git:         fakes.NewFakeGitInspector(),
		Sink:        sink,
		EventStream: stream,
	}, application.RunnerOptions{
		SnapshotRefreshTimeout: time.Second,
	})
	return r, state
}

func TestRunnerCreatesAndConsumesSSEUntilTerminalEvent(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, state := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-2"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	res, err := r.Run(context.Background(), application.RunInput{
		Project:       "p",
		Message:       "msg",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if len(sink.Events) < 2 {
		t.Errorf("expected ≥2 events, got %d", len(sink.Events))
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete final = %q", sink.Final)
	}

	gid, _ := state.GetGlobalLast(context.Background())
	if gid != res.ChangeID {
		t.Errorf("global last = %q, want %q", gid, res.ChangeID)
	}
}

func TestRunnerTranslatesApprovalRequiredEventToOnApprovalGate(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{
				Type:    "approval.required",
				EventID: "gate-1",
				Payload: map[string]any{
					"gate_url": "http://gate/1",
					"reason":   "policy",
					"risk":     "high",
					"policy":   "manual",
					"phase":    "apply",
				},
			})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.Gates) != 1 {
		t.Fatalf("expected 1 ApprovalGate, got %d", len(sink.Gates))
	}
	if sink.Gates[0].URL != "http://gate/1" {
		t.Errorf("gate URL = %q", sink.Gates[0].URL)
	}
	saw := false
	for _, ev := range sink.Events {
		if ev.Type == "approval.required" {
			saw = true
		}
	}
	if !saw {
		t.Error("approval.required should also be emitted via OnEvent")
	}
}

func TestRunnerSkipsHeartbeatEvents(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "heartbeat", EventID: "hb-1"})
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	if _, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	}); err != nil {
		t.Fatal(err)
	}
	for _, ev := range sink.Events {
		if ev.Type == "heartbeat" {
			t.Errorf("heartbeat should not reach OnEvent: %+v", ev)
		}
	}
}

func TestRunnerExitCode4WhenStreamEndsBeforeTerminal(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{Type: "phase.started", EventID: "evt-1"})
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}

func TestRunnerExitCode1OnTerminalFailureViaSnapshot(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusFailed)
			stream.Close(target)
		}()
	}

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 1 {
		t.Errorf("Code = %d, want 1", exit.Code)
	}
}

func TestRunnerExitCode3OnCreateFailure(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.CreateErr = errors.New("orchestrator unreachable")
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestRunnerCanceledContextReturnsExit4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(_ outbound.StreamTarget) {
		// Never push, never close — let ctx cancel us out.
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := r.Run(ctx, application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}

func TestRunnerInputRequiresProjectAndMessage(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	if _, err := r.Run(context.Background(), application.RunInput{Message: ""}); err == nil {
		t.Error("expected error on empty message")
	}
	if _, err := r.Run(context.Background(), application.RunInput{Message: "m"}); err == nil {
		t.Error("expected error on empty project")
	}
}

// ExitError.Error() / Unwrap() --------------------------------------------------

func TestExitErrorErrorWithCause(t *testing.T) {
	cause := errors.New("orchestrator unreachable")
	e := &application.ExitError{Code: 3, Err: cause}
	want := "exit code 3: orchestrator unreachable"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestExitErrorErrorWithoutCause(t *testing.T) {
	e := &application.ExitError{Code: 0}
	want := "exit code 0"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestExitErrorUnwrap(t *testing.T) {
	cause := errors.New("inner")
	e := &application.ExitError{Code: 1, Err: cause}
	if got := e.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

// Regression: when ctx is canceled mid-run, the sink must learn about it via
// OnError before the runner returns. We use a sink that records errors and
// verify OnError fired even though ctx is canceled.
func TestRunnerCancellationFiresOnError(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	stream.OnSubscribe = func(_ outbound.StreamTarget) {
		// Never push, never close — ctx cancel will end us.
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, _ = r.Run(ctx, application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if len(sink.Errors) == 0 {
		t.Error("expected OnError to fire on ctx cancellation")
	}
}

func TestObserveCallsOnSnapshotThenStreamThenFinish(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	existing := &domain.Change{ID: "ATTACH-1", Project: "p", Status: domain.ChangeStatusRunning}
	orch.SeedChange(existing)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		if target.ChangeID != existing.ID {
			t.Errorf("Subscribe target = %q, want %q", target.ChangeID, existing.ID)
		}
		go func() {
			stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-1"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	res, err := r.Observe(context.Background(), application.RunResult{ChangeID: existing.ID}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q, want done", res.FinalStatus)
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete final = %q", sink.Final)
	}
}

func TestObserveTerminalSnapshotShortCircuits(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	subscribed := false
	stream.OnSubscribe = func(_ outbound.StreamTarget) { subscribed = true }

	res, err := r.Observe(context.Background(), application.RunResult{
		ChangeID:    "TERM-1",
		FinalStatus: domain.ChangeStatusDone,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if subscribed {
		t.Error("Observe should not subscribe when caller signals terminal status")
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete should still fire even on the short-circuit path")
	}
}

func TestObserveExitCode4OnStreamEndsWithoutTerminal(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	orch.SeedChange(&domain.Change{ID: "X", Status: domain.ChangeStatusRunning})

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go stream.Close(target) // close immediately, no events, no terminal
	}

	_, err := r.Observe(context.Background(), application.RunResult{ChangeID: "X"}, sink)
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}
