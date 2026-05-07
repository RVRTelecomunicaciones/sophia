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

func newAttacher(orch *fakes.FakeOrchestrator, stream *fakes.FakeEventStream, sink *recordingSink) (*application.Attacher, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	runner := application.NewRunner(application.RunnerDeps{
		Orch:        orch,
		State:       state,
		Git:         git,
		Sink:        sink,
		EventStream: stream,
	}, application.RunnerOptions{SnapshotRefreshTimeout: time.Second})
	a := application.NewAttacher(application.AttacherDeps{
		Orch:   orch,
		State:  state,
		Git:    git,
		Runner: runner,
	})
	return a, state
}

func TestAttacherFetchesSnapshotPersistsAndObserves(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, state := newAttacher(orch, stream, sink)

	orch.SeedChange(&domain.Change{ID: "ATT-1", Project: "p", Status: domain.ChangeStatusRunning})

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		if target.ChangeID != "ATT-1" {
			t.Errorf("Subscribe target = %q, want ATT-1", target.ChangeID)
		}
		go func() {
			stream.Push(target, domain.Event{Type: "phase.completed", EventID: "evt-1"})
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	res, err := a.Attach(context.Background(), application.AttachInput{
		ChangeID: "ATT-1",
		Project:  "p",
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if res.ChangeID != "ATT-1" {
		t.Errorf("ChangeID = %q", res.ChangeID)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}

	if len(sink.Snapshots) == 0 {
		t.Fatal("expected at least one snapshot delivered to sink")
	}
	if sink.Snapshots[0].ID != "ATT-1" {
		t.Errorf("first snapshot ID = %q", sink.Snapshots[0].ID)
	}

	gid, _ := state.GetGlobalLast(context.Background())
	if gid != "ATT-1" {
		t.Errorf("global last = %q, want ATT-1", gid)
	}
}

func TestAttacherTerminalOnArrivalShortCircuits(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, _ := newAttacher(orch, stream, sink)

	orch.SeedChange(&domain.Change{ID: "ATT-DONE", Project: "p", Status: domain.ChangeStatusDone})

	subscribed := false
	stream.OnSubscribe = func(_ outbound.StreamTarget) { subscribed = true }

	res, err := a.Attach(context.Background(), application.AttachInput{
		ChangeID: "ATT-DONE",
		Project:  "p",
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if subscribed {
		t.Error("Attacher should not subscribe to a terminal Change")
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q, want done", res.FinalStatus)
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Error("OnComplete should fire on terminal-on-arrival")
	}
}

func TestAttacherChangeNotFoundExitCode3(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, _ := newAttacher(orch, stream, sink)

	_, err := a.Attach(context.Background(), application.AttachInput{
		ChangeID: "MISSING",
		Project:  "p",
	}, sink)
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3 (orchestrator/changeNotFound)", exit.Code)
	}
	if !errors.Is(err, domain.ErrChangeNotFound) {
		t.Errorf("expected wrapped ErrChangeNotFound; got %v", err)
	}
}

func TestAttacherEmptyChangeIDExitCode3(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, _ := newAttacher(orch, stream, sink)

	_, err := a.Attach(context.Background(), application.AttachInput{
		ChangeID: "",
		Project:  "p",
	}, sink)
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestAttacherCtxCancelDuringObserveExitCode4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, _ := newAttacher(orch, stream, sink)

	orch.SeedChange(&domain.Change{ID: "ATT-RUN", Project: "p", Status: domain.ChangeStatusRunning})

	subscribed := make(chan struct{})
	stream.OnSubscribe = func(_ outbound.StreamTarget) {
		close(subscribed)
		// Never push events — the subscribe channel stays open until ctx fires.
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-subscribed
		cancel()
	}()

	_, err := a.Attach(ctx, application.AttachInput{
		ChangeID: "ATT-RUN",
		Project:  "p",
	}, sink)
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4 (transient/ctx)", exit.Code)
	}
}

func TestAttacherUsesProvidedSink(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	construction := &recordingSink{}
	caller := &recordingSink{}

	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	runner := application.NewRunner(application.RunnerDeps{
		Orch: orch, State: state, Git: git, Sink: construction, EventStream: stream,
	}, application.RunnerOptions{SnapshotRefreshTimeout: time.Second})
	a := application.NewAttacher(application.AttacherDeps{
		Orch: orch, State: state, Git: git, Runner: runner,
	})

	orch.SeedChange(&domain.Change{ID: "X", Project: "p", Status: domain.ChangeStatusRunning})
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	if _, err := a.Attach(context.Background(), application.AttachInput{ChangeID: "X", Project: "p"}, caller); err != nil {
		t.Fatal(err)
	}
	if len(caller.Snapshots) == 0 {
		t.Error("caller's sink should have received the snapshot")
	}
	if caller.Final != domain.ChangeStatusDone {
		t.Error("caller's sink should have received OnComplete")
	}
	if len(construction.Snapshots) != 0 {
		t.Errorf("construction-time sink should NOT receive events, got %d snapshots", len(construction.Snapshots))
	}
}

// AttachFromSnapshot tests follow.

func TestAttacherFromSnapshotSkipsGetChange(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, state := newAttacher(orch, stream, sink)

	getChangeCalls := 0
	orch.OnGetChange = func(domain.ChangeID) { getChangeCalls++ }

	subscribed := false
	stream.OnSubscribe = func(_ outbound.StreamTarget) { subscribed = true }

	// A pre-fetched terminal snapshot exercises the AttachFromSnapshot short-
	// circuit: persist + OnSnapshot + Observe-short-circuit (Observe sees a
	// terminal FinalStatus and skips both Subscribe and the post-stream
	// refresh). Result: zero GetChange calls — the property that makes
	// AttachFromSnapshot the right primitive for cli.attachJSONL's eager-arm
	// path (D-M8-13).
	snap := &domain.Change{ID: "PRE-FETCHED", Project: "p", Status: domain.ChangeStatusDone}

	res, err := a.AttachFromSnapshot(context.Background(), snap, "p", sink)
	if err != nil {
		t.Fatal(err)
	}
	if getChangeCalls != 0 {
		t.Errorf("AttachFromSnapshot must NOT call GetChange (got %d calls)", getChangeCalls)
	}
	if subscribed {
		t.Error("AttachFromSnapshot must NOT subscribe when snapshot is terminal")
	}
	if res.ChangeID != "PRE-FETCHED" {
		t.Errorf("ChangeID = %q, want PRE-FETCHED", res.ChangeID)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if len(sink.Snapshots) == 0 || sink.Snapshots[0].ID != "PRE-FETCHED" {
		t.Errorf("snapshot not forwarded to sink: %+v", sink.Snapshots)
	}
	gid, _ := state.GetGlobalLast(context.Background())
	if gid != "PRE-FETCHED" {
		t.Errorf("global last = %q, want PRE-FETCHED", gid)
	}
}

func TestAttacherFromSnapshotNilSnapshotExitCode3(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, _ := newAttacher(orch, stream, sink)

	_, err := a.AttachFromSnapshot(context.Background(), nil, "p", sink)
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}
