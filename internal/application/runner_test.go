package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

type recordingSink struct {
	Snapshots []*domain.Change
	Events    []domain.Event
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
func (s *recordingSink) OnApprovalGate(_ context.Context, _ domain.ApprovalGate) error { return nil }
func (s *recordingSink) OnError(_ context.Context, err error) error {
	s.Errors = append(s.Errors, err)
	return nil
}
func (s *recordingSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	s.Final = st
	return nil
}
func (s *recordingSink) Close() error { s.closed = true; return nil }

func newRunner(orch *fakes.FakeOrchestrator, sink *recordingSink) (*application.Runner, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	r := application.NewRunner(application.RunnerDeps{
		Orch:  orch,
		State: state,
		Git:   fakes.NewFakeGitInspector(),
		Sink:  sink,
	}, application.RunnerOptions{
		PollMin: time.Millisecond,
		PollMax: 5 * time.Millisecond,
	})
	return r, state
}

func TestRunnerCreatesAndPollsUntilDone(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, state := newRunner(orch, sink)

	first := true
	orch.TickHook = func(c *domain.Change) {
		if first {
			c.Status = domain.ChangeStatusRunning
			first = false
		} else {
			c.Status = domain.ChangeStatusDone
		}
	}

	res, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if len(sink.Snapshots) < 2 {
		t.Errorf("expected ≥2 snapshots, got %d", len(sink.Snapshots))
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete final = %q", sink.Final)
	}

	// Verify last_change_id persisted globally.
	gid, _ := state.GetGlobalLast(context.Background())
	if gid != res.ChangeID {
		t.Errorf("global last = %q, want %q", gid, res.ChangeID)
	}
}

func TestRunnerExitCodeOnFailedTerminalStatus(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	orch.TickHook = func(c *domain.Change) { c.Status = domain.ChangeStatusFailed }

	res, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err == nil {
		t.Fatal("expected ExitError for failed terminal status")
	}
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exit.Code != 1 {
		t.Errorf("Code = %d, want 1", exit.Code)
	}
	if res.FinalStatus != domain.ChangeStatusFailed {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
}

func TestRunnerExitCode3OnCreateFailure(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.CreateErr = errors.New("orchestrator unreachable")
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

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
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	orch.GetBlockUntilCancel = true

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
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
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	_, err := r.Run(context.Background(), application.RunInput{Message: ""})
	if err == nil {
		t.Error("expected error on empty message")
	}
	_, err = r.Run(context.Background(), application.RunInput{Message: "m"})
	if err == nil {
		t.Error("expected error on empty project")
	}
}
