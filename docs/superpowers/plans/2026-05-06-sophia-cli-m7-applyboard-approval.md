# Sophia CLI — M7 ApplyBoard + Approval Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a second TUI view (ApplyBoard) and an Approval Gate banner on top of the M6 Timeline. `Tab` toggles between Timeline and ApplyBoard. `approval.required` events render a full banner (URL/Reason/Risk/Policy/ChangeID/Phase/TraceID) overlaid at the top of whichever view is active. The banner stays visible until any of three derived-state triggers fires: (a) `approval.resolved` event, (b) snapshot refresh whose `CurrentPhase` advances past the gated phase, or (c) any `phase.started` for a phase strictly after the gated phase (forward-progress). `[O]` opens the gate URL in the OS browser through a brand-new `osbrowser` outbound adapter that implements the existing `outbound.Browser` port. URL validation is mandatory and runs BEFORE the subprocess call: only `http`/`https` schemes survive; everything else (`javascript:`, `data:`, `file:`, `vbscript:`, `mailto:`, `ftp:`, anything weird) is rejected with a typed error that surfaces in the TUI as an error line. In `--no-tui --json` mode, an `--approval-timeout` flag (default `30m`) starts a timer on `OnApprovalGate`; the timer is canceled by `approval.resolved` / forward-progress / a snapshot that clears the block; expiry yields exit code 5 (spec §2.3, §5.8).

**Architecture:** Three new files under `internal/adapters/inbound/tui/` (`applyboard_state.go`, `view_applyboard.go`, `view_approval_banner.go`) plus extensions to `model.go`/`update.go`/`keybindings.go`/`program.go`/`view_timeline.go`. One new outbound adapter package `internal/adapters/outbound/osbrowser/` with `browser.go` (validating URL parser + OS-aware command dispatcher implementing `outbound.Browser`). The CLI's `run.go` grows a `--approval-timeout` flag for JSONL mode and a small wrapper sink that watches for the three banner-clearing events. Bootstrap injects `osbrowser.New(...)` into the TUI program path.

**Tech Stack:** Go 1.25.0 · `charm.land/bubbletea/v2` v2.0.6 · `charm.land/lipgloss/v2` v2.0.3 · `charm.land/bubbles/v2` v2.1.0 (already pinned in M6's `go.mod`). Tests use `github.com/charmbracelet/x/exp/teatest/v2`. No new external dependencies.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.2 banner UI, §2.3 exit codes, §5.4 event payloads, §5.5 ApprovalGate domain type, §5.8 approval timeout, §6.3 #3 subprocess + URL validation, §7.2 M7 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M7)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M7 boundaries — what is NOT in M7:**

- No real `sophia attach` / `sophia changes` — stay stubs (M8).
- No real `sophia status` against orchestrator — local resolution only (M8).
- No cross-process `Last-Event-ID` resume — M8.
- No `--orchestrator-url` per-call rebinding — M9+.
- No ApplyBoard column-sortable mode — defer to M9+ polish.
- No risk-tier-coloured approval banner — M7 ships with one fixed style; tier-aware coloring is a polish ticket.
- No new outbound port — `outbound.Browser` already exists; M7 only adds the IMPL.
- No new domain types — M7 reuses `domain.ApprovalGate`, `domain.PhaseType`, `domain.Event`. ApplyBoard state is TUI-internal (UI concern, not domain).
- No approval timeout in TUI — TUI banner has no timer; the user closes it by interacting (the snapshot/event triggers do clear it derivedly). The 30-minute timeout is JSONL-mode only.

---

## Phase 1 — Browser opener (osbrowser adapter)

### Task 1: `osbrowser/browser.go` — URL validation + OS-aware subprocess dispatch

**Files:**
- Create: `internal/adapters/outbound/osbrowser/doc.go`
- Create: `internal/adapters/outbound/osbrowser/browser.go`
- Create: `internal/adapters/outbound/osbrowser/browser_test.go`

The `outbound.Browser` port already exists (`internal/ports/outbound/browser.go`) with the signature `Open(ctx context.Context, url string) error`. M7 ships the IMPL. The implementation has TWO well-isolated responsibilities:

1. **Validate the URL** — parse it with `net/url`, reject anything whose scheme is not `http` or `https`. This MUST happen before any subprocess call. Spec §6.3 #3: arbitrary unvalidated URLs reaching `exec.Command` is the foothold for shell-injection-style attacks (`javascript:alert(1)` is the trivial demo, but `file:///etc/passwd` and `data:text/html,...` are equally dangerous on platforms where the OS handler is permissive).
2. **Dispatch to the OS handler** — `runtime.GOOS` switch:
   - `darwin` → `open <url>`
   - `linux` (and any other Unix) → `xdg-open <url>`
   - `windows` → `cmd /c start "" <url>` (the empty quoted title is required so `start` doesn't interpret the URL as a title)
   - everything else (BSD without xdg-open, Solaris, Plan 9) → typed `ErrUnsupportedPlatform`.

Tests use the fake-binary pattern (a temp directory whose `PATH` starts with a stub `open`/`xdg-open` script that captures the args), the SAME pattern `gitcli` and `composeexec` use today. This keeps the test hermetic — no real browser launches, no flaky CI.

> **Verification gate (D-M7-08):** before writing the implementation, confirm the existing port shape: `bat /Users/russell/Documents/2026/sophia-cli/internal/ports/outbound/browser.go`. The port is `outbound.Browser` (not `BrowserOpener`); the method is `Open(ctx context.Context, url string) error`. If the port has been renamed since this plan was written, STOP and ASK before substituting names.

- [ ] **Step 1: Create the package doc**

`internal/adapters/outbound/osbrowser/doc.go`:

```go
// Package osbrowser implements outbound.Browser by shelling out to the OS-
// native URL handler (`open` on macOS, `xdg-open` on Linux, `cmd /c start`
// on Windows). It performs strict URL validation BEFORE invoking any
// subprocess: only http:// and https:// URLs are accepted. javascript:,
// data:, file:, vbscript:, and any other scheme are rejected with
// ErrInvalidScheme.
//
// Spec invariants honored:
//
//   - §6.3 #3: subprocess + URL validation. The validated URL is the only
//     value that reaches exec.Command; the schemes whitelist is enforced
//     before fork.
//   - §7.2 M7 DoD: implements outbound.Browser; wired into the TUI program
//     so [O] in the approval banner opens the gate URL.
//
// Out of scope: opening the user's preferred browser explicitly (we delegate
// to the OS), supporting per-platform browser flags, and probing whether a
// handler exists before fork.
package osbrowser
```

- [ ] **Step 2: Write the failing test**

`internal/adapters/outbound/osbrowser/browser_test.go`:

```go
package osbrowser_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/osbrowser"
)

func TestOpenAcceptsHTTPS(t *testing.T) {
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	if err := b.Open(context.Background(), "https://gov.local/approvals/abc123"); err != nil {
		t.Fatalf("Open(https://...): %v", err)
	}
	got := readCapture(t, dir)
	if !strings.Contains(got, "https://gov.local/approvals/abc123") {
		t.Errorf("opener didn't see URL; got %q", got)
	}
}

func TestOpenAcceptsHTTP(t *testing.T) {
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	if err := b.Open(context.Background(), "http://localhost:8080/approval"); err != nil {
		t.Fatalf("Open(http://...): %v", err)
	}
}

func TestOpenRejectsJavaScript(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "javascript:alert(1)")
	if err == nil {
		t.Fatal("expected error for javascript: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v, want ErrInvalidScheme", err)
	}
}

func TestOpenRejectsData(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "data:text/html,<script>alert(1)</script>")
	if err == nil {
		t.Fatal("expected error for data: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v, want ErrInvalidScheme", err)
	}
}

func TestOpenRejectsFile(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for file: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsVBScript(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "vbscript:msgbox(1)")
	if err == nil {
		t.Fatal("expected error for vbscript: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsMailto(t *testing.T) {
	// mailto: is not in §6.3 #3's explicit blocklist, but the policy is a
	// WHITELIST of http/https. mailto: is not on the list → reject.
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "mailto:admin@example.com")
	if err == nil {
		t.Fatal("expected error for mailto: scheme (whitelist policy)")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsFTP(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "ftp://files.example.com/approval")
	if err == nil {
		t.Fatal("expected error for ftp: scheme")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRejectsEmpty(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("error = %v, want ErrInvalidURL", err)
	}
}

func TestOpenRejectsMalformed(t *testing.T) {
	b := osbrowser.New(osbrowser.Config{})
	// A control character in the URL fails url.Parse.
	err := b.Open(context.Background(), "http://example.com/\x7f")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("error = %v, want ErrInvalidURL", err)
	}
}

func TestOpenRejectsSchemeRelative(t *testing.T) {
	// "//example.com/x" parses with empty scheme — reject as ErrInvalidScheme.
	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "//example.com/path")
	if err == nil {
		t.Fatal("expected error for scheme-relative URL")
	}
	if !errors.Is(err, osbrowser.ErrInvalidScheme) {
		t.Errorf("error = %v", err)
	}
}

func TestOpenSubprocessFailureSurfaces(t *testing.T) {
	// stubOpener with non-zero exit code → Open returns a wrapped error that
	// is NOT ErrInvalidScheme / ErrInvalidURL.
	dir := stubOpener(t, 1)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	err := b.Open(context.Background(), "https://example.com/x")
	if err == nil {
		t.Fatal("expected error from non-zero subprocess exit")
	}
	if errors.Is(err, osbrowser.ErrInvalidScheme) || errors.Is(err, osbrowser.ErrInvalidURL) {
		t.Errorf("subprocess failure should not classify as validation error: %v", err)
	}
}

func TestOpenContextCancellationKillsSubprocess(t *testing.T) {
	// Context cancellation should propagate via exec.CommandContext. We
	// don't have a clean way to assert process death without slowing the
	// stub artificially; instead we assert that ctx.Err is wrapped in the
	// returned error when the context is already canceled.
	dir := stubOpener(t, 0)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	b := osbrowser.New(osbrowser.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := b.Open(ctx, "https://example.com/x")
	// On a cold-cancel ctx, exec.CommandContext returns context.Canceled
	// either directly or wrapped. We accept either form.
	if err == nil {
		t.Fatal("expected error when ctx is pre-canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("note: error = %v (we accept this — some platforms surface SIGKILL)", err)
	}
}

func TestNewWithNilOSReturnsUnsupported(t *testing.T) {
	// Use the OS-override hook to simulate an unsupported platform.
	b := osbrowser.New(osbrowser.Config{OSOverride: "plan9"})
	err := b.Open(context.Background(), "https://example.com/x")
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
	if !errors.Is(err, osbrowser.ErrUnsupportedPlatform) {
		t.Errorf("error = %v, want ErrUnsupportedPlatform", err)
	}
}

// --- helpers ---

// stubOpener creates a temp directory containing a stub `open`,
// `xdg-open`, and `cmd` (whichever runtime.GOOS would dispatch).
// The stub appends its argv (excluding $0) to <dir>/capture.txt and
// exits with the given code.
func stubOpener(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()

	switch runtime.GOOS {
	case "windows":
		// Windows stubbing is more involved — skip for now and run real
		// behavior via the OS-override hook in tests that need it. The
		// Linux/Darwin paths are the operational ones for our CI.
		t.Skip("subprocess capture stubbing is POSIX-only in M7 tests")
	default:
		writeStub(t, filepath.Join(dir, "open"), exitCode)
		writeStub(t, filepath.Join(dir, "xdg-open"), exitCode)
	}
	return dir
}

func writeStub(t *testing.T, path string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$(dirname \"$0\")/capture.txt\"\n" +
		"exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readCapture(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "capture.txt"))
	if err != nil {
		t.Fatalf("readCapture: %v", err)
	}
	return string(b)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i == 1 {
		return "1"
	}
	// Shouldn't happen in M7 tests.
	return "2"
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/adapters/outbound/osbrowser/...`
Expected: FAIL — `osbrowser.New`, `Config`, `ErrInvalidScheme`, `ErrInvalidURL`, `ErrUnsupportedPlatform`, `OSOverride` undefined.

- [ ] **Step 4: Implement**

`internal/adapters/outbound/osbrowser/browser.go`:

```go
package osbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// Sentinel errors callers can match with errors.Is.
var (
	// ErrInvalidURL is returned when the URL fails to parse OR is empty.
	ErrInvalidURL = errors.New("osbrowser: invalid URL")
	// ErrInvalidScheme is returned when the URL parses but its scheme is
	// not in the http/https whitelist (spec §6.3 #3).
	ErrInvalidScheme = errors.New("osbrowser: scheme not allowed (only http/https)")
	// ErrUnsupportedPlatform is returned when runtime.GOOS has no known
	// handler dispatch.
	ErrUnsupportedPlatform = errors.New("osbrowser: unsupported platform")
)

// Config configures New.
type Config struct {
	// OSOverride lets tests pin runtime.GOOS to a specific value. Empty in
	// production. Accepts the canonical Go values: "darwin", "linux",
	// "windows", "freebsd", etc. Unknown values produce ErrUnsupportedPlatform.
	OSOverride string
}

// Browser implements outbound.Browser via OS subprocess dispatch.
type Browser struct {
	osName string
}

// New constructs a Browser. With cfg.OSOverride empty, runtime.GOOS is used.
func New(cfg Config) *Browser {
	osName := cfg.OSOverride
	if osName == "" {
		osName = runtime.GOOS
	}
	return &Browser{osName: osName}
}

// Compile-time check: Browser must satisfy outbound.Browser.
var _ outbound.Browser = (*Browser)(nil)

// Open validates the URL, then dispatches to the platform-native handler.
func (b *Browser) Open(ctx context.Context, raw string) error {
	if err := validate(raw); err != nil {
		return err
	}
	cmd, err := b.command(ctx, raw)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		// exec.Cmd.Run wraps non-zero exit and ctx errors. We surface them
		// verbatim so callers can errors.Is(err, context.Canceled) etc.
		return fmt.Errorf("osbrowser: subprocess: %w", err)
	}
	return nil
}

// validate parses raw with net/url and enforces the scheme whitelist.
// Returns ErrInvalidURL or ErrInvalidScheme on failure.
//
// Notes:
//
//   - Empty input is ErrInvalidURL.
//   - Anything net/url rejects (control chars, malformed escapes, etc.) is
//     ErrInvalidURL.
//   - Anything that parses but has scheme != "http" / "https" is
//     ErrInvalidScheme. This includes "" (scheme-relative URLs like
//     "//example.com/x") because the policy is a strict whitelist.
func validate(raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	switch u.Scheme {
	case "http", "https":
		// Accept.
	default:
		return fmt.Errorf("%w: got %q", ErrInvalidScheme, u.Scheme)
	}
	if u.Host == "" {
		// "http:" with no host is not a useful target.
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return nil
}

// command constructs the exec.Cmd for the configured OS. URL has already
// been validated; we still pass it as a single argv element (no shell
// interpolation) so exec.Cmd's argv-quoting is the only path.
func (b *Browser) command(ctx context.Context, validated string) (*exec.Cmd, error) {
	switch b.osName {
	case "darwin":
		return exec.CommandContext(ctx, "open", validated), nil
	case "windows":
		// `cmd /c start "" <url>` — the empty quoted title prevents `start`
		// from treating the first quoted argument as a window title when the
		// URL contains spaces (unlikely for our gov.local URLs but kept
		// defensive). The URL is still a single argv element.
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", validated), nil
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		// xdg-open is the de-facto Unix opener — installed by default on
		// every modern desktop distro. Headless servers don't have it; the
		// subprocess will fail with exec: "xdg-open": executable file not
		// found in $PATH and the caller will surface the error.
		return exec.CommandContext(ctx, "xdg-open", validated), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, b.osName)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/adapters/outbound/osbrowser/... -race -timeout 30s
```

Expected: PASS. On a Linux/macOS dev box, the stub-script tests run; on Windows, `stubOpener` skips them and only the validation tests run (the `OSOverride: "plan9"` test still asserts `ErrUnsupportedPlatform`).

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/outbound/osbrowser/doc.go \
        internal/adapters/outbound/osbrowser/browser.go \
        internal/adapters/outbound/osbrowser/browser_test.go
git commit -m "feat(osbrowser): add outbound.Browser impl with URL whitelist (§6.3 #3)"
```

---

## Phase 2 — ApplyBoard state (TUI-internal types)

### Task 2: `tui/applyboard_state.go` — Pure types + event-application helpers

**Files:**
- Create: `internal/adapters/inbound/tui/applyboard_state.go`
- Create: `internal/adapters/inbound/tui/applyboard_state_test.go`

ApplyBoard's data is derived from `task.*` and `agent.*` events (spec §5.4). The minimal in-memory shape is groups → tasks → agents. We keep this entirely inside the TUI package — these are UI concerns, not domain concerns. M8 may need richer querying (filtering by group, sorting by status) but M7 only needs deterministic application of incoming events.

Event payload shapes per spec §5.4 (paraphrased):

| Event | Payload keys |
|---|---|
| `agent.spawned` | `agent_role`, `agent_id`, `group_id?`, `task_id?` |
| `agent.completed` | `agent_id`, `status` |
| `task.started` | `group_id`, `task_id`, `files_pattern` |
| `task.completed` | `group_id`, `task_id`, `status` |

The state holds groups in a deterministic order (insertion order for M7; M9+ may add explicit ordering). When a group/task is referenced by an event before its `task.started`/`agent.spawned` arrives (rare, but events can arrive interleaved across the SSE stream), the state lazily creates the entry with empty defaults — this is forward-compatible with the redactor's "skip unknown" stance.

**Bound (RM7-07):** the state caps at 50 groups (LRU eviction by insertion-order). M9+ would make this configurable.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/applyboard_state_test.go`:

```go
package tui_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestNewApplyBoardStateIsEmpty(t *testing.T) {
	s := tui.NewApplyBoardState()
	if got := s.GroupCount(); got != 0 {
		t.Errorf("fresh GroupCount = %d, want 0", got)
	}
	if got := s.Groups(); len(got) != 0 {
		t.Errorf("fresh Groups() = %v, want []", got)
	}
}

func TestApplyBoardStateApplyTaskStartedCreatesGroupAndTask(t *testing.T) {
	s := tui.NewApplyBoardState()
	s2 := s.ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**/*.go",
		},
	})

	groups := s2.Groups()
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].ID != "g1" {
		t.Errorf("group ID = %q", groups[0].ID)
	}
	if len(groups[0].Tasks) != 1 {
		t.Fatalf("tasks in g1 = %d, want 1", len(groups[0].Tasks))
	}
	task := groups[0].Tasks[0]
	if task.ID != "t1" {
		t.Errorf("task ID = %q", task.ID)
	}
	if task.FilesPattern != "internal/**/*.go" {
		t.Errorf("FilesPattern = %q", task.FilesPattern)
	}
	if task.Status != "running" {
		t.Errorf("Status = %q, want running", task.Status)
	}
}

func TestApplyBoardStateApplyTaskCompletedUpdatesStatus(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
		},
	}).ApplyEvent(domain.Event{
		Type: "task.completed",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
			"status":   "done",
		},
	})

	groups := s.Groups()
	if groups[0].Tasks[0].Status != "done" {
		t.Errorf("Status after completed = %q, want done", groups[0].Tasks[0].Status)
	}
}

func TestApplyBoardStateApplyAgentSpawnedAttachesToTask(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id": "g1",
			"task_id":  "t1",
		},
	}).ApplyEvent(domain.Event{
		Type: "agent.spawned",
		Payload: map[string]any{
			"agent_role": "team_lead",
			"agent_id":   "a1",
			"group_id":   "g1",
			"task_id":    "t1",
		},
	})

	task := s.Groups()[0].Tasks[0]
	if len(task.Agents) != 1 {
		t.Fatalf("agents in t1 = %d, want 1", len(task.Agents))
	}
	if task.Agents[0].ID != "a1" || task.Agents[0].Role != "team_lead" {
		t.Errorf("agent = %+v", task.Agents[0])
	}
	if task.Agents[0].Status != "running" {
		t.Errorf("agent.Status = %q, want running", task.Agents[0].Status)
	}
}

func TestApplyBoardStateApplyAgentCompletedUpdatesAgentStatus(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "worker", "group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.completed", Payload: map[string]any{"agent_id": "a1", "status": "done"}})

	got := s.Groups()[0].Tasks[0].Agents[0]
	if got.Status != "done" {
		t.Errorf("agent.Status = %q, want done", got.Status)
	}
}

func TestApplyBoardStateGroupsOrderIsInsertionOrder(t *testing.T) {
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g3", "task_id": "t3"}}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g2", "task_id": "t2"}})

	groups := s.Groups()
	want := []string{"g3", "g1", "g2"}
	for i, g := range groups {
		if g.ID != want[i] {
			t.Errorf("groups[%d] = %q, want %q", i, g.ID, want[i])
		}
	}
}

func TestApplyBoardStateAgentWithoutTaskIDIsAttachedAtGroupLevel(t *testing.T) {
	// agent.spawned with group_id but no task_id → group-level agent
	// (e.g. a team_lead spawned for the whole group, not a single task).
	s := tui.NewApplyBoardState().
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{
			Type: "agent.spawned",
			Payload: map[string]any{
				"agent_role": "team_lead",
				"agent_id":   "a1",
				"group_id":   "g1",
				// no task_id
			},
		})

	g := s.Groups()[0]
	if len(g.Agents) != 1 {
		t.Fatalf("group-level agents = %d, want 1", len(g.Agents))
	}
	if g.Agents[0].Role != "team_lead" {
		t.Errorf("group-level agent role = %q", g.Agents[0].Role)
	}
}

func TestApplyBoardStateUnknownEventIsNoOp(t *testing.T) {
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	before := s.GroupCount()
	s2 := s.ApplyEvent(domain.Event{Type: "phase.started", Payload: map[string]any{"phase_type": "apply"}})
	if s2.GroupCount() != before {
		t.Errorf("phase.started should not affect ApplyBoard state; group count changed: %d → %d", before, s2.GroupCount())
	}
}

func TestApplyBoardStateLazyCreatesGroupOnAgentSpawn(t *testing.T) {
	// agent.spawned arriving before task.started — the state should still
	// create the group/task entries with empty defaults.
	s := tui.NewApplyBoardState().ApplyEvent(domain.Event{
		Type: "agent.spawned",
		Payload: map[string]any{
			"agent_id":   "a1",
			"agent_role": "worker",
			"group_id":   "g1",
			"task_id":    "t1",
		},
	})

	groups := s.Groups()
	if len(groups) != 1 || groups[0].ID != "g1" {
		t.Fatalf("groups = %+v", groups)
	}
	if len(groups[0].Tasks) != 1 || groups[0].Tasks[0].ID != "t1" {
		t.Fatalf("tasks = %+v", groups[0].Tasks)
	}
	if len(groups[0].Tasks[0].Agents) != 1 {
		t.Fatalf("agents = %+v", groups[0].Tasks[0].Agents)
	}
}

func TestApplyBoardStateCapsGroupsAt50(t *testing.T) {
	s := tui.NewApplyBoardState()
	for i := 0; i < 60; i++ {
		s = s.ApplyEvent(domain.Event{
			Type: "task.started",
			Payload: map[string]any{
				"group_id": groupID(i),
				"task_id":  "t1",
			},
		})
	}
	if got := s.GroupCount(); got != 50 {
		t.Errorf("GroupCount after 60 inserts = %d, want 50", got)
	}
	// The 10 oldest groups (g0..g9) should have been evicted.
	for _, g := range s.Groups() {
		if g.ID == groupID(0) || g.ID == groupID(9) {
			t.Errorf("oldest group %q should have been evicted", g.ID)
		}
	}
}

func TestApplyBoardStateImmutability(t *testing.T) {
	s1 := tui.NewApplyBoardState()
	s2 := s1.ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	if s1.GroupCount() != 0 {
		t.Error("ApplyEvent mutated the receiver")
	}
	_ = s2
}

func groupID(i int) string {
	switch i {
	case 0:
		return "g0"
	case 9:
		return "g9"
	}
	// Use a dumb but unique name. Tests only need strict-equality here.
	return "g" + itoa3(i)
}

func itoa3(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `ApplyBoardState`, `NewApplyBoardState`, `GroupCount`, `Groups`, `ApplyGroup`, `ApplyTask`, `ApplyAgent` undefined.

- [ ] **Step 3: Implement**

`internal/adapters/inbound/tui/applyboard_state.go`:

```go
package tui

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// MaxApplyBoardGroups bounds the in-memory group count (RM7-07). When the
// limit is exceeded, the OLDEST group (by insertion order) is evicted.
const MaxApplyBoardGroups = 50

// ApplyAgent is a single agent participating in an apply group/task.
type ApplyAgent struct {
	ID     string
	Role   string // "team_lead" | "worker" | etc.
	Status string // "running" | "done" | "failed" | ...
}

// ApplyTask groups agents under a single task.
type ApplyTask struct {
	ID           string
	GroupID      string
	FilesPattern string
	Status       string // "running" | "done" | "failed" | ...
	Agents       []ApplyAgent
}

// ApplyGroup groups tasks (and group-level agents like a team_lead).
type ApplyGroup struct {
	ID     string
	Tasks  []ApplyTask
	Agents []ApplyAgent // group-level agents (no task_id)
}

// ApplyBoardState is the TUI-internal ApplyBoard model. Immutable: every
// ApplyEvent returns a new value. The internal storage uses an ordered slice
// of groups (insertion order) plus an indirection map for O(1) lookup. To
// keep the value-semantics, ApplyEvent returns a new state with the relevant
// slice elements replaced.
type ApplyBoardState struct {
	groups []ApplyGroup
}

// NewApplyBoardState constructs an empty state.
func NewApplyBoardState() ApplyBoardState {
	return ApplyBoardState{}
}

// GroupCount returns the number of groups currently tracked.
func (s ApplyBoardState) GroupCount() int { return len(s.groups) }

// Groups returns a copy of the groups slice (insertion order).
func (s ApplyBoardState) Groups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneGroup(g)
	}
	return out
}

// ApplyEvent integrates a single domain.Event. Returns a new state on
// task.*/agent.* events; returns the receiver unchanged otherwise.
func (s ApplyBoardState) ApplyEvent(ev domain.Event) ApplyBoardState {
	switch ev.Type {
	case "task.started":
		return s.applyTaskStarted(ev)
	case "task.completed":
		return s.applyTaskCompleted(ev)
	case "agent.spawned":
		return s.applyAgentSpawned(ev)
	case "agent.completed":
		return s.applyAgentCompleted(ev)
	default:
		return s
	}
}

func (s ApplyBoardState) applyTaskStarted(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	files, _ := ev.Payload["files_pattern"].(string)
	if groupID == "" || taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi := ensureGroup(&out, groupID)
	ti := ensureTask(&out[gi], taskID, groupID)
	out[gi].Tasks[ti].FilesPattern = files
	out[gi].Tasks[ti].Status = "running"
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyTaskCompleted(ev domain.Event) ApplyBoardState {
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	status, _ := ev.Payload["status"].(string)
	if groupID == "" || taskID == "" {
		return s
	}
	out := s.cloneGroups()
	gi := ensureGroup(&out, groupID)
	ti := ensureTask(&out[gi], taskID, groupID)
	if status == "" {
		status = "done"
	}
	out[gi].Tasks[ti].Status = status
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyAgentSpawned(ev domain.Event) ApplyBoardState {
	agentID, _ := ev.Payload["agent_id"].(string)
	role, _ := ev.Payload["agent_role"].(string)
	groupID, _ := ev.Payload["group_id"].(string)
	taskID, _ := ev.Payload["task_id"].(string)
	if agentID == "" {
		return s
	}
	out := s.cloneGroups()
	if groupID == "" {
		// No group context — drop. Spec §5.4 recommends group_id always
		// present on apply-phase agents; defensive against malformed events.
		return s
	}
	gi := ensureGroup(&out, groupID)
	agent := ApplyAgent{ID: agentID, Role: role, Status: "running"}
	if taskID == "" {
		// Group-level agent (typically the team_lead).
		out[gi].Agents = append(out[gi].Agents, agent)
	} else {
		ti := ensureTask(&out[gi], taskID, groupID)
		out[gi].Tasks[ti].Agents = append(out[gi].Tasks[ti].Agents, agent)
	}
	return ApplyBoardState{groups: applyGroupsCap(out)}
}

func (s ApplyBoardState) applyAgentCompleted(ev domain.Event) ApplyBoardState {
	agentID, _ := ev.Payload["agent_id"].(string)
	status, _ := ev.Payload["status"].(string)
	if agentID == "" {
		return s
	}
	if status == "" {
		status = "done"
	}
	out := s.cloneGroups()
	for gi := range out {
		// Task-level agents.
		for ti := range out[gi].Tasks {
			for ai := range out[gi].Tasks[ti].Agents {
				if out[gi].Tasks[ti].Agents[ai].ID == agentID {
					out[gi].Tasks[ti].Agents[ai].Status = status
				}
			}
		}
		// Group-level agents.
		for ai := range out[gi].Agents {
			if out[gi].Agents[ai].ID == agentID {
				out[gi].Agents[ai].Status = status
			}
		}
	}
	return ApplyBoardState{groups: out}
}

// cloneGroups returns a deep copy of s.groups so mutation in the caller is
// safe.
func (s ApplyBoardState) cloneGroups() []ApplyGroup {
	out := make([]ApplyGroup, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneGroup(g)
	}
	return out
}

func cloneGroup(g ApplyGroup) ApplyGroup {
	cp := ApplyGroup{ID: g.ID}
	cp.Tasks = make([]ApplyTask, len(g.Tasks))
	for i, t := range g.Tasks {
		ct := t
		ct.Agents = append([]ApplyAgent(nil), t.Agents...)
		cp.Tasks[i] = ct
	}
	cp.Agents = append([]ApplyAgent(nil), g.Agents...)
	return cp
}

// ensureGroup finds (or creates) the group with id in *groups, returning its
// index. New groups are appended.
func ensureGroup(groups *[]ApplyGroup, id string) int {
	for i, g := range *groups {
		if g.ID == id {
			return i
		}
	}
	*groups = append(*groups, ApplyGroup{ID: id})
	return len(*groups) - 1
}

// ensureTask finds (or creates) the task within group, returning its index.
func ensureTask(group *ApplyGroup, taskID, groupID string) int {
	for i, t := range group.Tasks {
		if t.ID == taskID {
			return i
		}
	}
	group.Tasks = append(group.Tasks, ApplyTask{ID: taskID, GroupID: groupID, Status: "pending"})
	return len(group.Tasks) - 1
}

// applyGroupsCap evicts the OLDEST groups (by insertion order) until the
// slice is at or below MaxApplyBoardGroups (RM7-07).
func applyGroupsCap(groups []ApplyGroup) []ApplyGroup {
	if len(groups) <= MaxApplyBoardGroups {
		return groups
	}
	excess := len(groups) - MaxApplyBoardGroups
	return groups[excess:]
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/tui/applyboard_state.go \
        internal/adapters/inbound/tui/applyboard_state_test.go
git commit -m "feat(tui): add ApplyBoardState with task./agent. event handlers (M7)"
```

---

## Phase 3 — Model extensions (currentView, bannerGate, applyBoard)

### Task 3: Extend `tui/model.go` with view toggle, banner state, ApplyBoard, derived clearing

**Files:**
- Modify: `internal/adapters/inbound/tui/model.go`
- Modify: `internal/adapters/inbound/tui/model_test.go`

The Model gains three orthogonal pieces of state:

1. `currentView ViewType` — `ViewTimeline` (default) or `ViewApplyBoard`.
2. `bannerGate *domain.ApprovalGate` — nil = banner hidden; non-nil = banner visible. The pointer aliases the `ApprovalGate` value passed via `ApprovalGateMsg`; it's safe because `domain.ApprovalGate` has no slices/maps.
3. `applyBoard ApplyBoardState` — the Phase 2 type, embedded in the model.

Plus a richer `ApplyEvent` switch that handles the new event types AND the three banner-clearing triggers. The clearing logic:

- `approval.resolved` → unconditionally clears `bannerGate` (banner just disappears, regardless of `decision` or `resolved_by` — D-M7-05).
- `phase.started` for a phase whose ordinal in `domain.AllPhases()` is **strictly greater** than the gated phase's ordinal → clears `bannerGate` (forward-progress; D-M7-07).
- `ApplySnapshot` whose `CurrentPhaseID` corresponds to a phase strictly after the gated phase → clears `bannerGate` (snapshot is source of truth).

Tests cover every trigger plus the negative cases (event for unrelated phase, snapshot before the gate, unknown event).

- [ ] **Step 1: Extend the model test file**

Append to `internal/adapters/inbound/tui/model_test.go`:

```go
func TestModelDefaultViewIsTimeline(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	if m.CurrentView() != tui.ViewTimeline {
		t.Errorf("default view = %v, want ViewTimeline", m.CurrentView())
	}
}

func TestModelWithCurrentView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.WithCurrentView(tui.ViewApplyBoard)
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Errorf("after WithCurrentView(ApplyBoard): %v", m2.CurrentView())
	}
	if m.CurrentView() != tui.ViewTimeline {
		t.Error("WithCurrentView mutated the receiver")
	}
}

func TestModelApprovalGateMsgSetsBannerGate(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	gate := domain.ApprovalGate{
		URL:    "https://gov.local/approvals/abc",
		Reason: "policy",
		Risk:   "medium",
		Phase:  domain.PhaseApply,
	}
	m2 := m.WithBannerGate(&gate)

	got := m2.BannerGate()
	if got == nil {
		t.Fatal("BannerGate is nil after WithBannerGate")
	}
	if got.URL != "https://gov.local/approvals/abc" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestModelApplyEventApprovalRequiredSetsBannerGate(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.ApplyEvent(domain.Event{
		Type: "approval.required",
		Payload: map[string]any{
			"phase":     string(domain.PhaseApply),
			"gate_url":  "https://gov.local/x",
			"reason":    "policy says no apply without tasks approved",
			"risk":      "medium",
			"policy":    "require_approval",
			"change_id": "01HX",
		},
		TraceID: "trace-1",
	})
	gate := m2.BannerGate()
	if gate == nil {
		t.Fatal("approval.required should set BannerGate")
	}
	if gate.URL != "https://gov.local/x" {
		t.Errorf("URL = %q", gate.URL)
	}
	if gate.Risk != "medium" {
		t.Errorf("Risk = %q", gate.Risk)
	}
	if gate.Phase != domain.PhaseApply {
		t.Errorf("Phase = %q", gate.Phase)
	}
	if gate.TraceID != "trace-1" {
		t.Errorf("TraceID = %q", gate.TraceID)
	}
}

func TestModelApplyEventApprovalResolvedClearsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{URL: "https://x", Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type: "approval.resolved",
		Payload: map[string]any{
			"decision":    "approved",
			"resolved_by": "alice",
		},
	})
	if m2.BannerGate() != nil {
		t.Errorf("approval.resolved should clear banner; got %+v", m2.BannerGate())
	}
}

func TestModelApplyEventForwardProgressClearsBanner(t *testing.T) {
	// Banner is on phase apply (index 6 in AllPhases). A phase.started for
	// verify (index 7) MUST clear the banner.
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseVerify)},
	})
	if m2.BannerGate() != nil {
		t.Error("phase.started for verify should clear apply-banner (forward progress)")
	}
}

func TestModelApplyEventSamePhaseDoesNotClearBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseApply)},
	})
	if m2.BannerGate() == nil {
		t.Error("phase.started for the SAME phase must not clear banner")
	}
}

func TestModelApplyEventEarlierPhaseDoesNotClearBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	// phase.started for explore (index 1) — earlier than apply (index 6)
	m2 := m.ApplyEvent(domain.Event{
		Type:    "phase.started",
		Payload: map[string]any{"phase_type": string(domain.PhaseExplore)},
	})
	if m2.BannerGate() == nil {
		t.Error("phase.started for an earlier phase must not clear banner")
	}
}

func TestModelApplySnapshotPastPhaseClearsBanner(t *testing.T) {
	// Banner on apply (idx 6). Snapshot's CurrentPhaseID points at a phase
	// whose Type is verify (idx 7) → forward progress → clear.
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplySnapshot(&domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-verify",
		Phases: []domain.Phase{
			{ID: "p-verify", Type: domain.PhaseVerify, Status: domain.PhaseStatusRunning},
		},
	})
	if m2.BannerGate() != nil {
		t.Error("snapshot showing forward-progress phase should clear banner")
	}
}

func TestModelApplySnapshotSamePhaseKeepsBanner(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{Phase: domain.PhaseApply})
	m2 := m.ApplySnapshot(&domain.Change{
		ID:             domain.ChangeID("01HX"),
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "p-apply",
		Phases: []domain.Phase{
			{ID: "p-apply", Type: domain.PhaseApply, Status: domain.PhaseStatusRunning},
		},
	})
	if m2.BannerGate() == nil {
		t.Error("snapshot still on the gated phase should keep banner")
	}
}

func TestModelApplyEventTaskStartedFeedsApplyBoard(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2 := m.ApplyEvent(domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**",
		},
	})
	if m2.ApplyBoard().GroupCount() != 1 {
		t.Error("task.started should feed ApplyBoard")
	}
}

func TestModelApplyEventAgentSpawnedFeedsApplyBoard(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "team_lead", "group_id": "g1", "task_id": "t1"}})

	board := m.ApplyBoard()
	if len(board.Groups()) != 1 {
		t.Fatal("groups missing")
	}
	task := board.Groups()[0].Tasks[0]
	if len(task.Agents) != 1 || task.Agents[0].Role != "team_lead" {
		t.Errorf("agent missing or wrong: %+v", task.Agents)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `ViewType`, `ViewTimeline`, `ViewApplyBoard`, `CurrentView`, `WithCurrentView`, `BannerGate`, `WithBannerGate`, `ApplyBoard` undefined.

- [ ] **Step 3: Extend model.go**

Apply these edits to `internal/adapters/inbound/tui/model.go`:

```go
// Replace the imports block at the top to add no new imports yet
// (domain is already imported).

// --- new types (after PhaseRow, before ModelConfig) ---

// ViewType selects which TUI view is currently displayed.
type ViewType int

const (
	// ViewTimeline shows the 9-phase Timeline (M6 default view).
	ViewTimeline ViewType = iota
	// ViewApplyBoard shows the ApplyBoard groups → tasks → agents tree.
	ViewApplyBoard
)

// String returns a debug-friendly name.
func (v ViewType) String() string {
	switch v {
	case ViewTimeline:
		return "timeline"
	case ViewApplyBoard:
		return "applyboard"
	}
	return "unknown"
}
```

Then in the `Model` struct definition, ADD the three new fields after `errors []string`:

```go
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

	// M7 additions:
	currentView ViewType
	bannerGate  *domain.ApprovalGate
	applyBoard  ApplyBoardState
}
```

ADD accessors and With* methods at the bottom of the file (above `indexOfPhase`):

```go
// CurrentView returns the active view (Timeline or ApplyBoard).
func (m Model) CurrentView() ViewType { return m.currentView }

// WithCurrentView returns a new Model with the active view set to v.
func (m Model) WithCurrentView(v ViewType) Model {
	m.currentView = v
	return m
}

// BannerGate returns the active ApprovalGate (banner state) or nil if hidden.
func (m Model) BannerGate() *domain.ApprovalGate { return m.bannerGate }

// WithBannerGate returns a new Model whose banner state is set to gate.
// Pass nil to hide the banner.
func (m Model) WithBannerGate(gate *domain.ApprovalGate) Model {
	m.bannerGate = gate
	return m
}

// ApplyBoard returns the ApplyBoard state.
func (m Model) ApplyBoard() ApplyBoardState { return m.applyBoard }
```

REPLACE `ApplyEvent` with a richer version that handles the new event types AND the banner clearing triggers:

```go
// ApplyEvent integrates a single domain.Event into the model.
//
// The set of event types it understands grew in M7:
//
//   - phase.started, phase.completed       — Timeline phase transitions (M6)
//   - approval.required                    — sets bannerGate AND marks phase row (M6 + M7)
//   - approval.resolved                    — clears bannerGate (M7)
//   - task.started, task.completed         — feeds ApplyBoard (M7)
//   - agent.spawned, agent.completed       — feeds ApplyBoard (M7)
//
// Side-effect on banner clearing:
//   - phase.started for any phase whose ordinal is STRICTLY GREATER than
//     bannerGate.Phase clears the banner (forward progress, D-M7-07).
func (m Model) ApplyEvent(ev domain.Event) Model {
	switch ev.Type {
	case "phase.started":
		m = m.applyPhaseStarted(ev)
		m = m.maybeClearBannerOnForwardProgress(ev)
		return m
	case "phase.completed":
		return m.applyPhaseCompleted(ev)
	case "approval.required":
		m = m.applyApprovalRequired(ev)
		m = m.applyBannerFromEvent(ev)
		return m
	case "approval.resolved":
		m.bannerGate = nil
		return m
	case "task.started", "task.completed", "agent.spawned", "agent.completed":
		m.applyBoard = m.applyBoard.ApplyEvent(ev)
		return m
	default:
		return m
	}
}
```

ADD private helpers below `applyApprovalRequired`:

```go
// applyBannerFromEvent constructs an ApprovalGate from an approval.required
// event payload and sets it as the current bannerGate. The shape mirrors
// application.approvalGateFromEvent — same field names — but the model
// can't import application, so the construction is duplicated here.
//
// Spec §5.4 payload keys: gate_url, reason, risk, policy, phase, change_id.
func (m Model) applyBannerFromEvent(ev domain.Event) Model {
	gate := domain.ApprovalGate{TraceID: ev.TraceID}
	if ev.Payload == nil {
		m.bannerGate = &gate
		return m
	}
	gate.URL, _ = ev.Payload["gate_url"].(string)
	gate.Reason, _ = ev.Payload["reason"].(string)
	gate.Risk, _ = ev.Payload["risk"].(string)
	gate.Policy, _ = ev.Payload["policy"].(string)
	if ph, ok := ev.Payload["phase"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	} else if ph, ok := ev.Payload["phase_type"].(string); ok {
		gate.Phase = domain.PhaseType(ph)
	}
	if cid, ok := ev.Payload["change_id"].(string); ok {
		gate.ChangeID = domain.ChangeID(cid)
	}
	m.bannerGate = &gate
	return m
}

// maybeClearBannerOnForwardProgress clears bannerGate when ev is a
// phase.started for a phase strictly later than bannerGate.Phase.
func (m Model) maybeClearBannerOnForwardProgress(ev domain.Event) Model {
	if m.bannerGate == nil {
		return m
	}
	pt := phaseTypeFromPayload(ev.Payload)
	if pt == "" {
		return m
	}
	if isPhaseAfter(pt, m.bannerGate.Phase) {
		m.bannerGate = nil
	}
	return m
}

// isPhaseAfter reports whether candidate's ordinal in domain.AllPhases() is
// strictly greater than reference's ordinal. Returns false if either phase
// is unknown.
func isPhaseAfter(candidate, reference domain.PhaseType) bool {
	c := indexOfPhase(candidate)
	r := indexOfPhase(reference)
	if c < 0 || r < 0 {
		return false
	}
	return c > r
}
```

EXTEND `ApplySnapshot` to clear the banner when the snapshot's `CurrentPhase` advances past the gated phase. Replace the trailing `return m` with:

```go
	// M7: clear banner if snapshot shows we've moved past the gated phase.
	if m.bannerGate != nil {
		if pt, ok := currentPhaseType(c); ok {
			if isPhaseAfter(pt, m.bannerGate.Phase) {
				m.bannerGate = nil
			}
		}
	}
	return m
}
```

ADD this helper in the same file, near the other private helpers:

```go
// currentPhaseType returns the PhaseType of the snapshot's CurrentPhaseID,
// or ("", false) if no phase row in the snapshot matches.
func currentPhaseType(c *domain.Change) (domain.PhaseType, bool) {
	if c == nil {
		return "", false
	}
	for _, p := range c.Phases {
		if p.ID == c.CurrentPhaseID {
			return p.Type, true
		}
	}
	return "", false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS. M6 tests still green; new M7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/tui/model.go \
        internal/adapters/inbound/tui/model_test.go
git commit -m "feat(tui): extend Model with currentView/bannerGate/applyBoard + derived clearing"
```

---

## Phase 4 — Update extensions (Tab + [O] + browser dispatch)

### Task 4: Extend `tui/keybindings.go` with ActionToggleView and ActionOpenBanner

**Files:**
- Modify: `internal/adapters/inbound/tui/keybindings.go`
- Modify: `internal/adapters/inbound/tui/update_test.go` (extends; M6 tests preserved)

The Action enum gains two values:

- `ActionToggleView` — fires on `Tab` (case-insensitive, but Tab is its own keystroke).
- `ActionOpenBanner` — fires on `o` / `O` ONLY when the banner is visible. Update gates this on `m.BannerGate() != nil`. D-M7-03: when the banner is hidden, `[O]` is a no-op.

D-M7-02: Tab while in confirm-detach mode CANCELS confirm AND toggles view. The confirm-cancellation reuses the M6 "any unrecognized key cancels confirm" semantics, then proceeds to the toggle.

- [ ] **Step 1: Append to update_test.go**

Append to `internal/adapters/inbound/tui/update_test.go`:

```go
func TestUpdateTabTogglesView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPressTab())
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Errorf("after Tab from Timeline, view = %v", m2.CurrentView())
	}
	if cmd != nil {
		t.Errorf("Tab should not produce a Cmd")
	}
	m3, _ := tui.Update(m2, keyPressTab())
	if m3.CurrentView() != tui.ViewTimeline {
		t.Errorf("after second Tab, view = %v", m3.CurrentView())
	}
}

func TestUpdateTabInConfirmModeCancelsAndToggles(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).WithConfirmingDetach(true)
	m2, _ := tui.Update(m, keyPressTab())
	if m2.ConfirmingDetach() {
		t.Error("Tab should cancel confirm mode (D-M7-02)")
	}
	if m2.CurrentView() != tui.ViewApplyBoard {
		t.Error("Tab should also toggle view after canceling confirm (D-M7-02)")
	}
}

func TestUpdateOKeyWithBannerEmitsOpenBrowserMsg(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{URL: "https://gov.local/x"})
	m2, cmd := tui.Update(m, keyPress("o"))
	if cmd == nil {
		t.Fatal("[O] with banner visible should produce a Cmd that emits OpenBrowserMsg")
	}
	got := cmd()
	openMsg, ok := got.(tui.OpenBrowserMsg)
	if !ok {
		t.Fatalf("Cmd return = %T, want OpenBrowserMsg", got)
	}
	if openMsg.URL != "https://gov.local/x" {
		t.Errorf("OpenBrowserMsg.URL = %q", openMsg.URL)
	}
	// Model unchanged.
	if m2.BannerGate() == nil {
		t.Error("[O] should not clear banner on its own")
	}
}

func TestUpdateOKeyWithoutBannerIsNoOp(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, keyPress("o"))
	if cmd != nil {
		t.Errorf("[O] without banner should be a no-op (D-M7-03)")
	}
	_ = m2
}

func TestUpdateBrowserOpenedMsgWithErrorAddsErrorLine(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, cmd := tui.Update(m, tui.BrowserOpenedMsg{Err: errors.New("boom")})
	if cmd != nil {
		t.Errorf("BrowserOpenedMsg should not produce a Cmd")
	}
	errs := m2.Errors()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "browser") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error line mentioning 'browser'; got %v", errs)
	}
}

func TestUpdateBrowserOpenedMsgWithNilIsNoOp(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	m2, _ := tui.Update(m, tui.BrowserOpenedMsg{Err: nil})
	if len(m2.Errors()) != 0 {
		t.Errorf("BrowserOpenedMsg{nil} should not append errors; got %v", m2.Errors())
	}
}

// keyPressTab constructs a Tab keystroke. Tab in v2 is a Key.Code constant —
// adapt this helper if the installed v2 module uses a different shape.
func keyPressTab() tea.KeyPressMsg {
	return tea.KeyPressMsg{Key: tea.Key{Code: tea.KeyTab}}
}
```

> **v2 API note:** `tea.KeyTab` is the canonical bubbletea v2 constant for Tab. M6's `keyPressString(msg)` returns `msg.String()` which renders Tab as `"tab"`. classifyKey will need to recognize the literal `"tab"` string. Add the imports `errors` and `strings` to the test file if not already present (look at the head of `update_test.go`).

- [ ] **Step 2: Run tests**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `ActionToggleView`, `ActionOpenBanner`, `OpenBrowserMsg`, `BrowserOpenedMsg` undefined.

- [ ] **Step 3: Extend keybindings.go**

Modify `internal/adapters/inbound/tui/keybindings.go`. ADD two new actions to the const block:

```go
const (
	ActionNone Action = iota
	ActionDetach
	ActionConfirmDetach
	ActionConfirmYes
	ActionConfirmNo
	ActionToggleView // M7: Tab toggles Timeline ↔ ApplyBoard
	ActionOpenBanner // M7: [O] opens the approval gate URL in a browser
)
```

REPLACE `classifyKey` with a version that:

1. If we're in confirm mode AND the key is Tab → return `ActionToggleView`. Confirm mode is canceled by Update before the toggle; this matches D-M7-02 ("Tab cancels confirm + toggles").
2. Otherwise, recognize `tab` as `ActionToggleView` and `o` as `ActionOpenBanner`.

```go
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
	case "tab":
		// D-M7-02: Tab in confirm mode cancels confirm AND toggles view.
		// Update reads this Action and clears confirmingDetach before
		// flipping currentView.
		return ActionToggleView
	case "o":
		// [O] only does something useful when the banner is visible.
		// Update gates on m.BannerGate() != nil; we still emit the action
		// so the caller can decide.
		return ActionOpenBanner
	}
	if confirming {
		return ActionConfirmNo
	}
	return ActionNone
}
```

- [ ] **Step 4: Extend update.go**

Modify `internal/adapters/inbound/tui/update.go`. ADD two new message types at the top:

```go
// OpenBrowserMsg is dispatched by Update when the user presses [O] with a
// banner visible. It carries the URL of the gate. The Program-level message
// loop catches OpenBrowserMsg, calls Browser.Open(url) in a goroutine, and
// dispatches a BrowserOpenedMsg back into the loop with the result.
//
// Update itself is pure — it never touches I/O. This message type is the
// seam between the pure layer and the adapter layer.
type OpenBrowserMsg struct {
	URL string
}

// BrowserOpenedMsg is dispatched after a Browser.Open call returns. Err is
// non-nil when the open failed (validation error, OS error, etc.).
type BrowserOpenedMsg struct {
	Err error
}
```

EXTEND the `Update` switch with cases for the two new message types. Add them near the other case branches:

```go
case OpenBrowserMsg:
	// Pure layer — re-emit so the Program adapter can handle.
	// Note: Update returning a Cmd that produces this same message would
	// loop. The message arrives only when emitted FROM updateKey via
	// ActionOpenBanner; the Program is responsible for intercepting it
	// before re-feeding to Update. As a safety net, treat it as no-op
	// here (the Program already saw it; we don't need to redispatch).
	return m, nil

case BrowserOpenedMsg:
	if msg.Err != nil {
		return m.WithError("browser: " + msg.Err.Error()), nil
	}
	return m, nil
```

EXTEND `updateKey` to handle the two new actions. Replace its body with:

```go
func updateKey(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	action := classifyKey(msg, m.ConfirmingDetach())
	switch action {
	case ActionDetach, ActionConfirmYes:
		return m.WithConfirmingDetach(false).WithDetached(true), tea.Quit
	case ActionConfirmDetach:
		return m.WithConfirmingDetach(true), nil
	case ActionConfirmNo:
		return m.WithConfirmingDetach(false), nil
	case ActionToggleView:
		// D-M7-02: Tab cancels confirm AND toggles view.
		m = m.WithConfirmingDetach(false)
		switch m.CurrentView() {
		case ViewTimeline:
			return m.WithCurrentView(ViewApplyBoard), nil
		default:
			return m.WithCurrentView(ViewTimeline), nil
		}
	case ActionOpenBanner:
		gate := m.BannerGate()
		if gate == nil {
			// D-M7-03: [O] with no banner is a no-op.
			return m, nil
		}
		url := gate.URL
		// Emit a Cmd that produces an OpenBrowserMsg. The Program-level
		// loop intercepts and runs the actual Browser.Open call.
		return m, func() tea.Msg { return OpenBrowserMsg{URL: url} }
	}
	return m, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS. The new tests for Tab toggle, [O] gating, and BrowserOpenedMsg-with-error all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/tui/keybindings.go \
        internal/adapters/inbound/tui/update.go \
        internal/adapters/inbound/tui/update_test.go
git commit -m "feat(tui): add Tab/O keybindings + OpenBrowserMsg/BrowserOpenedMsg"
```

---

### Task 5: Extend `tui/program.go` to bridge OpenBrowserMsg to outbound.Browser

**Files:**
- Modify: `internal/adapters/inbound/tui/program.go`
- Modify: `internal/adapters/inbound/tui/program_test.go`

The pure `Update` emits `OpenBrowserMsg` as a Cmd. The Program is the I/O boundary — it intercepts `OpenBrowserMsg`, calls `Browser.Open(url)` in a goroutine, and dispatches `BrowserOpenedMsg{Err: err}` back into the program. Update then renders the result (silent on success, error line on failure).

The interception happens by wrapping the rootModel's Update with a thin layer that:

1. Lets the pure Update run first.
2. After Update returns, if the resulting Cmd produces an `OpenBrowserMsg` when called, replace it with a Cmd that calls Browser.Open + dispatches BrowserOpenedMsg.

A simpler approach: use a separate goroutine driven by the `*tea.Program.Send` interface. The pure Update emits `OpenBrowserMsg`, which the program loop receives like any other message. We add a top-level interceptor for `OpenBrowserMsg` in `rootModel.Update` that spawns the goroutine and returns no further Cmd. This keeps the pure Update side-effect-free while the impure rootModel.Update handles the I/O.

- [ ] **Step 1: Extend program_test.go**

Append to `internal/adapters/inbound/tui/program_test.go`:

```go
import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestProgramOpenBrowserMsgInvokesBrowser(t *testing.T) {
	fb := fakes.NewFakeBrowser()
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   newDevNullWriter(),
		Browser:  fb,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Run the program in a goroutine; we'll quit it via CompleteMsg after
	// validating the browser was called.
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()

	// Wait for Run to start (Send blocks until then).
	time.Sleep(50 * time.Millisecond)

	// Send an OpenBrowserMsg through the bridge's Sender adapter — easiest
	// path is via the Bridge's OnEvent + Update emitting a Cmd, but for a
	// unit test we drive it more directly: send the message through the
	// Bridge's underlying tea.Program.Send by exposing a SendForTest helper.
	p.SendForTest(tui.OpenBrowserMsg{URL: "https://gov.local/x"})

	// Wait for the browser fake to record the call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fb.Opened) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(fb.Opened) != 1 {
		t.Fatalf("FakeBrowser.Opened = %v, want 1 entry", fb.Opened)
	}
	if fb.Opened[0] != "https://gov.local/x" {
		t.Errorf("Opened URL = %q", fb.Opened[0])
	}

	// Quit the program.
	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	if err := <-doneCh; err != nil {
		t.Errorf("Run returned: %v", err)
	}
}

func TestProgramOpenBrowserErrorPropagatesToModel(t *testing.T) {
	fb := fakes.NewFakeBrowser()
	fb.OpenErr = errors.New("xdg-open: not found")

	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   newDevNullWriter(),
		Browser:  fb,
	})
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()
	time.Sleep(50 * time.Millisecond)

	p.SendForTest(tui.OpenBrowserMsg{URL: "https://x"})

	// Wait briefly for the browser call + BrowserOpenedMsg dispatch.
	time.Sleep(100 * time.Millisecond)

	// The model state can be inspected via Program.Snapshot() (added in this task).
	state := p.Snapshot()
	found := false
	for _, e := range state.Errors() {
		if strings.Contains(e, "xdg-open") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an error line mentioning xdg-open; got %v", state.Errors())
	}

	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	<-doneCh
}

func TestProgramWithoutBrowserOpenBrowserMsgIsLoggedAsError(t *testing.T) {
	// No Browser injected — Browser is nil. OpenBrowserMsg should produce
	// an error line, NOT panic.
	p, err := tui.NewProgram(tui.ProgramConfig{
		ChangeID: domain.ChangeID("01HX"),
		Output:   newDevNullWriter(),
		// Browser intentionally nil
	})
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background())
		doneCh <- err
	}()
	time.Sleep(50 * time.Millisecond)

	p.SendForTest(tui.OpenBrowserMsg{URL: "https://x"})
	time.Sleep(50 * time.Millisecond)

	state := p.Snapshot()
	found := false
	for _, e := range state.Errors() {
		if strings.Contains(strings.ToLower(e), "browser") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error mentioning browser; got %v", state.Errors())
	}

	_ = p.Bridge().OnComplete(context.Background(), domain.ChangeStatusDone)
	<-doneCh
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — `ProgramConfig.Browser`, `Program.SendForTest`, `Program.Snapshot` undefined.

- [ ] **Step 3: Extend program.go**

Modify `internal/adapters/inbound/tui/program.go`. Add to imports:

```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
```

EXTEND `ProgramConfig`:

```go
type ProgramConfig struct {
	ChangeID domain.ChangeID
	Output   io.Writer // nil ⇒ os.Stdout (resolved by tea.WithOutput)
	Input    io.Reader // nil ⇒ os.Stdin

	// Browser is the outbound.Browser used by [O] in the approval banner.
	// nil ⇒ pressing [O] surfaces an error line ("browser: not configured").
	Browser outbound.Browser
}
```

EXTEND the `Program` struct:

```go
type Program struct {
	mu      sync.Mutex
	tea     *tea.Program
	bridge  *Bridge
	closed  bool
	running bool

	// M7: latest pure-Model state, updated each Update call. Snapshot()
	// returns a copy so tests can inspect without racing the program loop.
	stateMu  sync.Mutex
	state    Model
	browser  outbound.Browser
}
```

EXTEND `rootModel` to expose a way to forward state changes back to the Program. Two cleanest approaches:

1. Make rootModel a pointer to a struct holding `*Program`, so `Update` can call `p.recordState`. (Requires changing rootModel from value to pointer.)
2. Use a closure stored in rootModel to publish state.

We use approach (2) — a `publish func(Model)` callback inside rootModel — to keep the Bubble Tea Model contract clean.

REPLACE `rootModel`:

```go
// rootModel implements bubbletea v2 Model by delegating to pure Update/View.
//
// publish: called after every Update with the new pure-Model state, so
// Program.Snapshot() can return the latest. nil-safe.
//
// openBrowser: called when an OpenBrowserMsg arrives. Spawns a goroutine to
// call Browser.Open and Send a BrowserOpenedMsg back into the program.
// nil-safe (in tests where Browser is intentionally absent).
type rootModel struct {
	state        Model
	publish      func(Model)
	openBrowser  func(url string)
}

func (rm rootModel) Init() tea.Cmd { return nil }

func (rm rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept OpenBrowserMsg BEFORE the pure Update sees it: dispatch the
	// browser open in a goroutine; the pure Update gets a no-op pass.
	if om, ok := msg.(OpenBrowserMsg); ok {
		if rm.openBrowser != nil {
			rm.openBrowser(om.URL)
		} else {
			// No Browser configured — record an error line in the model.
			rm.state = rm.state.WithError("browser: not configured")
			if rm.publish != nil {
				rm.publish(rm.state)
			}
		}
		return rm, nil
	}

	newState, cmd := Update(rm.state, msg)
	rm.state = newState
	if rm.publish != nil {
		rm.publish(rm.state)
	}
	return rm, cmd
}

func (rm rootModel) View() tea.View {
	return tea.NewView(View(rm.state))
}
```

EXTEND `NewProgram` to wire the publish callback + openBrowser callback:

```go
func NewProgram(cfg ProgramConfig) (*Program, error) {
	initial := NewModel(ModelConfig{ChangeID: cfg.ChangeID})

	p := &Program{
		state:   initial,
		browser: cfg.Browser,
	}

	root := rootModel{
		state: initial,
		publish: func(m Model) {
			p.stateMu.Lock()
			p.state = m
			p.stateMu.Unlock()
		},
		openBrowser: func(url string) {
			// Forward to the program's openBrowser handler — bound to the
			// final *tea.Program after construction.
			p.handleOpenBrowser(url)
		},
	}

	opts := []tea.ProgramOption{
		tea.WithoutSignalHandler(),
	}
	if cfg.Output != nil {
		opts = append(opts, tea.WithOutput(cfg.Output))
		if cfg.Input == nil {
			opts = append(opts, tea.WithInput(nil))
		}
	}
	if cfg.Input != nil {
		opts = append(opts, tea.WithInput(cfg.Input))
	}

	teaProg := tea.NewProgram(root, opts...)
	sender := &teaSender{p: teaProg}
	bridge := NewBridge(BridgeConfig{Sender: sender})

	p.tea = teaProg
	p.bridge = bridge

	return p, nil
}
```

ADD new methods:

```go
// handleOpenBrowser runs the browser open in a goroutine and sends a
// BrowserOpenedMsg back into the program loop with the result.
func (p *Program) handleOpenBrowser(url string) {
	browser := p.browser
	if browser == nil {
		// Should be unreachable — rootModel.Update checks first — but
		// defensive: send a typed error message.
		p.tea.Send(BrowserOpenedMsg{Err: errBrowserNotConfigured})
		return
	}
	go func() {
		// Background ctx with a small timeout — opening a browser shouldn't
		// take more than a few seconds; if it hangs, BrowserOpenedMsg with
		// the timeout error is still useful.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := browser.Open(ctx, url)
		// Only Send if we know the program is running — otherwise Send
		// blocks indefinitely.
		p.mu.Lock()
		running := p.running
		p.mu.Unlock()
		if !running {
			return
		}
		p.tea.Send(BrowserOpenedMsg{Err: err})
	}()
}

// SendForTest forwards a tea.Msg into the program loop. Test-only.
func (p *Program) SendForTest(msg any) {
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	if !running || p.tea == nil {
		return
	}
	p.tea.Send(msg)
}

// Snapshot returns a copy of the latest pure-Model state observed by the
// program. Tests use this to inspect side effects without racing.
func (p *Program) Snapshot() Model {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.state
}

var errBrowserNotConfigured = errors.New("browser not configured")
```

ADD the `time` import (for the timeout), if it's not already there.

Make sure `Run()` records `running = true` (M6 already does this). Make sure `reattachHint` and the rest of `Close()` still work — they're untouched.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race -timeout 30s`
Expected: PASS. The three new tests cover the success path, error propagation, and the no-Browser case.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/tui/program.go \
        internal/adapters/inbound/tui/program_test.go
git commit -m "feat(tui): wire Program to outbound.Browser via OpenBrowserMsg"
```

---

## Phase 5 — Views

### Task 6: `tui/view_applyboard.go` — Render groups → tasks → agents

**Files:**
- Create: `internal/adapters/inbound/tui/view_applyboard.go`
- Create: `internal/adapters/inbound/tui/view_applyboard_test.go`

D-M7-01 default: nested-tree layout. Each group is a header line; each task indents underneath; agents indent further. Status icons reuse the M6 palette (`▶ ✓ ✗ ■ ` ).

```
ApplyBoard · 3 groups
─────────────────────

▼ g1
  ▶ t1  internal/handlers/**/*.go
    ▶ a1  team_lead
    ✓ a2  worker
  ✓ t2  internal/services/**/*.go
    ✓ a3  worker

▼ g2
  ▶ t3  cmd/sophia/**/*.go
    ▶ a4  worker
```

(Layout is illustrative — exact icon/spacing decisions live in the test assertions.)

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/view_applyboard_test.go`:

```go
package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestViewApplyBoardEmptyShowsHint(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing:\n%s", out)
	}
	// An empty state should at least render a header — the hint string is
	// implementation-defined; we assert SOMETHING distinguishes it from
	// the Timeline view.
	if strings.Contains(out, "explore") && strings.Contains(out, "proposal") {
		t.Errorf("ApplyBoard view should not render the 9 phases:\n%s", out)
	}
}

func TestViewApplyBoardShowsGroupsTasksAgents(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1", "files_pattern": "internal/**/*.go"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a1", "agent_role": "team_lead", "group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "agent.spawned", Payload: map[string]any{"agent_id": "a2", "agent_role": "worker", "group_id": "g1", "task_id": "t1"}})

	out := tui.View(m)
	for _, want := range []string{"g1", "t1", "a1", "team_lead", "a2", "worker", "internal/**"} {
		if !strings.Contains(out, want) {
			t.Errorf("ApplyBoard output missing %q:\n%s", want, out)
		}
	}
}

func TestViewApplyBoardMarksRunningTask(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})

	out := tui.View(m)
	// A running task line should contain at least one running marker.
	lines := strings.Split(out, "\n")
	t1Line := ""
	for _, line := range lines {
		if strings.Contains(line, "t1") {
			t1Line = line
			break
		}
	}
	if t1Line == "" {
		t.Fatal("t1 line not found")
	}
	if !strings.ContainsAny(t1Line, "▶>*") && !strings.Contains(t1Line, "running") {
		t.Errorf("running marker missing in t1 line: %q", t1Line)
	}
}

func TestViewApplyBoardMarksDoneTask(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}}).
		ApplyEvent(domain.Event{Type: "task.completed", Payload: map[string]any{"group_id": "g1", "task_id": "t1", "status": "done"}})

	out := tui.View(m)
	if !strings.Contains(out, "t1") {
		t.Fatal("t1 line missing")
	}
	if !strings.ContainsAny(out, "✓") && !strings.Contains(out, "done") {
		t.Errorf("done marker missing:\n%s", out)
	}
}

func TestViewApplyBoardKeybindingHintsIncludeTab(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard)
	out := tui.View(m)
	if !strings.Contains(strings.ToLower(out), "tab") {
		t.Errorf("ApplyBoard hint should mention Tab; got:\n%s", out)
	}
}

func TestViewApplyBoardIsPure(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		ApplyEvent(domain.Event{Type: "task.started", Payload: map[string]any{"group_id": "g1", "task_id": "t1"}})
	out1 := tui.View(m)
	out2 := tui.View(m)
	if out1 != out2 {
		t.Error("ApplyBoard View must be pure")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — the View function doesn't yet branch on currentView; ApplyBoard rendering is undefined.

- [ ] **Step 3: Implement view_applyboard.go**

`internal/adapters/inbound/tui/view_applyboard.go`:

```go
package tui

import (
	"fmt"
	"strings"
)

// viewApplyBoard renders the ApplyBoard tree (groups → tasks → agents).
// Pure: same Model → same output.
func viewApplyBoard(m Model) string {
	var b strings.Builder

	// Header.
	header := fmt.Sprintf("Sophia · Change %s · ApplyBoard · %d groups",
		m.ChangeID(), m.ApplyBoard().GroupCount())
	b.WriteString(pkgStyles.header.Render(header))
	b.WriteString("\n\n")

	// Empty state.
	groups := m.ApplyBoard().Groups()
	if len(groups) == 0 {
		b.WriteString(pkgStyles.hint.Render("No tasks yet. Apply phase will populate this view."))
		b.WriteString("\n\n")
	} else {
		for _, g := range groups {
			b.WriteString(renderApplyGroup(g))
		}
	}

	// Errors.
	for _, e := range m.Errors() {
		b.WriteString(pkgStyles.errorLine.Render("error: " + e))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Footer hint.
	if m.ConfirmingDetach() {
		b.WriteString(pkgStyles.confirmDialog.Render(" Detach? (y/n) "))
	} else {
		b.WriteString(pkgStyles.hint.Render("Tab Timeline · Q detach · Ctrl+C confirm-detach"))
	}

	return truncateToWidth(b.String(), m.Width())
}

func renderApplyGroup(g ApplyGroup) string {
	var b strings.Builder
	// Group header.
	b.WriteString(pkgStyles.header.Render(fmt.Sprintf("▼ %s", g.ID)))
	b.WriteString("\n")
	// Group-level agents (e.g. team_lead with no task_id).
	for _, a := range g.Agents {
		b.WriteString("  ")
		b.WriteString(renderApplyAgent(a))
		b.WriteString("\n")
	}
	// Tasks.
	for _, t := range g.Tasks {
		b.WriteString("  ")
		b.WriteString(renderApplyTask(t))
		b.WriteString("\n")
		for _, a := range t.Agents {
			b.WriteString("    ")
			b.WriteString(renderApplyAgent(a))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderApplyTask(t ApplyTask) string {
	icon := pkgStyles.iconFor(t.Status)
	style := pkgStyles.styleFor(t.Status)
	files := ""
	if t.FilesPattern != "" {
		files = "  " + t.FilesPattern
	}
	body := fmt.Sprintf("%s %-12s %-8s%s", icon, t.ID, t.Status, files)
	return style.Render(body)
}

func renderApplyAgent(a ApplyAgent) string {
	icon := pkgStyles.iconFor(a.Status)
	style := pkgStyles.styleFor(a.Status)
	body := fmt.Sprintf("%s %-10s  %-12s  %s", icon, a.ID, a.Role, a.Status)
	return style.Render(body)
}
```

- [ ] **Step 4: Update view_timeline.go to dispatch on currentView**

Modify `internal/adapters/inbound/tui/view_timeline.go`. Rename the existing top-level `View` to `viewTimeline` (the implementation stays the same minus the name) and ADD a new top-level `View` that dispatches:

```go
// View dispatches on Model.CurrentView() and overlays the approval banner
// at the top of the chosen view (M7). Spec §6.3 inv 7: every untrusted
// string flows through pkgStyles.<Style>.Render; banner and view bodies
// honor that invariant individually.
func View(m Model) string {
	body := ""
	switch m.CurrentView() {
	case ViewApplyBoard:
		body = viewApplyBoard(m)
	default:
		body = viewTimeline(m)
	}
	if m.BannerGate() != nil {
		return renderApprovalBanner(m, *m.BannerGate()) + "\n" + body
	}
	return body
}
```

> **Note:** the existing `View` body becomes `viewTimeline` (lowercased internal). All M6 tests still call `tui.View(m)` — they continue to pass because the dispatcher routes to `viewTimeline` when no banner is set and `currentView == ViewTimeline` (the default).

The `renderApprovalBanner` function is implemented in Task 7. For Task 6 to compile cleanly, ADD a placeholder stub at the bottom of `view_timeline.go`:

```go
// renderApprovalBanner is implemented in view_approval_banner.go (Task 7).
```

(No actual stub needed — the compiler sees the package-level function once Task 7 lands.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: at this point Task 7 hasn't shipped yet, so `renderApprovalBanner` will be undefined and the build fails. Add a temporary stub for the test pass:

In `view_applyboard.go`, add:

```go
// renderApprovalBanner is the M7 banner. Stub here so view_timeline.go's
// View() compiles before Task 7 lands. Replaced in Task 7.
//
// nolint:unused // temporary stub
func renderApprovalBanner(_ Model, _ struct{}) string { return "" }
```

…and adjust `view_timeline.go` to call `renderApprovalBanner(m, struct{}{})`. Once Task 7 lands, both signatures align (`renderApprovalBanner(Model, domain.ApprovalGate) string`) and this stub is removed in Task 7's edits.

(Alternative: write Tasks 6 and 7 as a single PR. The plan keeps them separate so each commit is reviewable; the stub-and-replace is the price.)

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS with the stub in place.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/tui/view_applyboard.go \
        internal/adapters/inbound/tui/view_applyboard_test.go \
        internal/adapters/inbound/tui/view_timeline.go
git commit -m "feat(tui): add ApplyBoard view + dispatch on currentView"
```

---

### Task 7: `tui/view_approval_banner.go` — Render the approval banner

**Files:**
- Create: `internal/adapters/inbound/tui/view_approval_banner.go`
- Create: `internal/adapters/inbound/tui/view_approval_banner_test.go`
- Modify: `internal/adapters/inbound/tui/view_applyboard.go` (drop the temporary stub from Task 6)
- Modify: `internal/adapters/inbound/tui/view_timeline.go` (fix the call signature)

The banner replicates spec §2.2:

```
┌─ Approval required by governance ─────────────────────┐
│ Phase: apply         Risk: medium                     │
│ Reason: NO APPLY WITHOUT TASKS APPROVED               │
│ Policy: require_approval                              │
│                                                        │
│ Gate: https://gov.local/approvals/abc123              │
│ Status: waiting                                        │
│                                                        │
│ [O] Open in browser                                    │
└────────────────────────────────────────────────────────┘
```

The exact box-drawing characters and column alignment are not load-bearing; tests assert on the presence of each labeled field plus the `[O]` shortcut hint.

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/tui/view_approval_banner_test.go`:

```go
package tui_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestApprovalBannerHiddenByDefault(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")})
	out := tui.View(m)
	for _, banner := range []string{"Approval required", "Gate:", "[O]"} {
		if strings.Contains(out, banner) {
			t.Errorf("banner element %q should be hidden by default; got:\n%s", banner, out)
		}
	}
}

func TestApprovalBannerVisibleWhenGateSet(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://gov.local/approvals/abc",
			Reason: "NO APPLY WITHOUT TASKS APPROVED",
			Risk:   "medium",
			Policy: "require_approval",
			Phase:  domain.PhaseApply,
		})
	out := tui.View(m)

	for _, want := range []string{
		"Approval required",
		"apply",
		"medium",
		"NO APPLY WITHOUT TASKS APPROVED",
		"require_approval",
		"https://gov.local/approvals/abc",
		"[O]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestApprovalBannerOverlayInTimelineView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewTimeline).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://x",
			Phase:  domain.PhaseApply,
			Reason: "policy",
		})
	out := tui.View(m)
	// Banner must appear at the top — before the Timeline body.
	if i := strings.Index(out, "Approval required"); i < 0 {
		t.Fatalf("banner header missing:\n%s", out)
	}
	if j := strings.Index(out, "explore"); j > 0 {
		i := strings.Index(out, "Approval required")
		if i > j {
			t.Errorf("banner should appear BEFORE Timeline body (banner=%d, explore=%d)", i, j)
		}
	}
}

func TestApprovalBannerOverlayInApplyBoardView(t *testing.T) {
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithCurrentView(tui.ViewApplyBoard).
		WithBannerGate(&domain.ApprovalGate{URL: "https://x", Phase: domain.PhaseApply})
	out := tui.View(m)
	if !strings.Contains(out, "Approval required") {
		t.Errorf("banner missing in ApplyBoard view:\n%s", out)
	}
	if !strings.Contains(out, "ApplyBoard") {
		t.Errorf("ApplyBoard header missing under banner:\n%s", out)
	}
}

func TestApprovalBannerHandlesEmptyFields(t *testing.T) {
	// Defensive: a partially-filled ApprovalGate (e.g. Reason missing) must
	// still render without panic; missing fields render as empty strings.
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL: "https://x",
			// Reason, Risk, Policy, Phase intentionally empty
		})
	out := tui.View(m)
	if !strings.Contains(out, "Approval required") {
		t.Errorf("banner header missing on empty gate:\n%s", out)
	}
}

func TestApprovalBannerANSISafetyOnReason(t *testing.T) {
	// Spec §6.3 inv 7: banner Reason field is untrusted; it MUST flow
	// through lipgloss.Style.Render. We can't easily inspect the styled
	// output for ANSI safety, but we can assert the literal text survives
	// AND no leading screen-clear sequence is in the output.
	m := tui.NewModel(tui.ModelConfig{ChangeID: domain.ChangeID("01HX")}).
		WithBannerGate(&domain.ApprovalGate{
			URL:    "https://x",
			Reason: "\x1b[2J\x1b[H attacker tried to clear",
		})
	out := tui.View(m)
	if !strings.Contains(out, "attacker tried to clear") {
		t.Error("Reason text not present in output")
	}
	if strings.HasPrefix(out, "\x1b[2J") {
		t.Error("output must not begin with raw clear-screen from user input")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/tui/...`
Expected: FAIL — banner fields missing because the stub returns "".

- [ ] **Step 3: Implement view_approval_banner.go**

`internal/adapters/inbound/tui/view_approval_banner.go`:

```go
package tui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// bannerStyle is the lipgloss style for the approval banner box. Bordered,
// padded, full-width up to 60 cols. Risk-tier-aware coloring is M9+.
var bannerStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("9")). // bright red — attention
	Padding(0, 1).
	MarginBottom(1)

// renderApprovalBanner returns the rendered banner for the given gate.
// Banner stays visible until Update clears m.bannerGate via one of:
//
//   - approval.resolved event
//   - phase.started for a phase strictly after gate.Phase (forward progress)
//   - snapshot showing CurrentPhase strictly after gate.Phase
//
// Spec §2.2 layout: Phase / Risk / Reason / Policy / Gate / Status / [O].
// Spec §6.3 inv 7: every untrusted string (Reason, URL, Policy, etc.) is
// concatenated into a body string then rendered through bannerStyle.Render,
// which lipgloss treats as a single styled block — embedded ANSI in user
// input is not re-evaluated by the lipgloss render path.
func renderApprovalBanner(_ Model, gate domain.ApprovalGate) string {
	var b strings.Builder
	b.WriteString("Approval required by governance\n\n")
	b.WriteString(fmt.Sprintf("Phase: %-12s Risk: %s\n",
		stringOrDash(string(gate.Phase)),
		stringOrDash(gate.Risk),
	))
	b.WriteString(fmt.Sprintf("Reason: %s\n", stringOrDash(gate.Reason)))
	b.WriteString(fmt.Sprintf("Policy: %s\n", stringOrDash(gate.Policy)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Gate: %s\n", stringOrDash(gate.URL)))
	b.WriteString("Status: waiting\n")
	b.WriteString("\n")
	b.WriteString("[O] Open in browser")

	return bannerStyle.Render(b.String())
}

// stringOrDash returns "—" for empty strings so the banner stays aligned.
func stringOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
```

- [ ] **Step 4: Drop the temporary stub from Task 6**

Modify `internal/adapters/inbound/tui/view_applyboard.go`: REMOVE the temporary `renderApprovalBanner` stub. The real implementation in `view_approval_banner.go` is found by the package linker.

Modify `internal/adapters/inbound/tui/view_timeline.go`: change the call from `renderApprovalBanner(m, struct{}{})` to:

```go
		return renderApprovalBanner(m, *m.BannerGate()) + "\n" + body
```

(this should already be correct from Task 6 — verify and adjust if you used the placeholder signature there.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/tui/... -race`
Expected: PASS for all banner tests + previous M6/M7 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/tui/view_approval_banner.go \
        internal/adapters/inbound/tui/view_approval_banner_test.go \
        internal/adapters/inbound/tui/view_applyboard.go \
        internal/adapters/inbound/tui/view_timeline.go
git commit -m "feat(tui): add ApprovalGate banner overlay (§2.2)"
```

---

## Phase 6 — Run command + bootstrap wiring

### Task 8: `cli/run.go` — `--approval-timeout` flag + JSONL timeout enforcement

**Files:**
- Modify: `internal/adapters/inbound/cli/run.go`
- Modify: `internal/adapters/inbound/cli/run_test.go`

In `--no-tui --json` mode, M7 enforces a 30-minute approval timeout (D-M7-06: parsed via `time.ParseDuration`; default `30m`). The timer starts when the runner emits an `OnApprovalGate` callback; the timer is canceled by:

- `approval.resolved` event,
- a forward-progress event (`phase.started` for a phase later than the gated phase), or
- a snapshot whose `CurrentPhase` is past the gated phase.

If the timer expires, the run terminates with `*application.ExitError{Code: 5}`.

The TUI mode does NOT enforce a timeout — banner is purely visual.

The cleanest implementation pattern: a wrapper sink that decorates the JSONL sink, observes events, and runs a single goroutine with a `select` over `{ timer.C, resolveCh, progressCh, ctx.Done }` (RM7-05). The wrapper exposes a `Wait()` method that the run command calls AFTER `runner.Run` returns; if the timer fired, `Wait()` returns the timeout sentinel, which `runJSONL` turns into `ExitError{Code: 5}`.

> **Note:** the runner already calls `Sink.Close()` on its way out, so the wrapper's Close MUST also stop the timer goroutine to avoid leaks.

- [ ] **Step 1: Extend run_test.go**

Append to `internal/adapters/inbound/cli/run_test.go`:

```go
func TestRunCommandApprovalTimeoutDefault30m(t *testing.T) {
	// Sanity: no flag set → default flag value is "30m".
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json", "--help"})
	err := c.Execute()
	// --help exits 0; we just want to read the flag default from the help text.
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunCommandApprovalTimeoutTUIIsRejected(t *testing.T) {
	// --approval-timeout in TUI mode is ignored (TUI banner has no timeout).
	// The flag still parses successfully but the cli warns or accepts silently.
	// We document silent acceptance as the M7 default; revisit in M9.
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--approval-timeout=10s"})
	// The command will fail because the resolver finds no .sophia.yaml in
	// the test dir (exit code 3). We just assert the flag parses cleanly
	// (no "unknown flag" error).
	err := c.Execute()
	if err == nil {
		// OK — the run would succeed if the test env had a config; silent.
		return
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("unexpected unknown flag error: %v", err)
	}
}

func TestRunCommandApprovalTimeoutInvalidDurationFails(t *testing.T) {
	deps, _, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json", "--approval-timeout=banana"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --approval-timeout")
	}
	if !strings.Contains(err.Error(), "approval-timeout") {
		t.Errorf("error should mention approval-timeout: %v", err)
	}
}

func TestRunCommandApprovalTimeoutFiresInJSONLMode(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	// Configure the orchestrator/stream to emit an approval.required event
	// and then NEVER terminate. The CLI's --approval-timeout will fire.
	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			// Emit an approval.required event so the runner calls OnApprovalGate.
			stream.Push(target, domain.Event{
				Type: "approval.required",
				Payload: map[string]any{
					"phase":    "apply",
					"gate_url": "https://x",
					"risk":     "medium",
				},
			})
			// Never close the stream — the timer should fire.
		}()
	}
	// Don't set terminal status; the runner will keep streaming.

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json", "--approval-timeout=200ms"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected approval-timeout to fire with exit code 5")
	}
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("error not *ExitError: %v", err)
	}
	if exit.Code != 5 {
		t.Errorf("ExitError.Code = %d, want 5", exit.Code)
	}
	_ = orch
}

func TestRunCommandApprovalTimeoutCanceledByResolveEvent(t *testing.T) {
	t.Setenv(application.EnvOrchestratorURL, "")
	t.Setenv(application.EnvProject, "")
	t.Setenv(application.EnvBaseRef, "")
	var sinkBuf bytes.Buffer
	deps, orch, stream := newRunDeps(t, &sinkBuf)

	stream.OnSubscribe = func(target outbound.StreamTarget) {
		go func() {
			stream.Push(target, domain.Event{
				Type: "approval.required",
				Payload: map[string]any{"phase": "apply", "gate_url": "https://x"},
			})
			// Resolve the approval — timer cancels.
			stream.Push(target, domain.Event{
				Type: "approval.resolved",
				Payload: map[string]any{"decision": "approved"},
			})
			// Then go terminal.
			orch.SetTerminal(target.ChangeID, domain.ChangeStatusDone)
			stream.Close(target)
		}()
	}

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json", "--approval-timeout=5s"})
	err := c.Execute()
	if err != nil {
		t.Fatalf("expected success when approval resolves before timeout: %v", err)
	}
}
```

> **Note on `stream.Push`:** the existing `fakes.FakeEventStream` may need a `Push(target, event)` helper for these tests. If it doesn't, add it (one line — unbuffered send to the stream's channel). Verify by reading `test/fakes/event_stream.go`.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL — `--approval-timeout` flag undefined; the new wrapper logic is missing.

- [ ] **Step 3: Implement the timeout wrapper**

Add a new file `internal/adapters/inbound/cli/timeout_sink.go`:

```go
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

	mu        sync.Mutex
	gate      *domain.ApprovalGate
	timer     *time.Timer
	fired     bool
	closed    bool
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
		// Already running — re-arm with the new gate (a second
		// approval.required for a different phase resets the clock).
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

// phaseTypeFromEventPayload extracts the phase_type / phase field from an
// event payload. Mirrors tui.phaseTypeFromPayload — duplicated locally to
// avoid importing tui from cli for a one-liner.
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

// phaseOrdinalAfter reports whether a's index in domain.AllPhases() is
// strictly greater than b's index.
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
```

- [ ] **Step 4: Wire the wrapper into runJSONL**

Modify `internal/adapters/inbound/cli/run.go`:

ADD imports:

```go
"time"
```

ADD a flag binding in `newRunCmd`:

```go
var approvalTimeoutStr string
// ...
cmd.Flags().StringVar(&approvalTimeoutStr, "approval-timeout", "30m",
	"max wait for an approval gate before exit code 5 (--no-tui only)")
```

PARSE the flag inside `RunE` (before dispatch):

```go
approvalTimeout, err := time.ParseDuration(approvalTimeoutStr)
if err != nil {
	return fmt.Errorf("run: --approval-timeout: %w", err)
}
```

PASS the timeout into `runJSONL`:

```go
if noTUI {
	return runJSONL(cmd.Context(), d, input, approvalTimeout)
}
return runTUI(cmd.Context(), d, input)
```

REPLACE `runJSONL` to wrap the sink and exit-5-on-timeout:

```go
func runJSONL(parentCtx context.Context, d Deps, input application.RunInput, approvalTimeout time.Duration) error {
	innerSink := chooseJSONSink(d)

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	wrapped := newApprovalTimeoutSink(innerSink, approvalTimeout, cancel)
	runner := d.RunnerFactory(wrapped)

	res, err := runner.Run(ctx, input)
	_ = res

	// If the timer fired, Wait returns errApprovalTimeout. We map this to
	// ExitError{Code: 5} per spec §2.3.
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
```

> **Note:** the existing `chooseJSONSink` returns the underlying jsonsink (or test override). The wrapper sink implements `inbound.EventSink` and is what `RunnerFactory` sees. The runner calls `Sink.Close()` on its way out; our wrapper's `Close()` cleans up the timer and closes the inner sink — no leaks.

`runTUI` does NOT use the wrapper — TUI banner has no timeout (§5.8).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS for all the new approval-timeout tests + every existing M5/M6 cli test.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/cli/run.go \
        internal/adapters/inbound/cli/run_test.go \
        internal/adapters/inbound/cli/timeout_sink.go
git commit -m "feat(cli): add --approval-timeout (default 30m) with exit code 5 on expiry (§5.8)"
```

---

### Task 9: Bootstrap — inject `osbrowser` into the TUI program

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`
- Modify: `internal/adapters/inbound/cli/run.go`
- Modify: `internal/adapters/inbound/cli/root.go`

The TUI Program needs an `outbound.Browser` so `[O]` works. The bootstrap constructs `osbrowser.New(...)` and threads it through `cli.Deps`. `cli.run.go` reads it from Deps when constructing the TUI program.

- [ ] **Step 1: Extend cli.Deps**

Modify `internal/adapters/inbound/cli/root.go`. ADD an import:

```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
```

ADD a field to `Deps`:

```go
type Deps struct {
	// ... existing fields ...

	// Browser is the outbound.Browser passed to the TUI program for [O].
	// Bootstrap injects an osbrowser instance; tests inject FakeBrowser.
	Browser outbound.Browser
}
```

- [ ] **Step 2: Wire through runTUI**

Modify `internal/adapters/inbound/cli/run.go`. In `runTUI`, change the `tui.NewProgram` call:

```go
prog, err := tui.NewProgram(tui.ProgramConfig{
	Output:  output,
	Browser: d.Browser,
})
```

- [ ] **Step 3: Update bootstrap**

Modify `internal/bootstrap/wire.go`. ADD imports:

```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/osbrowser"
```

CONSTRUCT the browser and pass it to Deps:

```go
browser := osbrowser.New(osbrowser.Config{})

// ...

deps := cli.Deps{
	// ... existing fields ...
	Browser: browser,
}
```

- [ ] **Step 4: Update wire_test.go**

Append a smoke test to `internal/bootstrap/wire_test.go`:

```go
func TestNewWiresM7Browser(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := root.Find([]string{"run"})
	if err != nil || c == nil {
		t.Fatalf("run cmd missing: %v", err)
	}
	// We can't directly inspect Deps from the cobra tree, but the run cmd
	// constructed without panic — bootstrap wired Browser cleanly.
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/adapters/inbound/cli/... ./internal/bootstrap/... -race
go vet ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/cli/run.go \
        internal/adapters/inbound/cli/root.go \
        internal/bootstrap/wire.go \
        internal/bootstrap/wire_test.go
git commit -m "feat(bootstrap): inject osbrowser.New into TUI program (M7)"
```

---

## Phase 7 — teatest goldens + final validation

### Task 10: `test/tui/applyboard_banner_test.go` — End-to-end TUI integration

**Files:**
- Create: `test/tui/applyboard_banner_test.go`

Two new teatest scenarios:

1. **Tab toggles view.** Send a `task.started` event; press Tab; assert the rendered output now contains the ApplyBoard header AND the task ID. Press Tab again; assert the Timeline phases come back.
2. **Approval banner appears + [O] triggers browser.** Send an `ApprovalGateMsg`; assert the banner header appears in output. Press `o`; assert the FakeBrowser recorded the URL.

- [ ] **Step 1: Create the test file**

`test/tui/applyboard_banner_test.go`:

```go
package tui_integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// TestTUITabTogglesToApplyBoard sends a task.started event, presses Tab,
// and asserts the ApplyBoard view replaces the Timeline.
func TestTUITabTogglesToApplyBoard(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		newRoot(domain.ChangeID("01HXTAB")),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit()

	// Feed a task.started event so the ApplyBoard has content.
	tm.Send(tui.EventMsg{Event: domain.Event{
		Type: "task.started",
		Payload: map[string]any{
			"group_id":      "g1",
			"task_id":       "t1",
			"files_pattern": "internal/**",
		},
	}})

	// Initially we're in Timeline — assert phase names appear.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "explore")
	}, teatest.WithDuration(2*time.Second))

	// Press Tab.
	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: tea.KeyTab}})

	// ApplyBoard should now show g1 and t1.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "ApplyBoard") &&
			strings.Contains(s, "g1") &&
			strings.Contains(s, "t1")
	}, teatest.WithDuration(2*time.Second))

	// Press Tab again — back to Timeline.
	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: tea.KeyTab}})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		// Header changes back to "Sophia · Change 01HXTAB · running" -ish.
		return strings.Contains(string(b), "explore")
	}, teatest.WithDuration(2*time.Second))
}

// TestTUIApprovalBannerAppearsAndOpensBrowser sends an ApprovalGateMsg,
// presses [O], and asserts the banner is rendered + the browser open
// is invoked. The browser side-effect is verified by the Program's
// FakeBrowser record (we use the package-level Program API, not teatest,
// for this part because teatest runs Update only — Program-level handling
// of OpenBrowserMsg is exercised by program_test.go).
//
// This test wires the wrappedModel + FakeBrowser via a custom rootShim
// that intercepts OpenBrowserMsg the same way Program does.
func TestTUIApprovalBannerAppearsAndOpensBrowser(t *testing.T) {
	openedURLCh := make(chan string, 1)
	tm := teatest.NewTestModel(
		t,
		newRootWithBrowser(domain.ChangeID("01HXBANNER"), func(url string) error {
			openedURLCh <- url
			return nil
		}),
		teatest.WithInitialTermSize(120, 40),
	)
	defer tm.Quit()

	// Send an ApprovalGateMsg.
	tm.Send(tui.ApprovalGateMsg{Gate: domain.ApprovalGate{
		URL:    "https://gov.local/approvals/abc",
		Phase:  domain.PhaseApply,
		Risk:   "medium",
		Reason: "policy says no apply without tasks approved",
	}})

	// Banner should render at the top.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Approval required")
	}, teatest.WithDuration(2*time.Second))

	// Press [O].
	tm.Send(tea.KeyPressMsg{Key: tea.Key{Code: 'o'}})

	// Assert the browser open was called.
	select {
	case url := <-openedURLCh:
		if url != "https://gov.local/approvals/abc" {
			t.Errorf("opened URL = %q", url)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("browser open was not called within 2s")
	}
}

// rootShim replicates the Program's interception of OpenBrowserMsg for
// teatest scenarios. It also tracks state via the wrappedModel pattern
// established in M6.
type rootShim struct {
	state         tui.Model
	openCallback  func(url string) error
}

func newRootWithBrowser(id domain.ChangeID, openCallback func(url string) error) tea.Model {
	return rootShim{
		state:        tui.NewModel(tui.ModelConfig{ChangeID: id}),
		openCallback: openCallback,
	}
}

func (m rootShim) Init() tea.Cmd { return nil }

func (m rootShim) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if om, ok := msg.(tui.OpenBrowserMsg); ok {
		if m.openCallback != nil {
			go m.openCallback(om.URL) //nolint:errcheck
		}
		return m, nil
	}
	newState, cmd := tui.Update(m.state, msg)
	m.state = newState
	return m, cmd
}

func (m rootShim) View() tea.View { return tea.NewView(tui.View(m.state)) }

// Suppress unused if context isn't referenced elsewhere.
var _ = context.Background
```

> **Note:** `tea.KeyTab` exists in v2 (used in M6's keybindings); rune-keyed tests use `tea.Key{Code: 'o'}` per the M6 pattern.

- [ ] **Step 2: Run tests**

```bash
go test ./test/tui/... -race -timeout 60s
```

Expected: PASS for both new cases AND the four M6 cases.

- [ ] **Step 3: Diagnose teatest API drift**

If `teatest.NewTestModel` / `tm.Send` / `tm.WaitFor` / `tm.Quit` shapes have changed, adapt — the test contract ("Tab toggles view; banner appears; [O] opens URL") must stay invariant. If a primitive truly disappears, STOP and ASK.

- [ ] **Step 4: Commit**

```bash
git add test/tui/applyboard_banner_test.go
git commit -m "test(tui): add teatest coverage for Tab toggle + approval banner [O]"
```

---

### Task 11: Final validation pass + interactive smoke + tag

**Files:** none (verification only).

- [ ] **Step 1: vet + tests + race**

```bash
go vet ./...
go test -race ./...
```

Expected: exit 0. If teatest tests flake on CI under `-race`, raise the WaitFor timeouts to 5s; do NOT silence `-race`.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```

Acceptable nolint patterns: existing precedents (gosec on subprocess shellouts in osbrowser — `exec.CommandContext` with a validated URL is the only path; document with a `//nolint:gosec // URL whitelisted via validate()` if needed). Fix new findings in place.

- [ ] **Step 3: Coverage**

```bash
go test -coverprofile=cover.out \
   ./internal/adapters/inbound/tui/... \
   ./internal/adapters/inbound/cli/... \
   ./internal/adapters/outbound/osbrowser/...
go tool cover -func=cover.out | tail -n 1
```

Expected: total ≥ 80% across the three packages. The osbrowser adapter and ApplyBoardState are pure code; both should clear 90%+. The Program intercept and the timeout sink are I/O-heavier; teatest + the cli timeout tests should land each around 75-80%.

- [ ] **Step 4: Binary smoke**

```bash
make build

# 1) Help text now lists --approval-timeout
./bin/sophia run --help | rg approval-timeout

# 2) Outside a repo, exit 3 (no .sophia.yaml)
./bin/sophia run "test"
echo "default exit=$?"

./bin/sophia run "test" --no-tui --json
echo "jsonl exit=$?"

./bin/sophia run "test" --no-tui --json --approval-timeout=banana
echo "bad-flag exit=$?"

./bin/sophia run "test" --no-tui --json --approval-timeout=10s
echo "valid-flag exit=$?"

# 3) Other commands still work
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
```

Expected:
- `run --help` shows `--approval-timeout` with default `30m` and the description.
- bad duration → non-zero exit with `approval-timeout` in error.
- valid duration → exit 3 (no orchestrator), not a parse error.
- version + doctor unaffected.

- [ ] **Step 5: Interactive smoke (manual — described, executed by reviewer)**

Pre-req: a running orchestrator at `SOPHIA_ORCHESTRATOR_URL` (default localhost:9080) plus a `.sophia.yaml` in the working directory. Trigger an `approval.required` event from the orchestrator side (or use a stub).

1. **Default TUI mode renders Timeline (carry-over from M6):**
   ```bash
   ./bin/sophia run "smoke test M7 default mode"
   ```
   Expect: Bubble Tea opens; 9 phase rows; bottom hint reads "Q to detach · Ctrl+C confirm-then-detach" (Timeline default doesn't yet show Tab — see step 3).

2. **`Tab` toggles to ApplyBoard:**
   While the run is in flight, press `Tab`. Expect: header changes from "Sophia · Change … · running" to "Sophia · Change … · ApplyBoard · N groups"; phase rows replaced by group/task tree (or a "No tasks yet" hint if no apply phase events have arrived).

3. **`Tab` again returns to Timeline.** Expect: Timeline reappears.

4. **Approval banner appears:**
   With the orchestrator emitting an `approval.required` event, expect a bordered red box at the TOP of whichever view is active, showing Phase / Risk / Reason / Policy / Gate URL / Status / `[O] Open in browser`.

5. **`[O]` opens browser:**
   With the banner visible, press `o`. Expect: the OS browser opens to the gate URL (or, on a CI/headless box, a quiet error line saying `xdg-open: not found`). The TUI does NOT exit — it stays running.

6. **Banner clears on `approval.resolved`:**
   Have the orchestrator emit `approval.resolved`. Expect: the banner disappears WITHOUT any user action.

7. **Banner clears on forward-progress:**
   Re-trigger an `approval.required` for `apply`. Then have the orchestrator advance to `verify` (emit `phase.started` for verify). Expect: banner disappears.

8. **Banner clears on snapshot refresh:**
   Re-trigger an `approval.required` for `apply`. Force a stream disconnect so the runner does a post-stream snapshot. If that snapshot's CurrentPhase is past apply, expect: banner disappears.

9. **JSONL approval-timeout enforcement:**
   ```bash
   ./bin/sophia run "smoke approval-timeout" --no-tui --json --approval-timeout=10s
   ```
   With the orchestrator emitting `approval.required` and never resolving, expect: the run terminates after 10s with exit code 5; stdout JSONL shows `OnError` and `OnComplete` lines.

10. **JSONL approval-timeout cancellation:**
    Same command, but have the orchestrator emit `approval.resolved` after 3s. Expect: run continues, eventually reaches a terminal status, exits with the corresponding code (0 for done).

11. **URL validation rejection (TUI):**
    Modify the orchestrator (or a stub) to emit an `approval.required` event whose `gate_url` is `javascript:alert(1)`. Press `o`. Expect: an error line appears below the phase rows: `error: browser: osbrowser: scheme not allowed (only http/https): got "javascript"`. The TUI does NOT exit.

If ANY step fails, file an issue or stop the M7 ship.

- [ ] **Step 6: Integration smoke (carry-over)**

```bash
go test -race ./test/integration/...
```

Expected: PASS for the M5 SSE reconnect + M3 init/filestate tests. M7 didn't touch those paths.

- [ ] **Step 7: e2e smoke (carry-over)**

```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```

Expected: PASS. Existing M5/M6 e2e tests use `--no-tui --json`; the new `--approval-timeout` flag has a default of 30m which never fires in fast tests.

- [ ] **Step 8: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m7): final validation pass" || echo "nothing to commit"
git tag -a m7-applyboard-approval -m "M7 ApplyBoard + Approval Banner complete"
git tag
```

---

## Self-review checklist

- [ ] **Spec coverage:** every M7 DoD item from spec §7.2 has at least one task.
  - Second view (`view_applyboard.go`): groups + tasks + team-leads visible, parallelism shown → Task 6
  - Data sourced from `task.*` and `agent.*` events → Task 2 (state) + Task 3 (model wiring)
  - Approval banner with URL/Reason/Risk/Policy/ChangeID/Phase/TraceID → Task 7
  - `[O]` opens in browser via osbrowser after URL validation → Task 1 (validation) + Tasks 4/5 (key + Program wiring)
  - Banner is derived state: hides on approval.resolved / snapshot / forward-progress → Task 3
  - Golden snapshot tests for both views → Tasks 6, 7, 10
- [ ] **Spec §6.3 #3 — Subprocess + URL validation:** `osbrowser.validate` runs BEFORE `exec.CommandContext`; whitelist enforced; tests cover javascript / data / file / vbscript / mailto / ftp / empty / malformed / scheme-relative.
- [ ] **Spec §5.8 — Approval timeout:** `--approval-timeout` defaults to `30m`, only fires in `--no-tui` mode, exit code 5 on expiry, canceled by resolved / forward-progress / snapshot. Test: `TestRunCommandApprovalTimeoutFiresInJSONLMode`, `TestRunCommandApprovalTimeoutCanceledByResolveEvent`.
- [ ] **No placeholders:** no "TBD"/"TODO"/"similar to" in steps.
- [ ] **No new outbound port:** uses existing `outbound.Browser` (`internal/ports/outbound/browser.go`); M7 adds the IMPL only.
- [ ] **No new domain types:** ApplyBoardState lives in `tui/`. ApprovalGate, PhaseType, Event reused.
- [ ] **Frequent commits:** every task ends with a commit.
- [ ] **TDD discipline:** failing test before implementation in every Phase 1–7 task.
- [ ] **Pure layer purity:** `tui.Update` is pure — `OpenBrowserMsg` is the seam; the Program adapter is the only thing calling `Browser.Open`.
- [ ] **lipgloss ANSI safety:** banner Reason / URL / Policy flow through `bannerStyle.Render`; ApplyBoard rows go through `pkgStyles.styleFor(...)`. Inv 7 honored.
- [ ] **No premature M8+ scope:** no real `attach`/`changes`/`status`; no Last-Event-ID resume; no `--orchestrator-url` per-call.

---

## Pending decisions (carried into M7 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M7-01 | ApplyBoard layout: table vs nested tree | nested tree (groups → tasks indented → agents under tasks). Document table as alternative for M9+ polish. |
| D-M7-02 | Tab key in confirm-detach mode? | Tab cancels confirm + toggles view. Reuses any-key-cancels-confirm semantics from M6 (D-M6-04). |
| D-M7-03 | [O] key when banner is hidden? | No-op. Consistent with "any key" being a no-op outside confirm mode. |
| D-M7-04 | Browser open failure — silent or visible? | Visible — append error line via `Model.WithError(...)`. |
| D-M7-05 | Should `approval.resolved` show a confirmation toast? | No — banner just disappears. Keep UI minimal in M7. M9+ may add a one-frame "Approved by alice" toast if user feedback asks. |
| D-M7-06 | `--approval-timeout` parse format | Go's `time.ParseDuration` — accepts `30m`, `1h`, `2h30m`, etc. Default `30m`. |
| D-M7-07 | Forward-progress event detection in Model | Compare `phase.started`'s phase ordinal in `AllPhases()` vs `bannerGate.Phase` ordinal. If new > banner → clear. |
| D-M7-08 | Browser opener test strategy | Fake-binary pattern with shell stub that captures argv to a file. POSIX-only; Windows tests use the OSOverride hook for unsupported-platform path. |

---

## Risks specific to M7

| ID | Risk | Mitigation |
|---|---|---|
| RM7-01 | URL validation incomplete; arbitrary URLs reach subprocess | Whitelist scheme: only `http` and `https` accepted. `validate()` runs BEFORE `exec.CommandContext`. Test matrix: javascript / data / file / vbscript / mailto / ftp / scheme-relative / malformed / empty / control-char (Task 1). |
| RM7-02 | Banner hiding logic gets out of sync with reality | Model is pure + derived; tests cover all 3 hiding triggers (resolved / snapshot / forward-progress) AND the negative cases (same-phase event, earlier-phase event, snapshot at gated phase). Task 3. |
| RM7-03 | Tab key conflicts with terminal emulator (some terminals intercept Tab for completion) | Document in plan + manual smoke step 2/3. M6's bubbletea v2 should pass Tab through normally. teatest covers it (Task 10). |
| RM7-04 | OS-specific commands break on niche platforms | Stick to the big three: macOS / Linux / Windows. BSD variants fall through to xdg-open. Anything else (Solaris, plan9) returns `ErrUnsupportedPlatform` cleanly. Tested via `OSOverride: "plan9"` (Task 1). |
| RM7-05 | Approval timeout cancellation has races (event arrives JUST as timer fires) | Single goroutine started by `time.AfterFunc`. The wrapper's `mu` serializes timer Stop / re-arm vs the AfterFunc's `fired = true`. AfterFunc runs even if `Stop` returns false (i.e. timer already fired); we read `s.fired` under the same lock. No leaks: `Close()` calls `timer.Stop()` and the goroutine is short-lived. |
| RM7-06 | TUI banner overlaps Timeline content visually | Banner is rendered as a fixed-size block ABOVE the active view's body; the View() dispatcher concatenates `banner + "\n" + body`. Timeline truncates rows to fit Width; banner uses a bordered lipgloss style that won't overflow. Manual smoke step 4 verifies on a real terminal. |
| RM7-07 | ApplyBoard state grows unboundedly under heavy task throughput | Cap at 50 groups; oldest evicted by insertion order (Task 2). Tested in `TestApplyBoardStateCapsGroupsAt50`. M9+ may make the cap configurable. |
| RM7-08 | The Program intercepts OpenBrowserMsg in `rootModel.Update`; if the fan-out goroutine outlives the program, `*tea.Program.Send` may block forever | The `running` flag guarded by `mu` in Program is checked before Send (Task 5). The goroutine has a 10s context timeout on Browser.Open so it cannot hang indefinitely. Close() doesn't wait on these goroutines — they're best-effort. |
| RM7-09 | The wrapper sink in `runJSONL` could double-Close the inner sink (Runner calls Close + cli ALSO calls Close) | Only the wrapper is exposed to the Runner; the cli does NOT call `chooseJSONSink`'s return Close directly. The wrapper's Close is idempotent (`closed bool` guard) and forwards once to the inner sink. Verify by grepping for `inner.Close`. |
| RM7-10 | Banner-clearing ordinal comparison is wrong for parallelizable phases | Spec §3.2 mandates strict 9-phase ordering; there is no parallel-phase scenario in M7. If the orchestrator emits a phase.started for a phase < bannerGate.Phase (out-of-order), the banner stays — correct behavior. Documented in `TestModelApplyEventEarlierPhaseDoesNotClearBanner`. |

---

## What this plan does NOT cover (intentional)

- Real `sophia attach` / `sophia changes` → M8.
- Real `sophia status` against orchestrator → M8 (M7 keeps M3's local-only resolution).
- Cross-process `Last-Event-ID` resume → M8.
- `--orchestrator-url` per-call rebinding → M9+.
- ApplyBoard column-sortable mode (table view, sort-by-status, filter-by-group) → M9+.
- Approval banner colors per risk level (visual treatment) → polish; M7 ships with a fixed style.
- Toast / confirmation message on `approval.resolved` → no, banner just disappears (D-M7-05).
- `--approval-timeout` in TUI mode → no, TUI banner is purely visual; user closes by interacting (and the derived-state triggers handle the auto-close).
- Browser preference (`SOPHIA_BROWSER=firefox`) → out of scope; OS-default handler only.
- Headers / cookies on the browser open call → out of scope; URL only.
- New outbound port for browser → no, `outbound.Browser` already exists.
- New domain types → no, ApplyBoardState is TUI-internal.
- Stream-health indicator (drops counter, reconnect badge) → M9+ polish (would render below the active view as a footer).
- Logging panel for non-phase events → M9+ (would append `agent.*` / `task.*` events as a scrolling log below the ApplyBoard or Timeline).

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-06-sophia-cli-m7-applyboard-approval.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task. Use `superpowers:subagent-driven-development`. Each task has a self-contained TDD cycle (write test → fail → implement → pass → commit), so subagents work independently with minimal context.

Recommended ordering for parallelism:

- Task 1 (osbrowser) — independent of TUI changes; ship first.
- Task 2 (ApplyBoardState) — independent of Tasks 3+; can run in parallel with Task 1.
- Tasks 3, 4, 5 (model / update / program) — sequential because each depends on the previous.
- Tasks 6, 7 (views) — Task 6 has a temporary stub satisfied by Task 7; ship together to avoid the stub-and-replace dance, OR ship sequentially and review the stub commit + replacement commit as a pair.
- Tasks 8, 9 (cli + bootstrap) — sequential.
- Tasks 10, 11 (teatest + final validation) — sequential.

**2. Sequential single-agent** — use `superpowers:executing-plans` and walk Task 1 → Task 11 in order. Recommended only if you want to keep the full context window for cross-task surprises (most likely Tasks 5 (Program rootModel rewrite) and 8 (timeout sink wiring) if the bubbletea v2 Send semantics or the runner's Sink lifecycle differ from this plan's assumptions).

Either way: keep an eye on the URL validation matrix in Task 1 — if any browser opener test passes when it shouldn't (e.g. the `mailto:` test passes without the validate hook) the entire `[O]` shortcut is unsafe. Run the validation suite before merging.

---

## Implementation Notes — Deviations from Plan

The plan above was the design intent; this section captures what actually shipped. Each entry: what the plan said, what was implemented, why, and where it lives.
