//go:build contract

package contract_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/ssestream"
	"github.com/RVRTelecomunicaciones/sophia/internal/application"
	"github.com/RVRTelecomunicaciones/sophia/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

// CLI-smoke contract tests exercise the cli's application services
// (Approver, Aborter, Lister, Runner, DoctorService) end-to-end against
// the synthetic spec server. Each test corresponds to one user-facing
// command (Phase 5 scope item 8: doctor, run, attach, changes, status,
// approve, reject, abort).
//
// Pre-Phase-7 (no real orchestrator binary smoke yet), these tests
// stand in for the "cli command works against the wire" gate. When the
// orchestrator binary becomes spawnable in CI (Phase 7 + Postgres),
// the same scenarios should run against a real instance — see
// HARNESS.md.

// --- doctor ---

func TestSmoke_DoctorReportsHealthOK(t *testing.T) {
	srv := newSpecServer().start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	doctor := application.NewDoctorService(application.DoctorDeps{
		Compose: fakes.NewFakeComposeRunner(),
		Git:     fakes.NewFakeGitInspector(),
		Paths:   fakes.NewFakePathResolver(),
		Orch:    orch,
	})
	report := doctor.Run(context.Background())
	for _, c := range report.Checks {
		if c.ID == "orchestrator" && c.Level != application.LevelOK {
			t.Errorf("orchestrator check = %q (%s)", c.Level, c.Detail)
		}
	}
}

// --- changes (list / abort) ---

func TestSmoke_ChangesList(t *testing.T) {
	spec := newSpecServer()
	spec.changes["01CONTRACT0001"] = contract.ChangeResponse{
		ChangeID: "01CONTRACT0001", Name: "x", Project: "demo", Status: "running",
	}
	srv := spec.start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	lister := application.NewLister(application.ListerDeps{Orch: orch})
	out, err := lister.List(context.Background(), application.ListInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 change, got %d", len(out))
	}
}

// --- approve / reject (idempotent on gate_already_decided) ---

func TestSmoke_Approve_Idempotent(t *testing.T) {
	spec := newSpecServer()
	spec.approveBehavior = approveGateAlreadyDone
	srv := spec.start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	approver := application.NewApprover(application.ApproverDeps{Orch: orch})
	err := approver.Approve(context.Background(), application.ApprovalInput{
		PhaseID: "01PH", Approver: "alice",
	})
	if !errors.Is(err, domain.ErrGateAlreadyDecided) {
		t.Fatalf("expected ErrGateAlreadyDecided, got %v", err)
	}
}

func TestSmoke_Reject_HappyPath(t *testing.T) {
	spec := newSpecServer()
	srv := spec.start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	approver := application.NewApprover(application.ApproverDeps{Orch: orch})
	if err := approver.Reject(context.Background(), application.ApprovalInput{
		PhaseID: "01PH", Approver: "alice", Reason: "bad",
	}); err != nil {
		t.Fatalf("Reject: %v", err)
	}
}

// --- abort ---

func TestSmoke_Abort_Idempotent(t *testing.T) {
	spec := newSpecServer()
	spec.abortBehavior = abortAlreadyTerminal
	srv := spec.start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	aborter := application.NewAborter(application.AborterDeps{Orch: orch})
	err := aborter.Abort(context.Background(), application.AbortInput{
		ChangeID: "01CONTRACT0001",
	})
	if !errors.Is(err, domain.ErrChangeAlreadyTerminal) {
		t.Fatalf("expected ErrChangeAlreadyTerminal, got %v", err)
	}
}

// --- run / attach (multiplexer + SSE end-to-end) ---

func TestSmoke_Run_StreamsThenFinishesDone(t *testing.T) {
	spec := newSpecServer()
	// Pre-seed: the runner.Run path POSTs CreateChange first; the
	// synthetic server returns "01CONTRACT0001" with status=running +
	// CurrentPhaseID="01CONTRACT0001PHASE". We pre-queue events on
	// that phase id so the multiplexer's first snapshot sees a
	// streamable phase.
	srv := spec.start()
	defer srv.Close()

	const phaseID = "01CONTRACT0001PHASE"
	const changeID = "01CONTRACT0001"

	spec.phaseEvents[phaseID] = []sseEvent{
		{Type: contract.EventPhaseStarted, ID: "e1", Payload: map[string]any{
			"phase_id": phaseID, "phase_type": "spec",
		}},
		{Type: contract.EventPhaseCompleted, ID: "e2", Payload: map[string]any{
			"phase_id": phaseID, "phase_type": "spec",
		}},
	}
	// When the SSE stream finishes, (a) flip the change to terminal,
	// and (b) mark the phase as terminal so any subsequent reconnect
	// receives 410 phase_terminal_no_events and the SSE client closes
	// its channel cleanly. The multiplexer then snapshots, sees the
	// done status, and finishes the run.
	spec.afterStream = func(pid string) {
		if pid != phaseID {
			return
		}
		spec.mu.Lock()
		c := spec.changes[changeID]
		c.Status = "done"
		spec.changes[changeID] = c
		spec.terminalPhases[phaseID] = true
		spec.mu.Unlock()
	}

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	stream := ssestream.New(ssestream.Config{
		BaseURL: srv.URL, Heartbeat: 5 * time.Second, MaxRetries: 1,
	})
	state := fakes.NewFakeStateStore()
	sink := newRecordingSink()
	runner := application.NewRunner(application.RunnerDeps{
		Orch:        orch,
		State:       state,
		Git:         fakes.NewFakeGitInspector(),
		Sink:        sink,
		EventStream: stream,
	}, application.RunnerOptions{SnapshotRefreshTimeout: 2 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := runner.Run(ctx, application.RunInput{
		Project: "demo", Message: "feat-x", BaseRef: "main",
		ArtifactStore: domain.ArtifactStoreMemoryEngine,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q, want done", res.FinalStatus)
	}
	if !sink.SawEventType(contract.EventPhaseStarted) {
		t.Error("sink did not receive phase.started")
	}
	if !sink.SawEventType(contract.EventPhaseCompleted) {
		t.Error("sink did not receive phase.completed")
	}
}

// --- status / attach (read-only snapshot path) ---

func TestSmoke_Attach_RetrievesSnapshot(t *testing.T) {
	spec := newSpecServer()
	spec.changes["01CH"] = contract.ChangeResponse{
		ChangeID: "01CH", Name: "x", Project: "demo",
		Status: "done", CurrentPhaseID: "01PH",
	}
	srv := spec.start()
	defer srv.Close()

	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := orch.GetChange(context.Background(), domain.ChangeID("01CH"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.ChangeStatusDone {
		t.Errorf("status = %q", got.Status)
	}
}
