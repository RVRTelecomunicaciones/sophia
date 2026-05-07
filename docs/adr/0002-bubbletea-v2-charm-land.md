# 0002 — Bubble Tea v2 on `charm.land`

- **Status:** Accepted
- **Date:** 2026-05-06 (M6 commit chain)
- **Deciders:** sophia-cli core team

## Context

M6 introduced the Sophia TUI — a timeline view that renders Change phases as
events stream in over SSE, with `Q` to detach and `Ctrl+C` for confirm-then-detach.
The TUI framework decision was made at M6 inception:

- **Bubble Tea v1** (`github.com/charmbracelet/bubbletea`) — the de-facto
  incumbent. Mature, large community, well-documented patterns. Imminent v2.
- **Bubble Tea v2** (`charm.land/bubbletea/v2`) — under active development at
  M6 time. Breaking API changes (`tea.View` instead of `string`,
  `lipgloss.Color()` as a function for composition,
  `tea.WindowSizeMsg` reshape, `teatest/v2` golden-file framework).
- **Alternative non-charm libs** (`tview`, `gocui`, raw `termenv` + custom
  loop) — viable but smaller communities, no obvious `bubbletea`-class advantage.

The relevant question at M6: **adopt v1 and migrate later, or adopt v2 and
absorb the API churn now?**

The team picked v2.

## Decision

Use Bubble Tea v2 from the `charm.land` module path.

**Pinned versions** (current at M8 tag — verify against `go.mod` if drifted):

```
charm.land/bubbletea/v2  v2.0.6
charm.land/bubbles/v2    v2.1.0
charm.land/lipgloss/v2   v2.0.3
```

Plus the matching test framework:

```
github.com/charmbracelet/x/exp/teatest/v2
```

**Pin policy:** stay on the v2 minor line (`v2.x.x`). Do not adopt v3 (when it
ships) until a feature actually demands it AND the migration cost is budgeted
into a milestone plan. Patch bumps are routine via `go get -u`.

## Consequences

### Pros

- **No forced migration mid-product.** v1 → v2 is a non-trivial rewrite. By
  starting on v2 we avoid the "rewrite the TUI in M9" tax.
- **Cleaner API for the TUI we built.** The `tea.View` model (a typed return
  instead of `string`) makes composition explicit; `lipgloss.Color()` as a
  function plays well with the M7 ApplyBoard view that derives colors from
  agent state.
- **`teatest/v2` is the test surface we'd need anyway.** v1's testing story
  is community-built; v2 ships golden-file integration alongside the runtime.
  M6 Task 8 ships first golden tests on day one.
- **Aligned with charm's release line.** The `charm.land` module path is
  charmbracelet's long-term home; staying on it means we're on the path the
  upstream maintainers are investing in.

### Cons

- **Smaller community at adoption time.** Stack Overflow, blog posts, and
  example repos overwhelmingly use v1 idioms. Newcomers to the codebase
  who've used Bubble Tea before will need to relearn `View` returning
  `tea.View`, the function-vs-method `Color`, etc. Mitigation: short
  in-line comments in `internal/adapters/inbound/tui/program.go` flag the
  v2-specific patterns.
- **Minor API still landing.** Between v2.0.0 and v2.0.6 the team observed
  one breaking surface (the `tea.WindowSizeMsg` reshape) within a single
  patch line. Pinned versions and `go.sum` integrity reduce risk.
- **`charm.land` redirect confusion.** `charm.land/bubbletea/v2` is a
  redirect to `github.com/charmbracelet/bubbletea/v2`, but Go module
  resolution caches it as a distinct module path. Snippets copied from
  the GitHub repo's documentation may use the `github.com/...` path; treat
  it as identical content but DO NOT mix paths in `go.mod` — pick one and
  stick with it (we picked `charm.land`).
- **Headless TTY tests required care.** M6 Task 6 surfaced that
  `tea.NewProgram` panics on `os.Stdin` reads when stdout is redirected
  in tests; `WithInput(nil)` is the workaround. Documented inline in
  `program.go`; teatest infrastructure works around it for golden tests.

### Reaffirmation in M7

M7 added the ApplyBoard view + approval banner using v2's `lipgloss.Color()`
function for dynamic agent-color mapping. The ergonomics confirmed the
choice — v1's method-based color builder would have been significantly
worse for this case.

## Verification

```bash
# go.mod pins agree with this ADR
rg "charm.land/(bubbletea|bubbles|lipgloss)" go.mod
```

If versions drift from this ADR, either (a) update the ADR, or (b) revert
the bump to honor the pin policy. Major-version bumps (v2 → v3) are NOT
allowed without a separate ADR superseding this one.

## References

- `internal/adapters/inbound/tui/program.go` — `tea.NewProgram` setup.
- `internal/adapters/inbound/tui/model.go` — pure Update + View model.
- `test/tui/timeline_test.go` — `teatest/v2` golden integration test.
- M6 plan, "Step 1: Pin Bubble Tea v2" decision narrative.
