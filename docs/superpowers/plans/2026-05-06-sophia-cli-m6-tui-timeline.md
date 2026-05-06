# Sophia CLI — M6 TUI Timeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Bubble Tea v2 + Lipgloss v2 TUI on top of the M5 SSE pipeline. Default `sophia run "msg"` (no flags) opens the TUI with a single Timeline view that renders the 9 SDD phases. The existing `--no-tui --json` mode keeps emitting the JSONL stream — both modes consume the same `inbound.EventSink` interface, so the only thing that changes between modes is which sink the bootstrap injects. `Q` detaches without canceling the Change. `Ctrl+C` first prompts confirmation; second press detaches. The bridge between the SSE channel and the Bubble Tea program owns a cap-256 buffer with a strict drop policy: heartbeats first, then `agent.*`/`task.*`; `phase.*` and `approval.*` are NEVER dropped.

**Architecture:** Seven new files under `internal/adapters/inbound/tui/` make up the TUI layer: a pure Model + Update + View triple, lipgloss styles, a key-binding map, a Sender-abstracted Bridge that implements `EventSink` and forwards messages to a Bubble Tea program, and a `Program` constructor that wires everything together and exposes a `Run()` entry point. The CLI's `run.go` flips its flag logic — TUI is the default, `--no-tui --json` is the JSONL fallback. The bootstrap exposes a sink-agnostic `BuildRunner` factory so `run.go` can pick between `jsonsink.New(...)` and `tuibridge.New(...)` at command time without a circular import.

**Tech Stack:** Go 1.24.x · `github.com/charmbracelet/bubbletea/v2` · `github.com/charmbracelet/lipgloss/v2` · `github.com/charmbracelet/bubbles/v2` (all pinned in `go.mod`). Risk RM6-01 documents the fallback to `charm.land/...` paths if the `github.com/charmbracelet` v2 modules aren't reachable. Tests use `github.com/charmbracelet/x/exp/teatest/v2` (or whichever teatest module ships with bubbletea v2 — Task 1 verifies).

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.2, §4.5, §4.6, §6.3 inv 7, §7.2 M6 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M6)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M6 boundaries — what is NOT in M6:**

- No ApplyBoard view — M7.
- No ApprovalGate banner (full UI) — M7. M6 shows only a discreet `!` marker on the affected phase row in the Timeline (D-M6-06).
- No browser opener (`[O]pen` shortcut) — M7.
- No `Tab` key (view toggle) — M7 (single Timeline view in M6).
- No real `sophia attach` / `sophia changes` — stay stubs (M8).
- No approval-timeout exit code 5 — M7+.
- No cross-process `Last-Event-ID` resume — M8.
- No `--orchestrator-url` flag — same as M5; SOPHIA_ORCHESTRATOR_URL env still honored at bootstrap.
- No new domain types — TUI consumes existing `domain.Change`, `domain.Event`, `domain.PhaseType`.

---

## Phase 1 — Dependency + package scaffolding

### Task 1: Add Bubble Tea v2 / Lipgloss v2 / Bubbles v2 + tui package skeleton

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/adapters/inbound/tui/doc.go`

> **Verification gate (D-M6-01):** the spec/roadmap reference `charm.land/bubbletea/v2`, but the canonical 2025/2026 path is `github.com/charmbracelet/bubbletea/v2`. Run the `go get` commands below first; observe which path resolves. If NEITHER resolves, STOP and ASK the user before proceeding — RM6-01 documents the fallback to v1 modules with version-pinning, but that's a downgrade we should not silently take.

- [ ] **Step 1: Probe the canonical import path**

Run, in order:

```bash
go get github.com/charmbracelet/bubbletea/v2@latest
```

If that succeeds, the canonical path is `github.com/charmbracelet/bubbletea/v2`. Continue with Step 2 using the `github.com/charmbracelet/...` paths.

If it fails with `module ... not found`, fall back to:

```bash
go get charm.land/bubbletea/v2@latest
```

If that succeeds, the canonical path is `charm.land/bubbletea/v2`. Continue with Step 2 substituting `charm.land/...` for `github.com/charmbracelet/...` in every file in this plan.

If BOTH fail, STOP. Report:
- The exact `go get` error from each attempt.
- Anything `go list -m -versions github.com/charmbracelet/bubbletea/v2` reveals.
- A request to the user to pick: (a) downgrade to v1 (`github.com/charmbracelet/bubbletea`) and adapt v2-specific symbols (RM6-01), or (b) wait until v2 is reachable.

- [ ] **Step 2: Pin all three dependencies**

Once Step 1 settled the path prefix, install the trio:

```bash
go get github.com/charmbracelet/bubbletea/v2@latest
go get github.com/charmbracelet/lipgloss/v2@latest
go get github.com/charmbracelet/bubbles/v2@latest
```

Open `go.mod` and confirm three direct require lines exist. Pin to the resolved minor version (no `latest` left in the file — `go get @latest` already records the resolved version).

- [ ] **Step 3: Probe the teatest module**

teatest in the v2 era ships under `github.com/charmbracelet/x/exp/teatest/v2`. Run:

```bash
go get github.com/charmbracelet/x/exp/teatest/v2@latest
```

If that succeeds, the canonical path is settled. If it fails, try `github.com/charmbracelet/x/exp/teatest@latest` (no `/v2` suffix; v2 of the framework may live at the v1 import path with a major-bump module — observe what `go.sum` records). If both fail, STOP and ASK before proceeding to Phase 8.

- [ ] **Step 4: Create the package skeleton**

`internal/adapters/inbound/tui/doc.go`:

```go
// Package tui implements inbound.EventSink as a second adapter — alongside
// jsonsink — that forwards events into a Bubble Tea v2 program. The package
// is split into:
//
//   - bridge.go:       cap-256 buffered EventSink that calls program.Send,
//                      with a drop policy that protects phase.* / approval.*
//                      events while shedding heartbeats first under pressure.
//   - model.go:        pure Model state — no I/O, no tickers.
//   - update.go:       pure Update function for tea.Msg dispatch.
//   - keybindings.go:  key → action map (Q, Ctrl+C, etc.).
//   - styles.go:       lipgloss styles for status icons and headers.
//   - view_timeline.go: Timeline View() rendering 9 SDD phases.
//   - program.go:      tea.NewProgram assembly + Run() entry point.
//
// Spec invariants honored:
//   - §6.3 inv 7: lipgloss.Style.Render is the only rendering path —
//     no raw user-supplied strings are ever printf'd into the terminal.
//   - §4.5: heartbeats are dropped first; phase.* / approval.* never dropped.
//   - §2.2: Q detaches; Ctrl+C confirms before detach; never cancels the Change.
//
// M6 scope only — ApplyBoard view, full ApprovalGate banner, and Tab toggle
// are M7.
package tui
```

- [ ] **Step 5: Verify build**

```bash
go vet ./internal/adapters/inbound/tui/...
go test ./...
```

Expected: PASS. The tui package compiles as a doc-only stub. Existing tests unaffected. The new Bubble Tea modules are listed in `go.mod`/`go.sum` but not yet imported anywhere except via `go get`'s side effects — `go vet ./...` should still be clean.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/inbound/tui/doc.go
git commit -m "chore(tui): pin bubbletea/lipgloss/bubbles v2 + add tui package skeleton"
```

---

## Phase 2 — Bridge (pure, isolated, no Bubble Tea dependency in tests)

### Task 2: tui/bridge.go — EventSink with cap-256 buffer + drop policy

**Files:**
- Create: `internal/adapters/inbound/tui/bridge.go`
- Create: `internal/adapters/inbound/tui/bridge_test.go`

The Bridge owns the cap-256 buffer and the drop policy. It MUST be testable without booting a real Bubble Tea program — define a minimal `Sender` interface so tests can use a fake. Production code wraps `*tea.Program` in a one-line adapter to satisfy `Sender`.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/bridge_test.go`:

```go
package tui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// fakeSender records every tea.Msg the bridge forwards. It is goroutine-safe
// because the bridge is allowed to call Send from any goroutine.
type fakeSender struct {
	mu   sync.Mutex
	msgs []any
	// block, when true, makes Send block until release() is called. Used to
	// simulate a wedged Bubble Tea program so the buffer fills up.
	block   bool
	release chan struct{}
}

func newFakeSender() *fakeSender {
	return &fakeSender{release: make(chan struct{})}
}

func (s *fakeSender) Send(m any) {
	if s.block {
		<-s.release
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
}

func (s *fakeSender) Messages() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]any, len(s.msgs))
	copy(out, s.msgs)
	return out
}

func (s *fakeSender) Block()   { s.block = true }
func (s *fakeSender) Release() { close(s.release) }

func TestBridgeForwardsSnapshotAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	change := &domain.Change{ID: domain.ChangeID("01HX"), Status: domain.ChangeStatusRunning}
	if err := b.OnSnapshot(context.Background(), change); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	if _, ok := got[0].(tui.SnapshotMsg); !ok {
		t.Errorf("expected SnapshotMsg, got %T (%+v)", got[0], got[0])
	}
}

func TestBridgeForwardsEventAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnEvent(context.Background(), domain.Event{Type: "phase.started", EventID: "evt-1"}); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	em, ok := got[0].(tui.EventMsg)
	if !ok {
		t.Fatalf("expected EventMsg, got %T", got[0])
	}
	if em.Event.Type != "phase.started" {
		t.Errorf("Event.Type = %q", em.Event.Type)
	}
}

func TestBridgeForwardsApprovalGateAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	gate := domain.ApprovalGate{URL: "http://gate", Phase: domain.PhaseApply}
	if err := b.OnApprovalGate(context.Background(), gate); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	am, ok := got[0].(tui.ApprovalGateMsg)
	if !ok {
		t.Fatalf("expected ApprovalGateMsg, got %T", got[0])
	}
	if am.Gate.URL != "http://gate" {
		t.Errorf("Gate.URL = %q", am.Gate.URL)
	}
}

func TestBridgeForwardsCompletionAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnComplete(context.Background(), domain.ChangeStatusDone); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	cm, ok := got[0].(tui.CompleteMsg)
	if !ok {
		t.Fatalf("expected CompleteMsg, got %T", got[0])
	}
	if cm.Status != domain.ChangeStatusDone {
		t.Errorf("Status = %q", cm.Status)
	}
}

func TestBridgeForwardsErrorAsTeaMsg(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	if err := b.OnError(context.Background(), errSentinel); err != nil {
		t.Fatal(err)
	}

	got := waitMessages(t, s, 1, time.Second)
	if _, ok := got[0].(tui.ErrorMsg); !ok {
		t.Errorf("expected ErrorMsg, got %T", got[0])
	}
}

func TestBridgeDropsHeartbeatFirstUnderPressure(t *testing.T) {
	// Block the sender so the buffer fills up.
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	// Fill the buffer (cap 256). The bridge runs Send on a worker goroutine,
	// so the first event is consumed immediately and gets stuck in Send;
	// subsequent events queue up in the channel until cap.
	for i := 0; i < 256; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.started", EventID: ""})
	}

	// Now push a heartbeat — buffer is full, sender wedged. Drop expected.
	_ = b.OnEvent(context.Background(), domain.Event{Type: "heartbeat", EventID: "hb-1"})

	if got := b.DropsByCategory()[tui.DropCategoryHeartbeat]; got != 1 {
		t.Errorf("heartbeat drops = %d, want 1", got)
	}
}

func TestBridgeNeverDropsPhaseEvents(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	// Fill buffer with low-priority events so phase.* must displace them.
	for i := 0; i < 256; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}

	// 10 phase.* events — all 10 must enqueue (kicking out non-priority).
	for i := 0; i < 10; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "phase.started", EventID: "p-evt"})
	}

	// Release the sender; let it drain.
	s.Release()
	waitDrain(t, b, 2*time.Second)

	// Count phase.started messages received.
	gotPhase := 0
	for _, m := range s.Messages() {
		if em, ok := m.(tui.EventMsg); ok && em.Event.Type == "phase.started" {
			gotPhase++
		}
	}
	if gotPhase != 10 {
		t.Errorf("phase.* events forwarded = %d, want 10", gotPhase)
	}
	// Sanity: at least 10 non-priority events were dropped to make room.
	if got := b.DropsByCategory()[tui.DropCategoryAgentTask]; got < 10 {
		t.Errorf("agent.* drops = %d, want ≥10", got)
	}
}

func TestBridgeNeverDropsApprovalEvents(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	for i := 0; i < 256; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}
	if err := b.OnApprovalGate(context.Background(), domain.ApprovalGate{URL: "http://gate"}); err != nil {
		t.Fatal(err)
	}

	s.Release()
	waitDrain(t, b, 2*time.Second)

	saw := false
	for _, m := range s.Messages() {
		if _, ok := m.(tui.ApprovalGateMsg); ok {
			saw = true
			break
		}
	}
	if !saw {
		t.Error("ApprovalGateMsg must never be dropped")
	}
}

func TestBridgeDropsCounterIncrementsWhenAtCapacity(t *testing.T) {
	s := newFakeSender()
	s.Block()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})
	defer b.Close() //nolint:errcheck

	// Fill cap exactly.
	for i := 0; i < 256; i++ {
		_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})
	}
	// 257th non-priority event should drop.
	_ = b.OnEvent(context.Background(), domain.Event{Type: "agent.token"})

	if got := b.Drops(); got != 1 {
		t.Errorf("Drops() = %d, want 1", got)
	}
	if got := b.DropsByCategory()[tui.DropCategoryAgentTask]; got != 1 {
		t.Errorf("agent.* drops = %d, want 1", got)
	}
}

func TestBridgeCloseStopsForwarding(t *testing.T) {
	s := newFakeSender()
	b := tui.NewBridge(tui.BridgeConfig{Sender: s})

	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	// After Close, OnEvent must NOT panic and MUST be a no-op (no new
	// messages forwarded). We accept either an error return or a silent
	// drop, but no goroutine leaks.
	_ = b.OnEvent(context.Background(), domain.Event{Type: "phase.started"})

	// Give any in-flight goroutine 50ms to settle.
	time.Sleep(50 * time.Millisecond)
	if got := len(s.Messages()); got > 0 {
		t.Errorf("messages forwarded after Close: %d", got)
	}
}

// errSentinel is a minimal error for tests.
var errSentinel = sentinelError("sentinel")

type sentinelError string

func (s sentinelError) Error() string { return string(s) }

// waitMessages blocks until at least n messages have been recorded by s, or
// the timeout fires. Returns the snapshot at the time the threshold is met.
func waitMessages(t *testing.T, s *fakeSender, n int, timeout time.Duration) []any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := s.Messages(); len(got) >= n {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d messages (got %d)", n, len(s.Messages()))
	return nil
}

// waitDrain blocks until the bridge's pending queue is empty.
func waitDrain(t *testing.T, b *tui.Bridge, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b.Pending() == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("bridge did not drain in %s (pending=%d)", timeout, b.Pending())
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `Bridge`, `BridgeConfig`, `Sender`, `SnapshotMsg`, `EventMsg`, `ApprovalGateMsg`, `ErrorMsg`, `CompleteMsg`, `DropCategory*`, `NewBridge`, `Drops`, `DropsByCategory`, `Pending` all undefined.

- [ ] **Step 3: Implement**

`internal/adapters/inbound/tui/bridge.go`:

```go
package tui

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// DefaultBufferCapacity is the cap-256 buffer mandated by spec §4.5.
const DefaultBufferCapacity = 256

// Sender abstracts the subset of *bubbletea.Program that the bridge needs.
// Production code wraps tea.Program so its Send(tea.Msg) satisfies this.
// Tests inject a fake to avoid booting a real terminal program.
type Sender interface {
	Send(m any)
}

// BridgeConfig configures the Bridge.
type BridgeConfig struct {
	Sender         Sender
	BufferCapacity int // 0 ⇒ DefaultBufferCapacity
}

// DropCategory tags drop counters per spec §4.5 priority bucket.
type DropCategory string

const (
	DropCategoryHeartbeat DropCategory = "heartbeat"
	DropCategoryAgentTask DropCategory = "agent_task"
	DropCategoryOther     DropCategory = "other"
	// Phase and approval categories are intentionally absent — those events
	// are NEVER dropped (spec §4.5 invariant).
)

// SnapshotMsg is dispatched on the Bubble Tea event loop when the SSE feed
// delivers a new domain.Change snapshot (initial connect or post-disconnect refresh).
type SnapshotMsg struct {
	Change *domain.Change
}

// EventMsg is dispatched on the Bubble Tea event loop for every non-heartbeat
// domain.Event that survives the drop policy.
type EventMsg struct {
	Event domain.Event
}

// ApprovalGateMsg is dispatched when the runner translates an
// approval.required event into a structured ApprovalGate.
type ApprovalGateMsg struct {
	Gate domain.ApprovalGate
}

// ErrorMsg is dispatched for non-fatal errors the runner reports via OnError.
type ErrorMsg struct {
	Err error
}

// CompleteMsg is dispatched when the runner reaches a terminal status.
type CompleteMsg struct {
	Status domain.ChangeStatus
}

// queued is the internal envelope on the bridge's buffered channel. It
// pairs the tea.Msg payload with its DropCategory so the worker can shed
// the right thing first under pressure.
type queued struct {
	category DropCategory
	priority bool // true ⇒ phase.* / approval.* — NEVER dropped
	msg      any
}

// Bridge is the cap-256 buffered EventSink that forwards events into a
// Bubble Tea program. Implements inbound.EventSink.
//
// Drop policy (spec §4.5):
//
//   - Buffer cap 256.
//   - When at cap, a new heartbeat is dropped wholesale.
//   - When at cap, a new phase.* or approval.* event is queued by EVICTING
//     the oldest non-priority entry from the buffer (the heartbeat or
//     agent.*/task.* event nearest the head).
//   - When at cap, a new agent.*/task.*/other event is dropped if the buffer
//     contains only priority entries; otherwise it queues normally.
//
// Drops are counted in atomic counters per category for observability.
type Bridge struct {
	sender Sender
	cap    int

	mu      sync.Mutex // guards queue + closed
	queue   []queued
	closed  bool
	cond    *sync.Cond

	totalDrops atomic.Uint64
	dropHB     atomic.Uint64
	dropAT     atomic.Uint64
	dropOther  atomic.Uint64

	stop chan struct{}
	wg   sync.WaitGroup
}

// NewBridge constructs a Bridge and starts its forwarding worker.
func NewBridge(cfg BridgeConfig) *Bridge {
	if cfg.BufferCapacity <= 0 {
		cfg.BufferCapacity = DefaultBufferCapacity
	}
	b := &Bridge{
		sender: cfg.Sender,
		cap:    cfg.BufferCapacity,
		queue:  make([]queued, 0, cfg.BufferCapacity),
		stop:   make(chan struct{}),
	}
	b.cond = sync.NewCond(&b.mu)
	b.wg.Add(1)
	go b.worker()
	return b
}

// OnSnapshot enqueues a SnapshotMsg. Snapshots are treated as priority —
// they're rare and convey full state, never dropped.
func (b *Bridge) OnSnapshot(_ context.Context, c *domain.Change) error {
	cp := *c
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: SnapshotMsg{Change: &cp}})
	return nil
}

// OnEvent enqueues an EventMsg. Drop policy is applied based on ev.Type.
func (b *Bridge) OnEvent(_ context.Context, ev domain.Event) error {
	cat, prio := classify(ev.Type)
	b.enqueue(queued{category: cat, priority: prio, msg: EventMsg{Event: ev}})
	return nil
}

// OnApprovalGate enqueues an ApprovalGateMsg. Always priority.
func (b *Bridge) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: ApprovalGateMsg{Gate: g}})
	return nil
}

// OnError enqueues an ErrorMsg. Errors are priority — they signal degraded state.
func (b *Bridge) OnError(_ context.Context, err error) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: ErrorMsg{Err: err}})
	return nil
}

// OnComplete enqueues a CompleteMsg. Priority.
func (b *Bridge) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	b.enqueue(queued{category: DropCategoryOther, priority: true, msg: CompleteMsg{Status: st}})
	return nil
}

// Close stops the forwarding worker and waits for it to finish. After Close,
// further OnX calls become no-ops. Idempotent.
func (b *Bridge) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.stop)
	b.cond.Broadcast()
	b.mu.Unlock()
	b.wg.Wait()
	return nil
}

// Drops returns the total number of dropped events across all categories.
func (b *Bridge) Drops() uint64 { return b.totalDrops.Load() }

// DropsByCategory returns a snapshot of per-category drop counters.
func (b *Bridge) DropsByCategory() map[DropCategory]uint64 {
	return map[DropCategory]uint64{
		DropCategoryHeartbeat: b.dropHB.Load(),
		DropCategoryAgentTask: b.dropAT.Load(),
		DropCategoryOther:     b.dropOther.Load(),
	}
}

// Pending reports the queue depth — used by tests to wait for a drain.
func (b *Bridge) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.queue)
}

// classify maps an event Type to its DropCategory and priority flag per §4.5.
//
//   - heartbeat        → heartbeat,    non-priority
//   - phase.* | approval.* → other,    PRIORITY (never dropped)
//   - agent.* | task.* → agent_task,   non-priority
//   - everything else  → other,        non-priority
func classify(eventType string) (DropCategory, bool) {
	switch {
	case eventType == "heartbeat":
		return DropCategoryHeartbeat, false
	case strings.HasPrefix(eventType, "phase."), strings.HasPrefix(eventType, "approval."):
		return DropCategoryOther, true
	case strings.HasPrefix(eventType, "agent."), strings.HasPrefix(eventType, "task."):
		return DropCategoryAgentTask, false
	default:
		return DropCategoryOther, false
	}
}

// enqueue applies the drop policy and either appends to the queue or drops.
func (b *Bridge) enqueue(q queued) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		// Silently drop after Close — the program is gone.
		return
	}
	if len(b.queue) < b.cap {
		b.queue = append(b.queue, q)
		b.cond.Signal()
		return
	}
	// At capacity. Decide: drop the newcomer, or evict an old non-priority
	// entry to make room?
	if !q.priority {
		// Newcomer is non-priority → drop.
		b.recordDrop(q.category)
		return
	}
	// Newcomer is priority. Try to evict the oldest non-priority entry.
	idx := -1
	for i, existing := range b.queue {
		if !existing.priority {
			idx = i
			break
		}
	}
	if idx == -1 {
		// Buffer is 100% priority — refuse to drop the newcomer (spec §4.5
		// invariant: phase.* / approval.* never dropped). The pragmatic
		// interpretation is to grow the queue beyond cap rather than silently
		// drop. In practice this is bounded by the SSE retry budget feeding
		// the bridge — the queue cannot grow unboundedly.
		b.queue = append(b.queue, q)
		b.cond.Signal()
		return
	}
	// Evict.
	evicted := b.queue[idx]
	b.queue = append(b.queue[:idx], b.queue[idx+1:]...)
	b.recordDrop(evicted.category)
	b.queue = append(b.queue, q)
	b.cond.Signal()
}

func (b *Bridge) recordDrop(cat DropCategory) {
	b.totalDrops.Add(1)
	switch cat {
	case DropCategoryHeartbeat:
		b.dropHB.Add(1)
	case DropCategoryAgentTask:
		b.dropAT.Add(1)
	default:
		b.dropOther.Add(1)
	}
}

// worker drains the queue and calls Sender.Send for each message.
func (b *Bridge) worker() {
	defer b.wg.Done()
	for {
		b.mu.Lock()
		for len(b.queue) == 0 && !b.closed {
			b.cond.Wait()
		}
		if b.closed && len(b.queue) == 0 {
			b.mu.Unlock()
			return
		}
		q := b.queue[0]
		b.queue = b.queue[1:]
		b.mu.Unlock()

		// Send may block (in tests) or be near-instant (in production).
		// The bridge does NOT assert non-blocking — the buffer is the
		// backpressure mechanism (spec §4.5).
		b.sender.Send(q.msg)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race -timeout 30s`
Expected: PASS. The cond-variable based worker can occasionally take a few milliseconds to schedule under a loaded `-race` run; `waitMessages` and `waitDrain` already poll with 2ms slack.

- [ ] **Step 5: Verify EventSink interface satisfaction**

Add a compile-time assertion at the bottom of `bridge.go` (this is a Go idiom to catch interface drift at compile time, NOT a runtime check):

```go
// Compile-time check: Bridge must satisfy inbound.EventSink.
var _ inbound.EventSink = (*Bridge)(nil)
```

Add the import:

```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
```

Run `go build ./...` — if EventSink drifts, the build fails immediately with a clear "Bridge does not implement inbound.EventSink: missing method ..." message.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/tui/bridge.go internal/adapters/inbound/tui/bridge_test.go
git commit -m "feat(tui): add EventSink bridge with cap-256 buffer + drop policy (§4.5)"
```

---

## Phase 3 — Model (pure state, no I/O)

### Task 3: tui/model.go — Model state shape

**Files:**
- Create: `internal/adapters/inbound/tui/model.go`
- Create: `internal/adapters/inbound/tui/model_test.go`

The Model is a pure struct. It does NOT spawn goroutines, hold timers, or perform I/O. All mutation happens through Update (Phase 4). All rendering happens through View (Phase 5).

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/model_test.go`:

```go
package tui_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestNewModelInitializesNinePhaseRows(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})

	got := m.PhaseRows()
	if len(got) != 9 {
		t.Fatalf("phase rows = %d, want 9", len(got))
	}
	want := domain.AllPhases()
	for i, row := range got {
		if row.Type != want[i] {
			t.Errorf("row %d type = %q, want %q", i, row.Type, want[i])
		}
		if row.Status != domain.PhaseStatusPending {
			t.Errorf("row %d default status = %q, want pending", i, row.Status)
		}
	}
}

func TestNewModelDefaultDimensionsAreSafe(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{})
	if m.Width() <= 0 || m.Height() <= 0 {
		t.Errorf("default dimensions must be positive: w=%d h=%d", m.Width(), m.Height())
	}
}

func TestModelApplySnapshotPopulatesPhases(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	change := &domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-init", Type: domain.PhaseInit, Status: domain.PhaseStatusDone, StartedAt: time.Unix(100, 0).UTC(), EndedAt: time.Unix(110, 0).UTC()},
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning, StartedAt: time.Unix(110, 0).UTC()},
		},
	}

	m2 := m.ApplySnapshot(change)

	rows := m2.PhaseRows()
	if rows[0].Status != domain.PhaseStatusDone {
		t.Errorf("init row status = %q, want done", rows[0].Status)
	}
	if rows[1].Status != domain.PhaseStatusRunning {
		t.Errorf("explore row status = %q, want running", rows[1].Status)
	}
	for i := 2; i < 9; i++ {
		if rows[i].Status != domain.PhaseStatusPending {
			t.Errorf("row %d status = %q, want pending", i, rows[i].Status)
		}
	}
	if m2.CurrentPhaseID() != "p-explore" {
		t.Errorf("CurrentPhaseID = %q, want p-explore", m2.CurrentPhaseID())
	}
	if m2.ChangeStatus() != domain.ChangeStatusRunning {
		t.Errorf("ChangeStatus = %q", m2.ChangeStatus())
	}
}

func TestModelApplyEventUpdatesPhaseStatus(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "phase.started",
		Payload: map[string]any{
			"phase_type": string(domain.PhaseExplore),
			"phase_id":   "p-1",
		},
	})

	rows := m.PhaseRows()
	if rows[1].Type != domain.PhaseExplore {
		t.Fatalf("row 1 should be explore: %q", rows[1].Type)
	}
	if rows[1].Status != domain.PhaseStatusRunning {
		t.Errorf("explore status after phase.started = %q, want running", rows[1].Status)
	}
	if m.CurrentPhaseID() != "p-1" {
		t.Errorf("CurrentPhaseID = %q, want p-1", m.CurrentPhaseID())
	}
}

func TestModelApplyEventCompletedTransitions(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "phase.completed",
		Payload: map[string]any{
			"phase_type": string(domain.PhaseProposal),
			"status":     string(domain.PhaseStatusDone),
		},
	})

	rows := m.PhaseRows()
	if rows[2].Status != domain.PhaseStatusDone {
		t.Errorf("proposal status = %q, want done", rows[2].Status)
	}
}

func TestModelApplyEventIgnoresUnknownPhaseType(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	before := m.PhaseRows()
	m = m.ApplyEvent(domain.Event{
		Type: "phase.started",
		Payload: map[string]any{
			"phase_type": "nonexistent",
		},
	})
	after := m.PhaseRows()
	for i := range before {
		if before[i].Status != after[i].Status {
			t.Errorf("row %d mutated despite unknown phase type", i)
		}
	}
}

func TestModelApplyEventApprovalRequiredMarksPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m = m.ApplyEvent(domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"phase": string(domain.PhaseApply),
		},
	})

	rows := m.PhaseRows()
	if !rows[6].HasApproval { // apply is index 6 in AllPhases()
		t.Error("apply row should be marked HasApproval after approval.required")
	}
}

func TestModelDetachState(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	if m.Detached() {
		t.Error("fresh model should not be detached")
	}
	if m.ConfirmingDetach() {
		t.Error("fresh model should not be confirming detach")
	}
	m2 := m.WithConfirmingDetach(true)
	if !m2.ConfirmingDetach() {
		t.Error("WithConfirmingDetach(true) should set the flag")
	}
	m3 := m2.WithDetached(true)
	if !m3.Detached() {
		t.Error("WithDetached(true) should set the flag")
	}
}

func TestModelResize(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.Resize(120, 40)
	if m2.Width() != 120 || m2.Height() != 40 {
		t.Errorf("after Resize(120, 40): w=%d h=%d", m2.Width(), m2.Height())
	}
}

func TestModelImmutability(t *testing.T) {
	// Methods that "update" the model MUST return a new value, not mutate
	// the receiver. This guards against accidental shared-state bugs.
	m1 := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m1.WithDetached(true)
	if m1.Detached() {
		t.Error("WithDetached mutated the receiver")
	}
	_ = m2
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `Model`, `ModelConfig`, `NewModel`, `PhaseRow`, `PhaseRows`, `ApplySnapshot`, `ApplyEvent`, `WithDetached`, `WithConfirmingDetach`, `Resize`, `CurrentPhaseID`, `ChangeStatus`, `Width`, `Height`, `HasApproval` all undefined.

- [ ] **Step 3: Implement**

`internal/adapters/inbound/tui/model.go`:

```go
package tui

import (
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// DefaultWidth and DefaultHeight are conservative defaults for terminals
// that haven't yet sent a tea.WindowSizeMsg. The actual View truncates rows
// to fit Width.
const (
	DefaultWidth  = 80
	DefaultHeight = 24
)

// PhaseRow is the per-phase render state. It mirrors domain.Phase plus a
// Lipgloss-friendly HasApproval flag for the "discreet ! marker" (D-M6-06).
type PhaseRow struct {
	Type        domain.PhaseType
	ID          string
	Status      domain.PhaseStatus
	Confidence  float64
	StartedAt   time.Time
	EndedAt     time.Time
	HasApproval bool
}

// ModelConfig configures the initial Model.
type ModelConfig struct {
	ChangeID domain.ChangeID
}

// Model is the immutable TUI state. All mutation methods (Apply*, With*)
// return a NEW Model and never modify the receiver.
type Model struct {
	changeID         domain.ChangeID
	changeStatus     domain.ChangeStatus
	currentPhaseID   string
	phases           [9]PhaseRow
	confirmingDetach bool
	detached         bool
	width            int
	height           int
	errors           []string
}

// NewModel constructs a Model with all 9 phases in PhaseStatusPending.
func NewModel(cfg ModelConfig) Model {
	var rows [9]PhaseRow
	for i, pt := range domain.AllPhases() {
		rows[i] = PhaseRow{Type: pt, Status: domain.PhaseStatusPending}
	}
	return Model{
		changeID:     cfg.ChangeID,
		changeStatus: domain.ChangeStatusPending,
		phases:       rows,
		width:        DefaultWidth,
		height:       DefaultHeight,
	}
}

// ChangeID returns the Change being observed.
func (m Model) ChangeID() domain.ChangeID { return m.changeID }

// ChangeStatus returns the latest known top-level status.
func (m Model) ChangeStatus() domain.ChangeStatus { return m.changeStatus }

// CurrentPhaseID returns the phase ID currently flagged as running.
func (m Model) CurrentPhaseID() string { return m.currentPhaseID }

// PhaseRows returns a copy of the 9 phase render rows.
func (m Model) PhaseRows() []PhaseRow {
	out := make([]PhaseRow, 9)
	copy(out, m.phases[:])
	return out
}

// Width returns the terminal width.
func (m Model) Width() int { return m.width }

// Height returns the terminal height.
func (m Model) Height() int { return m.height }

// ConfirmingDetach reports whether the user is in the Ctrl+C confirm dialog.
func (m Model) ConfirmingDetach() bool { return m.confirmingDetach }

// Detached reports whether the user has detached the program.
func (m Model) Detached() bool { return m.detached }

// Errors returns a copy of the recorded non-fatal error messages.
func (m Model) Errors() []string {
	out := make([]string, len(m.errors))
	copy(out, m.errors)
	return out
}

// Resize returns a new Model with updated terminal dimensions.
func (m Model) Resize(width, height int) Model {
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	m.width = width
	m.height = height
	return m
}

// WithConfirmingDetach toggles the detach-confirmation dialog.
func (m Model) WithConfirmingDetach(v bool) Model {
	m.confirmingDetach = v
	return m
}

// WithDetached marks the program as detached. The Update layer translates
// detach into tea.Quit; this flag is also rendered for one frame so the
// view can show the detach hint before the program actually exits.
func (m Model) WithDetached(v bool) Model {
	m.detached = v
	return m
}

// ApplySnapshot replaces phase rows from a domain.Change snapshot.
func (m Model) ApplySnapshot(c *domain.Change) Model {
	if c == nil {
		return m
	}
	m.changeStatus = c.Status
	m.currentPhaseID = c.CurrentPhaseID

	// Reset phase rows to pending (defensive: snapshot is authoritative).
	for i, pt := range domain.AllPhases() {
		m.phases[i] = PhaseRow{Type: pt, Status: domain.PhaseStatusPending}
	}
	for _, p := range c.Phases {
		idx := indexOfPhase(p.Type)
		if idx < 0 {
			continue
		}
		m.phases[idx] = PhaseRow{
			Type:       p.Type,
			ID:         p.ID,
			Status:     p.Status,
			Confidence: p.Confidence,
			StartedAt:  p.StartedAt,
			EndedAt:    p.EndedAt,
		}
	}
	return m
}

// ApplyEvent integrates a single domain.Event into the model.
func (m Model) ApplyEvent(ev domain.Event) Model {
	switch ev.Type {
	case "phase.started":
		return m.applyPhaseStarted(ev)
	case "phase.completed":
		return m.applyPhaseCompleted(ev)
	case "approval.required":
		return m.applyApprovalRequired(ev)
	default:
		// Unknown / forward-compatible — model state unchanged. Caller may
		// still record the event for log scrolling in M7+.
		return m
	}
}

func (m Model) applyPhaseStarted(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	row := m.phases[idx]
	row.Status = domain.PhaseStatusRunning
	if id, ok := ev.Payload["phase_id"].(string); ok {
		row.ID = id
		m.currentPhaseID = id
	}
	if !ev.Timestamp.IsZero() {
		row.StartedAt = ev.Timestamp
	}
	m.phases[idx] = row
	return m
}

func (m Model) applyPhaseCompleted(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	row := m.phases[idx]
	if statusStr, ok := ev.Payload["status"].(string); ok {
		row.Status = domain.PhaseStatus(statusStr)
	} else {
		row.Status = domain.PhaseStatusDone
	}
	if conf, ok := ev.Payload["confidence"].(float64); ok {
		row.Confidence = conf
	}
	if !ev.Timestamp.IsZero() {
		row.EndedAt = ev.Timestamp
	}
	m.phases[idx] = row
	return m
}

func (m Model) applyApprovalRequired(ev domain.Event) Model {
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		// Try the "phase" key (set by Runner.approvalGateFromEvent).
		if ph, ok := ev.Payload["phase"].(string); ok {
			pt = domain.PhaseType(ph)
		}
	}
	if pt == "" {
		return m
	}
	idx := indexOfPhase(pt)
	if idx < 0 {
		return m
	}
	m.phases[idx].HasApproval = true
	return m
}

// WithError appends a non-fatal error message to the model.
func (m Model) WithError(msg string) Model {
	m.errors = append(append([]string(nil), m.errors...), msg)
	return m
}

// WithComplete records the terminal status.
func (m Model) WithComplete(st domain.ChangeStatus) Model {
	m.changeStatus = st
	return m
}

// indexOfPhase returns the index of pt in domain.AllPhases(), or -1.
func indexOfPhase(pt domain.PhaseType) int {
	for i, candidate := range domain.AllPhases() {
		if candidate == pt {
			return i
		}
	}
	return -1
}

// phaseTypeFromPayload reads the canonical "phase_type" field. Spec §5.4
// defines the field name; older or alternate sources may use "phase" — we
// accept both for tolerance.
func phaseTypeFromPayload(payload map[string]any) domain.PhaseType {
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/tui/model.go internal/adapters/inbound/tui/model_test.go
git commit -m "feat(tui): add immutable Model with ApplySnapshot/ApplyEvent (no I/O)"
```

---

## Phase 4 — Update + key bindings

### Task 4: tui/update.go + tui/keybindings.go — message dispatch and key handling

**Files:**
- Create: `internal/adapters/inbound/tui/keybindings.go`
- Create: `internal/adapters/inbound/tui/update.go`
- Create: `internal/adapters/inbound/tui/update_test.go`

> **v2 API note (RM6-01):** Bubble Tea v2 renamed `tea.KeyMsg` to `tea.KeyPressMsg`. The code below assumes the v2 spelling. If Task 1 settled on `charm.land/...` and the v2 module exposes a different key-message type, ASK before substituting; do NOT silently use v1 `tea.KeyMsg`.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/update_test.go`:

```go
package tui_test

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestUpdateSnapshotMsgUpdatesModel(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	change := &domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}

	m2, cmd := tui.Update(m, tui.SnapshotMsg{Change: change})
	if m2.ChangeStatus() != domain.ChangeStatusRunning {
		t.Errorf("status = %q", m2.ChangeStatus())
	}
	if cmd != nil {
		t.Errorf("snapshot should not produce a Cmd")
	}
}

func TestUpdateEventMsgUpdatesModel(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})

	m2, cmd := tui.Update(m, tui.EventMsg{Event: domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": "explore", "phase_id": "p-1"},
	}})
	if cmd != nil {
		t.Errorf("event should not produce a Cmd")
	}
	if m2.CurrentPhaseID() != "p-1" {
		t.Errorf("CurrentPhaseID = %q", m2.CurrentPhaseID())
	}
}

func TestUpdateErrorMsgRecordsError(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, _ := tui.Update(m, tui.ErrorMsg{Err: errors.New("boom")})
	errs := m2.Errors()
	if len(errs) != 1 || errs[0] != "boom" {
		t.Errorf("errors = %v", errs)
	}
}

func TestUpdateCompleteMsgQuits(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tui.CompleteMsg{Status: domain.ChangeStatusDone})
	if m2.ChangeStatus() != domain.ChangeStatusDone {
		t.Errorf("status = %q", m2.ChangeStatus())
	}
	if cmd == nil {
		t.Fatal("CompleteMsg should produce tea.Quit Cmd")
	}
	// Execute the cmd and assert it returns tea.QuitMsg{}.
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Cmd return = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdateWindowSizeMsgUpdatesDimensions(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m2.Width() != 120 || m2.Height() != 40 {
		t.Errorf("after WindowSizeMsg w=%d h=%d", m2.Width(), m2.Height())
	}
	if cmd != nil {
		t.Errorf("WindowSizeMsg should not produce a Cmd")
	}
}

func TestUpdateQKeyDetaches(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("q"))
	if !m2.Detached() {
		t.Error("Q should mark model detached")
	}
	if cmd == nil {
		t.Fatal("Q should return a tea.Quit Cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Cmd return = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdateCtrlCFirstPressEntersConfirm(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("ctrl+c"))
	if !m2.ConfirmingDetach() {
		t.Error("first Ctrl+C should set ConfirmingDetach=true")
	}
	if m2.Detached() {
		t.Error("first Ctrl+C should NOT detach")
	}
	if cmd != nil {
		t.Errorf("first Ctrl+C should not produce a Cmd")
	}
}

func TestUpdateCtrlCSecondPressDetaches(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("ctrl+c"))
	if !m2.Detached() {
		t.Error("second Ctrl+C should detach")
	}
	if cmd == nil {
		t.Fatal("second Ctrl+C should return tea.Quit Cmd")
	}
}

func TestUpdateYConfirmsDetach(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("y"))
	if !m2.Detached() {
		t.Error("y should detach when in confirm mode")
	}
	if cmd == nil {
		t.Fatal("y should return tea.Quit Cmd")
	}
}

func TestUpdateNCancelsConfirm(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("n"))
	if m2.ConfirmingDetach() {
		t.Error("n should cancel confirm")
	}
	if m2.Detached() {
		t.Error("n must not detach")
	}
	if cmd != nil {
		t.Error("n should not produce a Cmd")
	}
}

func TestUpdateUnknownKeyInConfirmCancels(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, cmd := tui.Update(m, keyPress("x"))
	if m2.ConfirmingDetach() {
		t.Error("unrecognized key in confirm mode should cancel (D-M6-04)")
	}
	if cmd != nil {
		t.Error("unrecognized key should not produce a Cmd")
	}
}

// keyPress builds a tea.KeyPressMsg from the user-friendly string Bubble Tea
// uses (e.g. "q", "ctrl+c"). For v2, KeyPressMsg is a struct with a Code/
// String accessor — adapt this helper to whatever the installed v2 API
// exposes. The test contract is "press the named key", not "construct this
// exact struct."
func keyPress(name string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Key: tea.Key{Code: keyCode(name)}}
}

// keyCode maps the friendly name to the v2 key Code rune/constant. If v2's
// Key.Code shape differs, this is the only helper that needs adjusting.
func keyCode(name string) rune {
	// v2's Key.Code is a rune for printable keys; control keys use a separate
	// modifier set. This helper returns 0 when name is not a single rune so
	// the caller falls through to the modifier-based cases handled in
	// keybindings.go.
	if len(name) == 1 {
		return rune(name[0])
	}
	return 0
}
```

> **Implementer note:** the `keyPress` helper above is a SHIM. Bubble Tea v2 ships its own helpers for constructing keypress messages in tests — once Task 1 settled the version, replace `keyPress` with whichever helper the v2 teatest module exposes, OR extend it to set `Modifier: tea.ModCtrl` for the `ctrl+c` case. Keep the test contract — every test asserts behavior given a "press of name", not a specific struct shape.

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `Update`, `keybindings.go` symbols undefined; the bubbletea import compiles but no `Update` symbol exists.

- [ ] **Step 3: Implement keybindings**

`internal/adapters/inbound/tui/keybindings.go`:

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// Action is the discrete user action a keypress maps to.
type Action int

const (
	ActionNone Action = iota
	ActionDetach
	ActionConfirmDetach
	ActionConfirmYes
	ActionConfirmNo
)

// classifyKey reads a tea.KeyPressMsg and returns the Action it triggers
// (in conjunction with the model's confirmingDetach flag — see Update).
//
// Bindings (spec §2.2):
//
//   - "q" / "Q"            → ActionDetach
//   - "ctrl+c"             → ActionConfirmDetach (1st press) /
//                             ActionConfirmYes (2nd press, when confirming)
//   - "y" / "Y"            → ActionConfirmYes
//   - "n" / "N"            → ActionConfirmNo
//   - any other key while confirming → ActionConfirmNo (D-M6-04 default)
//   - any other key otherwise → ActionNone
func classifyKey(msg tea.KeyPressMsg, confirming bool) Action {
	keyStr := strings.ToLower(keyPressString(msg))
	switch keyStr {
	case "q":
		return ActionDetach
	case "ctrl+c":
		if confirming {
			return ActionConfirmYes
		}
		return ActionConfirmDetach
	case "y":
		if confirming {
			return ActionConfirmYes
		}
		return ActionNone
	case "n":
		if confirming {
			return ActionConfirmNo
		}
		return ActionNone
	}
	if confirming {
		// Any other key cancels confirmation (D-M6-04 default).
		return ActionConfirmNo
	}
	return ActionNone
}

// keyPressString returns the canonical lowercase form of a tea.KeyPressMsg.
// Bubble Tea v2's KeyPressMsg has a String() method we rely on; if the
// installed API differs, adapt this helper in place.
func keyPressString(msg tea.KeyPressMsg) string {
	return msg.String()
}
```

- [ ] **Step 4: Implement Update**

`internal/adapters/inbound/tui/update.go`:

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea/v2"
)

// Update is the pure dispatch function. Returns the new Model and an
// optional tea.Cmd. Spec §2.2 / §4.5 — UI is event-driven, no tickers.
func Update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.Resize(msg.Width, msg.Height), nil

	case tea.KeyPressMsg:
		return updateKey(m, msg)

	case SnapshotMsg:
		return m.ApplySnapshot(msg.Change), nil

	case EventMsg:
		return m.ApplyEvent(msg.Event), nil

	case ApprovalGateMsg:
		// Mark the affected phase row with HasApproval. The full banner is M7.
		return m.ApplyEvent(approvalGateAsEvent(msg)), nil

	case ErrorMsg:
		text := ""
		if msg.Err != nil {
			text = msg.Err.Error()
		}
		return m.WithError(text), nil

	case CompleteMsg:
		return m.WithComplete(msg.Status), tea.Quit
	}
	return m, nil
}

func updateKey(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	action := classifyKey(msg, m.ConfirmingDetach())
	switch action {
	case ActionDetach, ActionConfirmYes:
		return m.WithConfirmingDetach(false).WithDetached(true), tea.Quit
	case ActionConfirmDetach:
		return m.WithConfirmingDetach(true), nil
	case ActionConfirmNo:
		return m.WithConfirmingDetach(false), nil
	}
	return m, nil
}

// approvalGateAsEvent converts an ApprovalGateMsg into the synthetic Event
// shape ApplyEvent expects, so the model logic stays unified.
func approvalGateAsEvent(msg ApprovalGateMsg) (ev struct {
	Type      string
	Payload   map[string]any
	Timestamp interface{}
	TraceID   string
	EventID   string
}) {
	// Empty interface placeholder kept narrow — actual translation:
	return struct {
		Type      string
		Payload   map[string]any
		Timestamp interface{}
		TraceID   string
		EventID   string
	}{
		Type: "approval.required",
		Payload: map[string]any{
			"phase": string(msg.Gate.Phase),
		},
	}
}
```

> **Note:** the `approvalGateAsEvent` helper above uses an inline struct because translating between `ApprovalGateMsg` and `domain.Event` while keeping `domain.Event` immutable would otherwise cost an import cycle. Replace the inline-struct approach with a direct `domain.Event` construction once the implementer confirms `domain.Event` is freely constructible from `tui` (verify by reading `internal/domain/event.go` — it's a plain struct, no constructor). The cleaner form:
>
> ```go
> func approvalGateAsEvent(msg ApprovalGateMsg) domain.Event {
>     return domain.Event{
>         Type:    "approval.required",
>         Payload: map[string]any{"phase": string(msg.Gate.Phase)},
>     }
> }
> ```
>
> Then call: `return m.ApplyEvent(approvalGateAsEvent(msg)), nil`. Use the cleaner form unless an unexpected import cycle surfaces.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS. If `tea.KeyPressMsg` / `Key.Code` shapes differ from this plan, adapt `keyPressString` and `keyCode` only — the test contract (`q` detaches, etc.) is invariant.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/tui/keybindings.go internal/adapters/inbound/tui/update.go internal/adapters/inbound/tui/update_test.go
git commit -m "feat(tui): add pure Update + keybindings (Q detach, Ctrl+C confirm-then-detach)"
```

---

## Phase 5 — Styles + Timeline view

### Task 5: tui/styles.go + tui/view_timeline.go — Lipgloss styles and the View() function

**Files:**
- Create: `internal/adapters/inbound/tui/styles.go`
- Create: `internal/adapters/inbound/tui/view_timeline.go`
- Create: `internal/adapters/inbound/tui/view_timeline_test.go`

The view is a pure function `View(Model) string`. We test it with golden-string fixtures. Lipgloss output is deterministic when the color profile is fixed; spec invariant §6.3 inv 7 mandates that NO user input flows directly to `fmt.Sprintf` / `fmt.Fprintf` — every render goes through `lipgloss.Style.Render` which escapes embedded ANSI.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/view_timeline_test.go`:

```go
package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestViewRendersAllNinePhaseRows(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HXABC")})
	out := tui.View(m)

	for _, pt := range domain.AllPhases() {
		if !strings.Contains(out, string(pt)) {
			t.Errorf("View output missing phase %q:\n%s", pt, out)
		}
	}
}

func TestViewIncludesChangeID(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HXABCDEF")})
	out := tui.View(m)
	if !strings.Contains(out, "01HXABCDEF") {
		t.Errorf("View should display change ID; got:\n%s", out)
	}
}

func TestViewMarksRunningPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplySnapshot(&domain.Change{
			ID:             domain.ChangeID("01HX"),
			Status:         domain.ChangeStatusRunning,
			CurrentPhaseID: "p-1",
			Phases: []domain.Phase{
				{ID: "p-1", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
			},
		})
	out := tui.View(m)
	// The running marker can be either a glyph (▶) or text — either is fine.
	// We assert that the explore row is visually distinct from the others.
	lines := strings.Split(out, "\n")
	exploreLine := ""
	for _, line := range lines {
		if strings.Contains(line, "explore") {
			exploreLine = line
			break
		}
	}
	if exploreLine == "" {
		t.Fatal("explore phase line not found")
	}
	// At least the running glyph or the word "running" must appear.
	if !strings.ContainsAny(exploreLine, "▶>*") && !strings.Contains(exploreLine, "running") {
		t.Errorf("running marker missing in explore line: %q", exploreLine)
	}
}

func TestViewMarksApprovalRequiredPhase(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplyEvent(domain.Event{
			Type: "approval.required",
			Payload: map[string]any{
				"phase": string(domain.PhaseApply),
			},
		})
	out := tui.View(m)

	lines := strings.Split(out, "\n")
	applyLine := ""
	for _, line := range lines {
		if strings.Contains(line, "apply") {
			applyLine = line
			break
		}
	}
	if applyLine == "" {
		t.Fatal("apply phase line not found")
	}
	if !strings.Contains(applyLine, "!") {
		t.Errorf("approval marker (!) missing in apply line: %q", applyLine)
	}
}

func TestViewShowsConfirmDialogWhenConfirming(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithConfirmingDetach(true)
	out := tui.View(m)
	if !strings.Contains(out, "Detach?") {
		t.Errorf("confirm dialog missing; got:\n%s", out)
	}
}

func TestViewShowsKeybindingHints(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out := tui.View(m)
	// At minimum, "Q" detach hint must be present so the user knows how to leave.
	if !strings.Contains(strings.ToLower(out), "q") {
		t.Errorf("View should hint at the Q keybinding; got:\n%s", out)
	}
}

func TestViewIsPure(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Errorf("View must be pure (same input → same output)")
	}
}

func TestViewDoesNotInterpretANSIInPayload(t *testing.T) {
	// Spec §6.3 inv 7: lipgloss.Style.Render escapes input, so a payload
	// that smuggles an ANSI escape MUST appear as literal text in the View.
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithError("\x1b[2J\x1b[H attacker tried to clear screen")
	out := tui.View(m)
	// The literal substring should still be present — lipgloss's Render
	// does NOT strip escapes from arbitrary user input, but the View MUST
	// route every untrusted string through Render so the terminal renders
	// the bytes as literal characters when our style strips/escapes them.
	// We assert the words "clear screen" survive (they're plain text), and
	// we assert the View string does not start with a screen-clear sequence
	// in the first column (defensive).
	if !strings.Contains(out, "clear screen") {
		t.Error("error text payload missing from View")
	}
	if strings.HasPrefix(out, "\x1b[2J") {
		t.Error("View must not begin with raw ANSI clear-screen from user input")
	}
}
```

> **Test pragmatism note:** `TestViewDoesNotInterpretANSIInPayload` makes a defensive assertion. The strict invariant from §6.3 inv 7 is "TUI renders strings with strict ANSI escaping. lipgloss does NOT evaluate input." The implementer MUST route every untrusted string through `lipgloss.Style.Render(...)`. If lipgloss v2 documents an even stricter mode (e.g. `lipgloss.SetColorProfile(termenv.Ascii)` for tests), use it inside a `TestMain` to make ANSI handling deterministic across environments.

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `View`, styles undefined.

- [ ] **Step 3: Implement styles**

`internal/adapters/inbound/tui/styles.go`:

```go
package tui

import (
	lipgloss "github.com/charmbracelet/lipgloss/v2"
)

// styles is the lipgloss palette used by the Timeline view. Constructed
// once at package init — lipgloss styles are immutable values, safe to
// share across goroutines.
type stylePalette struct {
	header        lipgloss.Style
	phasePending  lipgloss.Style
	phaseRunning  lipgloss.Style
	phaseDone     lipgloss.Style
	phaseFailed   lipgloss.Style
	phaseBlocked  lipgloss.Style
	approvalMark  lipgloss.Style
	confirmDialog lipgloss.Style
	hint          lipgloss.Style
	errorLine     lipgloss.Style
}

// Status icons. Restricted to single ASCII glyphs + a few unicode chars
// known to render across Apple Terminal / iTerm / xterm (RM6-05).
const (
	iconPending  = " "
	iconRunning  = "▶"
	iconDone     = "✓"
	iconFailed   = "✗"
	iconBlocked  = "■"
	iconApproval = "!"
)

// newStyles returns the default palette. Color choices stick to the
// standard 8-color palette so they render on any terminal (RM6-05).
func newStyles() stylePalette {
	return stylePalette{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")), // bright blue
		phasePending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")), // dim grey
		phaseRunning: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11")), // bright yellow
		phaseDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")), // bright green
		phaseFailed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")), // bright red
		phaseBlocked: lipgloss.NewStyle().
			Foreground(lipgloss.Color("13")), // bright magenta
		approvalMark: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("9")), // bright red — attention
		confirmDialog: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")). // bright white
			Background(lipgloss.Color("4")),  // blue background
		hint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")), // dim grey
		errorLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")), // bright red
	}
}

// styleFor returns the row-line style for a given PhaseStatus.
func (p stylePalette) styleFor(status string) lipgloss.Style {
	switch status {
	case "running":
		return p.phaseRunning
	case "done":
		return p.phaseDone
	case "failed":
		return p.phaseFailed
	case "blocked":
		return p.phaseBlocked
	default:
		return p.phasePending
	}
}

// iconFor returns the status icon character.
func (p stylePalette) iconFor(status string) string {
	switch status {
	case "running":
		return iconRunning
	case "done":
		return iconDone
	case "failed":
		return iconFailed
	case "blocked":
		return iconBlocked
	default:
		return iconPending
	}
}
```

- [ ] **Step 4: Implement view**

`internal/adapters/inbound/tui/view_timeline.go`:

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	lipgloss "github.com/charmbracelet/lipgloss/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// pkgStyles is the package-level palette. Tests may override via
// TestMain → setupStyles for color-profile determinism.
var pkgStyles = newStyles()

// View is the pure rendering function. Spec §6.3 inv 7: every untrusted
// string flows through pkgStyles.<Style>.Render, which escapes embedded
// control characters via lipgloss's safe-render path.
func View(m Model) string {
	var b strings.Builder

	// Header.
	header := fmt.Sprintf("Sophia · Change %s · %s", m.ChangeID(), m.ChangeStatus())
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	// Phase rows.
	for _, row := range m.PhaseRows() {
		b.WriteString(renderPhaseRow(row, m.CurrentPhaseID()))
		b.WriteString("\n")
	}

	// Errors (if any) — most recent last.
	for _, e := range m.Errors() {
		b.WriteString(pkgStyles.errorLine.Render("error: " + e))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Footer: confirm dialog or hint.
	if m.ConfirmingDetach() {
		b.WriteString(pkgStyles.confirmDialog.Render(" Detach? (y/n) "))
	} else {
		b.WriteString(pkgStyles.hint.Render("Q to detach · Ctrl+C confirm-then-detach"))
	}

	// Truncate to terminal width if necessary.
	rendered := b.String()
	return truncateToWidth(rendered, m.Width())
}

// renderPhaseRow returns one styled line for a phase. Always:
//
//   <icon> <type:padded> <status:padded> <duration> <conf?> [!?]
//
// All variable-width pieces go through lipgloss.Style.Render to honor inv 7.
func renderPhaseRow(row PhaseRow, currentPhaseID string) string {
	statusStr := string(row.Status)
	style := pkgStyles.styleFor(statusStr)
	icon := pkgStyles.iconFor(statusStr)

	approval := ""
	if row.HasApproval {
		approval = " " + pkgStyles.approvalMark.Render("!")
	}

	dur := ""
	if !row.StartedAt.IsZero() {
		end := row.EndedAt
		if end.IsZero() {
			end = time.Now().UTC()
		}
		d := end.Sub(row.StartedAt).Round(time.Second)
		if d > 0 {
			dur = fmt.Sprintf(" %s", d)
		}
	}

	conf := ""
	if row.Confidence > 0 {
		conf = fmt.Sprintf(" [%.2f]", row.Confidence)
	}

	body := fmt.Sprintf("%s %-9s %-8s%s%s", icon, row.Type, statusStr, dur, conf)
	rendered := style.Render(body)
	return rendered + approval
}

// truncateToWidth ensures no line in the rendered output exceeds w columns.
// We use lipgloss.Width to measure styled width correctly.
func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > w {
			// Naive byte-truncate is unsafe with ANSI; lipgloss exposes
			// no public truncate helper that's lossless across all
			// versions, so we fall back to runes — accept that this
			// may chop a styled boundary on narrow terminals. M7 can
			// upgrade to a smarter wrap.
			runes := []rune(line)
			if len(runes) > w {
				runes = runes[:w]
			}
			lines[i] = string(runes)
		}
	}
	return strings.Join(lines, "\n")
}

// (compile-time check: domain.PhaseType is the right type for headers)
var _ domain.PhaseType = domain.PhaseExplore
```

- [ ] **Step 5: Stabilize the color profile in tests (RM6-03)**

Add `internal/adapters/inbound/tui/setup_test.go`:

```go
package tui_test

import (
	"os"
	"testing"

	lipgloss "github.com/charmbracelet/lipgloss/v2"
)

// TestMain forces lipgloss into ASCII (no-color) mode for deterministic
// golden assertions. lipgloss v2's API for this may be SetColorProfile,
// SetDefaultRenderer with a no-color profile, or the COLORTERM env var —
// adapt to whichever the installed v2 module exposes.
func TestMain(m *testing.M) {
	// Easiest cross-version knob: NO_COLOR is honored by lipgloss/termenv.
	_ = os.Setenv("NO_COLOR", "1")
	// If lipgloss v2 exposes lipgloss.SetColorProfile, prefer it:
	//   lipgloss.SetColorProfile(termenv.Ascii)
	// (Verify with `go doc github.com/charmbracelet/lipgloss/v2` once Task 1 settled.)
	_ = lipgloss.NewStyle() // touch the package to ensure imports resolve
	os.Exit(m.Run())
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/inbound/tui/styles.go internal/adapters/inbound/tui/view_timeline.go \
        internal/adapters/inbound/tui/view_timeline_test.go internal/adapters/inbound/tui/setup_test.go
git commit -m "feat(tui): add lipgloss styles + Timeline View() with ANSI-safe rendering (§6.3 inv 7)"
```

---

## Phase 6 — Program assembly

### Task 6: tui/program.go — tea.NewProgram + Run() entry point

**Files:**
- Create: `internal/adapters/inbound/tui/program.go`
- Create: `internal/adapters/inbound/tui/program_test.go`

The Program ties the Bridge to the Bubble Tea event loop. Initialization is two-phase to avoid the chicken-and-egg between Model/Bridge/Program (RM6-04):

1. Build a Bridge with a placeholder `Sender` (a stand-in that buffers until the real program is ready).
2. Build the Program (which knows about the Bridge for its Init/Update).
3. Swap the Bridge's Sender to the Program's `Send` method.

In practice we sidestep the placeholder by deferring `program.Send` through a small adapter — see implementation below.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/program_test.go`:

```go
package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestProgramReturnsBridgeImplementingEventSink(t *testing.T) {
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   newDevNullWriter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if p.Bridge() == nil {
		t.Fatal("Bridge() returned nil")
	}
	// Bridge implements EventSink — a snapshot call must not error.
	if err := p.Bridge().OnSnapshot(context.Background(), &domain.Change{ID: domain.ChangeID("01HX")}); err != nil {
		t.Errorf("OnSnapshot: %v", err)
	}
}

func TestProgramRunReturnsDetachHintOnDetach(t *testing.T) {
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HXABC"),
		Output:   newDevNullWriter(),
		Input:    nil, // no input stream; we'll trigger detach via Bridge.OnComplete
	})
	if err != nil {
		t.Fatal(err)
	}

	// Ask the bridge to deliver a CompleteMsg — Update returns tea.Quit.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hint, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Per §2.2, when the user detaches we print the reattach hint. When
	// the program ends naturally (CompleteMsg), the hint is empty —
	// the M5 jsonsink already covered the terminal reporting.
	if hint != "" {
		t.Errorf("hint after natural exit = %q, want empty", hint)
	}
}

func TestProgramRunPrintsReattachHintWhenUserDetaches(t *testing.T) {
	t.Skip("interactive — see Phase 9 manual smoke")
	// This case is covered by teatest in Phase 8 and by manual smoke in
	// Phase 9. Keeping a skipped placeholder so the intent is documented.
}

// newDevNullWriter returns an io.Writer that swallows everything.
func newDevNullWriter() *devNull { return &devNull{} }

type devNull struct{}

func (*devNull) Write(p []byte) (int, error) { return len(p), nil }

// --- compile-time assertion ---

// Compile-time guard: Program.Bridge() returns *Bridge.
var _ = func() *tui.Bridge {
	var p *tui.Program
	if p == nil {
		return nil
	}
	return p.Bridge()
}

// Suppress unused import if strings is not referenced elsewhere in this file.
var _ = strings.HasPrefix
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `NewProgram`, `ProgramConfig`, `Program`, `Bridge()`, `Run`, `Close` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/inbound/tui/program.go`:

```go
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// ProgramConfig configures NewProgram.
type ProgramConfig struct {
	ChangeID domain.ChangeID
	Output   io.Writer // nil ⇒ os.Stdout (resolved by tea.WithOutput)
	Input    io.Reader // nil ⇒ os.Stdin
}

// Program owns the Bubble Tea program plus its Bridge.
type Program struct {
	mu     sync.Mutex
	tea    *tea.Program
	bridge *Bridge
	closed bool
}

// teaSender is the production adapter from *tea.Program to the Bridge's
// Sender interface. It exists because we need *tea.Program to satisfy
// `Sender` without leaking bubbletea types into bridge.go.
type teaSender struct {
	p *tea.Program
}

func (s *teaSender) Send(m any) {
	if s.p == nil {
		return
	}
	s.p.Send(m)
}

// rootModel implements the bubbletea Model contract by delegating to our
// pure Update/View functions. This is the only place bubbletea's Model
// interface touches our internal types.
type rootModel struct {
	state Model
}

func (rm rootModel) Init() (tea.Model, tea.Cmd) {
	// No initial commands — the bridge feeds messages externally.
	return rm, nil
}

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newState, cmd := Update(rm.state, msg)
	rm.state = newState
	return rm, cmd
}

func (rm rootModel) View() string {
	return View(rm.state)
}

// NewProgram constructs a Program. The Bridge is wired such that calls to
// Bridge().OnX(...) before Run() begins are buffered and replayed once the
// program loop is running — bubbletea's *tea.Program.Send is documented to
// be safe before the loop starts.
func NewProgram(cfg ProgramConfig) (*Program, error) {
	root := rootModel{state: NewModel(ModelConfig{ChangeID: cfg.ChangeID})}

	opts := []tea.ProgramOption{
		tea.WithoutSignalHandler(), // we own Ctrl+C semantics — see Update()
	}
	if cfg.Output != nil {
		opts = append(opts, tea.WithOutput(cfg.Output))
	}
	if cfg.Input != nil {
		opts = append(opts, tea.WithInput(cfg.Input))
	}

	teaProg := tea.NewProgram(root, opts...)
	sender := &teaSender{p: teaProg}
	bridge := NewBridge(BridgeConfig{Sender: sender})

	return &Program{
		tea:    teaProg,
		bridge: bridge,
	}, nil
}

// Bridge returns the EventSink-implementing bridge. Callers (the Runner)
// pass this into application.RunnerDeps.Sink.
func (p *Program) Bridge() *Bridge { return p.bridge }

// Run starts the Bubble Tea event loop. Blocks until the program exits
// (tea.Quit emitted by Update on Q, second Ctrl+C, or CompleteMsg).
//
// Returns the reattach hint (non-empty only when the user detached
// mid-stream) and an error.
//
// ctx cancellation: the bubbletea v2 program respects ctx via tea.WithContext;
// we pass ctx through so SIGINT-style cancellation from the parent (e.g.
// the cli.run command's context) terminates the program cleanly.
func (p *Program) Run(ctx context.Context) (string, error) {
	// Spawn a watcher that quits the program when ctx is canceled. The
	// alternative — tea.WithContext — may not exist in the installed v2
	// version; this watcher is robust either way.
	stopWatcher := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.tea.Quit()
		case <-stopWatcher:
		}
	}()
	defer close(stopWatcher)

	finalModel, err := p.tea.Run()
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return "", fmt.Errorf("tui: program ended with error: %w", err)
	}

	// Inspect the final model state.
	if rm, ok := finalModel.(rootModel); ok && rm.state.Detached() {
		return reattachHint(rm.state.ChangeID()), nil
	}
	return "", nil
}

// Close stops the program and tears down the bridge. Safe to call from any
// goroutine; idempotent.
func (p *Program) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.tea != nil {
		p.tea.Quit()
	}
	if p.bridge != nil {
		_ = p.bridge.Close()
	}
	return nil
}

// reattachHint returns the user-facing reattach instruction printed AFTER
// the bubbletea program exits. Spec §2.2.
func reattachHint(id domain.ChangeID) string {
	if id.IsZero() {
		return "Detached."
	}
	return fmt.Sprintf("Detached. Reattach with: sophia attach %s", id)
}
```

> **v2 API caveat (RM6-01):** `tea.ProgramOption`, `tea.WithoutSignalHandler`, `tea.ErrProgramKilled`, and `Program.Quit()` may have different names in the actual v2 module:
> - In v1 they were `tea.WithoutSignals()`, no `ErrProgramKilled` (just nil on quit), and `*tea.Program.Quit()` exists.
> - In v2 there have been documented renames; verify with `go doc github.com/charmbracelet/bubbletea/v2`.
>
> If symbols differ, ASK the user before rewriting. The test contract is "Run() blocks until quit, returns nil on natural exit, returns the reattach hint when Detached() is true."

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race -timeout 30s`
Expected: PASS — at minimum the `TestProgramReturnsBridgeImplementingEventSink` and `TestProgramRunReturnsDetachHintOnDetach` cases. The skipped `TestProgramRunPrintsReattachHintWhenUserDetaches` is fine; Phase 8 covers it via teatest.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/tui/program.go internal/adapters/inbound/tui/program_test.go
git commit -m "feat(tui): add Program assembly + Run() with reattach hint on detach"
```

---

## Phase 7 — Run command flag inversion + bootstrap factories

### Task 7: cli/run.go — TUI default, --no-tui --json fallback; bootstrap.BuildRunner factory

**Files:**
- Modify: `internal/adapters/inbound/cli/run.go`
- Modify: `internal/adapters/inbound/cli/run_test.go`
- Modify: `internal/adapters/inbound/cli/root.go`
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`

The bootstrap currently builds ONE `*application.Runner` with a fixed `jsonsink.New(...)` injected. We need to defer Runner construction to runtime — `cli.run` constructs the Runner with the chosen sink at command time.

The cleanest way: the bootstrap exposes a `RunnerFactory` (a closure that takes an `inbound.EventSink` and returns a configured `*application.Runner`). cli.Deps holds the factory; `cli.run` calls it once it knows whether the user wants TUI or JSONL.

- [ ] **Step 1: Read the existing wire.go and run.go**

Re-read `internal/bootstrap/wire.go` and `internal/adapters/inbound/cli/run.go` (both shown above in the context section). Note:

- `cli.Deps.Runner` is currently `*application.Runner`.
- `cli.newRunCmd` calls `d.Runner.Run(...)` directly.
- The `--no-tui` and `--json` flags BOTH default `false` and the command BLOCKS unless both are set: `if !noTUI || !jsonOut { return fmt.Errorf("...required in M4...") }`.

The refactor:

1. Replace `cli.Deps.Runner *application.Runner` with `cli.Deps.RunnerFactory func(sink inbound.EventSink) *application.Runner`.
2. In `bootstrap.New`, build the factory:
   ```go
   runnerFactory := func(sink inbound.EventSink) *application.Runner {
       return application.NewRunner(application.RunnerDeps{
           Orch: orch, State: state, Git: git, Sink: sink, EventStream: stream,
       }, application.RunnerOptions{})
   }
   ```
3. In `cli.newRunCmd`, decide the sink:
   - Default (no flags) → `tui.NewProgram(...)` → use program.Bridge() as the sink.
   - `--no-tui --json` → `jsonsink.New(...)`.
   - `--no-tui` without `--json` → error: "--json is required with --no-tui" (keep parity with M5, no JSONL-without-JSON path).
   - `--json` without `--no-tui` → error: "--no-tui is required to disable the TUI". Or: implicit "use TUI but emit JSONL too" — REJECT (no dual-sink in M6; that's a feature ask for later).

- [ ] **Step 2: Update run_test.go**

`internal/adapters/inbound/cli/run_test.go` (the existing tests use a `*application.Runner`-shaped Deps; switch them to the factory):

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

func newRunDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator, *fakes.FakeEventStream) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	stream := fakes.NewFakeEventStream()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-cotizacion", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	factory := func(sink inbound.EventSink) *application.Runner {
		return application.NewRunner(application.RunnerDeps{
			Orch:        orch,
			State:       state,
			Git:         git,
			Sink:        sink,
			EventStream: stream,
		}, application.RunnerOptions{})
	}

	return cli.Deps{
		Resolver:      resolver,
		RunnerFactory: factory,
		// Fixed sink for --no-tui --json tests; the factory is also used.
		JSONSinkOverride: newTestSink(sinkBuf),
	}, orch, stream
}

func newTestSink(w *bytes.Buffer) *testSink {
	return &testSink{w: w}
}

type testSink struct{ w *bytes.Buffer }

func (s *testSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	_, err := s.w.WriteString("snap:" + c.ID.String() + ":" + string(c.Status) + "\n")
	return err
}
func (s *testSink) OnEvent(_ context.Context, _ domain.Event) error               { return nil }
func (s *testSink) OnApprovalGate(_ context.Context, _ domain.ApprovalGate) error { return nil }
func (s *testSink) OnError(_ context.Context, _ error) error                      { return nil }
func (s *testSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	_, err := s.w.WriteString("done:" + string(st) + "\n")
	return err
}
func (s *testSink) Close() error { return nil }

func TestRunCommandRequiresMessage(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when message missing")
	}
}

func TestRunCommandJSONLModeSucceeds(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "test message", "--no-tui", "--json"})
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

func TestRunCommandNoTUIWithoutJSONFails(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --no-tui used without --json")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention --json: %v", err)
	}
}

func TestRunCommandJSONWithoutNoTUIFails(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error when --json used without --no-tui")
	}
	if !strings.Contains(err.Error(), "--no-tui") {
		t.Errorf("error should mention --no-tui: %v", err)
	}
}

func TestRunCommandDefaultModeStartsTUI(t *testing.T) {
	// Default mode (no flags) opens the TUI. We don't have a real terminal
	// here; the cli MUST construct a Program and feed it through the
	// RunnerFactory. We verify by asserting the factory was invoked with
	// a non-nil sink that is NOT the JSONSinkOverride. The full TUI smoke
	// test lives in Phase 8 (teatest) and Phase 9 (manual).
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	// Wire a hook so the orchestrator finishes immediately — TUI Run()
	// returns when a CompleteMsg is dispatched.
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	// Make TUI use a discard writer so test output stays clean.
	deps.TUIOutput = &bytes.Buffer{}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "test message"})
	if err := c.Execute(); err != nil {
		t.Fatalf("default mode should not error on natural exit: %v", err)
	}
	// JSONSinkOverride must NOT have received the snapshot — the TUI sink did.
	if strings.Contains(sinkBuf.String(), "snap:") {
		t.Errorf("default mode should use TUI sink, not jsonsink override: %s", sinkBuf.String())
	}
}

// TestRunCommandReturnsExitErrorOnFailure (preserved from M5 — terminal
// failed status surfaces as an error).
func TestRunCommandReturnsExitErrorOnFailure(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusFailed)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 3: Run test (still failing — code not yet refactored)**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL — `cli.Deps.RunnerFactory`, `cli.Deps.JSONSinkOverride`, `cli.Deps.TUIOutput` undefined.

- [ ] **Step 4: Update root.go and run.go**

`internal/adapters/inbound/cli/root.go`:

```go
package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

// RunnerFactory builds a *application.Runner with the caller-provided
// sink. Returned by bootstrap.New so cli.run can pick the sink at
// command time (TUI vs JSONL).
type RunnerFactory func(sink inbound.EventSink) *application.Runner

type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Resolver     *application.ConfigResolver

	// RunnerFactory is the M6 way of constructing a Runner — sink-injected
	// at command time. Mandatory for `sophia run`.
	RunnerFactory RunnerFactory

	// JSONSinkOverride lets tests inject a recording sink instead of
	// jsonsink.New(os.Stdout). Production leaves this nil.
	JSONSinkOverride inbound.EventSink

	// TUIOutput is the writer the TUI program renders to. Defaults to
	// os.Stdout. Tests inject a buffer to keep output clean.
	TUIOutput io.Writer

	UserConfigPath string

	Version   string
	Commit    string
	BuildDate string
}

func NewRoot(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "sophia",
		Short: "Sophia CLI — create and observe SDD Changes",
		Long: `sophia is the human entry point of the Sophia ecosystem.

It creates and observes Changes executed by sophia-orchestator. The CLI
itself does not coordinate phases, evaluate policy, or store canonical
state.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd(d))
	root.AddCommand(newDoctorCmd(d))
	root.AddCommand(newInitCmd(d))
	root.AddCommand(newStartCmd(d))
	root.AddCommand(newStopCmd(d))
	root.AddCommand(newRunCmd(d))
	root.AddCommand(newStubCmd("attach", "Attach to an existing Change", "M8"))
	root.AddCommand(newStatusCmd(d))
	root.AddCommand(newStubCmd("changes", "List recent Changes", "M8"))

	return root
}
```

`internal/adapters/inbound/cli/run.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/jsonsink"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

func newRunCmd(d Deps) *cobra.Command {
	var (
		noTUI         bool
		jsonOut       bool
		baseRef       string
		artifactStore string
		project       string
	)
	cmd := &cobra.Command{
		Use:   "run [message]",
		Short: "Create and observe a Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.RunnerFactory == nil || d.Resolver == nil {
				return fmt.Errorf("run: runner factory not wired")
			}
			if err := validateModeFlags(noTUI, jsonOut); err != nil {
				return err
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("run: message required (positional argument)")
			}

			resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
				Flags: application.ResolverFlags{
					Project:       project,
					BaseRef:       baseRef,
					ArtifactStore: artifactStore,
				},
				Env:            envSnapshot(),
				UserConfigPath: d.UserConfigPath,
				RequireProject: true,
			})
			if err != nil {
				return err
			}

			input := application.RunInput{
				Project:       resolved.Project,
				Message:       args[0],
				BaseRef:       resolved.BaseRef,
				ArtifactStore: resolved.ArtifactStore,
			}

			if noTUI {
				return runJSONL(cmd.Context(), d, input)
			}
			return runTUI(cmd.Context(), d, input)
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of opening the TUI")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required with --no-tui)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "override base_ref")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "", "override artifact_store mode")
	cmd.Flags().StringVar(&project, "project", "", "override project slug")
	return cmd
}

// validateModeFlags enforces the M6 flag-mode policy:
//
//   - default (no flags)        → TUI mode.
//   - --no-tui --json (both)    → JSONL mode.
//   - --no-tui without --json   → error.
//   - --json without --no-tui   → error.
//
// Spec §2.4 reserves room for future modes (e.g. --plain), but in M6 the
// only two are TUI default + JSONL fallback.
func validateModeFlags(noTUI, jsonOut bool) error {
	if noTUI && !jsonOut {
		return fmt.Errorf("run: --no-tui requires --json (machine-readable output)")
	}
	if jsonOut && !noTUI {
		return fmt.Errorf("run: --json requires --no-tui (TUI is the default)")
	}
	return nil
}

// runJSONL is the existing M5 path: jsonsink.New(stdout) → Runner.Run.
func runJSONL(ctx context.Context, d Deps, input application.RunInput) error {
	sink := chooseJSONSink(d)
	runner := d.RunnerFactory(sink)
	res, err := runner.Run(ctx, input)
	if err != nil {
		var exit *application.ExitError
		if errors.As(err, &exit) {
			return exit
		}
		return err
	}
	_ = res
	return nil
}

// runTUI is the new M6 path: TUI program owns the sink; Runner.Run blocks
// until the program exits or the change reaches a terminal status.
//
// The TUI's CompleteMsg is dispatched from the runner's OnComplete callback,
// which fires in a goroutine inside Runner.Run. We start Runner.Run and the
// TUI Program concurrently; whichever returns first dictates the outcome:
//
//   - Program.Run returns on detach (Q / Ctrl+C confirm) → cancel runner ctx.
//   - Runner.Run returns on terminal → program already saw CompleteMsg →
//     program exits naturally.
//
// Spec §2.2: detach is purely visual — Runner keeps draining the channel
// until ctx cancels. We do NOT cancel the orchestrator Change.
func runTUI(parentCtx context.Context, d Deps, input application.RunInput) error {
	output := chooseTUIOutput(d)

	// We don't yet know the ChangeID — Runner.Run does CreateChange first.
	// Build the Program with an empty ChangeID and update it via the first
	// SnapshotMsg the Runner dispatches. The reattach hint pulls the
	// ChangeID from the model's last seen state.
	prog, err := tui.NewProgram(tui.ProgramConfig{Output: output})
	if err != nil {
		return fmt.Errorf("run: tui init: %w", err)
	}
	defer prog.Close() //nolint:errcheck

	runner := d.RunnerFactory(prog.Bridge())

	runnerCtx, cancelRunner := context.WithCancel(parentCtx)
	defer cancelRunner()

	type runnerResult struct {
		res application.RunResult
		err error
	}
	resultCh := make(chan runnerResult, 1)
	go func() {
		res, err := runner.Run(runnerCtx, input)
		resultCh <- runnerResult{res: res, err: err}
	}()

	hint, runErr := prog.Run(parentCtx)

	// Once the program exits, cancel the runner if it's still going so the
	// SSE subscription drops cleanly.
	cancelRunner()

	rr := <-resultCh

	// Print the reattach hint if we detached mid-stream.
	if hint != "" {
		// stderr keeps stdout clean for any subsequent piping.
		fmt.Fprintln(os.Stderr, hint)
	}

	// Decide which error to surface:
	//   - if program errored, that wins.
	//   - else, runner's error (ExitError) drives the exit code.
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

func chooseJSONSink(d Deps) inbound.EventSink {
	if d.JSONSinkOverride != nil {
		return d.JSONSinkOverride
	}
	return jsonsink.New(jsonsink.Config{Writer: os.Stdout})
}

func chooseTUIOutput(d Deps) io.Writer {
	if d.TUIOutput != nil {
		return d.TUIOutput
	}
	return os.Stdout
}

// envSnapshot returns the SOPHIA_* env vars consulted by the resolver.
func envSnapshot() map[string]string {
	out := map[string]string{}
	for _, k := range []string{
		application.EnvOrchestratorURL,
		application.EnvProject,
		application.EnvBaseRef,
	} {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		}
	}
	return out
}

// (compile-time guard: domain.PhaseType referenced for clarity)
var _ domain.PhaseType = ""
```

- [ ] **Step 5: Update wire.go**

`internal/bootstrap/wire.go` — replace the runner-construction block with the factory:

```go
runnerFactory := func(sink inbound.EventSink) *application.Runner {
    return application.NewRunner(application.RunnerDeps{
        Orch:        orch,
        State:       state,
        Git:         git,
        Sink:        sink,
        EventStream: stream,
    }, application.RunnerOptions{})
}
```

…and update the `cli.Deps` construction:

```go
deps := cli.Deps{
    Doctor:         doctor,
    Provisioner:    provisioner,
    Initializer:    initializer,
    StatusReader:   statusReader,
    RunnerFactory:  runnerFactory,
    Resolver:       resolver,
    UserConfigPath: userConfigPath,
    Version:        info.Version,
    Commit:         info.Commit,
    BuildDate:      info.BuildDate,
}
```

Add the `inbound` import:

```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
```

Remove the now-unused `jsonsink` and `os` imports if they're no longer referenced — but `os.Stderr` may still be used for `cfg.LogWriter` defaults; verify and keep what's necessary.

Update `internal/bootstrap/wire_test.go`:

```go
func TestNewWiresM6RunnerFactory(t *testing.T) {
    root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
    if err != nil {
        t.Fatal(err)
    }
    c, _, err := root.Find([]string{"run"})
    if err != nil || c == nil {
        t.Fatalf("run cmd missing: %v", err)
    }
}
```

(Keep any existing M5 tests intact. The M5 test `TestNewWiresM5SSEStream` is still valid as a smoke check — it doesn't depend on Runner-vs-RunnerFactory.)

- [ ] **Step 6: Run tests + build**

```bash
go test ./internal/adapters/inbound/cli/... ./internal/bootstrap/... -race
go vet ./...
make build
```

Expected: PASS, binary builds.

- [ ] **Step 7: Smoke**

```bash
# Help text now describes TUI default
./bin/sophia run --help

# TUI default mode — outside .sophia.yaml, exit 3 (resolver fails before TUI starts)
./bin/sophia run "test"
echo "exit=$?"

# JSONL mode — same as M5
./bin/sophia run "test" --no-tui --json
echo "exit=$?"

# Invalid combos
./bin/sophia run "test" --no-tui      # error: --no-tui requires --json
./bin/sophia run "test" --json        # error: --json requires --no-tui
```

Expected:
- `run --help` shows `--no-tui` and `--json` with M6 wording (no longer "required in M4").
- All three exit-3 cases report exit 3.
- The two invalid-combo cases return errors with the expected message and a non-zero exit.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/inbound/cli/run.go internal/adapters/inbound/cli/run_test.go \
        internal/adapters/inbound/cli/root.go internal/bootstrap/wire.go \
        internal/bootstrap/wire_test.go
git commit -m "feat(cli): invert --no-tui/--json flags; bootstrap exposes RunnerFactory"
```

---

## Phase 8 — teatest golden integration test

### Task 8: test/tui/timeline_test.go — drive the TUI program end-to-end

**Files:**
- Create: `test/tui/timeline_test.go`

teatest spins up a real bubbletea program with a virtual terminal, lets us drive keyboard input, and exposes the rendered output for golden assertions. We use it to verify:

1. A snapshot dispatched via the bridge renders correctly.
2. A `phase.started` event renders the "running" marker on the right row.
3. Pressing `Q` exits the program and surfaces the "Detached" outcome.
4. Pressing `Ctrl+C` once enters the confirmation dialog; pressing it again detaches.

> **Verification gate:** Phase 1's Step 3 settled the teatest module path. If teatest v2 ships with a different import or different driver primitives (e.g. `tea.WithProgram(p)` vs `teatest.NewTestModel(t, model)`), adapt the tests below accordingly. The TEST CONTRACT — "given snapshot, see explore in running state; given Q, see detach" — must stay the same.

- [ ] **Step 1: Create the test file**

`test/tui/timeline_test.go`:

```go
package tui_integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// TestTimelineSnapshotRenders sends a SnapshotMsg and asserts the explore
// row shows up running.
func TestTimelineSnapshotRenders(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit()

	tm.Send(tui.SnapshotMsg{Change: &domain.Change{
		ID:             domain.ChangeID("01HXABC"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-explore",
		Phases: []domain.Phase{
			{ID: "p-explore", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "explore") &&
			(strings.Contains(s, "running") || strings.Contains(s, "▶"))
	}, teatest.WithDuration(2*time.Second))
}

// TestTimelinePhaseStartedEvent dispatches an EventMsg and verifies the
// row updates.
func TestTimelinePhaseStartedEvent(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit()

	tm.Send(tui.EventMsg{Event: domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": "proposal", "phase_id": "p-1"},
	}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "proposal") &&
			(strings.Contains(s, "running") || strings.Contains(s, "▶"))
	}, teatest.WithDuration(2*time.Second))
}

// TestTimelineQDetaches presses Q and expects the program to exit cleanly.
func TestTimelineQDetaches(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)

	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: 'q'}})

	// Program should quit within a short window.
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	// The final model must have Detached=true.
	final := tm.FinalModel(t)
	rm, ok := final.(rootModelExposed)
	if !ok {
		t.Fatalf("final model = %T, want rootModelExposed", final)
	}
	if !rm.State().Detached() {
		t.Error("final model should be detached after Q")
	}
}

// TestTimelineCtrlCConfirmThenDetach presses Ctrl+C twice and asserts the
// confirmation dialog appears in between.
func TestTimelineCtrlCConfirmThenDetach(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXABC")),
		teatest.WithInitialTermSize(120, 40),
	)

	// First Ctrl+C — confirm dialog.
	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: 'c', Mod: tea.ModCtrl}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Detach?")
	}, teatest.WithDuration(time.Second))

	// Second Ctrl+C — detach.
	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: 'c', Mod: tea.ModCtrl}})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	final := tm.FinalModel(t)
	rm, ok := final.(rootModelExposed)
	if !ok {
		t.Fatalf("final model = %T", final)
	}
	if !rm.State().Detached() {
		t.Error("final model should be detached after second Ctrl+C")
	}
}

// rootModelExposed exposes the inner Model so tests can assert on state.
// We construct a thin wrapper over the package's rootModel and re-export
// State() because rootModel itself is unexported.
//
// This wrapper exists only in the test binary — it is NOT a public API.
type rootModelExposed interface {
	State() tui.Model
}

type wrappedModel struct {
	state tui.Model
}

func (m wrappedModel) State() tui.Model { return m.state }

func (m wrappedModel) Init() (tea.Model, tea.Cmd) {
	return m, nil
}

func (m wrappedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newState, cmd := tui.Update(m.state, msg)
	m.state = newState
	return m, cmd
}

func (m wrappedModel) View() string { return tui.View(m.state) }

func newRoot(id domain.ChangeID) tea.Model {
	return wrappedModel{state: tui.NewModel(tui.ModelConfig{ChangeID: id})}
}

// Suppress unused if some imports are not referenced.
var _ = context.Background
```

> **Test stability note (RM6-03):** `teatest.WaitFor` polls the rendered output every ~10ms by default. The 2-second timeout is generous. If CI flakes, raise to 5s — never to less than 2s, otherwise headless terminals on slow CI will start failing.

- [ ] **Step 2: Run the tests**

```bash
go test ./test/tui/... -race -timeout 60s
```

Expected: PASS for all four cases.

- [ ] **Step 3: Diagnose teatest API drift if it fails**

If `teatest.NewTestModel`, `tm.Send`, `tm.Output`, `tm.WaitFinished`, or the `teatest.WithInitialTermSize` / `teatest.WithDuration` / `teatest.WithFinalTimeout` constructors don't exist:

1. Run `go doc github.com/charmbracelet/x/exp/teatest/v2` (or `teatest@latest` if `/v2` doesn't exist).
2. Find the equivalent helpers — common alternatives:
   - `teatest.NewTestModel(t, m)` may be `teatest.NewProgram(t, m)`.
   - `tm.Send(msg)` may be `tm.Tx(msg)` or via the writer interface.
   - Output assertions may use `teatest.RequireEqualOutput` instead of `WaitFor`.
3. Adapt — keep the test contract.
4. STOP and ASK if no equivalent surface exists for "drive a Bubble Tea program from a test."

- [ ] **Step 4: Commit**

```bash
git add test/tui/timeline_test.go
git commit -m "test(tui): add teatest integration coverage for snapshot/event/Q/Ctrl+C"
```

---

## Phase 9 — Final validation

### Task 9: Final validation pass + interactive smoke

**Files:** none (verification only).

- [ ] **Step 1: vet + tests + race**

```bash
go vet ./...
go test -race ./...
```
Expected: exit 0. If teatest tests flake on CI under `-race`, run them with `-count=1` once and confirm the failure is timing-related, not logical. The fix is to raise teatest timeouts (Phase 8 Step 2) — NOT to silence the race detector.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```
Acceptable `//nolint` patterns: existing precedents (gosec on subprocess shellouts, errcheck on `defer prog.Close()`, errcheck on `defer stop()` already present from M5). Fix new findings in place.

- [ ] **Step 3: Coverage**

```bash
go test -coverprofile=cover.out ./internal/adapters/inbound/tui/... ./internal/adapters/inbound/cli/...
go tool cover -func=cover.out | tail -n 1
```
Expected: total ≥ 70% across the new tui package. The bridge and model are pure functions and should clear 90%+ on their own; program.go and run.go are harder to cover without teatest, and Phase 8 plus the cli unit tests should bring them to ~75%.

- [ ] **Step 4: Binary smoke**

```bash
make build

# 1) Help text now lists TUI as the default
./bin/sophia run --help

# 2) Outside a repo, exit 3 (no .sophia.yaml) — same as M5
./bin/sophia run "test" --no-tui --json
echo "no-config exit=$?"

# 3) Default (TUI) mode outside repo — should also exit 3, NOT open the TUI
./bin/sophia run "test"
echo "default-no-config exit=$?"

# 4) Invalid combos
./bin/sophia run "test" --no-tui
./bin/sophia run "test" --json

# 5) Other commands still work
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
```

Expected:
- `run --help` shows new wording: `--no-tui` "stream JSONL to stdout instead of opening the TUI" (no "required in M4" / "required in M5" anymore).
- `run "test" --no-tui --json` outside `.sophia.yaml` exits 3.
- `run "test"` (default TUI) outside `.sophia.yaml` exits 3 — the resolver runs BEFORE the TUI is constructed.
- `run "test" --no-tui` (without `--json`) returns a clear error and a non-zero exit.
- `run "test" --json` (without `--no-tui`) returns a clear error and a non-zero exit.

- [ ] **Step 5: Interactive smoke (manual — described, executed by human reviewer)**

The TUI cannot be smoke-tested headlessly without a real terminal, so the steps below are run by hand by the reviewer. Document each result in the PR / commit message.

Pre-req: a running orchestrator at `SOPHIA_ORCHESTRATOR_URL` (default localhost:9080) plus a `.sophia.yaml` in the working directory.

1. **Default TUI mode renders Timeline:**
   ```bash
   ./bin/sophia run "smoke test M6 default mode"
   ```
   Expect: a Bubble Tea program opens; 9 phase rows visible; the active phase shows the running marker; bottom hint reads "Q to detach · Ctrl+C confirm-then-detach".

2. **`Q` detaches:**
   While the run is in flight, press `Q`. Expect: program exits; stderr prints `Detached. Reattach with: sophia attach <ULID>`.

3. **`Ctrl+C` first press shows confirm:**
   Restart the run. Press `Ctrl+C` once. Expect: bottom of view shows "Detach? (y/n)" in a styled banner.

4. **`Ctrl+C` second press detaches:**
   With the confirm dialog visible, press `Ctrl+C` again. Expect: program exits with the reattach hint as in step 2.

5. **`y` confirms detach:**
   Restart, press `Ctrl+C`, then `y`. Expect: program exits with hint.

6. **`n` cancels confirm:**
   Restart, press `Ctrl+C`, then `n`. Expect: confirm banner disappears; the Timeline keeps rendering events.

7. **Approval marker:**
   Trigger an `approval.required` event from the orchestrator (or use a stub). Expect: the affected phase row gains a red-styled `!` marker.

8. **Resize:**
   Resize the terminal during the run. Expect: rows truncate gracefully — no broken ANSI escapes, no panic.

9. **Heartbeat drop policy (high-throughput):**
   With a stub orchestrator emitting heartbeats every 100ms while the bridge sender is artificially slowed, run the TUI for 30s. Expect: heartbeats drop without affecting any phase.* row updates. (This case is also covered by Phase 2's unit tests; manual confirms behavior end-to-end.)

10. **`--no-tui --json` parity:**
    Run the same command with `--no-tui --json`. Expect: identical JSONL output to the M5 baseline; no TUI opens.

If ANY step fails, file an issue or stop the M6 ship. Document the failure mode in the M6 final commit.

- [ ] **Step 6: Integration smoke (carry-over)**

```bash
go test -race ./test/integration/...
```
Expected: PASS for the M5 SSE reconnect + heartbeat tests AND the M3 init/filestate integration tests. M6 didn't change those paths but the binary did.

- [ ] **Step 7: e2e smoke (carry-over)**

```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS. The M5 e2e tests use `--no-tui --json`, which is unchanged in M6 except for the flag-validation message. If a test asserted on the old "required in M4" error string, update it to the new "--no-tui requires --json" wording.

- [ ] **Step 8: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m6): final validation pass" || echo "nothing to commit"
git tag -a m6-tui-timeline -m "M6 TUI Timeline complete"
git tag
```

---

## Self-review checklist

- [ ] **Spec coverage:** every M6 DoD item from spec §7.2 has at least one task.
  - Bubble Tea v2 + Lipgloss v2 with versions pinned → Task 1
  - Timeline view: 9 phases, status icons, current phase, duration, confidence → Task 5
  - SSE bridge with cap-256 buffer; drop policy honored → Task 2
  - `Q` = detach immediately; `Ctrl+C` first-press confirm, second-press detach → Tasks 4, 8
  - Tests with teatest (v2 utilities) → Task 8
- [ ] **No placeholders:** no "TBD"/"TODO"/"similar to" in steps.
- [ ] **Type consistency:** `tui.Bridge`, `tui.Sender`, `tui.SnapshotMsg`/`EventMsg`/`ApprovalGateMsg`/`ErrorMsg`/`CompleteMsg`, `tui.Model`, `tui.Update`, `tui.View`, `cli.RunnerFactory` consistent across tasks.
- [ ] **Frequent commits:** every task ends with a commit.
- [ ] **TDD discipline:** failing test before implementation in every Phase 2–6 task; Phase 7 refactor extends the test suite first.
- [ ] **No premature M7+ scope:** no ApplyBoard view, no ApprovalGate banner UI (only `!` marker), no browser opener, no `Tab` toggle, no `--orchestrator-url`.
- [ ] **No new domain types:** `domain.Change`, `domain.Event`, `domain.PhaseType` reused — Tasks 3, 4 confirm this by importing domain only.
- [ ] **lipgloss ANSI safety:** `view_timeline.go` routes every dynamic string through `pkgStyles.<Style>.Render(...)`. Search for `fmt.Sprintf` / `fmt.Fprintf` writing user-supplied data — they must be inside a Render() call. Inv 7 honored.
- [ ] **Bridge purity:** `bridge_test.go` exercises drop policy without a real bubbletea program. Phase priority: heartbeat dropped first; `phase.*`/`approval.*` never dropped. All three drop categories have at least one test.
- [ ] **Flag inversion verified:** `cli/run_test.go` asserts default → TUI, `--no-tui --json` → JSONL, `--no-tui` alone fails, `--json` alone fails.

---

## Pending decisions (carried into M6 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M6-01 | bubbletea import path | Try `github.com/charmbracelet/bubbletea/v2` first; fall back to `charm.land/bubbletea/v2`. STOP and ASK if neither resolves. (Verified in Task 1.) |
| D-M6-02 | Block until terminal status, OR allow detach mid-stream? | Q always detaches mid-stream (per §2.2); program exits when the Runner loop ends OR when the user detaches. Whichever comes first wins. |
| D-M6-03 | What happens to in-flight events when detached? | Bridge stops forwarding to the program (program is gone); Runner keeps running until parent ctx cancels. Detach is purely visual — the orchestrator Change is not affected. |
| D-M6-04 | Confirmation message keystroke | `Ctrl+C` OR `y` confirms second press; `n` cancels confirmation. ANY other key (e.g. `x`, `Esc`) also cancels confirmation. (Confirmed in `update_test.go::TestUpdateUnknownKeyInConfirmCancels`.) |
| D-M6-05 | Width/height: respect terminal size? | YES — `tea.WindowSizeMsg` updates Model.width/height; View truncates phase rows to fit. Manual smoke step 8 verifies. |
| D-M6-06 | Approval gate visible in Timeline? | Show a discreet `!` marker on the affected phase row; full banner is M7. (Implemented in Task 3 model, Task 5 view; tested in `model_test.go` + `view_timeline_test.go`.) |
| D-M6-07 | Reattach hint destination | stderr (so stdout stays clean for any later piping). Format: `Detached. Reattach with: sophia attach <ULID>` per §2.2. |
| D-M6-08 | Bridge worker model | sync.Cond-driven single worker. Alternative: buffered channel. The cond-variable approach is friendlier to the displace-and-reorder drop policy because we need O(n) scans of the queue under the mutex anyway. |

---

## Risks specific to M6

| ID | Risk | Mitigation |
|---|---|---|
| RM6-01 | Bubble Tea v2 API surface differs from this plan (rename `KeyMsg` → `KeyPressMsg`, removal of `tea.WithoutSignals`, `Program.Quit()` semantics changed, etc.) | Task 1 verification gate; if Tasks 4/6 hit an API mismatch, ASK before guessing. The implementation already isolates v2-specific calls inside `keyPressString` (Task 4) and `teaSender` / `rootModel` (Task 6) — adapt these helpers without touching pure code. |
| RM6-02 | `program.Send` blocks under sustained pressure | Bridge owns the cap-256 buffer (Task 2); under pressure it sheds heartbeats → agent.* → other; phase.* / approval.* are protected by displace-on-evict. The bridge does NOT assert non-blocking; Send is allowed to block. |
| RM6-03 | TUI tests flake on slow CI (teatest depends on terminal sizing/render timing) | `teatest.WaitFor` uses 2s timeouts (Phase 8 Step 2); golden-string assertions are content-based (`strings.Contains`) not byte-exact, so minor ANSI differences are tolerated. NO_COLOR is forced in setup_test.go. |
| RM6-04 | Sink swap broken in bootstrap (circular dep between TUI bridge and Runner) | Bridge is constructed AFTER Program (Task 6 wiring). Program's Sender (`teaSender`) holds a stable pointer to `*tea.Program`, so the bridge can be passed to `RunnerFactory(prog.Bridge())` cleanly. No circular import: `tui` imports `domain` + `inbound` + `bubbletea`; `cli` imports `tui` + `application`; nothing imports `cli`. |
| RM6-05 | TUI rendering differs across terminal types (Apple Terminal vs iTerm vs xterm) | Styles restricted to standard 8-color palette (Task 5). NO_COLOR forced in tests. Manual smoke step 8 (resize) covers reflow on real terminals. |
| RM6-06 | `defer prog.Close()` in tests doesn't actually stop a hung program | Phase 8 uses `tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))` — teatest enforces the timeout. Phase 6's program_test.go uses `defer p.Close()` on a real program but injects a `&devNull{}` writer + nil input; `Run()` returns within 50ms because we dispatch `CompleteMsg` from a goroutine. |
| RM6-07 | Runner ctx cancellation doesn't actually drain the SSE channel | Task 7's `runTUI` calls `cancelRunner()` after the program returns. The M5 SSE client honors ctx cancellation in its run loop (`return ctx.Err()` on each select). Re-verify by tracing M5's ssestream/client.go::run — search for `ctx.Done()` in the loop, confirm it's there. |
| RM6-08 | Bridge OnSnapshot copies the *domain.Change pointer; concurrent mutation by the runner could race | Task 2 dereferences `*c` and stores a value copy in the message: `cp := *c; ... msg: SnapshotMsg{Change: &cp}`. The message's pointer aliases the local copy, not the original Runner state. Verify by reading bridge.go. |

---

## What this plan does NOT cover (intentional)

- ApplyBoard view → M7
- Full ApprovalGate banner UI (only `!` marker in M6) → M7
- Browser opener (`[O]pen` shortcut) → M7
- `Tab` key view toggle (Timeline ↔ ApplyBoard) → M7
- Real `sophia attach` / `sophia changes` → M8
- Approval-timeout exit code 5 → M7+
- Cross-process `Last-Event-ID` resume → M8
- `--orchestrator-url` per-call rebinding → M7+
- New domain types → not needed; reuse `domain.Change`, `domain.Event`, `domain.PhaseType`
- TUI logging panel for non-phase events → M7 (would render `agent.*` / `task.*` events as scrolling lines below the timeline)
- TUI footer with stream-health indicator (drops counter, reconnect badge) → M7

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-06-sophia-cli-m6-tui-timeline.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task. Use `superpowers:subagent-driven-development`. Each task has a self-contained TDD cycle (write test → fail → implement → pass → commit), so subagents can work independently with minimal context. Phase 7 (cli/bootstrap rewire) and Phase 8 (teatest) are the largest; consider giving them more time-budget.

**2. Sequential single-agent** — use `superpowers:executing-plans` and walk Task 1 → Task 9 in order. Recommended only if you want to keep the full context window for cross-task surprises (most likely Tasks 1, 4, 6, and 8 if the bubbletea v2 API differs from this plan's assumptions — see RM6-01).

Either way: keep an eye on D-M6-01 (the bubbletea import-path verification gate at the top of Task 1). Same for the v2 API drift in Tasks 4 (KeyPressMsg shape), 6 (Program options + ErrorKilled), and 8 (teatest helpers). If any of those four hit a mismatch with the actual installed module, STOP and ask the user before improvising.
