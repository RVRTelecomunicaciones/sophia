# 0001 — Hexagonal Architecture (Ports & Adapters)

- **Status:** Accepted
- **Date:** 2026-05-05 (M1 inception; reaffirmed at M8)
- **Deciders:** sophia-cli core team

## Context

`sophia` is a CLI that drives an external orchestrator over HTTP + SSE, renders
events in a Bubble Tea TUI, persists local state on disk, and shells out to
`docker compose` and `git`. The unknowns at M1 included:

- Which TUI framework (Bubble Tea v1 vs v2 vs alternatives)
- Whether to poll the orchestrator or consume Server-Sent Events (M4 shipped
  polling; M5 swapped to SSE)
- How to test command flows without a real orchestrator
- How to keep terminal-rendering concerns out of business logic

Picking a layered MVC structure or a flat package layout would have entangled
those choices: a polling-vs-SSE swap would touch business logic; a TUI rewrite
would touch persistence; testing a use case would require running a real HTTP
client. None of those are acceptable for a CLI that has to evolve through nine
milestones and ship as a stable v0.1.0.

The M5 → M8 evolution validates the framing in retrospect:

- **M5** swapped polling for SSE without touching `domain` or any use case
  except `Runner` (the consumer of the `EventStream` port).
- **M6** added a TUI without touching `application` — `tui.Bridge` implements
  the inbound `EventSink` port that the use cases already produced.
- **M7** added an approval gate banner + `[O]pen browser` with a new outbound
  `Browser` port; no use case changed.
- **M8** extracted `Runner.Observe(ctx, RunResult, sink)` and reused it from
  `Attacher` because the port boundary between use case and event sink was
  already clean.

## Decision

Use **hexagonal architecture** (Cockburn / Ports & Adapters) with a strict
four-tier package layout and a one-way import lattice.

```
                        ┌──────────────────────┐
                        │   adapters/inbound   │  cobra cli, bubbletea tui, jsonl sink
                        └──────────┬───────────┘
                                   │ implement inbound ports
                        ┌──────────▼───────────┐
                        │     application      │  Runner, Attacher, Lister, StatusReader,
                        │  (use cases)         │  Initializer, Provisioner, ConfigResolver, …
                        └──────────┬───────────┘
                                   │ depends on inbound + outbound port interfaces
                        ┌──────────▼───────────┐
                        │     ports            │  EventSink (inbound)
                        │  inbound + outbound  │  OrchestratorClient, EventStream,
                        │                      │  StateStore, ProjectConfigStore,
                        │                      │  GitInspector, Browser, Compose, …
                        └──────────┬───────────┘
                                   │ defined in terms of domain types
                        ┌──────────▼───────────┐
                        │     domain           │  Change, Phase, Event, ApprovalGate,
                        │  (no I/O)            │  ProjectConfig, sentinel errors
                        └──────────────────────┘
                                   ▲
                                   │ implement outbound ports
                        ┌──────────┴───────────┐
                        │  adapters/outbound   │  orchestratorhttp, ssestream, sseprobe,
                        │                      │  filestate, yamlconfig, gitcli,
                        │                      │  composeexec, osbrowser, xdgpaths
                        └──────────────────────┘
```

**Import rules** (verified mechanically — see Verification below):

1. `domain` imports **only** stdlib + other `domain` files. No port, application,
   or adapter imports.
2. `ports/{inbound,outbound}` imports **only** `domain`. Ports define interfaces
   in terms of domain types; the orchestrator wire shape lives in `adapters`,
   not here.
3. `application` imports **only** `domain` + `ports/{inbound,outbound}`. NO
   `adapters` imports — use cases never see HTTP, SSE, files, or terminals.
4. `adapters/{inbound,outbound}` imports `domain`, `ports`, `application` (for
   inbound adapters that wire deps), and stdlib + third-party. **No adapter
   imports another adapter.** Cross-cutting helpers either move into a separate
   `internal/infrastructure/<name>` package (e.g. `httpclient`) or get duplicated.
5. `bootstrap` is the composition root. It is the ONLY package that imports
   every adapter; it constructs adapters, injects them into use cases, and hands
   the wired `cli.Deps` to Cobra.

## Consequences

### Pros (observed across M1–M8)

- **Testability**: every outbound port has a fake under `test/fakes/`. Use case
  tests run in microseconds without Docker, network, or a real orchestrator.
- **Swappability**: M4's polling client was a `Runner.deps.EventStream`
  implementation; M5's SSE client implements the same interface. Zero changes
  in the use case beyond the rewrite of the consumption loop itself.
- **TUI isolation**: Bubble Tea never reaches into `application`. `tui.Bridge`
  is an inbound adapter that buffers events and forwards them to the
  `tea.Program`; the bridge is the only `bubbletea` import in the path from
  use case to terminal.
- **Composition root clarity**: `bootstrap/wire.go` is the single file a new
  contributor reads to understand the dependency graph. It's ~140 lines.

### Cons (acknowledged)

- **Boilerplate**: a new outbound capability needs (a) a port interface, (b) a
  fake, (c) an adapter, (d) wiring in `bootstrap`. For a one-method capability
  this is 3+ files of mostly trivial code.
- **Onboarding cost**: newcomers ask "where does this live?" more than they
  would in a flat layout. The cure is reading this ADR plus
  `internal/ports/outbound/orchestrator.go` as the canonical example.
- **Refactor friction**: renaming a port method is a four-file change
  (interface, fake, adapter, every caller).

The team accepts these costs because they buy us the M5 → M8 swap velocity.

## Verification

The lattice is grep-verifiable. Run from the repo root; each command MUST
produce empty output (or no listed cross-imports):

```bash
# (1) domain imports only stdlib + other domain
rg -l '^\s*"github\.com/RVRTelecomunicaciones/sophia-cli/internal/(application|adapters|ports)' \
   internal/domain/

# (2) application imports no adapters
rg -l '^\s*"github\.com/RVRTelecomunicaciones/sophia-cli/internal/adapters' \
   internal/application/

# (3) adapters do not cross-import
for d in internal/adapters/inbound/* internal/adapters/outbound/*; do
  pkg=$(basename "$d")
  rg -l '^\s*"github\.com/RVRTelecomunicaciones/sophia-cli/internal/adapters' \
     "$d" | grep -v "$pkg"
done
```

Verified clean at M8 tag (`m8-attach-changes-status`). CI does NOT enforce
these greps automatically yet — that is a candidate for M10+.

## References

- Alistair Cockburn, "Hexagonal Architecture" (2005).
- `internal/ports/outbound/orchestrator.go` — canonical outbound port shape.
- `internal/application/runner.go` — canonical use case using only ports.
- `internal/bootstrap/wire.go` — the composition root.
- M8 plan, "Architecture" section, for the `Runner.Observe` extraction
  rationale.
