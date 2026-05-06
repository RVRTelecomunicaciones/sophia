# Sophia CLI — M8 Attach + Changes + Real-Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the three remaining read-side commands of the v1 spec on top of the M5 SSE pipeline and the M6/M7 TUI. `sophia attach <change-id>` opens the same Timeline + ApplyBoard + ApprovalBanner experience as `sophia run`, but starts from a snapshot of an existing Change (no `CreateChange`) and observes it until terminal status. `sophia changes [--limit N] [--status S] [--project P] [--json]` lists recent Changes with sensible project-default scoping. `sophia status [<change-id>] [--json]` upgrades the M3 placeholder to fetch the real Change snapshot from the orchestrator using the spec §2.5 resolution order: `<change-id>` arg → project-scoped `last_change_id` → global `last_change_id` → empty (exit 0). `attach` and `run` BOTH update both the project-scoped and global `last_change_id` files atomically (spec §3.5). The architectural backbone is a refactor of `application.Runner`: the post-create observation loop (sink.OnSnapshot → stream → refresh-after-stream-end → finish) is extracted into a public `Runner.Observe(ctx, RunResult, sink)` method that `Run` invokes after `CreateChange` and the new `Attacher` invokes after `GetChange`. One source of truth, two entry points. No new outbound port, no new domain type — every M8 capability slots into the surface the M1–M7 milestones already shipped.

**Architecture:** Three new application services (`Lister`, `Attacher`, upgraded `StatusReader`) plus three real CLI commands replacing the M3/M7 stubs. The Runner is split in half: `Run` keeps its public `(ctx, RunInput) (RunResult, error)` signature but its body shrinks to "validate → CreateChange → persistChangeID → OnSnapshot → Observe → finish-or-bubble"; the new `Observe(ctx, RunResult, sink) (RunResult, error)` method holds the stream/refresh/finish logic. `Attacher.Attach(ctx, changeID, sink)` does "validate → GetChange → persistChangeID → OnSnapshot → Observe". `cli/attach.go` reuses the same `--no-tui --json` mode validation, the same `approvalTimeoutSink` wrapper, and the same `tui.NewProgram` path as `cli/run.go`. `cli/changes.go` is a thin presentation layer over `Lister` with a column-aligned table printer and a JSON array printer. `cli/status.go` becomes a small dispatcher that calls the upgraded `StatusReader.Resolve(ctx, ResolveInput)` and prints either a human summary or `ChangeResponse`-shaped JSON. Bootstrap wiring grows three lines: a `Lister`, an `AttacherFactory` (sink-injected at command time, mirroring `RunnerFactory`), and a `StatusReader` that now takes an `OrchestratorClient` dependency. End-to-end coverage is one new build-tag-gated test (`attach_workflow_test.go`) that drives a `run → detach → attach → done` cycle against an extended `httptest` orchestrator stub.

**Tech Stack:** Go 1.26.2 (per repo `go.mod`) · `charm.land/bubbletea/v2` v2.0.6 · `charm.land/lipgloss/v2` v2.0.3 (M6/M7 TUI dependencies; M8 reuses without modification). No new dependencies.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.1 commands, §2.2 attach snapshot+stream, §2.3 exit codes, §2.4 changes table, §2.5 status resolution order, §3.5 last_change_id state invariants, §5.1 OrchestratorClient surface, §7.2 M8 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M8)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M8 boundaries — what is NOT in M8:**

- No cross-process `Last-Event-ID` resume across `attach` invocations → M9. `attach` always opens a fresh subscription; the orchestrator decides what to replay.
- No `--orchestrator-url` per-call rebinding for `attach`/`status`/`changes` → M9+. The bootstrap-time URL (env or default) is the only knob.
- No `sophia changes` advanced filters: date range, agent role, free-text search, sorted output → M9+. M8 ships `--limit`, `--status`, `--project`, `--json` only.
- No `sophia status --watch` for periodic refresh → M9+. Status is one-shot.
- No pagination UI for `changes` (next/prev page) → M9+. `--limit N` and `--offset N` are the only knobs (offset deferred too if not strictly needed).
- No `sophia status` rendering of approval gate state — M8 exposes whatever `ChangeResponse` already carries through the existing JSON; no extra rendering.
- No new outbound port — `outbound.OrchestratorClient.GetChange` / `ListChanges` already exist (M4). `outbound.StateStore.GetGlobalLast` / `GetLast` already exist (M3).
- No new domain types — `domain.Change`, `domain.ChangeStatus`, `domain.ChangeID`, `domain.Phase` are reused.
- No alternative attach path for terminal Changes — when `GetChange` returns a Change whose `Status.IsTerminal()`, `Attacher` short-circuits to `OnComplete + finish` without subscribing.
- No `attach` against a Change ID that has not been seen by the orchestrator — `GetChange` returns `ErrChangeNotFound` → exit 3.
- No new TUI views — `attach` reuses Timeline + ApplyBoard + ApprovalBanner exactly as M6/M7 ship them.

---

## Phase 1 — Application: Lister (sophia changes)

### Task 1: `application.Lister` — thin OrchestratorClient wrapper

**Files:**
- Create: `internal/application/lister.go`
- Create: `internal/application/lister_test.go`

`Lister` is the smallest service in M8. It is a pure pass-through over `OrchestratorClient.ListChanges` — it does NOT resolve project defaults from `.sophia.yaml`, does NOT apply CLI flag semantics, does NOT impose limits. It receives `Project`, `Status`, `Limit`, `Offset` exactly as the caller passes them and forwards them to the orchestrator. **All project-default resolution lives in `cli/changes.go`** (Task 5): the CLI reads `.sophia.yaml` via `ConfigResolver` to derive a default project, and passes whatever it computes (including `""` when the user explicitly opts out) directly into `ListInput`. This split keeps the application layer free of framework-specific flag heuristics and keeps the orchestrator wire shape obvious.

The list result is `[]*domain.Change` returned verbatim. No sorting (orchestrator decides), no truncation (the orchestrator honors `Limit`).

> **Verification gate:** read `internal/ports/outbound/orchestrator.go` to confirm `ListChangesFilter` shape (`Project, Status string`; `Limit, Offset int`). If the field set has changed since this plan was written, STOP and ASK before substituting names.

- [ ] **Step 1: Write the failing test**

`internal/application/lister_test.go`:

```go
package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newLister(orch *fakes.FakeOrchestrator) *application.Lister {
	return application.NewLister(application.ListerDeps{Orch: orch})
}

func TestListerForwardsFilters(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p1", Status: domain.ChangeStatusRunning})
	orch.SeedChange(&domain.Change{ID: "01H2", Project: "p2", Status: domain.ChangeStatusDone})
	orch.SeedChange(&domain.Change{ID: "01H3", Project: "p1", Status: domain.ChangeStatusDone})

	l := newLister(orch)
	got, err := l.List(context.Background(), application.ListInput{
		Project: "p1",
		Status:  string(domain.ChangeStatusDone),
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].ID != "01H3" {
		t.Errorf("ID = %q, want 01H3", got[0].ID)
	}
}

func TestListerEmptyProjectMeansNoFilter(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p1", Status: domain.ChangeStatusRunning})
	orch.SeedChange(&domain.Change{ID: "01H2", Project: "p2", Status: domain.ChangeStatusRunning})

	l := newLister(orch)
	// Project="" → no project filter is forwarded. Lister never invents a
	// default; the CLI is responsible for resolving project defaults before
	// calling List.
	got, err := l.List(context.Background(), application.ListInput{
		Project: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 changes (no project filter), got %d", len(got))
	}
}

func TestListerDefaultLimitIs10(t *testing.T) {
	// The Lister itself does NOT impose a default — the CLI layer does.
	// This test asserts that List(Limit=0) forwards Limit=0 to the
	// orchestrator (i.e. server decides). Defaulting is the cli's job.
	orch := fakes.NewFakeOrchestrator()
	var seen outbound.ListChangesFilter
	orch.OnListChanges = func(f outbound.ListChangesFilter) {
		seen = f
	}
	orch.SeedChange(&domain.Change{ID: "01H1", Project: "p"})

	l := newLister(orch)
	if _, err := l.List(context.Background(), application.ListInput{Project: "p"}); err != nil {
		t.Fatal(err)
	}
	if seen.Limit != 0 {
		t.Errorf("Limit forwarded as %d; expected 0 (cli applies default, not Lister)", seen.Limit)
	}
}

func TestListerSurfacesOrchestratorError(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.ListErr = errors.New("boom")

	l := newLister(orch)
	_, err := l.List(context.Background(), application.ListInput{Project: "p"})
	if err == nil {
		t.Fatal("expected error from orchestrator")
	}
}
```

> **Note:** The tests above reference `FakeOrchestrator.OnListChanges` (a hook for asserting the forwarded filter) and `FakeOrchestrator.ListErr` (a synthetic error path). If those fields don't yet exist on the fake, add them inline as part of this task — they slot next to the existing `CreateErr`, `HealthzErr`, `TickHook` hooks. Concretely, append to `test/fakes/orchestrator.go`:
>
> ```go
> // fields on FakeOrchestrator:
> ListErr       error
> OnListChanges func(outbound.ListChangesFilter)
> ```
>
> and update `ListChanges` to honor both:
>
> ```go
> func (f *FakeOrchestrator) ListChanges(_ context.Context, filter outbound.ListChangesFilter) ([]*domain.Change, error) {
>     if f.OnListChanges != nil {
>         f.OnListChanges(filter)
>     }
>     if f.ListErr != nil {
>         return nil, f.ListErr
>     }
>     // ... existing body
> }
> ```

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL — `application.Lister`, `application.NewLister`, `application.ListerDeps`, `application.ListInput`, `Lister.List` undefined.

- [ ] **Step 3: Implement**

`internal/application/lister.go`:

```go
package application

import (
	"context"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ListerDeps groups the ports the Lister needs.
type ListerDeps struct {
	Orch outbound.OrchestratorClient
}

// ListInput controls List. All four fields are forwarded verbatim to
// OrchestratorClient.ListChanges. Lister does NOT resolve project defaults,
// does NOT impose limits, does NOT translate empty strings into wildcards.
// The CLI layer (cli/changes.go) is responsible for any project-default
// resolution from .sophia.yaml before invoking List.
type ListInput struct {
	Project string
	Status  string
	Limit   int
	Offset  int
}

// Lister implements `sophia changes`.
type Lister struct {
	deps ListerDeps
}

// NewLister constructs a Lister.
func NewLister(d ListerDeps) *Lister { return &Lister{deps: d} }

// List queries the orchestrator and returns the matching Changes.
func (l *Lister) List(ctx context.Context, in ListInput) ([]*domain.Change, error) {
	if l.deps.Orch == nil {
		return nil, fmt.Errorf("lister: orchestrator client not wired")
	}
	filter := outbound.ListChangesFilter{
		Project: in.Project,
		Status:  in.Status,
		Limit:   in.Limit,
		Offset:  in.Offset,
	}
	out, err := l.deps.Orch.ListChanges(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS. The four `Lister` tests pass and existing M3–M7 application tests stay green.

- [ ] **Step 5: Commit**

```bash
git add internal/application/lister.go \
        internal/application/lister_test.go \
        test/fakes/orchestrator.go
git commit -m "feat(application): add Lister service for sophia changes (M8)"
```

---

## Phase 2 — Application: Runner.Observe extraction (shared observe loop)

### Task 2: Refactor `application.Runner` to expose `Observe(ctx, RunResult, sink) (RunResult, error)`

**Files:**
- Modify: `internal/application/runner.go`
- Modify: `internal/application/runner_test.go` (extend — existing tests stay green)

The post-create observation loop in `Runner.Run` (lines roughly 121–138 of M7's `runner.go`: `OnSnapshot → terminal-short-circuit → stream → refresh → finish`) is the EXACT logic `Attacher` needs after its `GetChange`. Rather than duplicate it, M8 extracts that block into a new exported method `Runner.Observe(ctx, res, sink) (RunResult, error)`. The signature is chosen so:

1. `Run` keeps its public API unchanged. Callers see `RunResult, error` exactly as before.
2. `Observe` accepts a `RunResult` rather than a bare `ChangeID` so the caller controls how the result is initialized (Run sets it from CreateChange's response; Attach sets it from GetChange's response).
3. `Observe` accepts an `inbound.EventSink` argument distinct from `r.deps.Sink`. This sounds redundant (both Run and Attach use the same sink that's already in deps), but having it explicit means `Attacher` can call `Observe` without needing to mutate or rebuild the `Runner` struct. Cleaner test surface, too.

The internal helpers (`stream`, `dispatchEvent`, `approvalGateFromEvent`, `refreshAfterStreamEnd`, `finish`, `persistChangeID`) stay private. `Observe` orchestrates them. `Run`'s body shrinks to validation + `CreateChange` + persist + initial `OnSnapshot` + `Observe`.

> **Verification gate:** before refactoring, confirm the helper names referenced in this plan still exist in `internal/application/runner.go`:
>
> - `stream(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error)`
> - `dispatchEvent(ctx, ev)`
> - `refreshAfterStreamEnd(ctx, id)`
> - `finish(ctx, res, st)`
> - `persistChangeID(ctx, project, id)`
> - `approvalGateFromEvent(ev)`
>
> If any of these have been renamed or merged since this plan was written, STOP and update the rename in this task's edits. The behavior contract (the test list below) is the invariant.

- [ ] **Step 1: Add the failing test**

Append to `internal/application/runner_test.go` (alongside the existing M5/M7 tests):

```go
func TestObserveCallsOnSnapshotThenStreamThenFinish(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	// Seed an existing Change as if attach had just done GetChange.
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
	// If the caller passes a result whose ChangeID points to an already-
	// terminal Change, Observe must NOT subscribe — it should call
	// finish() directly. Concretely: the FakeEventStream Subscribe hook
	// must NEVER fire.
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	r, _ := newRunner(orch, stream, sink)

	subscribed := false
	stream.OnSubscribe = func(_ outbound.StreamTarget) { subscribed = true }

	// Caller's responsibility to populate FinalStatus when terminal-on-arrival.
	// Observe trusts the caller — it does NOT re-fetch the snapshot.
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

func TestRunStillCallsObserveBehavior(t *testing.T) {
	// Existing M5 test in spirit: Run still goes create → snapshot →
	// stream → terminal. We don't need a NEW test for this; the existing
	// TestRunnerCreatesAndConsumesSSEUntilTerminalEvent covers it. This
	// "test" is a comment to document the expectation — the refactor
	// must not regress M5/M7 behavior.
	t.Skip("documented by TestRunnerCreatesAndConsumesSSEUntilTerminalEvent")
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL — `Runner.Observe` undefined.

- [ ] **Step 3: Refactor runner.go**

Modify `internal/application/runner.go`. REPLACE the body of `Run` from the line `created, err := r.deps.Orch.CreateChange(...)` through the closing brace, AND ADD a new exported `Observe` method.

The new structure of `Run`:

```go
// Run creates a Change and observes it via SSE until terminal status.
// Returns RunResult and either nil (DONE) or *ExitError with the spec code.
func (r *Runner) Run(ctx context.Context, in RunInput) (RunResult, error) {
	if in.Message == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: --message required")}
	}
	if in.Project == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: project not set")}
	}
	if r.deps.EventStream == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: event stream not wired")}
	}
	if in.ArtifactStore == "" {
		in.ArtifactStore = domain.ArtifactStoreEngram
	}
	if in.BaseRef == "" {
		in.BaseRef = "main"
	}

	defer r.deps.Sink.Close() //nolint:errcheck // best-effort

	created, err := r.deps.Orch.CreateChange(ctx, outbound.CreateChangeInput{
		Name:              in.Message,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: string(in.ArtifactStore),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = r.deps.Sink.OnError(context.WithoutCancel(ctx), err)
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
		return RunResult{}, &ExitError{Code: 3, Err: err}
	}

	res := RunResult{ChangeID: created.ID}
	if err := r.persistChangeID(ctx, in.Project, created.ID); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if err := r.deps.Sink.OnSnapshot(ctx, created); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}

	if created.Status.IsTerminal() {
		// Caller-side short-circuit: prime res.FinalStatus and let Observe
		// take the same path attach uses for terminal-on-arrival.
		res.FinalStatus = created.Status
	}

	return r.Observe(ctx, res, r.deps.Sink)
}
```

ADD the new `Observe` method directly below `Run`:

```go
// Observe drives the post-create observation loop on an existing or just-
// created Change. The caller is responsible for:
//
//   - calling OnSnapshot with the initial Change snapshot (Run does this
//     after CreateChange; Attacher does it after GetChange);
//   - persisting last_change_id (Run / Attacher both do this).
//
// Observe will:
//
//   - if res.FinalStatus is already set to a terminal status, fire
//     OnComplete + finish() (no SSE subscription, no extra HTTP);
//   - otherwise subscribe to the SSE feed for res.ChangeID, dispatch
//     events to the sink, and on stream-end refresh the snapshot to
//     determine terminal status.
//
// The returned RunResult carries the final status. The error is nil on
// terminal=DONE, *ExitError otherwise (per spec §2.3).
//
// Note: Observe does NOT re-call OnSnapshot or persistChangeID. Those are
// caller responsibilities — they precede Observe.
func (r *Runner) Observe(ctx context.Context, res RunResult, sink inbound.EventSink) (RunResult, error) {
	if res.FinalStatus.IsTerminal() {
		return r.finishWithSink(ctx, res, res.FinalStatus, sink)
	}

	final, err := r.streamWithSink(ctx, res.ChangeID, sink)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = sink.OnError(context.WithoutCancel(ctx), err)
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = sink.OnError(ctx, err)
		return res, &ExitError{Code: 4, Err: err}
	}
	return r.finishWithSink(ctx, res, final, sink)
}
```

Now refactor `stream`, `dispatchEvent`, `refreshAfterStreamEnd`, and `finish` to take an explicit sink argument. The simplest path is to add `*WithSink` siblings that take the sink, and have the existing methods delegate. REPLACE the helpers with these versions:

```go
// stream is preserved for backward compatibility; it delegates to streamWithSink
// using the sink stored in deps.
func (r *Runner) stream(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	return r.streamWithSink(ctx, id, r.deps.Sink)
}

// streamWithSink subscribes to the SSE feed for id and forwards events to the
// given sink (which is normally the same as r.deps.Sink, but Attacher may pass
// a different one when reusing a Runner under a different command).
func (r *Runner) streamWithSink(ctx context.Context, id domain.ChangeID, sink inbound.EventSink) (domain.ChangeStatus, error) {
	ch, stop, err := r.deps.EventStream.Subscribe(ctx, outbound.StreamTarget{ChangeID: id}, outbound.SubscribeOptions{})
	if err != nil {
		return "", fmt.Errorf("subscribe: %w", err)
	}
	defer stop() //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return r.refreshAfterStreamEndWithSink(ctx, id, sink)
			}
			r.dispatchEventWithSink(ctx, ev, sink)
		}
	}
}

// dispatchEvent is preserved for backward compatibility.
func (r *Runner) dispatchEvent(ctx context.Context, ev domain.Event) {
	r.dispatchEventWithSink(ctx, ev, r.deps.Sink)
}

// dispatchEventWithSink mirrors dispatchEvent but forwards to a caller-supplied
// sink. Heartbeats are dropped; approval.required gets translated into
// OnApprovalGate AND emitted via OnEvent (D-M5-02).
func (r *Runner) dispatchEventWithSink(ctx context.Context, ev domain.Event, sink inbound.EventSink) {
	if ev.Type == "heartbeat" {
		return
	}
	if err := sink.OnEvent(ctx, ev); err != nil {
		_ = sink.OnError(ctx, err)
	}
	if ev.Type == "approval.required" {
		gate := approvalGateFromEvent(ev)
		if err := sink.OnApprovalGate(ctx, gate); err != nil {
			_ = sink.OnError(ctx, err)
		}
	}
}

// refreshAfterStreamEnd is preserved for backward compatibility.
func (r *Runner) refreshAfterStreamEnd(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	return r.refreshAfterStreamEndWithSink(ctx, id, r.deps.Sink)
}

// refreshAfterStreamEndWithSink mirrors refreshAfterStreamEnd but forwards
// snapshots to a caller-supplied sink.
func (r *Runner) refreshAfterStreamEndWithSink(ctx context.Context, id domain.ChangeID, sink inbound.EventSink) (domain.ChangeStatus, error) {
	rctx, cancel := context.WithTimeout(ctx, r.opts.SnapshotRefreshTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(rctx, id)
	if err != nil {
		return "", fmt.Errorf("post-stream snapshot: %w", err)
	}
	if err := sink.OnSnapshot(ctx, snap); err != nil {
		_ = sink.OnError(ctx, err)
	}
	if !snap.Status.IsTerminal() {
		return "", fmt.Errorf("stream ended before terminal status (current=%q)", snap.Status)
	}
	return snap.Status, nil
}

// finish is preserved for backward compatibility.
func (r *Runner) finish(ctx context.Context, res RunResult, st domain.ChangeStatus) (RunResult, error) {
	return r.finishWithSink(ctx, res, st, r.deps.Sink)
}

// finishWithSink mirrors finish but uses the given sink for OnComplete.
func (r *Runner) finishWithSink(ctx context.Context, res RunResult, st domain.ChangeStatus, sink inbound.EventSink) (RunResult, error) {
	res.FinalStatus = st
	_ = sink.OnComplete(ctx, st)
	switch st {
	case domain.ChangeStatusDone:
		return res, nil
	case domain.ChangeStatusBlocked, domain.ChangeStatusFailed:
		return res, &ExitError{Code: 1, Err: fmt.Errorf("change ended %s", st)}
	default:
		return res, &ExitError{Code: 4, Err: fmt.Errorf("unexpected non-terminal status %q", st)}
	}
}
```

The original `Run` body now ends at the new short-circuit + `Observe` call (already shown above). The old branch that called `r.stream` directly is GONE — replaced by `Observe`.

> **Note on terminal-on-arrival via Run:** the M5 path "CreateChange returned a Change whose Status is already terminal" is preserved by setting `res.FinalStatus = created.Status` BEFORE calling `Observe`. `Observe` then takes the short-circuit branch (`res.FinalStatus.IsTerminal()`) and calls `finishWithSink` immediately. End result is identical to M5: `OnSnapshot` fired once (in Run), `OnComplete` fired once (in Observe via finishWithSink). No double-snapshot, no extra HTTP.

- [ ] **Step 4: Run all application tests**

```bash
go test -race ./internal/application/...
```

Expected: PASS. The new `Observe` tests pass, and ALL existing M5/M7 runner tests stay green:

- `TestRunnerCreatesAndConsumesSSEUntilTerminalEvent`
- `TestRunnerTranslatesApprovalRequiredEventToOnApprovalGate`
- `TestRunnerSkipsHeartbeatEvents`
- `TestRunnerExitCode4WhenStreamEndsBeforeTerminal`
- `TestRunnerExitCode1OnTerminalFailureViaSnapshot`
- (any other Runner tests added in M5–M7)

If a previously-passing test fails, the refactor is wrong. The behavior contract is invariant; only the internal structure changed.

- [ ] **Step 5: Commit**

```bash
git add internal/application/runner.go \
        internal/application/runner_test.go
git commit -m "refactor(application): extract Runner.Observe for shared run+attach loop (M8)"
```

---

## Phase 3 — Application: Attacher (sophia attach)

### Task 3: `application.Attacher` — GetChange + persist + Observe pipeline

**Files:**
- Create: `internal/application/attacher.go`
- Create: `internal/application/attacher_test.go`

`Attacher` is the second consumer of `Runner.Observe`. It exposes **two entry points** to support D-M8-13's "approval-timeout starts at attach time" requirement without forcing a double `GetChange`:

- `Attach(ctx, AttachInput, sink)` — full pipeline. Validates the ChangeID, fetches the snapshot via `OrchestratorClient.GetChange`, then delegates to `AttachFromSnapshot`. This is the path the TUI uses (no eager-arm — TUI draws its own approval banner from snapshot data).
- `AttachFromSnapshot(ctx, snap, project, sink)` — accepts a pre-fetched snapshot and runs only the post-fetch portion (persist + OnSnapshot + Observe). The CLI's `attachJSONL` uses this so it can do its own `GetChange`, scan `snap.Phases` for `PhaseStatusBlocked`, and call `wrapped.OnApprovalGate(...)` to eager-arm `approvalTimeoutSink` BEFORE invoking the Attacher. One `GetChange`, one Observe loop.

Sequential responsibilities of the full `Attach` path:

1. Validate `changeID` is non-zero.
2. `GetChange` from the orchestrator. On 404 → ExitError{Code: 3}; on transport errors → ExitError{Code: 3} (same as Run's CreateChange handling). On ctx cancel → ExitError{Code: 4}.
3. Persist `last_change_id` (project-scoped + global) — same `persistChangeID` logic Run uses. Spec §3.5 mandates this for both `run` and `attach`.
4. Call sink.OnSnapshot with the fetched Change, then call `runner.Observe(ctx, res, sink)` to drive the rest.

`AttachFromSnapshot` is steps 3–4 only: callers that pre-fetch own validation of the snapshot.

`Attacher` is a thin service that wraps a `*Runner` (so it inherits the helpers) plus its own `OrchestratorClient` and `StateStore` references for the GetChange + persist phases. The duplication of `Orch` and `State` between `Attacher` and `Runner` is intentional: `Attacher` is constructed at command time with the same outbound adapters but a fresh sink (TUI bridge or jsonsink), and depending on the runner's deps means we're fishing them out of an already-built Runner. The cleaner shape is to construct an `Attacher` with its own deps, then delegate ONLY the observe loop to a Runner instance.

Concretely: `AttachInput` carries the change-id and an optional project for persisting (recovered from .sophia.yaml by the CLI layer). `AttachResult` is `RunResult` reused — same fields, same semantics. The sink is passed explicitly because the CLI picks TUI bridge vs jsonsink at command time, mirroring `RunnerFactory`.

> **Verification gate:** read `internal/application/runner.go` after Task 2 to confirm `Observe(ctx, RunResult, inbound.EventSink) (RunResult, error)` is exported and working. If Task 2 hasn't landed yet, this task is blocked.

- [ ] **Step 1: Write the failing test**

`internal/application/attacher_test.go`:

```go
package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
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

	// Snapshot was forwarded to the sink.
	if len(sink.Snapshots) == 0 {
		t.Fatal("expected at least one snapshot delivered to sink")
	}
	if sink.Snapshots[0].ID != "ATT-1" {
		t.Errorf("first snapshot ID = %q", sink.Snapshots[0].ID)
	}

	// last_change_id persisted globally (spec §3.5).
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

	// Snapshot is already terminal — Attacher must NOT subscribe.
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

	// Subscribe but never push anything — ctx cancel will trip the select.
	ctx, cancel := context.WithCancel(context.Background())
	stream.OnSubscribe = func(_ outbound.StreamTarget) {
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
	}

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
	// Two sinks: one that the attacher is constructed with (via Runner),
	// one that the caller passes to Attach. Caller's sink wins.
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	construction := &recordingSink{} // wired into Runner.deps.Sink at construction
	caller := &recordingSink{}       // passed to Attach()

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

// AttachFromSnapshot is the second entry point used by `cli.attachJSONL` to
// avoid a double GetChange when the CLI must scan for blocked phases (D-M8-13)
// before delegating to the Attacher. These two tests exercise that path.

func TestAttacherFromSnapshotSkipsGetChange(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	sink := &recordingSink{}
	a, state := newAttacher(orch, stream, sink)

	getChangeCalls := 0
	orch.OnGetChange = func(domain.ChangeID) { getChangeCalls++ }

	snap := &domain.Change{ID: "PRE-FETCHED", Project: "p", Status: domain.ChangeStatusRunning}
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	res, err := a.AttachFromSnapshot(context.Background(), snap, "p", sink)
	if err != nil {
		t.Fatal(err)
	}
	if getChangeCalls != 0 {
		t.Errorf("AttachFromSnapshot must NOT call GetChange (got %d calls)", getChangeCalls)
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
```

> **Note on `OnGetChange`:** the test references `FakeOrchestrator.OnGetChange` — a hook that fires on every `GetChange` call so tests can count them. If the field doesn't yet exist on the fake, append it inline as part of this task next to `OnListChanges`. Concretely:
>
> ```go
> // field on FakeOrchestrator:
> OnGetChange func(domain.ChangeID)
> ```
>
> Update `GetChange` to invoke the hook before returning.

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL — `application.Attacher`, `application.NewAttacher`, `application.AttacherDeps`, `application.AttachInput`, `Attacher.Attach`, `Attacher.AttachFromSnapshot` undefined.

- [ ] **Step 3: Implement**

`internal/application/attacher.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// AttacherDeps groups the ports the Attacher needs.
//
// Runner is the M8 shared observation engine. Attacher delegates the
// stream/refresh/finish loop to Runner.Observe; it owns only the GetChange +
// persistChangeID + initial OnSnapshot prelude.
type AttacherDeps struct {
	Orch   outbound.OrchestratorClient
	State  outbound.StateStore
	Git    outbound.GitInspector
	Runner *Runner
}

// AttachInput carries the user-facing inputs for `sophia attach`.
//
// Project is used solely for persisting project-scoped last_change_id (spec
// §3.5). It's optional: when empty (e.g. attaching outside a repo or with a
// changed cwd), Attacher persists only the global last_change_id.
type AttachInput struct {
	ChangeID domain.ChangeID
	Project  string
}

// Attacher implements `sophia attach`.
type Attacher struct {
	deps AttacherDeps
}

// NewAttacher constructs an Attacher.
func NewAttacher(d AttacherDeps) *Attacher { return &Attacher{deps: d} }

// Attach fetches the snapshot, then delegates to AttachFromSnapshot. The sink
// argument lets the CLI pick TUI bridge vs JSONL at command time, just like
// RunnerFactory does for `run`.
//
// Errors map to spec §2.3 exit codes:
//
//   - empty ChangeID                       → exit 3
//   - GetChange ErrChangeNotFound (404)    → exit 3
//   - GetChange transport / 5xx            → exit 3 (orchestrator unreachable)
//   - GetChange ctx canceled / deadline    → exit 4
//   - Observe exit codes (0/1/4)           → forwarded as-is
func (a *Attacher) Attach(ctx context.Context, in AttachInput, sink inbound.EventSink) (RunResult, error) {
	if a.deps.Runner == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: runner not wired")}
	}
	if in.ChangeID.IsZero() {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: change-id required")}
	}

	snap, err := a.deps.Orch.GetChange(ctx, in.ChangeID)
	if err != nil {
		// sink.Close is the caller's responsibility on the AttachFromSnapshot
		// path; here we close on the GetChange-failure branch only.
		defer sink.Close() //nolint:errcheck
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = sink.OnError(context.WithoutCancel(ctx), err)
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		_ = sink.OnError(ctx, err)
		return RunResult{}, &ExitError{Code: 3, Err: fmt.Errorf("get change: %w", err)}
	}

	return a.AttachFromSnapshot(ctx, snap, in.Project, sink)
}

// AttachFromSnapshot is the second entry point for `sophia attach`. It accepts
// a pre-fetched snapshot — the CLI's `attachJSONL` uses this to do its own
// `GetChange`, scan for `PhaseStatusBlocked` to eager-arm `approvalTimeoutSink`
// (D-M8-13), and only then hand the snapshot to the Attacher. This avoids a
// double `GetChange` round-trip while preserving the "approval-timeout starts
// at attach time" guarantee.
//
// Behavior matches Attach minus the GetChange + 404/timeout handling:
//
//  1. persistChangeID (spec §3.5)
//  2. sink.OnSnapshot
//  3. Runner.Observe
//
// The caller (CLI) owns sink.Close — AttachFromSnapshot defers it so a single
// AttachFromSnapshot call mirrors Attach's lifecycle exactly. Callers that do
// their own arming MUST NOT double-close.
func (a *Attacher) AttachFromSnapshot(ctx context.Context, snap *domain.Change, project string, sink inbound.EventSink) (RunResult, error) {
	if a.deps.Runner == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: runner not wired")}
	}
	if snap == nil {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("attach: snapshot required")}
	}

	defer sink.Close() //nolint:errcheck // best-effort

	res := RunResult{ChangeID: snap.ID}

	if err := a.persistChangeID(ctx, project, snap.ID); err != nil {
		// Persistence failure is NOT fatal — surface as a sink error and continue.
		_ = sink.OnError(ctx, err)
	}
	if err := sink.OnSnapshot(ctx, snap); err != nil {
		_ = sink.OnError(ctx, err)
	}

	if snap.Status.IsTerminal() {
		// Prime res.FinalStatus so Observe takes the short-circuit branch.
		res.FinalStatus = snap.Status
	}

	return a.deps.Runner.Observe(ctx, res, sink)
}

// persistChangeID mirrors Runner.persistChangeID exactly (spec §3.5: "run and
// attach both update project-scoped + global last_change_id"). Duplicated
// rather than shared because Runner.persistChangeID is unexported and the
// shape is small enough that exposing it would overgeneralize the API.
func (a *Attacher) persistChangeID(ctx context.Context, project string, id domain.ChangeID) error {
	if err := a.deps.State.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := a.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo — keep only the global record. Not fatal.
		return nil
	}
	if project == "" {
		// Without a project name we can't compute the fingerprint. Caller
		// (CLI) should have resolved the project from .sophia.yaml; if it's
		// still empty we settle for global-only.
		return nil
	}
	remote, _ := a.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := a.deps.State.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
```

> **Note on the `defer sink.Close()`:** mirrors `Runner.Run`. The CLI layer doesn't double-close because `runJSONL`/`runTUI`-style helpers don't call `sink.Close()` themselves — they hand the sink to the application service which owns the close.

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/application/...
```

Expected: PASS — eight new Attacher tests (six on `Attach`, two on `AttachFromSnapshot`) plus all existing Runner / Lister / StatusReader tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/application/attacher.go \
        internal/application/attacher_test.go
git commit -m "feat(application): add Attacher service for sophia attach (M8)"
```

---

## Phase 4 — Application: Real StatusReader (HTTP fetch + resolution order)

### Task 4: Upgrade `application.StatusReader` to fetch the snapshot from orchestrator

**Files:**
- Modify: `internal/application/status.go`
- Modify: `internal/application/status_test.go`

The M3 `StatusReader` resolves a `ChangeID` from local state but stops there — it returns `(ChangeID, Source)` and the CLI prints "last change: 01HX (source=project)". M8 upgrades it: after resolving the ID, `Resolve` calls `OrchestratorClient.GetChange` to fetch the live snapshot. The result type grows to `StatusReport` (renamed from `StatusOutput` to signal the change). Resolution order per spec §2.5 is:

1. If `<change-id>` was passed explicitly → use it (`Source: StatusSourceFlag`).
2. Else, project-scoped `last_change_id` (`Source: StatusSourceProject`).
3. Else, global `last_change_id` (`Source: StatusSourceGlobal`).
4. Else, `IsEmpty: true` and `Source: StatusSourceNone` — no fetch, no error, exit 0.

The HTTP call applies a timeout (`opts.FetchTimeout`, default 10s, mirroring Runner's snapshot timeout). Error mapping (per cambio 5):

- 404 → `ExitError{Code: 3}` wrapping `ErrChangeNotFound` (locally-cached ID is stale; user clears it or passes a different one).
- 5xx / network / `ErrUnreachable` → `ExitError{Code: 3}` (orchestrator unreachable).
- Parent ctx canceled mid-fetch → `ExitError{Code: 4}` (transient, user aborted).
- **Fetch timeout (`context.DeadlineExceeded` from the internal `FetchTimeout` deadline, parent ctx still alive) → `ExitError{Code: 4}`**. Treating it as transient matches the `attach` semantics where ctx cancel is exit 4 — a stuck/slow orchestrator is a "try again" signal, not a "your config is broken" signal (which would be exit 3).

Project-scoped resolution (cambio 4): if `.sophia.yaml` exists and is **malformed** (YAML parse error or missing `project:` field), `status` MUST fail with exit 3 wrapping the parse error. Only fall through to global when (a) we are outside a git repo, (b) `.sophia.yaml` does not exist, or (c) `.sophia.yaml` exists, parses cleanly, but no `last_change_id` is recorded yet for the resulting fingerprint. This is stricter than M3's "any error in tryProject silently falls through" — the `changes` command keeps the lenient behavior with a stderr warning, but `status` is meant to be a precise truth signal and must not hide config errors. Distinguishing parse errors from "missing file" relies on `errors.Is(err, domain.ErrConfigMissing)` and `errors.Is(err, domain.ErrInvalidYAML)` returned by `ProjectConfigStore.Read`.

The new `Resolve` signature: `Resolve(ctx, ResolveInput) (StatusReport, error)`. `ResolveInput` carries the optional `ChangeID` arg; an empty value triggers the project→global fallback.

> **Verification gate:** before refactoring, read `internal/application/status.go` to confirm M3's exported types: `StatusReader`, `StatusDeps`, `StatusOutput`, `StatusSource`, `StatusSourceProject`, `StatusSourceGlobal`. M8 RENAMES `StatusOutput` → `StatusReport` and ADDS `StatusSourceFlag` and `StatusSourceNone`. Existing tests will need updates to match. The CLI layer (Task 7) will also need updating.

- [ ] **Step 1: Replace status_test.go with the M8 test set**

OVERWRITE `internal/application/status_test.go` with:

```go
package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newStatus() (*application.StatusReader, *fakes.FakeOrchestrator, *fakes.FakeStateStore, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch:         orch,
		State:        state,
		Git:          git,
		ProjectStore: store,
	}, application.StatusOptions{FetchTimeout: time.Second})
	return r, orch, state, git, store
}

func TestStatusEmptyWhenNoArgNoProjectNoGlobal(t *testing.T) {
	r, _, _, _, _ := newStatus()
	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsEmpty {
		t.Errorf("expected empty, got %+v", out)
	}
	if out.Source != application.StatusSourceNone {
		t.Errorf("Source = %q, want none", out.Source)
	}
	if out.Change != nil {
		t.Errorf("Change should be nil on empty result, got %+v", out.Change)
	}
}

func TestStatusFlagArgWinsAndFetchesSnapshot(t *testing.T) {
	r, orch, _, _, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "FROM-ARG", Status: domain.ChangeStatusRunning})

	out, err := r.Resolve(context.Background(), application.ResolveInput{
		ChangeID: "FROM-ARG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.Source != application.StatusSourceFlag {
		t.Errorf("Source = %q, want flag", out.Source)
	}
	if out.Change == nil || out.Change.ID != "FROM-ARG" {
		t.Errorf("Change = %+v", out.Change)
	}
	if out.Change.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", out.Change.Status)
	}
}

func TestStatusPrefersProjectScopedOverGlobal(t *testing.T) {
	r, orch, state, git, store := newStatus()
	orch.SeedChange(&domain.Change{ID: "PROJ", Status: domain.ChangeStatusDone})
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "PROJ")
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceProject {
		t.Errorf("Source = %q, want project", out.Source)
	}
	if out.Change.ID != "PROJ" {
		t.Errorf("ID = %q, want PROJ", out.Change.ID)
	}
}

func TestStatusFallsBackToGlobalWhenNoProjectScoped(t *testing.T) {
	r, orch, state, _, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusBlocked})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q, want global", out.Source)
	}
	if out.Change.ID != "GLOB" {
		t.Errorf("ID = %q", out.Change.ID)
	}
	if out.Change.Status != domain.ChangeStatusBlocked {
		t.Errorf("Status = %q", out.Change.Status)
	}
}

func TestStatusFlagArgChangeNotFoundExitCode3(t *testing.T) {
	r, _, _, _, _ := newStatus()
	// orch has no seeded change → GetChange returns ErrChangeNotFound.
	_, err := r.Resolve(context.Background(), application.ResolveInput{
		ChangeID: "MISSING",
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
	if !errors.Is(err, domain.ErrChangeNotFound) {
		t.Errorf("expected wrapped ErrChangeNotFound; got %v", err)
	}
}

func TestStatusStaleProjectIDChangeNotFoundExitCode3(t *testing.T) {
	// Local state has a project-scoped ID but orchestrator no longer knows it.
	r, _, state, git, store := newStatus()
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "STALE")

	_, err := r.Resolve(context.Background(), application.ResolveInput{})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestStatusOutsideRepoFallsBackToGlobalAndFetches(t *testing.T) {
	r, orch, state, git, _ := newStatus()
	git.NotARepo = true
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q", out.Source)
	}
	if out.Change == nil || out.Change.ID != "GLOB" {
		t.Errorf("Change = %+v", out.Change)
	}
}

func TestStatusCtxCanceledDuringFetchExitCode4(t *testing.T) {
	r, orch, _, _, _ := newStatus()
	orch.GetBlockUntilCancel = true

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := r.Resolve(ctx, application.ResolveInput{ChangeID: "X"})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4 (ctx cancel during fetch)", exit.Code)
	}
}

// cambio 5: an internal FetchTimeout (parent ctx still alive) is exit 4,
// not exit 3. A slow/stuck orchestrator is transient, not a config bug.
func TestStatusInternalFetchTimeoutExitCode4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch:         orch,
		State:        state,
		Git:          git,
		ProjectStore: store,
	}, application.StatusOptions{FetchTimeout: 20 * time.Millisecond})

	orch.SeedChange(&domain.Change{ID: "X"})
	orch.GetBlockUntilCancel = true // hold until fctx times out

	_, err := r.Resolve(context.Background(), application.ResolveInput{ChangeID: "X"})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4 (internal fetch timeout)", exit.Code)
	}
}

// cambio 4: a malformed .sophia.yaml is fatal — status MUST NOT silently fall
// through to the global last_change_id.
func TestStatusInvalidProjectYAMLExitCode3(t *testing.T) {
	r, orch, state, git, store := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")
	git.Root = "/repo"
	// ProjectStore.Read returns ErrInvalidYAML for /repo/.sophia.yaml.
	store.ReadErr = map[string]error{
		"/repo/.sophia.yaml": fmt.Errorf("yaml: line 3: %w", domain.ErrInvalidYAML),
	}

	_, err := r.Resolve(context.Background(), application.ResolveInput{})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError; got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3 (invalid .sophia.yaml)", exit.Code)
	}
	if !errors.Is(err, domain.ErrInvalidYAML) {
		t.Errorf("expected wrapped ErrInvalidYAML; got %v", err)
	}
}

// cambio 4 (negative): a MISSING .sophia.yaml is NOT fatal — status falls
// through to global. This test guards the distinction.
func TestStatusMissingProjectYAMLFallsThroughToGlobal(t *testing.T) {
	r, orch, state, git, _ := newStatus()
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone})
	_ = state.SetGlobalLast(context.Background(), "GLOB")
	git.Root = "/repo"
	// store has no .sophia.yaml at /repo/.sophia.yaml; Read returns ErrConfigMissing.

	out, err := r.Resolve(context.Background(), application.ResolveInput{})
	if err != nil {
		t.Fatalf("expected fall-through to global; got %v", err)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q, want global", out.Source)
	}
}
```

> **Note on `ProjectConfigStore.ReadErr`:** the YAML-invalid test references `FakeProjectConfigStore.ReadErr map[string]error`. If the fake doesn't yet model per-path Read errors, append it inline as part of this task. Concretely on `test/fakes/projectconfig.go`:
>
> ```go
> // field on FakeProjectConfigStore:
> ReadErr map[string]error
> ```
>
> Update Read to honor it: `if e, ok := f.ReadErr[path]; ok { return nil, e }` before the file-store lookup.
>
> **Sentinel reuse:** `domain.ErrInvalidYAML` already exists in `internal/domain/errors.go` (verified pre-Task-3). The `yamlconfig` adapter (`internal/adapters/outbound/yamlconfig/project.go:67`) returns it as a bare sentinel on parse failure (no `%w` wrapping at the source). For tests we wrap it with `fmt.Errorf("yaml: line 3: %w", domain.ErrInvalidYAML)` so `errors.Is` resolves through richer messages. Do NOT introduce a separate `ErrConfigInvalid` — the existing `ErrInvalidYAML` covers parse errors and missing required fields.

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL — `StatusReader.Resolve` no longer matches the new signature; `ResolveInput`, `StatusReport`, `StatusOptions`, `StatusSourceFlag`, `StatusSourceNone` undefined.

- [ ] **Step 3: Implement**

OVERWRITE `internal/application/status.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// StatusSource indicates where the resolved last_change_id came from.
type StatusSource string

const (
	// StatusSourceFlag → caller passed an explicit <change-id> arg.
	StatusSourceFlag StatusSource = "flag"
	// StatusSourceProject → resolved from project-scoped last_change_id.
	StatusSourceProject StatusSource = "project"
	// StatusSourceGlobal → resolved from global last_change_id.
	StatusSourceGlobal StatusSource = "global"
	// StatusSourceNone → no source available; result is empty.
	StatusSourceNone StatusSource = "none"
)

// StatusDeps groups the ports StatusReader needs.
type StatusDeps struct {
	Orch         outbound.OrchestratorClient
	State        outbound.StateStore
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// StatusOptions tunes StatusReader.
type StatusOptions struct {
	// FetchTimeout caps the GetChange call. Default 10s.
	FetchTimeout time.Duration
}

// ResolveInput controls Resolve.
type ResolveInput struct {
	// ChangeID, when non-empty, takes precedence over project/global state.
	ChangeID domain.ChangeID
}

// StatusReport is the shape returned by Resolve.
type StatusReport struct {
	IsEmpty bool
	Source  StatusSource
	// Change is the live snapshot fetched from the orchestrator. nil when
	// IsEmpty is true.
	Change *domain.Change
}

// StatusReader implements `sophia status`.
type StatusReader struct {
	deps StatusDeps
	opts StatusOptions
}

// NewStatusReader constructs a StatusReader. Pass StatusOptions{} for defaults.
func NewStatusReader(d StatusDeps, opts StatusOptions) *StatusReader {
	if opts.FetchTimeout <= 0 {
		opts.FetchTimeout = 10 * time.Second
	}
	return &StatusReader{deps: d, opts: opts}
}

// Resolve walks: arg → project-scoped → global → empty. When a source is
// found, fetches the snapshot from the orchestrator and returns it on the
// StatusReport.
func (r *StatusReader) Resolve(ctx context.Context, in ResolveInput) (StatusReport, error) {
	id, src, err := r.locate(ctx, in)
	if err != nil {
		return StatusReport{}, err
	}
	if id.IsZero() {
		return StatusReport{IsEmpty: true, Source: StatusSourceNone}, nil
	}
	snap, err := r.fetch(ctx, id)
	if err != nil {
		return StatusReport{}, err
	}
	return StatusReport{Source: src, Change: snap}, nil
}

// locate runs the resolution order: arg → project-scoped → global. It returns
// (zero, none, nil) when nothing was found (caller must turn that into an
// IsEmpty StatusReport).
//
// Project-scoped errors (cambio 4): only ErrConfigMissing or "outside git repo"
// (ErrNotARepo / underlying exec failure) fall through to global. A malformed
// .sophia.yaml (ErrInvalidYAML) is fatal and surfaces as ExitError{Code: 3}
// — `status` must not hide config corruption.
func (r *StatusReader) locate(ctx context.Context, in ResolveInput) (domain.ChangeID, StatusSource, error) {
	if !in.ChangeID.IsZero() {
		return in.ChangeID, StatusSourceFlag, nil
	}
	id, src, err := r.tryProject(ctx)
	if err == nil && !id.IsZero() {
		return id, src, nil
	}
	if err != nil && errors.Is(err, domain.ErrInvalidYAML) {
		// Malformed .sophia.yaml: status fails; do NOT fall through.
		return "", "", &ExitError{Code: 3, Err: fmt.Errorf("project config invalid: %w", err)}
	}
	gid, err := r.deps.State.GetGlobalLast(ctx)
	if err != nil {
		return "", "", fmt.Errorf("global last: %w", err)
	}
	if !gid.IsZero() {
		return gid, StatusSourceGlobal, nil
	}
	return "", StatusSourceNone, nil
}

// fetch GETs the snapshot from the orchestrator with the configured timeout.
// Maps errors to spec §2.3 exit codes (cambio 5):
//
//   - parent ctx canceled                         → exit 4
//   - internal FetchTimeout deadline exceeded     → exit 4
//   - ErrChangeNotFound (404)                     → exit 3
//   - ErrUnreachable / 5xx / network              → exit 3
//   - orchestrator client not wired (config bug)  → exit 3
func (r *StatusReader) fetch(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if r.deps.Orch == nil {
		return nil, &ExitError{Code: 3, Err: errors.New("status: orchestrator client not wired")}
	}
	fctx, cancel := context.WithTimeout(ctx, r.opts.FetchTimeout)
	defer cancel()
	snap, err := r.deps.Orch.GetChange(fctx, id)
	if err != nil {
		// Parent ctx canceled — user aborted: transient.
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, &ExitError{Code: 4, Err: err}
		}
		// Internal FetchTimeout fired (parent still alive): also transient.
		// We check the returned err (and fctx.Err) rather than ctx because the
		// parent is by definition NOT canceled here.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(fctx.Err(), context.DeadlineExceeded) {
			return nil, &ExitError{Code: 4, Err: fmt.Errorf("get change timed out after %s: %w", r.opts.FetchTimeout, err)}
		}
		// Everything else (ErrChangeNotFound, ErrUnreachable, 5xx) → orchestrator/config error.
		return nil, &ExitError{Code: 3, Err: fmt.Errorf("get change: %w", err)}
	}
	return snap, nil
}

// tryProject returns:
//
//   - (id, StatusSourceProject, nil) when .sophia.yaml exists, parses cleanly,
//     and a project-scoped last_change_id is recorded.
//   - ("", "", ErrConfigMissing) when there's no .sophia.yaml (locate falls
//     through to global).
//   - ("", "", ErrInvalidYAML-wrapped error) when .sophia.yaml exists but is
//     malformed (locate maps to ExitError{Code: 3}).
//   - ("", "", git-repo error) when not in a git repo (locate falls through).
//   - ("", "", state-store error) on a state lookup failure (rare; locate
//     falls through to global as best-effort).
func (r *StatusReader) tryProject(ctx context.Context) (domain.ChangeID, StatusSource, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo or git not installed — caller falls through to global.
		return "", "", err
	}
	cfgPath := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, cfgPath)
	if err != nil {
		// Surface the error verbatim so locate can distinguish
		// ErrConfigMissing (fall through) from ErrInvalidYAML (fatal).
		return "", "", err
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(cfg.Project, root, remote)
	id, err := r.deps.State.GetLast(ctx, fp)
	if err != nil {
		return "", "", err
	}
	return id, StatusSourceProject, nil
}
```

> **Note on exit-code mapping in `fetch` (cambio 5):** the fetch timeout is `r.opts.FetchTimeout` (10s default). Both "parent ctx canceled" and "internal FetchTimeout fired" map to exit 4 (transient — the user can retry). 404 (`ErrChangeNotFound`), 5xx, and `ErrUnreachable` map to exit 3 (orchestrator-unreachable / config-stale). `orchestratorhttp.StatusError.Is(domain.ErrChangeNotFound)` is true on 404 (verified in `internal/adapters/outbound/orchestratorhttp/errors.go`). Detecting the timeout requires both `errors.Is(err, context.DeadlineExceeded)` and a check on `fctx.Err()` because some HTTP clients return their own wrapped error when the inner context expires; the `fctx.Err()` fallback is a defensive belt-and-braces.

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/application/...
```

Expected: PASS — eleven new StatusReader tests (eight base + three for cambios 4 & 5: invalid YAML exit 3, missing YAML falls through, internal fetch timeout exit 4) plus all existing application tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/application/status.go \
        internal/application/status_test.go
git commit -m "feat(application): upgrade StatusReader with HTTP fetch + resolution order (M8)"
```

---

## Phase 5 — CLI: changes command

### Task 5: `cli/changes.go` — flags + table/JSON output

**Files:**
- Create: `internal/adapters/inbound/cli/changes.go`
- Create: `internal/adapters/inbound/cli/changes_test.go`
- Modify: `internal/adapters/inbound/cli/root.go` (drop `changes` stub)
- Modify: `internal/adapters/inbound/cli/stubs_test.go` (drop `changes` from the stub-list)

`sophia changes` is the simplest CLI command in M8. Five flags, a `Lister` call, a table or JSON printer. The table is column-aligned text using `text/tabwriter` (stdlib, no new dependency). Columns: ID, Status, Project, BaseRef, CreatedAt. The JSON output is a JSON array of `orchestratorhttp.ChangeResponse` shapes (D-M8-08) — but the application layer returns `[]*domain.Change`, so the CLI handles JSON encoding via a thin DTO converter that mirrors the wire shape.

Project resolution: the user's intent is communicated by whether the flag was passed.

- Flag NOT passed (cobra default empty string and the user didn't say `--project=...`) → resolve project from `.sophia.yaml` via `ConfigResolver`. Pass that as the filter.
- Flag passed with non-empty value → use it verbatim.
- Flag passed with empty string (`--project=""`) → no project filter; list all projects.

cobra's `Changed` predicate on the flag distinguishes "not passed" from "passed with empty value", so this is a one-liner check at runtime.

> **Verification gate:** before writing the CLI command, confirm `internal/adapters/outbound/orchestratorhttp/dto.go` exports `ChangeResponse` (it does — verified) so the JSON output shape can re-marshal a `*domain.Change` into the same wire shape the orchestrator emits. If the DTO is private or moved, adjust the converter shape inline.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/cli/changes_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newChangesDeps(t *testing.T) (cli.Deps, *fakes.FakeOrchestrator) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "default-proj", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})
	lister := application.NewLister(application.ListerDeps{Orch: orch})
	return cli.Deps{Resolver: resolver, Lister: lister}, orch
}

func TestChangesCommandPrintsTableByDefault(t *testing.T) {
	deps, orch := newChangesDeps(t)
	orch.SeedChange(&domain.Change{
		ID: "01H1", Status: domain.ChangeStatusDone, Project: "default-proj", BaseRef: "main",
	})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"changes"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "01H1") {
		t.Errorf("output missing change ID: %q", got)
	}
	if !strings.Contains(got, "done") {
		t.Errorf("output missing status: %q", got)
	}
	// Header row.
	if !strings.Contains(got, "ID") || !strings.Contains(got, "STATUS") {
		t.Errorf("output missing header: %q", got)
	}
}

func TestChangesCommandJSONFlagEmitsArray(t *testing.T) {
	deps, orch := newChangesDeps(t)
	orch.SeedChange(&domain.Change{
		ID: "01H1", Status: domain.ChangeStatusRunning, Project: "default-proj",
	})
	orch.SeedChange(&domain.Change{
		ID: "01H2", Status: domain.ChangeStatusDone, Project: "default-proj",
	})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"changes", "--json"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	var arr []map[string]any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("output not valid JSON array: %v\n%s", err, out.String())
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
	for _, item := range arr {
		if _, ok := item["change_id"]; !ok {
			t.Errorf("item missing change_id: %+v", item)
		}
	}
}

func TestChangesCommandDefaultLimitIs10(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenLimit int
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenLimit = f.Limit }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenLimit != 10 {
		t.Errorf("default --limit forwarded as %d, want 10", seenLimit)
	}
}

func TestChangesCommandLimitFlagOverrides(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenLimit int
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenLimit = f.Limit }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--limit", "5"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenLimit != 5 {
		t.Errorf("--limit=5 forwarded as %d", seenLimit)
	}
}

func TestChangesCommandStatusFilterPassesThrough(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenStatus string
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenStatus = f.Status }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--status", "done"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenStatus != "done" {
		t.Errorf("--status forwarded as %q, want done", seenStatus)
	}
}

func TestChangesCommandProjectDefaultFromConfig(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	// No --project → default-proj from .sophia.yaml.
	c.SetArgs([]string{"changes"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "default-proj" {
		t.Errorf("default project = %q, want default-proj", seenProject)
	}
}

func TestChangesCommandProjectFlagOverrides(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"changes", "--project", "other"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "other" {
		t.Errorf("--project = %q, want other", seenProject)
	}
}

func TestChangesCommandEmptyProjectFlagMeansNoFilter(t *testing.T) {
	deps, orch := newChangesDeps(t)
	var seenProject string
	orch.OnListChanges = func(f outbound_ListChangesFilterShim) { seenProject = f.Project }
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	// --project="" → no filter (D-M8-07).
	c.SetArgs([]string{"changes", "--project", ""})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if seenProject != "" {
		t.Errorf("--project=\"\" should disable project filter, got %q", seenProject)
	}
}
```

> **Note:** `outbound_ListChangesFilterShim` is a stand-in for `outbound.ListChangesFilter` — to keep the `cli_test` package free of an outbound import, define a tiny alias in the test file:
>
> ```go
> import "github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
> type outbound_ListChangesFilterShim = outbound.ListChangesFilter
> ```
>
> Or just use `outbound.ListChangesFilter` directly with the import. Pick whichever reads cleaner — both work.

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL — `cli.Deps.Lister` undefined; `newChangesCmd` doesn't exist; the stub still prints "not implemented yet".

- [ ] **Step 3: Implement**

`internal/adapters/inbound/cli/changes.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// DefaultChangesLimit is the default value for `sophia changes --limit`.
// D-M8-01: spec §2.5 / §5.1 default. Higher values pass through unchanged;
// the orchestrator decides the upper bound.
const DefaultChangesLimit = 10

func newChangesCmd(d Deps) *cobra.Command {
	var (
		limit   int
		status  string
		project string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List recent Changes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Lister == nil {
				return fmt.Errorf("changes: lister not wired")
			}

			// Project resolution per D-M8-07 (CLI-only — Lister is a pure
			// pass-through):
			//
			//   --project not passed     → resolve default from .sophia.yaml
			//                              (best-effort; missing config or
			//                              malformed YAML logs a warning and
			//                              falls through to no filter)
			//   --project="anything"      → use as-is
			//   --project=""              → no filter (list all)
			var effectiveProject string
			projectFlagSet := cmd.Flags().Changed("project")
			if projectFlagSet {
				effectiveProject = project // may be "" — user opted out explicitly
			} else if d.Resolver != nil {
				resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
					UserConfigPath: d.UserConfigPath,
					RequireProject: false,
				})
				if err != nil {
					// Best-effort fallback per spec §2.5: warn but list all.
					// `sophia status` (Task 4) is stricter — it errors on bad
					// YAML — but `changes` is a discovery tool and a missing or
					// malformed .sophia.yaml shouldn't block the list.
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: project default unavailable (%v); listing all projects\n", err)
				} else {
					effectiveProject = resolved.Project
				}
			}

			out, err := d.Lister.List(cmd.Context(), application.ListInput{
				Project: effectiveProject,
				Status:  status,
				Limit:   limit,
				Offset:  0,
			})
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if jsonOut {
				return printChangesJSON(w, out)
			}
			return printChangesTable(w, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", DefaultChangesLimit, "max number of Changes to return")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (pending|running|done|blocked|failed)")
	cmd.Flags().StringVar(&project, "project", "", "filter by project; empty value disables the project filter")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON array")
	return cmd
}

// printChangesTable renders a column-aligned table.
func printChangesTable(w io.Writer, items []*domain.Change) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tSTATUS\tPROJECT\tBASE_REF\tCREATED_AT"); err != nil {
		return err
	}
	for _, c := range items {
		created := ""
		if !c.CreatedAt.IsZero() {
			created = c.CreatedAt.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.ID, c.Status, c.Project, c.BaseRef, created); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// printChangesJSON renders a JSON array using the orchestrator's wire shape
// (D-M8-08).
func printChangesJSON(w io.Writer, items []*domain.Change) error {
	out := make([]orchestratorhttp.ChangeResponse, 0, len(items))
	for _, c := range items {
		out = append(out, changeResponseFromDomain(c))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// changeResponseFromDomain mirrors orchestratorhttp.ChangeResponse.ToDomain in
// reverse: it converts a domain.Change back to the wire shape so the JSON
// output matches what the orchestrator would emit.
func changeResponseFromDomain(c *domain.Change) orchestratorhttp.ChangeResponse {
	r := orchestratorhttp.ChangeResponse{
		ChangeID:          c.ID.String(),
		Name:              c.Name,
		Project:           c.Project,
		BaseRef:           c.BaseRef,
		ArtifactStoreMode: c.ArtifactStoreMode,
		Status:            string(c.Status),
		CurrentPhaseID:    c.CurrentPhaseID,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
	if len(c.Phases) > 0 {
		r.Phases = make([]orchestratorhttp.PhaseDTO, len(c.Phases))
		for i, p := range c.Phases {
			r.Phases[i] = orchestratorhttp.PhaseDTO{
				ID:         p.ID,
				Type:       string(p.Type),
				Status:     string(p.Status),
				Confidence: p.Confidence,
				StartedAt:  p.StartedAt,
				EndedAt:    p.EndedAt,
			}
		}
	}
	return r
}
```

`internal/adapters/inbound/cli/root.go` — REPLACE the line `root.AddCommand(newStubCmd("changes", ...))` with `root.AddCommand(newChangesCmd(d))`. ALSO add a `Lister *application.Lister` field to the `Deps` struct (between `StatusReader` and `Resolver`):

```go
type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Lister       *application.Lister // M8: sophia changes
	Resolver     *application.ConfigResolver

	// Orch is required by attachJSONL for D-M8-13's eager-arm GetChange before
	// the Attacher takes over. Bootstrap (Task 8) wires the same orchestrator
	// client used to construct RunnerFactory/AttacherFactory.
	Orch outbound.OrchestratorClient // M8: required for cli.attachJSONL eager-arm

	RunnerFactory   RunnerFactory
	AttacherFactory AttacherFactory // M8: sophia attach (added in Task 6)
	// ... rest unchanged
}
```

(The `AttacherFactory` is referenced now but its type is added in Task 6 — declare both types together OR add `Lister` here and `AttacherFactory` in Task 6's edits. Same for `Orch`.)

`internal/adapters/inbound/cli/stubs_test.go` — the test asserts that `attach` and `changes` print "not implemented yet". After M8 wires real commands, the stubs list shrinks to ZERO entries. Replace the test body with:

```go
package cli_test

import (
	"testing"
)

// TestStubsAnnounceMilestone is intentionally empty after M8 — every M1–M8
// command is now real. Kept as a placeholder so future stubs (post-v1) have
// a known location to plug into.
func TestStubsAnnounceMilestone(t *testing.T) {
	t.Skip("no stubs left after M8 — every v1 command is now real")
}
```

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/adapters/inbound/cli/...
```

Expected: PASS — eight new `changes` tests plus all M3–M7 cli tests (run, status, init, etc.) still green. The skipped stubs test is fine.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/changes.go \
        internal/adapters/inbound/cli/changes_test.go \
        internal/adapters/inbound/cli/root.go \
        internal/adapters/inbound/cli/stubs_test.go
git commit -m "feat(cli): add real sophia changes command (M8)"
```

---

## Phase 6 — CLI: attach command

### Task 6: `cli/attach.go` — positional change-id + TUI/JSONL dispatch

**Files:**
- Create: `internal/adapters/inbound/cli/attach.go`
- Create: `internal/adapters/inbound/cli/attach_test.go`
- Modify: `internal/adapters/inbound/cli/root.go` (drop `attach` stub; add `AttacherFactory` field if not already present)

`sophia attach` mirrors `sophia run` almost line-for-line. The only differences are:

1. Positional arg is `<change-id>`, not a free-text message.
2. The application service is `Attacher.Attach(ctx, in, sink)`, not `Runner.Run(ctx, in)`.
3. There's no `--message`, `--base-ref`, `--artifact-store` — those are CreateChange concerns.
4. Project is resolved (best-effort) ONLY for persisting project-scoped last_change_id. If `.sophia.yaml` is missing, attach still works; only global last_change_id is updated.

The flag matrix is the same: `--no-tui --json` mode validation reused; `--approval-timeout` reused (the JSONL approval timeout sink wrapper applies to attach too — RM8-08); TUI mode reused. The `AttacherFactory` pattern mirrors `RunnerFactory`: `cli.Deps.AttacherFactory(sink) *application.Attacher`. Bootstrap (Task 8) wires it.

> **Verification gate:** read `internal/adapters/inbound/cli/run.go` and `internal/adapters/inbound/cli/timeout_sink.go` to confirm:
>
> - `validateModeFlags(noTUI, jsonOut bool) error` is exported as a helper inside the cli package (or duplicate the small validation in attach.go);
> - `chooseJSONSink(d Deps) inbound.EventSink` exists;
> - `chooseTUIOutput(d Deps) io.Writer` exists;
> - `newApprovalTimeoutSink(...)` is the wrapper signature;
> - `envSnapshot()` returns the env map.
>
> If any of those names changed, adapt the references below to match.

#### Sub-task 6.A — Fix `approvalTimeoutSink.startTimer` (cambio 3)

The eager-arm flow added below relies on `startTimer` being **idempotent while a timer is already armed**. The M7 implementation in `internal/adapters/inbound/cli/timeout_sink.go` does the opposite: every `OnApprovalGate` stops the previous timer and starts a fresh one, which would reset the eager-arm timestamp every time SSE delivers a real `approval.required` event. Fix it before wiring the CLI's eager-arm path so the new tests can lock the contract.

- [ ] **Step 6.A.1: Add the regression tests for the sink**

Append to `internal/adapters/inbound/cli/timeout_sink_test.go` (create the file if it doesn't yet exist; it lives next to `timeout_sink.go`):

```go
package cli

import (
	"context"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// noopSink lets us inspect the wrapper without driving a full sink.
type noopSink struct{}

func (noopSink) OnSnapshot(context.Context, *domain.Change) error          { return nil }
func (noopSink) OnEvent(context.Context, domain.Event) error               { return nil }
func (noopSink) OnApprovalGate(context.Context, domain.ApprovalGate) error { return nil }
func (noopSink) OnError(context.Context, error) error                      { return nil }
func (noopSink) OnComplete(context.Context, domain.ChangeStatus) error     { return nil }
func (noopSink) Close() error                                              { return nil }

// cambio 3: a second OnApprovalGate while the timer is already running must
// NOT reset the timer. The eager-arm timestamp from the FIRST call wins.
func TestApprovalTimeoutSinkDoesNotResetOnReGate(t *testing.T) {
	timeout := 60 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	g1 := domain.ApprovalGate{Phase: "implement", ChangeID: "X"}
	if err := sink.OnApprovalGate(context.Background(), g1); err != nil {
		t.Fatal(err)
	}

	// Burn ~half the budget, then re-arm with the SAME gate. If startTimer
	// were buggy and reset, the timer would fire 60ms from now (=~90ms total
	// from t=0). With the fix, it fires ~60ms from t=0 (≈30ms from now).
	time.Sleep(30 * time.Millisecond)
	g2 := domain.ApprovalGate{Phase: "implement", ChangeID: "X", URL: "https://x.test/g"}
	if err := sink.OnApprovalGate(context.Background(), g2); err != nil {
		t.Fatal(err)
	}

	select {
	case <-canceled:
		// Fired close to original 60ms — pass.
	case <-time.After(80 * time.Millisecond):
		t.Fatal("timer was reset by the second OnApprovalGate; cambio 3 is not applied")
	}
	if err := sink.Wait(); err == nil {
		t.Error("expected errApprovalTimeout from Wait after timer fired")
	}
}

// cambio 3: approval.resolved clears the gate; a SUBSEQUENT approval.required
// must start a fresh timer.
func TestApprovalTimeoutSinkResolvedThenNewGateStartsFresh(t *testing.T) {
	timeout := 50 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	if err := sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X"}); err != nil {
		t.Fatal(err)
	}
	// Resolve before timer fires.
	time.Sleep(10 * time.Millisecond)
	if err := sink.OnEvent(context.Background(), domain.Event{Type: "approval.resolved", EventID: "evt-r"}); err != nil {
		t.Fatal(err)
	}

	// Wait long enough that the OLD timer would have fired if it weren't stopped.
	time.Sleep(60 * time.Millisecond)
	select {
	case <-canceled:
		t.Fatal("approval.resolved did NOT stop the timer")
	default:
	}

	// Now arm a brand-new gate. The fresh timer must use the FULL timeout.
	if err := sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "verify", ChangeID: "X"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
		// Fires after a fresh ~50ms — pass.
	case <-time.After(120 * time.Millisecond):
		t.Fatal("post-resolved gate did NOT start a fresh timer")
	}
}

// cambio 3 (positive control): re-armed timer from a SUBSEQUENT real event
// preserves the eager-arm timestamp — caller observes the same fired-at point.
func TestApprovalTimeoutSinkSecondGatePreservesArmTime(t *testing.T) {
	timeout := 40 * time.Millisecond
	canceled := make(chan struct{})
	sink := newApprovalTimeoutSink(noopSink{}, timeout, func() { close(canceled) })

	start := time.Now()
	_ = sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X"})
	time.Sleep(20 * time.Millisecond)
	_ = sink.OnApprovalGate(context.Background(), domain.ApprovalGate{Phase: "implement", ChangeID: "X", URL: "https://x.test/g"})

	select {
	case <-canceled:
		elapsed := time.Since(start)
		if elapsed >= 60*time.Millisecond {
			t.Errorf("timer was reset (fired after %s, expected ~%s)", elapsed, timeout)
		}
	case <-time.After(120 * time.Millisecond):
		t.Fatal("timer never fired")
	}
}
```

- [ ] **Step 6.A.2: Run tests — expect FAIL**

```bash
go test -race -run 'TestApprovalTimeoutSink' ./internal/adapters/inbound/cli/...
```

Expected: FAIL on `TestApprovalTimeoutSinkDoesNotResetOnReGate` and `TestApprovalTimeoutSinkSecondGatePreservesArmTime` because M7's `startTimer` resets the timer on every call.

- [ ] **Step 6.A.3: Patch `startTimer`**

In `internal/adapters/inbound/cli/timeout_sink.go`, replace the body of `startTimer` with:

```go
func (s *approvalTimeoutSink) startTimer(g domain.ApprovalGate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := g
	// If a timer is already armed (and hasn't fired or been cleared by
	// stopTimer), this is a re-emit of the same logical gate (or a fresh
	// approval.required arriving after our eager-arm). Refresh s.gate so
	// `observe` keeps the latest phase metadata, but DO NOT restart the
	// timer — the eager-arm timestamp must be preserved (D-M8-13 / cambio 3).
	if s.timer != nil && !s.fired {
		s.gate = &cp
		return
	}
	s.gate = &cp
	if s.timeout <= 0 {
		return
	}
	s.timer = time.AfterFunc(s.timeout, func() {
		s.mu.Lock()
		s.fired = true
		s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
	})
}
```

- [ ] **Step 6.A.4: Re-run tests — expect PASS**

```bash
go test -race -run 'TestApprovalTimeoutSink' ./internal/adapters/inbound/cli/...
```

All three new tests plus the existing M7 timeout tests must pass.

- [ ] **Step 6.A.5: Commit the sink fix on its own**

```bash
git add internal/adapters/inbound/cli/timeout_sink.go \
        internal/adapters/inbound/cli/timeout_sink_test.go
git commit -m "fix(cli): approvalTimeoutSink no longer resets on re-emit (D-M8-13/cambio 3)"
```

---

#### Sub-task 6.B — `cli/attach.go` proper

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/cli/attach_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newAttachDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeEventStream) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-x", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	factory := func(sink inbound.EventSink) *application.Attacher {
		runner := application.NewRunner(application.RunnerDeps{
			Orch: orch, State: state, Git: git, Sink: sink, EventStream: stream,
		}, application.RunnerOptions{})
		return application.NewAttacher(application.AttacherDeps{
			Orch: orch, State: state, Git: git, Runner: runner,
		})
	}

	return cli.Deps{
		Resolver:         resolver,
		AttacherFactory:  factory,
		Orch:             orch, // M8: required for cli.attachJSONL eager-arm GetChange
		JSONSinkOverride: newTestSink(sinkBuf),
	}, orch, stream
}

func TestAttachCommandRequiresChangeID(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when change-id missing")
	}
}

func TestAttachCommandJSONLModeSucceeds(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")

	var sinkBuf bytes.Buffer
	deps, orch, stream := newAttachDeps(t, &sinkBuf)
	orch.SeedChange(&domain.Change{ID: "ATT-1", Project: "ms-x", Status: domain.ChangeStatusRunning})

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--no-tui", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sinkBuf.String(), "snap:") {
		t.Errorf("sink missing snapshot: %s", sinkBuf.String())
	}
	if !strings.Contains(sinkBuf.String(), "done") {
		t.Errorf("sink missing terminal status: %s", sinkBuf.String())
	}
}

func TestAttachCommandNoTUIWithoutJSONFails(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--no-tui"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --no-tui used without --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json: %v", err)
	}
}

func TestAttachCommandJSONWithoutNoTUIFails(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-1", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --json used without --no-tui")
	}
}

func TestAttachCommandPropagatesExitCode3OnNotFound(t *testing.T) {
	deps, _, _ := newAttachDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "MISSING", "--no-tui", "--json"})

	err := c.Execute()
	var exit *application.ExitError
	if !ok(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestAttachCommandPropagatesExitCode0OnDone(t *testing.T) {
	deps, orch, stream := newAttachDeps(t, &bytes.Buffer{})
	orch.SeedChange(&domain.Change{ID: "ATT-OK", Status: domain.ChangeStatusRunning, Project: "ms-x"})
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-OK", "--no-tui", "--json"})
	if err := c.Execute(); err != nil {
		t.Errorf("expected nil err on done, got %v", err)
	}
}

// D-M8-13: when the snapshot already shows a phase blocked on approval and no
// SSE event ever arrives (mock orchestrator never pushes), the
// --approval-timeout MUST start at attach time and exit code MUST be 5.
func TestAttachJSONLEagerArmsTimeoutOnPendingApproval(t *testing.T) {
	deps, orch, stream := newAttachDeps(t, &bytes.Buffer{})
	orch.SeedChange(&domain.Change{
		ID:      "ATT-PEND",
		Project: "ms-x",
		Status:  domain.ChangeStatusRunning,
		Phases: []domain.Phase{
			{ID: "p1", Type: "implement", Status: domain.PhaseStatusBlocked},
		},
	})
	// Subscribe blocks indefinitely — the only path out is the timer firing.
	stream.OnSubscribe = func(_ outbound.StreamTarget) {}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"attach", "ATT-PEND", "--no-tui", "--json", "--approval-timeout", "40ms"})

	err := c.Execute()
	var exit *application.ExitError
	if !ok(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 5 {
		t.Errorf("Code = %d, want 5 (approval-timeout)", exit.Code)
	}
}

// ok is a tiny errors.As helper — keep this file self-contained.
func ok(err error, target any) bool {
	return errAs(err, target)
}
```

> **Note on `errAs`:** if `errors.As` isn't already imported in the test file, the cleanest pattern is to use `errors.As` directly with an inline import. The `ok(err, &exit)` shim above is just a readability helper; if you prefer `errors.As(err, &exit)` directly, drop the helper.

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL — `cli.AttacherFactory` undefined; `newAttachCmd` doesn't exist; the stub still announces "not implemented yet (M8)".

- [ ] **Step 3: Implement**

`internal/adapters/inbound/cli/attach.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

func newAttachCmd(d Deps) *cobra.Command {
	var (
		noTUI              bool
		jsonOut            bool
		approvalTimeoutStr string
	)
	cmd := &cobra.Command{
		Use:   "attach <change-id>",
		Short: "Attach to an existing Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.AttacherFactory == nil {
				return fmt.Errorf("attach: attacher factory not wired")
			}
			if err := validateModeFlags(noTUI, jsonOut); err != nil {
				return err
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("attach: change-id required (positional argument)")
			}

			// Resolve project for persisting last_change_id (best-effort).
			// Missing .sophia.yaml is fine — Attacher falls back to global-only.
			var project string
			if d.Resolver != nil {
				resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
					Env:            envSnapshot(),
					UserConfigPath: d.UserConfigPath,
					RequireProject: false,
				})
				if err == nil {
					project = resolved.Project
				}
			}

			approvalTimeout, err := time.ParseDuration(approvalTimeoutStr)
			if err != nil {
				return fmt.Errorf("attach: --approval-timeout: %w", err)
			}

			input := application.AttachInput{
				ChangeID: domain.ChangeID(args[0]),
				Project:  project,
			}

			if noTUI {
				return attachJSONL(cmd.Context(), d, input, approvalTimeout)
			}
			return attachTUI(cmd.Context(), d, input)
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of opening the TUI")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required with --no-tui)")
	cmd.Flags().StringVar(&approvalTimeoutStr, "approval-timeout", "30m",
		"max wait for an approval gate before exit code 5 (--no-tui only)")
	return cmd
}

func attachJSONL(parentCtx context.Context, d Deps, input application.AttachInput, approvalTimeout time.Duration) error {
	innerSink := chooseJSONSink(d)

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	wrapped := newApprovalTimeoutSink(innerSink, approvalTimeout, cancel)
	attacher := d.AttacherFactory(wrapped)

	// D-M8-13: eager-arm path. The CLI fetches the snapshot itself so it can
	// scan for an in-flight approval gate and start the timeout BEFORE handing
	// the snapshot to the Attacher. No double GetChange.
	snap, err := d.Orch.GetChange(ctx, input.ChangeID)
	if err != nil {
		// Map errors to spec §2.3 exit codes — same shape as Attacher.Attach
		// would have produced if it had done the GetChange itself.
		_ = wrapped.Close()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return &application.ExitError{Code: 4, Err: err}
		}
		return &application.ExitError{Code: 3, Err: fmt.Errorf("attach: get change: %w", err)}
	}

	// Scan for any phase already blocked on approval and eager-arm the timer
	// at attach-time t=0 (D-M8-13). The synthetic gate carries enough fields
	// for the JSONL sink to print a meaningful row; the SSE replay will
	// later deliver the full event with URL/Reason/Risk/Policy, but
	// approvalTimeoutSink.startTimer will be a no-op then (cambio 3) so the
	// original eager-arm timestamp is preserved.
	if blocked := firstBlockedApprovalPhase(snap); blocked != nil {
		gate := domain.ApprovalGate{
			Phase:    blocked.Type,
			ChangeID: snap.ID,
		}
		_ = wrapped.OnApprovalGate(ctx, gate)
	}

	_, err = attacher.AttachFromSnapshot(ctx, snap, input.Project, wrapped)

	if waitErr := wrapped.Wait(); waitErr != nil {
		return &application.ExitError{Code: 5, Err: waitErr}
	}
	if err != nil {
		var exit *application.ExitError
		if errors.As(err, &exit) {
			return exit
		}
		return err
	}
	return nil
}

// firstBlockedApprovalPhase returns the first phase in snap whose status is
// PhaseStatusBlocked, or nil if none. Used by attachJSONL for D-M8-13's
// eager-arm of approvalTimeoutSink.
func firstBlockedApprovalPhase(snap *domain.Change) *domain.Phase {
	if snap == nil {
		return nil
	}
	for i := range snap.Phases {
		if snap.Phases[i].Status == domain.PhaseStatusBlocked {
			return &snap.Phases[i]
		}
	}
	return nil
}

func attachTUI(parentCtx context.Context, d Deps, input application.AttachInput) error {
	output := chooseTUIOutput(d)

	prog, err := tui.NewProgram(tui.ProgramConfig{
		Output:  output,
		Browser: d.Browser,
	})
	if err != nil {
		return fmt.Errorf("attach: tui init: %w", err)
	}
	defer prog.Close() //nolint:errcheck

	attacher := d.AttacherFactory(prog.Bridge())

	attacherCtx, cancelAttacher := context.WithCancel(parentCtx)
	defer cancelAttacher()

	type attacherResult struct {
		res application.RunResult
		err error
	}
	resultCh := make(chan attacherResult, 1)
	go func() {
		res, err := attacher.Attach(attacherCtx, input, prog.Bridge())
		resultCh <- attacherResult{res: res, err: err}
	}()

	hint, runErr := prog.Run(parentCtx)

	cancelAttacher()

	rr := <-resultCh

	if hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}

	if runErr != nil {
		return runErr
	}
	if rr.err != nil {
		var exit *application.ExitError
		if errors.As(rr.err, &exit) {
			return exit
		}
		return rr.err
	}
	return nil
}
```

> **Note on the double `prog.Bridge()` call:** the first goes to the factory (constructing the Attacher), the second goes to `Attach(...)` as the explicit sink. Both reference the same singleton — `prog.Bridge()` returns a stable pointer. This mirrors the M7 `runTUI` pattern. If `Bridge()` returns a fresh wrapper each call, the construction-time sink is unused (the explicit-arg sink wins because of how Attacher passes it to Observe). Either way, the test `TestAttacherUsesProvidedSink` proves the explicit-arg sink wins.

`internal/adapters/inbound/cli/root.go` — add `AttacherFactory` type and field.

ADD near the top, alongside `RunnerFactory`:

```go
// AttacherFactory builds a *application.Attacher with the caller-provided sink.
type AttacherFactory func(sink inbound.EventSink) *application.Attacher
```

ADD to the `Deps` struct:

```go
// AttacherFactory mirrors RunnerFactory for `sophia attach` (M8).
AttacherFactory AttacherFactory
```

REPLACE `root.AddCommand(newStubCmd("attach", "Attach to an existing Change", "M8"))` with `root.AddCommand(newAttachCmd(d))`.

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/adapters/inbound/cli/...
```

Expected: PASS — six new attach tests plus all M3–M7 cli tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/attach.go \
        internal/adapters/inbound/cli/attach_test.go \
        internal/adapters/inbound/cli/root.go
git commit -m "feat(cli): add real sophia attach command (M8)"
```

---

## Phase 7 — CLI: status command (real)

### Task 7: Upgrade `cli/status.go` to use the new StatusReader and emit human/JSON output

**Files:**
- Modify: `internal/adapters/inbound/cli/status.go`
- Modify: `internal/adapters/inbound/cli/status_test.go`

The CLI command now passes the optional positional `<change-id>` arg to the StatusReader, supports `--json`, and prints a richer human-readable summary. Empty result still exits 0 (D-M8-03).

Human format (multiple lines, source-aware):

```
Change: 01HX
Status: running (current_phase=apply)
Project: ms-cotizacion
BaseRef: main
Source:  global
Updated: 2026-05-06T12:34:56Z
```

JSON format: the `orchestratorhttp.ChangeResponse` shape (D-M8-08) — same converter as `cli/changes.go` reuses. (Pull the `changeResponseFromDomain` helper out of `changes.go` into a shared `cli` file? Optional. For M8, duplicate it inline if it's small; refactor later. Below we DO call `changeResponseFromDomain` directly — `changes.go` exports it as a package-internal function so both files share.)

> **Verification gate:** read the post-Task-4 `internal/application/status.go` to confirm `StatusReport` is the type, `Resolve(ctx, ResolveInput) (StatusReport, error)` is the method, and `StatusSourceFlag/Project/Global/None` are the constants.

- [ ] **Step 1: Replace status_test.go (cli) with the M8 test set**

`internal/adapters/inbound/cli/status_test.go` — REPLACE with:

```go
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newStatusDeps(t *testing.T) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeStateStore, *fakes.FakeProjectConfigStore, *fakes.FakeGitInspector) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		Orch: orch, State: state, Git: git, ProjectStore: pc,
	}, application.StatusOptions{})
	return cli.Deps{StatusReader: r}, orch, state, pc, git
}

func TestStatusCommandEmptyExitsZero(t *testing.T) {
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})

	if err := c.Execute(); err != nil {
		t.Errorf("status with no resolution should NOT error (exit 0); got %v", err)
	}
	if !strings.Contains(out.String(), "No local change found") {
		t.Errorf("output missing empty-state message: %s", out.String())
	}
}

func TestStatusCommandPositionalArgFetches(t *testing.T) {
	deps, orch, _, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "ARG", Status: domain.ChangeStatusRunning, Project: "p"})
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "ARG"})

	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ARG") {
		t.Errorf("output missing change ID: %s", out.String())
	}
	if !strings.Contains(out.String(), "running") {
		t.Errorf("output missing status: %s", out.String())
	}
	if !strings.Contains(out.String(), "flag") {
		t.Errorf("output missing source=flag: %s", out.String())
	}
}

func TestStatusCommandFallsBackToGlobal(t *testing.T) {
	deps, orch, state, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "GLOB", Status: domain.ChangeStatusDone, Project: "p"})
	_ = state.SetGlobalLast(context.Background(), "GLOB")

	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "GLOB") {
		t.Errorf("output missing global change: %s", out.String())
	}
	if !strings.Contains(out.String(), "global") {
		t.Errorf("output missing source=global: %s", out.String())
	}
}

func TestStatusCommandJSONFlagEmitsObject(t *testing.T) {
	deps, orch, _, _, _ := newStatusDeps(t)
	orch.SeedChange(&domain.Change{ID: "ARG", Status: domain.ChangeStatusRunning, Project: "p"})

	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "ARG", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if got["change_id"] != "ARG" {
		t.Errorf("change_id = %v", got["change_id"])
	}
	if got["status"] != "running" {
		t.Errorf("status = %v", got["status"])
	}
}

func TestStatusCommandJSONEmptyEmitsNullOrEmptyObject(t *testing.T) {
	// Empty result with --json: spec is silent; we choose JSON null
	// (most parser-friendly; any consumer can null-check). Document via
	// test so future changes don't silently flip the shape.
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimSpace(out.String())
	if trimmed != "null" {
		t.Errorf("empty --json output = %q, want null", trimmed)
	}
}

func TestStatusCommandPropagatesExitCode3OnStaleArg(t *testing.T) {
	deps, _, _, _, _ := newStatusDeps(t)
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"status", "MISSING"})

	err := c.Execute()
	var exit *application.ExitError
	if !errAs(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}
```

> **Note on `errAs`:** keep one shared helper at the top of `cli_test/internals_test.go` or define it in this file:
>
> ```go
> import "errors"
> func errAs(err error, target any) bool { return errors.As(err, target) }
> ```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL — `cli/status.go` doesn't accept positional args, doesn't have `--json`, doesn't print the new format.

- [ ] **Step 3: Implement**

OVERWRITE `internal/adapters/inbound/cli/status.go`:

```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func newStatusCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [<change-id>]",
		Short: "Show status of a Change (resolution: arg → project-scoped → global → empty)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.StatusReader == nil {
				return fmt.Errorf("status: reader not wired")
			}
			in := application.ResolveInput{}
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				in.ChangeID = domain.ChangeID(strings.TrimSpace(args[0]))
			}
			report, err := d.StatusReader.Resolve(cmd.Context(), in)
			if err != nil {
				// Forward ExitError verbatim; cobra surfaces the code via main.
				var exit *application.ExitError
				if errors.As(err, &exit) {
					return exit
				}
				return err
			}
			w := cmd.OutOrStdout()
			if jsonOut {
				return printStatusJSON(w, report)
			}
			return printStatusHuman(w, report)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// printStatusHuman renders a multi-line human-readable summary.
func printStatusHuman(w io.Writer, r application.StatusReport) error {
	if r.IsEmpty {
		if _, err := fmt.Fprintln(w, "No local change found."); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w, "Use sophia changes or pass <change-id> explicitly.")
		return err
	}
	c := r.Change
	if _, err := fmt.Fprintf(w, "Change:  %s\n", c.ID); err != nil {
		return err
	}
	statusLine := string(c.Status)
	if c.CurrentPhaseID != "" {
		statusLine += fmt.Sprintf(" (current_phase=%s)", c.CurrentPhaseID)
	}
	if _, err := fmt.Fprintf(w, "Status:  %s\n", statusLine); err != nil {
		return err
	}
	if c.Project != "" {
		if _, err := fmt.Fprintf(w, "Project: %s\n", c.Project); err != nil {
			return err
		}
	}
	if c.BaseRef != "" {
		if _, err := fmt.Fprintf(w, "BaseRef: %s\n", c.BaseRef); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Source:  %s\n", r.Source); err != nil {
		return err
	}
	if !c.UpdatedAt.IsZero() {
		if _, err := fmt.Fprintf(w, "Updated: %s\n", c.UpdatedAt.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// printStatusJSON renders a single ChangeResponse object (or null when empty).
func printStatusJSON(w io.Writer, r application.StatusReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if r.IsEmpty || r.Change == nil {
		return enc.Encode(nil)
	}
	resp := changeResponseFromDomain(r.Change)
	return enc.Encode(resp)
}
```

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/adapters/inbound/cli/...
```

Expected: PASS — six new status tests plus all earlier cli tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/status.go \
        internal/adapters/inbound/cli/status_test.go
git commit -m "feat(cli): upgrade sophia status to fetch live snapshot (M8)"
```

---

## Phase 8 — Bootstrap wiring

### Task 8: `bootstrap/wire.go` — construct Lister, AttacherFactory, real StatusReader

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go` (if it exists; otherwise carry the smoke via `make build`)

Bootstrap grows three lines:

1. `lister := application.NewLister(application.ListerDeps{Orch: orch})`
2. `statusReader` is now constructed with the orchestrator: `application.NewStatusReader(application.StatusDeps{Orch: orch, State: state, Git: git, ProjectStore: projectStore}, application.StatusOptions{})`
3. `attacherFactory := func(sink inbound.EventSink) *application.Attacher { ... }`

The attacher factory builds a fresh `*application.Runner` with the caller's sink and wraps it in `application.NewAttacher`. Bootstrap doesn't share the `*Runner` instance between `RunnerFactory` and `AttacherFactory` — each command's invocation gets its own pair. This matches how the existing `RunnerFactory` works and keeps the lifecycle isolation crisp.

Inject all three into `cli.Deps`.

> **Verification gate:** read `internal/bootstrap/wire.go` to confirm the existing variable names: `orch`, `state`, `git`, `projectStore`, `stream` (SSE client). The new wires plug in next to them. If any rename happened, adapt.

- [ ] **Step 1: Modify wire.go**

In `internal/bootstrap/wire.go`, find the section that constructs `statusReader` (currently with three deps). REPLACE the entire `statusReader := ...` line with:

```go
statusReader := application.NewStatusReader(application.StatusDeps{
	Orch:         orch,
	State:        state,
	Git:          git,
	ProjectStore: projectStore,
}, application.StatusOptions{})
```

After the existing `runnerFactory := func(...) ...` block, ADD:

```go
lister := application.NewLister(application.ListerDeps{Orch: orch})

attacherFactory := func(sink inbound.EventSink) *application.Attacher {
	runner := application.NewRunner(application.RunnerDeps{
		Orch:        orch,
		State:       state,
		Git:         git,
		Sink:        sink,
		EventStream: stream,
	}, application.RunnerOptions{})
	return application.NewAttacher(application.AttacherDeps{
		Orch:   orch,
		State:  state,
		Git:    git,
		Runner: runner,
	})
}
```

Update the `cli.Deps` literal to include the new fields:

```go
deps := cli.Deps{
	Doctor:          doctor,
	Provisioner:     provisioner,
	Initializer:     initializer,
	StatusReader:    statusReader,
	Lister:          lister,
	Orch:            orch, // M8: cli.attachJSONL needs direct GetChange for D-M8-13 eager-arm
	RunnerFactory:   runnerFactory,
	AttacherFactory: attacherFactory,
	Resolver:        resolver,
	Browser:         browser,
	UserConfigPath:  userConfigPath,
	Version:         info.Version,
	Commit:          info.Commit,
	BuildDate:       info.BuildDate,
}
```

- [ ] **Step 2: Build the binary**

```bash
go build ./...
```

Expected: clean build. Any reference to the old `application.NewStatusReader(StatusDeps{...})` 3-arg form (without `Orch` field, without `StatusOptions`) will fail compilation — that's the canary.

- [ ] **Step 3: Run all tests**

```bash
go test -race ./...
```

Expected: PASS across the entire workspace. Bootstrap doesn't have its own unit tests today (it's a composition root), but every downstream cli/application test runs against the new dep injection.

- [ ] **Step 4: Commit**

```bash
git add internal/bootstrap/wire.go
git commit -m "feat(bootstrap): wire Lister + AttacherFactory + real StatusReader (M8)"
```

---

## Phase 9 — End-to-End coverage

### Task 9: `test/e2e/attach_workflow_test.go` — full run → detach → attach → done cycle

**Files:**
- Create: `test/e2e/attach_workflow_test.go`

Build-tag-gated (`//go:build e2e_smoke`) so it only runs when `make e2e` (or `go test -tags=e2e_smoke ./test/e2e/...`) is invoked. Spins up an extended in-process `httptest` orchestrator stub that:

1. Accepts `POST /api/v1/changes` and returns a fresh Change ID with `status:"running"`.
2. Holds `GET /api/v1/changes/{id}` requests, returning the current status from an in-memory map. The map is mutated by an external trigger (a chan) so the test can step the Change through `running → done`.
3. Serves `/events` SSE: emits a heartbeat every 200ms, plus phase events on demand. After the first `/events` connection closes (test-driven), subsequent connects return 401 to abort the retry loop fast.

The test flow:

1. Run `./bin/sophia run "msg" --no-tui --json` against the stub. Capture the assigned Change ID from the first `snap:` line.
2. Send a SIGTERM (or context-cancel) midway, simulating a detach.
3. Run `./bin/sophia attach <captured-id> --no-tui --json` against the same stub.
4. Cause the stub to mark the Change `done` and close the SSE.
5. Verify the second invocation exits 0, prints `done` in its JSONL output, and `last_change_id` is the captured ID.

This test is the gold standard for "attach observes a Change you've already started somewhere else". It catches snapshot/SSE race regressions (RM8-01) by being a real wire-to-wire round trip.

> **Verification gate:** read `test/e2e/run_polling_test.go` to confirm the existing patterns for httptest stubs, binary path resolution (`absBinary(t)`), and XDG isolation. Reuse those helpers verbatim.

- [ ] **Step 1: Write the test**

`test/e2e/attach_workflow_test.go`:

```go
//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSmokeAttachWorkflow validates the full run → (simulate detach) → attach
// → terminal cycle against an in-process orchestrator stub. The stub keeps
// the same Change alive across both CLI invocations.
//
// Verifies:
//   - exit 0 from `attach <change-id> --no-tui --json`
//   - JSONL stream contains the snapshot and the terminal "final_status":"done"
//   - last_change_id persisted to <stateRoot>/sophia/last_change_id
//
// This catches RM8-01 (snapshot/SSE race) and validates the M8 contract that
// `attach` and `run` share the same observation pipeline.
func TestSmokeAttachWorkflow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	binary := absBinary(t)

	var (
		mu             sync.Mutex
		changeStatus   = "running"
		ssePerChange   = map[string]int{}
		assignedID     = ""
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/changes":
			assignedID = "01HX-ATTACH-E2E"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"change_id":%q,"status":%q,"name":"msg","project":"p"}`, assignedID, changeStatus)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
			// Pull the change ID out of the path (/api/v1/changes/{id}/events).
			id := pathChangeID(r.URL.Path)
			ssePerChange[id]++
			if ssePerChange[id] > 1 {
				// Subsequent connections — abort the retry loop fast.
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			// Emit one phase event then close. The runner/attacher will then
			// call GetChange once to determine terminal status.
			fmt.Fprint(w, "event: phase.completed\nid: evt-1\ndata: {\"payload\":{\"status\":\"done\"}}\n\n")
			flusher.Flush()

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/changes/"):
			// GetChange — returns whatever changeStatus currently is.
			id := pathChangeID(r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"change_id":%q,"status":%q,"project":"p"}`, id, changeStatus)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Temp repo with .sophia.yaml.
	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	yaml := []byte("version: 1\nproject: p\nbase_ref: main\nartifact_store: engram\n")
	if err := os.WriteFile(filepath.Join(tmp, ".sophia.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	dataDir := t.TempDir()
	configDir := t.TempDir()

	// Step 1: run — stub will return done quickly because the SSE emits
	// phase.completed and the post-stream snapshot returns the running status,
	// then we flip it to done before the second invocation.
	mu.Lock()
	changeStatus = "running"
	mu.Unlock()

	runCmd := exec.Command(binary, "run", "msg", "--no-tui", "--json")
	runCmd.Dir = tmp
	runCmd.Env = append(os.Environ(),
		"SOPHIA_ORCHESTRATOR_URL="+srv.URL,
		"XDG_STATE_HOME="+stateDir,
		"XDG_DATA_HOME="+dataDir,
		"XDG_CONFIG_HOME="+configDir,
	)
	var runOut, runErr bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runErr
	// Run is expected to exit non-zero (status flipped to running mid-fetch),
	// but the captured ID is in the JSONL stream regardless.
	_ = runCmd.Run()

	mu.Lock()
	capturedID := assignedID
	mu.Unlock()
	if capturedID == "" {
		t.Fatalf("run did not assign change_id; stdout=%q stderr=%q", runOut.String(), runErr.String())
	}

	// Step 2: flip to done (simulating real-world progress).
	mu.Lock()
	changeStatus = "done"
	mu.Unlock()

	// Step 3: attach to the captured ID and wait for terminal.
	time.Sleep(50 * time.Millisecond) // let the stub settle
	atCmd := exec.Command(binary, "attach", capturedID, "--no-tui", "--json")
	atCmd.Dir = tmp
	atCmd.Env = runCmd.Env
	var atOut, atErr bytes.Buffer
	atCmd.Stdout = &atOut
	atCmd.Stderr = &atErr
	if err := atCmd.Run(); err != nil {
		t.Fatalf("attach failed: %v\nstdout: %s\nstderr: %s", err, atOut.String(), atErr.String())
	}

	// Verify each line is valid JSON.
	lines := strings.Split(strings.TrimRight(atOut.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines from attach")
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\n%s", err, l)
		}
	}
	if !strings.Contains(atOut.String(), `"final_status":"done"`) {
		t.Errorf("missing terminal status in attach output: %s", atOut.String())
	}

	// last_change_id persisted globally.
	gpath := filepath.Join(stateDir, "sophia", "last_change_id")
	if _, err := os.Stat(gpath); err != nil {
		t.Errorf("expected global last_change_id at %s: %v", gpath, err)
	}
	got, _ := os.ReadFile(gpath)
	if !strings.Contains(string(got), capturedID) {
		t.Errorf("global last_change_id = %q, want contains %q", got, capturedID)
	}
}

// pathChangeID extracts the change-id segment from /api/v1/changes/{id}[/...].
func pathChangeID(p string) string {
	const prefix = "/api/v1/changes/"
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, prefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}
```

> **Note on `absBinary(t)`:** that helper already exists in `test/e2e/run_polling_test.go`. Since both tests live in `e2e_test`, the helper is shared automatically. If for some reason it's been moved or renamed, adapt the binary-path resolution at the top of `TestSmokeAttachWorkflow` to match.

- [ ] **Step 2: Build the binary**

```bash
make build
```

Required because the e2e test execs `./bin/sophia` directly. If `make build` is unavailable, run `go build -o ./bin/sophia ./cmd/sophia` (adapt to the actual main package path).

- [ ] **Step 3: Run the e2e suite**

```bash
go test -tags=e2e_smoke ./test/e2e/... -timeout 60s
```

Expected: PASS for both `TestSmokeRunAgainstStub` (carry-over from M5) and `TestSmokeAttachWorkflow` (new).

- [ ] **Step 4: Commit**

```bash
git add test/e2e/attach_workflow_test.go
git commit -m "test(e2e): add run→attach→done smoke for M8"
```

---

## Phase 10 — Final validation pass + tag

### Task 10: vet + test + lint + smoke + tag

**Files:** none (verification only).

- [ ] **Step 1: vet + race tests**

```bash
go vet ./...
go test -race ./...
```

Expected: exit 0 across the workspace. If a flake appears under `-race`, raise the WaitFor / Sleep timeouts; do NOT silence `-race`.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```

Acceptable nolint patterns: existing precedents (the `//nolint:errcheck // best-effort` lines in Runner / Attacher / Lister). Fix new findings in place.

- [ ] **Step 3: Coverage**

```bash
go test -coverprofile=cover.out \
   ./internal/application/... \
   ./internal/adapters/inbound/cli/...
go tool cover -func=cover.out | tail -n 1
```

Expected: total ≥ 80%. The Lister and StatusReader are pure code; both should clear 90%+. The Attacher and the new cli commands depend on the Runner's `Observe` extraction and lipgloss-free printers; they should land around 80–85%.

- [ ] **Step 4: Binary smoke**

```bash
make build

# 1) Help text now lists the new commands
./bin/sophia --help | rg -e attach -e changes
./bin/sophia attach --help | rg approval-timeout
./bin/sophia changes --help | rg -e limit -e status -e project

# 2) Outside an orchestrator, exit 3 (network error or change-not-found)
./bin/sophia status MISSING --json
echo "status-missing exit=$?"

./bin/sophia attach MISSING --no-tui --json
echo "attach-missing exit=$?"

./bin/sophia changes
echo "changes exit=$?"

# 3) Status with no resolution exits 0
mkdir -p /tmp/sophia-empty && cd /tmp/sophia-empty
./bin/sophia status
echo "status-empty exit=$?"
cd -

# 4) JSON shapes parse
./bin/sophia changes --json | python3 -m json.tool > /dev/null && echo "changes json valid"
./bin/sophia status --json | python3 -m json.tool > /dev/null && echo "status json valid"
```

Expected:
- `--help` shows `attach` and `changes` (no longer "M8 stub").
- `status MISSING` → exit 3 with friendly error.
- `attach MISSING --no-tui --json` → exit 3 with friendly error.
- `changes` → either prints empty table (exit 0) or exits non-zero with orchestrator-down message; both are acceptable as smoke output.
- `status` with no resolution → exit 0, prints "No local change found.".
- All `--json` outputs parse via `python3 -m json.tool`.

- [ ] **Step 5: Interactive smoke (manual — described, executed by reviewer)**

Pre-req: a running orchestrator at `SOPHIA_ORCHESTRATOR_URL` (default localhost:9080) plus a `.sophia.yaml` in the working directory. Ideally with at least one in-progress Change.

1. **`sophia run` then `sophia attach` to the same Change ID:**
   ```bash
   ./bin/sophia run "smoke M8 attach"
   # Note the Change ID shown in the TUI header. Press Q to detach.
   ./bin/sophia attach <change-id>
   ```
   Expect: TUI reopens on the same Change. Timeline shows the current snapshot. SSE stream resumes. Pressing Q detaches cleanly. Reattaching is idempotent.

2. **`sophia changes` lists what you've done:**
   ```bash
   ./bin/sophia changes
   ./bin/sophia changes --limit 3
   ./bin/sophia changes --status running
   ./bin/sophia changes --project "" # all projects
   ./bin/sophia changes --json | python3 -m json.tool
   ```
   Expect: table aligns; flags work; JSON is parseable; project default falls back to `.sophia.yaml` when no `--project` given.

3. **`sophia status` resolution order:**
   ```bash
   ./bin/sophia status                     # falls back to project-scoped or global
   ./bin/sophia status <change-id>         # explicit arg
   ./bin/sophia status --json
   cd /tmp && ./bin/sophia status          # outside repo → falls back to global
   cd /tmp/empty && ./bin/sophia status    # no global state → "No local change found." exit 0
   ```
   Expect: each resolution case behaves per spec §2.5.

4. **Stale local last_change_id:**
   Manually edit `~/.local/state/sophia/last_change_id` to point to a non-existent Change. Run `sophia status`. Expect: exit 3 with `change not found`. Document for the user that `sophia changes --limit 1` recovers a valid ID.

5. **`attach` with TUI on an already-terminal Change:**
   Find a Change whose `status: done`. `sophia attach <id>`. Expect: TUI opens, renders the snapshot, immediately closes (OnComplete fires; finish returns code 0). No SSE subscription appears in orchestrator logs.

6. **`attach` with `--approval-timeout` in JSONL mode:**
   ```bash
   ./bin/sophia attach <id-with-pending-approval> --no-tui --json --approval-timeout=10s
   ```
   With the orchestrator emitting `approval.required` and never resolving, expect: run terminates after 10s with exit 5. Same behavior as `run`'s timeout (RM8-08).

If ANY step fails, file an issue or stop the M8 ship.

- [ ] **Step 6: Integration smoke (carry-over)**

```bash
go test -race ./test/integration/...
```

Expected: PASS for the M5 SSE reconnect + M3 init/filestate tests. M8 didn't touch those paths.

- [ ] **Step 7: e2e smoke**

```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```

Expected: PASS — `TestSmokeRunAgainstStub` (M5) + `TestSmokeAttachWorkflow` (M8) + `TestVersionSmoke` (M1) + `TestDoctorSmoke` (M2).

- [ ] **Step 8: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m8): final validation pass" || echo "nothing to commit"
git tag -a m8-attach-changes-status -m "M8 attach + changes + real-status complete"
git tag
```

---

## Self-review checklist

- [ ] **Spec coverage:** every M8 DoD item from spec §7.2 has at least one task.
  - `sophia attach <change-id>`: snapshot + stream + persist last_change_id → Tasks 2, 3, 6
  - `sophia changes [--limit] [--status] [--project] [--json]` → Tasks 1, 5
  - `sophia status [<change-id>] [--json]` with §2.5 resolution order → Tasks 4, 7
  - Both `run` and `attach` update project-scoped + global `last_change_id` (§3.5) → Task 3 (Attacher.persistChangeID mirrors Runner.persistChangeID)
  - Empty `status` exits 0, NOT an error (§2.5) → Task 7
  - JSONL approval-timeout applies to attach too (§5.8) → Task 6 (`approvalTimeoutSink` reused)
- [ ] **Spec §2.3 — Exit codes:** 0=DONE, 1=BLOCKED/FAILED, 3=config/orchestrator-unreachable/change-not-found, 4=transient/ctx-canceled, 5=approval-timeout. Verified across Runner.Observe, Attacher.Attach, StatusReader.Resolve.
- [ ] **Spec §3.5 — last_change_id invariant:** project-scoped fingerprint + global file, both updated on `run` AND `attach`. Atomic via the existing `outbound.StateStore` impl.
- [ ] **Spec §5.1 — OrchestratorClient surface:** no method added; `GetChange`, `ListChanges`, `CreateChange`, `Healthz` already exist.
- [ ] **No placeholders:** no "TBD"/"TODO"/"similar to" in steps.
- [ ] **No new outbound port:** every adapter method M8 uses already exists in M4–M7.
- [ ] **No new domain types:** `domain.Change`, `domain.ChangeStatus`, `domain.ChangeID`, `domain.Phase` reused unchanged.
- [ ] **Frequent commits:** every task ends with a commit.
- [ ] **TDD discipline:** failing test before implementation in every Phase 1–9 task.
- [ ] **Refactor safety (RM8-02):** Runner.Run public API unchanged; existing M5/M7 tests stay green; the Observe extraction is internal restructure only.
- [ ] **Mode validation reused:** `cli/attach.go` calls the same `validateModeFlags(noTUI, jsonOut)` helper as `cli/run.go`. No new validation logic.
- [ ] **Approval timeout sink reused:** `cli/attach.go`'s JSONL path uses `newApprovalTimeoutSink(...)` exactly like `cli/run.go`'s does. RM8-08 verified.
- [ ] **No premature M9+ scope:** no `--orchestrator-url` per-call rebinding, no `--watch`, no Last-Event-ID resume across invocations, no advanced filters.

---

## Pending decisions (carried into M8 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M8-01 | Default `--limit` for `changes` | `10` (per spec §2.5 / §5.1). Higher values pass through unchanged; orchestrator decides upper bound. |
| D-M8-02 | `changes` output format default | Human-readable `text/tabwriter` table; `--json` switches to JSON array. Columns: ID, STATUS, PROJECT, BASE_REF, CREATED_AT. |
| D-M8-03 | `status` no-args empty state | Print "No local change found. Use sophia changes or pass <change-id> explicitly." and exit 0 (per spec §2.5). NOT an error. |
| D-M8-04 | `attach` updates `last_change_id`? | YES — both project-scoped + global, atomically (per spec §3.5). Mirrors Runner's persistChangeID. |
| D-M8-05 | Refactor strategy for shared observe logic | Option A: extract `Runner.Observe(ctx, RunResult, sink) (RunResult, error)` as exported method; both `Run` and `Attacher.Attach` call it. Single source of truth. |
| D-M8-06 | `--orchestrator-url` flag for attach/status/changes | NO — same as run; defer to M9+. Bootstrap-time URL only. |
| D-M8-07 | `changes --project=""` semantics | Empty string flag value = no project filter (list all). To use the project from `.sophia.yaml`, omit the flag entirely. cobra's `Flags().Changed("project")` distinguishes the two. |
| D-M8-08 | `status` / `changes` `--json` schema | Reuse `orchestratorhttp.ChangeResponse` JSON shape. NO custom envelope. `status --json` emits a single object (or `null` when empty); `changes --json` emits a JSON array. |
| D-M8-09 | `status --json` empty-state shape | Emit JSON `null` on empty (most parser-friendly). Documented via `TestStatusCommandJSONEmptyEmitsNullOrEmptyObject`. Alternative `{}` was considered and rejected — `null` is unambiguously "no result". |
| D-M8-10 | Should `attach` accept `--message`/`--base-ref`? | NO — `attach` does NOT create. Those flags belong only to `run`. |
| D-M8-11 | Should `Attacher` re-fetch the snapshot before subscribing? | NO. `GetChange` is the snapshot; the post-stream-end refresh in `Observe` covers reconnect-after-close. M9+ may add periodic refresh for long-running attaches. |
| D-M8-12 | Should `status` with `<change-id>` arg also update `last_change_id`? | NO. `status` is a read-only command; it does NOT touch persistent state. `attach` is the way to "remember" a change. Documented by absence of persistChangeID call in Task 4's StatusReader. |
| D-M8-13 | When `attach` connects to a Change already pending approval, when does `--approval-timeout` start? | **From the moment of attach, NOT from the original `approval.required` event.** Rationale: the user just invoked `attach`; they should not inherit a budget that may already be near-expiry (or already-expired) from when the original approval event fired. Implementation (avoid double-`GetChange`): `Attacher` exposes a second entry point `AttachFromSnapshot(ctx, snap, project, sink)` that takes a pre-fetched snapshot and skips its own `GetChange`. `cli.attachJSONL` does the orchestration: (a) `GetChange(ctx, id)`; (b) scan `snap.Phases` for any phase whose `Status == PhaseStatusBlocked`; (c) if found, synthesize `domain.ApprovalGate{Phase: blocked.Type, ChangeID: snap.ID}` and call `wrapped.OnApprovalGate(ctx, gate)` to eager-arm the timer with `time.Now()` as t=0; (d) call `Attacher.AttachFromSnapshot(ctx, snap, project, wrapped)`. The `wrapped` sink is `approvalTimeoutSink(inner, timeout, cancel)`; calling `OnApprovalGate` flows through to the inner sink (jsonsink prints a synthetic gate row marking the resume) and starts the timer. When a real `approval.required` event later arrives via SSE with full payload, the timer is **already running**, so `startTimer` must be a no-op (cambio 3 below). The TUI path does not eager-arm in `wrapped` because the TUI doesn't use `approvalTimeoutSink` — it draws its own banner from snapshot data. **Cambio 3 — `timeout_sink.go::startTimer` correction (Task 6 sub-step):** if `s.timer != nil && !s.fired`, do NOT stop/reset; just refresh `s.gate` to keep the snapshot of the most recent gate. `approval.resolved` (already handled in `observe`) clears `s.gate` and `s.timer` via `stopTimer`, so a SUBSEQUENT `approval.required` after a resolved correctly starts a new timer. Test coverage added in Task 6: (i) `TestApprovalTimeoutSinkDoesNotResetOnReGate` — first OnApprovalGate, advance fake clock, second OnApprovalGate, assert remaining duration is consistent with the first call (not reset); (ii) `TestApprovalTimeoutSinkResolvedThenNewGateStartsFresh` — gate, resolved, gate, assert timer fires after the second gate's full duration; (iii) `TestAttachJSONLEagerArmsTimeoutOnPendingApproval` — snapshot with blocked phase, no SSE traffic, assert timer fires after `--approval-timeout` from attach time and exit code is 5. |

---

## Risks specific to M8

| ID | Risk | Mitigation |
|---|---|---|
| RM8-01 | Snapshot/SSE race in `attach` (events arrive before snapshot is rendered) | `Attacher.Attach` calls `OnSnapshot(ctx, snap)` BEFORE handing off to `Runner.Observe`. Observe then subscribes to SSE; events arriving on that subscription land AFTER the snapshot. The TUI bridge's `ApplyEvent` is also defensive (Inv from M6: events before snapshot are buffered). Verified by `TestAttacherFetchesSnapshotPersistsAndObserves`. |
| RM8-02 | `Runner.Observe` extraction breaks existing M5 tests | The refactor preserves the `Run(ctx, RunInput) (RunResult, error)` public API. Internal helpers (`stream`, `dispatchEvent`, `refreshAfterStreamEnd`, `finish`) get `*WithSink` siblings; the originals delegate. Existing tests assert behavior, not internal method names. CI gate: ALL M5/M7 runner tests must stay green AFTER Task 2. |
| RM8-03 | `attach <change-id>` against a non-existent ID could infinite-poll | `GetChange` returns `domain.ErrChangeNotFound` (mapped from 404 by `orchestratorhttp.StatusError.Is`). Attacher returns `*ExitError{Code: 3}` immediately. No retry. Verified by `TestAttacherChangeNotFoundExitCode3`. |
| RM8-04 | `status` HTTP 5xx during fetch | `orchestratorhttp.StatusError.Is` maps 5xx to `domain.ErrUnreachable`. StatusReader.fetch wraps in `*ExitError{Code: 3}`. NO silent fallback to local cache (D-M8-12 — status is a snapshot of orchestrator truth). |
| RM8-05 | `changes` over-paging default | `--limit=10` is safe. Higher values pass through unchanged; orchestrator decides upper bound. The Lister itself does NOT clamp — clamping is the orchestrator's contract. |
| RM8-06 | Concurrent `attach` from multiple terminals | Each is independent; no shared CLI-side state. The orchestrator sees multiple subscribers — its concern, not the CLI's. The state file write race (`SetGlobalLast`) is already thread-safe via the existing filestate adapter (`os.WriteFile` is atomic on POSIX). |
| RM8-07 | Project filter resolution order in `changes` | `--project FOO` always wins. No flag → resolver default (from `.sophia.yaml`). `--project=""` → no filter (list all). Documented in D-M8-07 and verified by `TestChangesCommandProjectDefaultFromConfig` / `TestChangesCommandProjectFlagOverrides` / `TestChangesCommandEmptyProjectFlagMeansNoFilter`. |
| RM8-08 | `--no-tui --json` mode for attach uses approval-timeout? | YES — same as run. The `approvalTimeoutSink` wrapper applies to any JSONL sink that observes events. Plumb the flag through `attach.go` same as `run.go`. Verified by reading `attachJSONL` in Task 6. |
| RM8-09 | `status`'s resolution order forgets the env var precedence | Spec §2.5 doesn't list env vars in the status resolution order — only arg → project → global → empty. The resolver's broader env-var handling (used by `run` and `changes`) does not apply to `status`. StatusReader does NOT read env vars; only `tryProject` (config-file-driven) and `GetGlobalLast` (state-file-driven). Documented by the empty `ResolveInput` containing no env field. |
| RM8-10 | Sink double-close in attach JSONL path (Attacher's `defer sink.Close()` + cli's wrapper Close) | Mirrors the M5/M7 Runner case: only the wrapper is exposed to Attacher; the cli's `runJSONL`/`attachJSONL` helpers do NOT call `chooseJSONSink`'s return Close directly. The wrapper's Close is idempotent. |
| RM8-11 | `Lister.List` returns `[]*domain.Change` but downstream expects sorted output | M8 does NOT sort — the orchestrator decides ordering. If the orchestrator returns unsorted results, the table output preserves that order. Spec §2.5 is silent on ordering; M9+ may add `--sort created-desc`. Documented in "what's NOT in M8". |
| RM8-12 | StatusReader silently ignores `tryProject` errors | **Resolved by cambio 4.** M3's behavior was "any error in tryProject falls through to global". M8 splits the behavior by command: `status` is strict — `domain.ErrInvalidYAML` from a malformed `.sophia.yaml` produces `ExitError{Code: 3}` and stops the resolution chain. `changes` keeps the lenient fall-through but emits a stderr warning. Missing `.sophia.yaml` (`ErrConfigMissing`) and "outside git repo" still fall through silently in both commands — they're "no project context" signals, not config corruption. Tests `TestStatusInvalidProjectYAMLExitCode3` and `TestStatusMissingProjectYAMLFallsThroughToGlobal` lock the contract. |

---

## What this plan does NOT cover (intentional)

- Cross-process `Last-Event-ID` resume across `attach` invocations → M9. `attach` always opens a fresh subscription; the orchestrator decides what to replay (it has the wire-level Last-Event-ID).
- `--orchestrator-url` per-call rebinding for `attach`/`status`/`changes` → M9+. Bootstrap-time URL only (env or default).
- `sophia changes` advanced filters (date range, agent role, free-text search, per-column sort) → M9+. M8 ships `--limit`, `--status`, `--project`, `--json` only.
- `sophia status --watch` for periodic refresh → M9+. Status is one-shot.
- Pagination UI for `changes` (next/prev page, cursor-based) → M9+.
- `sophia status` showing approval gate state visually → carries through whatever `ChangeResponse` provides via JSON; no extra rendering in human output.
- New TUI views — `attach` reuses Timeline + ApplyBoard + ApprovalBanner exactly as M6/M7 ship them. No "Snapshot Diff" view, no "Phase History" view.
- New ports — `outbound.OrchestratorClient` and `outbound.StateStore` already cover every M8 capability.
- New domain types — `domain.Change`, `domain.ChangeStatus`, `domain.ChangeID`, `domain.Phase` are reused unchanged.
- Persisting last_change_id from `sophia status` → no, status is read-only (D-M8-12).
- Auto-clearing stale local last_change_id when GetChange returns 404 → no. The user explicitly opts in via `sophia changes --limit 1` to find a fresh ID. M9+ may add `sophia status --clear-stale` if requested.
- `sophia attach --follow=false` (snapshot-only mode) → out of scope. Attach observes until terminal; for a one-shot snapshot the user runs `sophia status <id>`.
- Browser preference for `attach`'s approval-banner `[O]` shortcut → already covered by M7's `osbrowser` adapter, untouched in M8.

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-06-sophia-cli-m8-attach-changes-status.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task. Use `superpowers:subagent-driven-development`. Each task has a self-contained TDD cycle (write test → fail → implement → pass → commit), so subagents work independently with minimal context.

Recommended ordering for parallelism:

- Task 1 (Lister) — independent of every other task; ship first.
- Task 2 (Runner.Observe extraction) — must land before Task 3 (Attacher) and Task 6 (cli/attach). Independent of Tasks 1, 4, 5, 7.
- Task 4 (StatusReader upgrade) — independent of Task 2; can run in parallel with Tasks 1 and 2.
- Task 3 (Attacher) — depends on Task 2.
- Tasks 5, 6, 7 (cli changes / attach / status) — Task 5 depends on Task 1; Task 6 depends on Tasks 2 and 3; Task 7 depends on Task 4. All three modify `cli/root.go`'s Deps; coordinate the three commits to avoid conflicts on the struct literal.
- Task 8 (bootstrap) — depends on Tasks 1, 3, 4. Last application-layer wiring step.
- Task 9 (e2e) — depends on Task 6 (and indirectly all others). Build-tag-gated; safe to ship after the unit/integration tests pass.
- Task 10 (final validation + tag) — gates the milestone close.

**2. Sequential single-agent** — use `superpowers:executing-plans` and walk Task 1 → Task 10 in order. Recommended only if you want to keep the full context window for cross-task surprises (most likely Task 2 (Runner.Observe extraction) and Task 6 (cli/attach factory wiring) if the existing helpers' shapes differ from this plan's assumptions).

Either way: keep an eye on the M5/M7 Runner tests after Task 2 — if any previously-passing test fails, the refactor is wrong. The Runner's behavior contract is invariant; only the internal structure changes.

---

## Implementation Notes — Deviations from Plan

### Pre-execution patch (2026-05-06) — five corrections before continuing M8

User audit caught five issues in the as-written plan and the M5/M7 code already on `main`. All five are addressed in-plan above; this entry records the deltas so the next executor doesn't re-apply them.

1. **Cambio 1 — `application.Lister` is a pure pass-through.** The original Task 1 description and `ListInput` struct carried a `IgnoreProjectDefault bool` field with prose that gestured at "Lister falls back to the project resolved from .sophia.yaml". That violated hexagonal layering — config-file resolution is a CLI concern. Fix: removed the field, simplified `ListInput` to `{Project, Status, Limit, Offset}`, moved the `.sophia.yaml`-driven default into `cli/changes.go` with a stderr warning when the resolver fails. **Side effect on already-merged code:** the M8 Task 1 commit (`e9795db` and the subsequent fakes wiring `45fb34f`-style commits) shipped the deprecated field. A follow-up cleanup commit needs to: drop `IgnoreProjectDefault` from `internal/application/lister.go` and its test; update any internal call sites; ensure `cli/changes.go` (Task 5) does the resolver dance instead.

2. **Cambio 2 — `Attacher.AttachFromSnapshot` second entry point.** D-M8-13 originally said "the CLI synthesizes an approval gate and calls `wrapped.OnApprovalGate` BEFORE `Attacher.Attach`", which would have caused two `GetChange` round-trips (one in CLI to scan blocked phases, one inside `Attacher.Attach`). Fix: `Attacher` now exposes `AttachFromSnapshot(ctx, snap, project, sink)` that takes a pre-fetched snapshot and runs only persist + OnSnapshot + Observe. `Attach(ctx, in, sink)` now delegates: it does the `GetChange` and hands the snapshot to `AttachFromSnapshot`. `cli.attachJSONL` does its own `GetChange` (via the new `Deps.Orch` field) and uses `AttachFromSnapshot` directly, eager-arming the timer between the two calls. Tests added: `TestAttacherFromSnapshotSkipsGetChange`, `TestAttacherFromSnapshotNilSnapshotExitCode3`.

3. **Cambio 3 — `approvalTimeoutSink.startTimer` is no-reset while armed.** The M7 implementation in `internal/adapters/inbound/cli/timeout_sink.go` (committed in `c45e25e` as part of the M7 milestone tag) stops and restarts the timer on every `OnApprovalGate` call, which would defeat D-M8-13's eager-arm guarantee — a fresh `approval.required` arriving via SSE would reset the t=0 anchor. Fix: when `s.timer != nil && !s.fired`, refresh `s.gate` but skip the `time.AfterFunc` reset. The existing `stopTimer` clears both `s.timer` and `s.gate` on `approval.resolved` / `OnComplete` / `Close`, so a SUBSEQUENT `approval.required` after a resolved correctly starts a fresh timer. New tests in `timeout_sink_test.go`: `TestApprovalTimeoutSinkDoesNotResetOnReGate`, `TestApprovalTimeoutSinkResolvedThenNewGateStartsFresh`, `TestApprovalTimeoutSinkSecondGatePreservesArmTime`. Lands as Sub-task 6.A with its own commit (`fix(cli): approvalTimeoutSink no longer resets on re-emit`).

4. **Cambio 4 — `StatusReader` distinguishes invalid vs missing `.sophia.yaml`.** RM8-12 originally said "M8 keeps M3's silent fall-through" for any tryProject error. That hides config corruption. Fix: reuse the **already-existing** `domain.ErrInvalidYAML` sentinel (defined in `internal/domain/errors.go` since M3 / yamlconfig adapter); `tryProject` surfaces it verbatim; `StatusReader.locate` maps it to `ExitError{Code: 3}`. Missing config (`ErrConfigMissing`) and "outside repo" still fall through to global. The `changes` command keeps its lenient warn-and-fall-through behavior. New tests: `TestStatusInvalidProjectYAMLExitCode3`, `TestStatusMissingProjectYAMLFallsThroughToGlobal`. **No new sentinel needed** — verified `ErrInvalidYAML` already covers parse failures and missing required fields. The yamlconfig adapter currently returns it bare; consumers either compare equality (M3 tests) or `errors.Is` (M8 tests) — both work.

5. **Cambio 5 — fetch timeout is exit 4, not exit 3.** The original Task 4 mapping was "ctx cancel → 4; everything else → 3". That conflated a slow/stuck orchestrator (transient — try again) with a config-stale cache (fatal — clear it). Fix: `StatusReader.fetch` now maps both parent ctx cancel and the internal `FetchTimeout` `context.DeadlineExceeded` to exit 4, while 404 / 5xx / `ErrUnreachable` stay at exit 3. Detection uses both `errors.Is(err, context.DeadlineExceeded)` and `errors.Is(fctx.Err(), context.DeadlineExceeded)` to defend against HTTP clients that wrap the inner ctx error. New test: `TestStatusInternalFetchTimeoutExitCode4`.

### Pre-Task-3 follow-up (cleanup of already-shipped M5–M7 code)

Before continuing M8 Task 3, two pre-existing commits need follow-up commits to bring them in line with the patched plan:

- **Lister cleanup:** drop `IgnoreProjectDefault` from `internal/application/lister.go` + test. Single mechanical commit.
- **`approvalTimeoutSink` fix:** apply the cambio 3 patch + tests in `internal/adapters/inbound/cli/timeout_sink.go`. This was originally tagged as part of M7 (`m7-applyboard-approval`); the fix lands on `main` as a follow-up commit referenced as Sub-task 6.A. M7's tag is not retroactively edited; the milestone history shows M7 shipped with the bug and M8 fixed it before depending on the corrected behavior.

Both cleanups should run BEFORE the implementer subagent starts Task 3 so the working tree matches the as-patched plan.
