---
title: Sophia CLI — V1 Design
date: 2026-05-05
status: Draft (awaiting user review)
version: 1.1.0-spec
authors:
  - rfactperu@gmail.com
related:
  - ../../../../sophia-orchestator/docs/superpowers/specs/2026-05-03-sophia-orchestator-design.md
  - sibling project agent-governance-core
  - sibling project sophia-memory-engine
  - sibling project sophia-runtime-adapters
research:
  - charmbracelet/bubbletea v2 (charm.land/bubbletea/v2)
  - charmbracelet/lipgloss v2 (charm.land/lipgloss/v2)
  - charmbracelet/bubbles v2 (charm.land/bubbles/v2)
  - tmaxmax/go-sse
  - spf13/cobra
  - obra/superpowers (v5.0.7)
  - OWASP LLM Top 10 (2026)
---

# Sophia CLI — V1 Design

## Executive Summary

`sophia-cli` is the human entry point of the Sophia ecosystem. It is a Go CLI
that lets a developer create and observe SDD **Changes** executed by
`sophia-orchestator`, on a local workspace. V1 ships a **vertical slice**: from
`sophia init` through `sophia run "msg"` to a Bubble Tea TUI rendering the nine
SDD phases in real time, plus a non-interactive JSONL mode for CI.

The CLI is intentionally minimal: it does not coordinate phases, does not
evaluate policy, does not execute side effects, and does not store the
canonical state of a Change. Its tagline:

> **El CLI no es autoridad de ejecución; es una interfaz local de creación,
> observación y recuperación.**

V1 includes nine commands (`version`, `init`, `doctor`, `start`, `stop`, `run`,
`attach`, `status`, `changes`), a Timeline + ApplyBoard TUI, a passive approval
banner that points to governance-issued URLs, an SSE event consumer with
reconnect, and a security model focused on a tight filesystem/network/subprocess
boundary plus best-effort secret redaction. Context7, MCP, model profiles,
memory search, and remote endpoints are deferred to V1.1+.

Architecture follows the same hexagonal layout as the four sibling services
(`sophia-orchestator`, `agent-governance-core`, `sophia-memory-engine`,
`sophia-runtime-adapters`). All ports are designed so the use cases (`run`,
`attach`, `status`, `doctor`) can be tested without Docker, network, or a real
orchestrator.

## Glossary

| Term | Definition |
|---|---|
| **Change** | The SDD unit of work owned by `sophia-orchestator`. Has 9 canonical phases. The CLI never owns Changes; it creates and observes them. |
| **Phase** | One of `init → explore → proposal → spec → design → tasks → apply → verify → archive`. |
| **Mantra** | The five-line ecosystem boundary statement (see §1.1). |
| **EventSink** | The single inbound port that abstracts how events are rendered. The TUI and the JSONL writer are two implementations. |
| **ApprovalGate** | A governance-issued URL the CLI displays passively when a phase needs approval. |
| **ApplyBoard** | Sub-aggregate of phase `apply`: groups + tasks coordinated by team-lead agents. |
| **Fingerprint** | 16-hex-char SHA-256 digest of `(project_name, repo_root, remote_url)` used to scope local state. |
| **Vertical slice** | The chosen V1 scope: end-to-end `sophia run` + `sophia attach`, deferring Context7, profiles, MCP, and `mem`. |

---

## 1. Boundaries

### 1.1. Mantra (boundaries of the Sophia ecosystem)

> Sophia CLI **crea y observa** Changes.
> Sophia Orchestrator **ejecuta** las 9 fases SDD.
> Governance **decide** `allow | allow_with_constraints | require_approval | deny`.
> Runtime-adapters **ejecutan** acciones reales.
> Memory-engine **conserva** contexto, decisiones y heurísticas.

> El CLI no es autoridad de ejecución; es una interfaz local de creación,
> observación y recuperación.

### 1.2. What the CLI DOES

- Provision the local stack via Docker Compose (`sophia start/stop`).
- Diagnose the local environment (`sophia doctor`).
- Inspect the repo to compute a project fingerprint and discover `Project`,
  `BaseRef`.
- Create Changes via `POST /api/v1/changes`.
- Subscribe to per-phase SSE streams and render progress (Timeline + ApplyBoard).
- Show approval gates as passive banners with the governance-issued URL.
- Emit events as JSON lines for CI (`--no-tui --json`).
- Persist user/project local config and per-project last_change_id under XDG
  paths.

### 1.3. What the CLI DOES NOT do (V1)

- It does **not** decide phase routing, agent role, or strategy.
- It does **not** approve/reject — that decision belongs to governance and is
  resolved through governance's UI.
- It does **not** call governance or memory-engine directly. Everything goes
  through `sophia-orchestator`.
- It does **not** fetch documentation via Context7 (V1.1).
- It does **not** manage model profiles (V1.2).
- It does **not** run `sophia mem search` (V1.2).
- It does **not** write inside the user's repo, except `.sophia.yaml` created
  exclusively by `sophia init`.

### 1.4. Decisions baked into V1

- `sophia attach <change-id>` is in V1: it does `GET /api/v1/changes/{id}` for
  a snapshot and then subscribes to SSE. Cost is marginal because `attach` and
  `run` share the SSE/TUI pipeline.
- `Q` in the TUI = **detach** explicitly. It does not cancel the Change, does
  not stop the orchestrator, does not kill agents. It only closes the local
  view.

---

## 2. Commands V1

### 2.1. Set of nine commands

| Command | Signature | Effect | Output |
|---|---|---|---|
| `sophia version` | — | Prints version + commit + build date. | stdout |
| `sophia init` | `[--project NAME] [--base-ref REF] [--artifact-store MODE] [--force]` | Writes `.sophia.yaml` at the resolved repo root. | file |
| `sophia doctor` | `[--json] [--fix]` | Runs nine read-only checks (see §6). | stdout / JSON |
| `sophia start` | `[--reset-compose]` | `docker compose -p sophia ... up -d` from embedded compose. | stdout |
| `sophia stop` | — | `docker compose -p sophia ... down`. | stdout |
| `sophia run` | `"msg" [--no-tui] [--json] [--artifact-store MODE]` | Creates a Change and observes it. | TUI or JSONL |
| `sophia attach` | `<change-id> [--no-tui] [--json]` | GET snapshot + SSE stream. | TUI or JSONL |
| `sophia status` | `[<change-id>] [--json]` | Shows status of a Change (default: last project-scoped). | stdout / JSON |
| `sophia changes` | `[--limit N] [--status S] [--project P] [--json]` | Lists recent Changes. | stdout / JSON |

### 2.2. Critical flows

**`sophia run "msg"`** — vertical slice end-to-end:

```
1. Read .sophia.yaml (fail with exit 3 if missing/invalid).
2. Compute Change Name (slug from message + short random suffix).
3. Verify orchestrator reachable (1 fast healthz ping).
4. POST /api/v1/changes
   body: { name, project, base_ref, artifact_store_mode }
   response: { change_id, ... }
5. Persist change_id to <state>/projects/<fingerprint>/last_change_id and to
   <state>/last_change_id (global fallback). Atomic write.
6. If --no-tui: stream SSE events as JSON lines to stdout, exit when terminal
   status is reached. Otherwise, start the Bubble Tea program with Timeline +
   ApplyBoard views.
7. Subscribe to SSE for the current phase; on phase completion, GET snapshot
   and re-subscribe to the next active phase.
```

**`sophia attach <change-id>`** — snapshot + stream:

```
1. GET /api/v1/changes/{change-id} → full snapshot (phases + statuses).
2. Render snapshot as initial state in the TUI (or emit snapshot event if
   --no-tui --json).
3. GET the current running phase and subscribe to its SSE stream.
4. Apply incremental events on top of the snapshot.
5. When a phase ends, refresh the snapshot and re-subscribe to the next phase.
```

**`Q` in the TUI = detach** (explicit invariant):

- Closes the local view.
- Does not cancel the Change.
- Does not stop the orchestrator.
- Does not kill agents.
- Prints: `Detached. Reattach with: sophia attach <change-id>`.

**`Ctrl+C` in the TUI**:

- First press: prompts confirmation `Detach? (y/n)`.
- Second press: detaches immediately.
- Never cancels the Change in V1.

**Approval gate banner** (passive, no `[C]opy`):

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

The banner stays visible until either (a) `approval.resolved` is received, or
(b) a snapshot refresh shows the phase no longer blocked, or (c) any
forward-progress event arrives. The banner is **derived state**: the snapshot
is the source of truth.

### 2.3. Exit codes

| Code | Meaning |
|---|---|
| `0` | Change reached terminal status `DONE`. |
| `1` | Change ended `BLOCKED` or `FAILED`. |
| `2` | User-initiated detach in `--no-tui`. |
| `3` | Configuration error (`.sophia.yaml` missing/invalid, orchestrator unreachable, change not found). |
| `4` | Transient error (SSE reconnects exhausted, doctor crashed unexpectedly). |
| `5` | Approval timeout in `--no-tui` (waiting on a gate longer than `--approval-timeout`, default 30 min). |

### 2.4. `--no-tui --json` contract (CI mode)

- `stdout` is **machine-readable only**: a stream of JSON lines.
- Human/technical logs go to `stderr` AND to `<state>/logs/cli-YYYY-MM-DD.log`.
- Stream events use the shape:

```json
{"type":"snapshot","change_id":"01HX...","status":"running","current_phase":"explore","phases":[...]}
{"type":"event","ev":"phase.started","ts":"2026-05-05T14:23:01Z","payload":{...}}
{"type":"event","ev":"approval.required","ts":"...","payload":{"gate_url":"https://...","reason":"...","risk":"medium"}}
```

### 2.5. Defaults and resolution

`changes` filtering:

- Default `--limit 10`.
- If `.sophia.yaml` exists and is valid, default `--project` is the current
  project. If `.sophia.yaml` is invalid, log warning and list global (do not
  fail — `changes` is a recovery tool).

`status` resolution (no args):

1. If `.sophia.yaml` exists and a project-scoped `last_change_id` is found,
   GET that change.
2. Else, if a global `last_change_id` exists, GET it.
3. Else, print:
   ```
   No local change found.
   Use sophia changes or pass <change-id> explicitly.
   ```
   Exit 0 (empty state is not an error).

---

## 3. Local file contracts

### 3.1. XDG layout

Variables resolved by the CLI on every invocation:

```
configRoot = $XDG_CONFIG_HOME/sophia       (default: ~/.config/sophia)
stateRoot  = $XDG_STATE_HOME/sophia        (default: ~/.local/state/sophia)
dataRoot   = $XDG_DATA_HOME/sophia         (default: ~/.local/share/sophia)
cacheRoot  = $XDG_CACHE_HOME/sophia        (default: ~/.cache/sophia, reserved V1.1)
```

On macOS, when XDG variables are not set, the CLI **defaults to the same
Linux-style paths** (`~/.config/sophia` etc.) for cross-platform consistency.
This is documented in `--help` and `doctor` reports the resolved paths.

Permissions: directories `0700`, sensitive files `0600`, non-sensitive files
`0644`. `os.MkdirAll` is the only directory-creation primitive used.

### 3.2. `.sophia.yaml` (in repo, committable)

Created by `sophia init` at the **resolved repo root** (output of
`git rev-parse --show-toplevel`). If `sophia init` runs outside a git
repository, it fails with exit code 3 and a clear message — the CLI never
creates `.sophia.yaml` in an arbitrary CWD or subdirectory. Schema:

```yaml
version: 1
project: ms-cotizacion
base_ref: main
artifact_store: engram   # engram | openspec | hybrid | none
```

Rules:

- `project` is required (slug).
- `base_ref` defaults to `main`; override with `sophia init --base-ref`.
- `artifact_store` defaults to `engram`; override with `sophia init --artifact-store`.
- Unknown fields generate a warning, not an error (forward-compatible).
- `sophia init` is idempotent with confirmation: if `.sophia.yaml` exists,
  abort unless `--force`.
- Invalid `.sophia.yaml`:
  - `run` / `status`: fail with exit 3.
  - `init`: warn and require `--force` to overwrite.
  - `changes`: warn and fall back to global listing.
- The YAML key `artifact_store` maps to the request body field
  `artifact_store_mode`.

### 3.3. `config.yaml` (user-level, lazy-created)

Path: `<configRoot>/config.yaml`. Created lazily by the first command that
needs it. Schema:

```yaml
version: 1
orchestrator:
  url: http://localhost:8080
  timeout_seconds: 30
ui:
  default_view: timeline   # reserved V1.1
  detach_on_q: true        # reserved V1.1
```

V1 honors only `orchestrator.*`. The `ui.*` block is reserved and ignored.
`sophia doctor` is **read-only by default** and does not create this file;
`sophia doctor --fix` may create it with defaults if missing.

### 3.4. Configuration precedence

From highest to lowest:

1. CLI flags (`--orchestrator-url`, `--project`, `--base-ref`, `--artifact-store`).
2. Environment variables (`SOPHIA_ORCHESTRATOR_URL`, `SOPHIA_PROJECT`,
   `SOPHIA_BASE_REF`).
3. `.sophia.yaml` (project-level: `project`, `base_ref`, `artifact_store`).
4. `config.yaml` (user-level: `orchestrator.*`).
5. Built-in defaults.

`sophia doctor` reports which layer provided each value.

### 3.5. Project fingerprint and last_change_id

```
fingerprint = hex(sha256( project_name + "\x00" + repo_root + "\x00" + remote_url ))[:16]
```

Where:

- `project_name` comes from `.sophia.yaml`.
- `repo_root` = `git rev-parse --show-toplevel`, normalized through
  `filepath.Abs` → `filepath.Clean` → `filepath.EvalSymlinks` (with tolerant
  error handling: if `EvalSymlinks` fails, fall back to the previous value).
- `remote_url` = `git config --get remote.origin.url`, or `""` if absent.

Storage layout:

```
stateRoot/
├── last_change_id                          # global fallback
└── projects/
    └── <fingerprint>/
        ├── last_change_id
        └── meta.json                       # { project, repo_root, remote_url, fingerprint, created_at }
```

`run` and `attach` update both files. Atomic write: write to `last_change_id.tmp`
and `os.Rename`.

### 3.6. Compose materialization

The `compose.yaml` is **embedded into the binary** via `//go:embed`. On every
`sophia start`, the CLI compares the embedded SHA-256 against the materialized
file at `<dataRoot>/compose/compose.yaml`:

- If they match, do nothing.
- If they differ and the materialized file matches the previous embedded hash
  (recorded in `<dataRoot>/compose/compose.meta.json`), back up to
  `compose.yaml.previous` and rewrite.
- If they differ and the file appears manually edited, abort with a request to
  use `sophia start --reset-compose`.

The compose project name is **always `sophia`**, independent of CWD:

```
docker compose -p sophia -f <dataRoot>/compose/compose.yaml up -d
```

In V1 the embedded compose is a **dev-only stub** marked with labels:

```yaml
labels:
  sophia.stack: "dev"
  sophia.profile: "stub"
```

It includes the orchestrator (real if its release is ready, stub otherwise)
plus minimal mocks for governance, memory-engine, and runtime-adapters so the
end-to-end flow runs even before sibling services are release-ready.

### 3.7. Error messages

Errors must always state **what happened**, **why**, and **how to fix**:

```
$ sophia run "agregar refresh token"
Error: .sophia.yaml not found in /home/user/repo or ancestors
Run `sophia init` first.
exit 3
```

```
$ sophia run "..."
Error: orchestrator unreachable at http://localhost:8080
Try: sophia start && sophia doctor
exit 3
```

```
$ sophia attach abc123
Error: change abc123 not found
Run `sophia changes` to list recent.
exit 3
```

---

## 4. Internal architecture

Hexagonal Go, identical layout to the four sibling services. The single
non-negotiable rule: **the orchestration logic does not know whether it is
painting a TUI or emitting JSONL — both are adapters of the same port**.

### 4.1. Package layout

```
sophia-cli/
├── cmd/
│   └── sophia/                    # main; thin: parse flags, call bootstrap.New
├── internal/
│   ├── bootstrap/                 # composition root: wiring of adapters → services
│   │   ├── wire.go                # New(cfg) (*App, error)
│   │   ├── version.go             # ldflags injection: version, commit, date
│   │   └── logger.go              # slog setup
│   │
│   ├── domain/                    # PURE: entities, value objects, errors. No external imports.
│   │   ├── change.go              # Change, ChangeID, ChangeStatus
│   │   ├── phase.go               # Phase, PhaseType, PhaseStatus (the 9 phases)
│   │   ├── event.go               # Event { Type, Timestamp, Payload, TraceID }
│   │   ├── approval.go            # ApprovalGate { URL, Reason, Risk, Policy, ChangeID, Phase, TraceID }
│   │   ├── config.go              # ProjectConfig, UserConfig — pure types
│   │   ├── fingerprint.go         # Fingerprint, Compute(name, root, remote) Fingerprint
│   │   └── errors.go              # ErrConfigMissing, ErrChangeNotFound, ErrUnreachable, ErrInvalidYAML
│   │
│   ├── application/               # use cases. No Cobra, no Bubble Tea.
│   │   ├── runner.go              # RunChange
│   │   ├── attacher.go            # AttachChange
│   │   ├── status.go              # ResolveStatus
│   │   ├── lister.go              # ListChanges
│   │   ├── doctor.go              # RunDiagnostics
│   │   ├── initializer.go         # InitProject
│   │   └── provisioner.go         # Start / Stop
│   │
│   ├── ports/
│   │   ├── inbound/
│   │   │   └── eventsink.go       # EventSink interface (TUI and JSONL implementations)
│   │   │
│   │   └── outbound/
│   │       ├── orchestrator.go    # OrchestratorClient
│   │       ├── eventstream.go     # EventStreamClient with StreamTarget
│   │       ├── compose.go         # ComposeRunner
│   │       ├── git.go             # GitInspector
│   │       ├── projectconfig.go   # ProjectConfigStore
│   │       ├── userconfig.go      # UserConfigStore
│   │       ├── statestore.go      # StateStore
│   │       ├── browser.go         # Browser
│   │       └── clock.go           # Clock
│   │
│   ├── adapters/
│   │   ├── inbound/
│   │   │   ├── cli/               # Cobra commands
│   │   │   ├── tui/               # Bubble Tea v2 program
│   │   │   └── jsonsink/          # EventSink that writes JSON lines to stdout
│   │   │
│   │   └── outbound/
│   │       ├── orchestratorhttp/  # net/http client
│   │       │   ├── client.go
│   │       │   ├── dto.go         # YAML/JSON DTOs distinct from domain types
│   │       │   └── errors.go
│   │       ├── ssestream/         # tmaxmax/go-sse
│   │       │   ├── client.go
│   │       │   ├── reconnect.go
│   │       │   ├── parser.go
│   │       │   └── redactor.go    # redaction pipeline pre-EventSink
│   │       ├── composeexec/
│   │       │   ├── runner.go
│   │       │   └── embed.go       # //go:embed compose.yaml
│   │       ├── gitcli/
│   │       │   └── inspector.go
│   │       ├── yamlconfig/
│   │       │   ├── project.go     # YAML DTOs live here, not in domain
│   │       │   ├── user.go
│   │       │   └── dto.go
│   │       ├── filestate/
│   │       │   └── store.go
│   │       ├── osbrowser/
│   │       │   └── browser.go
│   │       └── stdclock/
│   │           └── clock.go
│   │
│   └── infrastructure/
│       ├── httpclient/
│       └── logging/
│
├── compose.yaml                    # source of truth, embedded at build time
├── docs/
│   ├── adr/
│   └── superpowers/specs/
├── api/                            # reserved
├── test/
│   ├── e2e/
│   ├── integration/
│   └── fixtures/
├── go.mod
└── Makefile
```

### 4.2. The `EventSink` port

```go
// internal/ports/inbound/eventsink.go
package inbound

import (
    "context"

    "github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

type EventSink interface {
    OnSnapshot(ctx context.Context, change *domain.Change) error
    OnEvent(ctx context.Context, ev domain.Event) error
    OnApprovalGate(ctx context.Context, gate domain.ApprovalGate) error
    OnError(ctx context.Context, err error) error
    OnComplete(ctx context.Context, finalStatus domain.ChangeStatus) error
    Close() error
}
```

`application.RunChange` and `application.AttachChange` push events to a sink
without knowing which one is wired. `bootstrap` decides at startup based on
flags (`--no-tui`, `--json`).

### 4.3. Outbound ports — testable without Docker or network

| Port | Production adapter | Test fake |
|---|---|---|
| `OrchestratorClient` | `orchestratorhttp` (net/http) | in-memory map |
| `EventStreamClient` | `ssestream` (tmaxmax/go-sse) | `chan domain.Event` |
| `ComposeRunner` | `composeexec` (`exec.Command`) | log-only |
| `GitInspector` | `gitcli` | hard-coded values |
| `ProjectConfigStore` | `yamlconfig` | `afero.MemMapFs` |
| `UserConfigStore` | `yamlconfig` | `afero.MemMapFs` |
| `StateStore` | `filestate` | `afero.MemMapFs` |
| `Browser` | `osbrowser` | record-only |
| `Clock` | `stdclock` | manual fake |

`afero` is used **only in tests**. Production code uses `os`, `io/fs`,
`path/filepath` directly.

### 4.4. EventStreamClient flexibility

```go
type StreamTarget struct {
    ChangeID ChangeID
    PhaseID  string // optional; if empty, subscribe to whatever the backend exposes for the Change
}

type SubscribeOptions struct {
    LastEventID string
}

type EventStreamClient interface {
    Subscribe(ctx context.Context, target StreamTarget, opts SubscribeOptions) (<-chan domain.Event, func() error, error)
}
```

V1 uses per-phase subscription (the only contract documented today). If the
backend later exposes a per-Change global stream, the CLI can switch without
changing the use cases.

### 4.5. SSE → TUI bridge with backpressure

The Bubble Tea bridge is implemented as an `EventSink` whose `OnEvent` calls
`program.Send`. The bridge holds a buffered channel of capacity 256:

- `heartbeat` is dropped first under sustained pressure.
- `agent.*` and `task.*` may be visually compacted if the queue fills.
- `phase.*` and `approval.*` are **never dropped**.
- Drops are logged with a counter exposed in the JSONL log.

We do not assert that `program.Send` is non-blocking; we provide our own
buffering and discard policy.

### 4.6. Technical choices

- **Logging**: `log/slog` (stdlib). JSONL handler. No zap, no zerolog.
- **HTTP**: `net/http` + `context.Context` everywhere.
- **YAML**: `gopkg.in/yaml.v3`, decoder `KnownFields(false)`, max input size
  100 KB for `.sophia.yaml`.
- **Hash**: `crypto/sha256`.
- **TUI**: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`,
  `charm.land/bubbles/v2`. Pin exact versions in `go.mod`. Note v2 breaking
  changes documented in ADR-0002.
- **SSE**: `github.com/tmaxmax/go-sse`.
- **CLI**: `github.com/spf13/cobra`.
- **Tests**: stdlib `testing` + `github.com/stretchr/testify/require` +
  `github.com/spf13/afero` (test-only).

---

## 5. Backend contracts

### 5.1. HTTP endpoints consumed by V1

| CLI command | HTTP call | Body / Query |
|---|---|---|
| `sophia run` | `POST /api/v1/changes` | `{ name, project, base_ref, artifact_store_mode }` |
| `sophia attach <id>` | `GET /api/v1/changes/{id}` then SSE | — |
| `sophia status [<id>]` | `GET /api/v1/changes/{id}` | — |
| `sophia changes` | `GET /api/v1/changes` | `?project=&status=&limit=10&offset=0` |
| `sophia doctor` (ping) | `GET /api/v1/healthz` | — *(verify exact path at M1)* |

V1 does not call `/abort`, `/phases/run`, `/phases/:id/resume`,
`/phases/:id/approve`, `/phases/:id/reject`, or `/phases/:id/board`.

### 5.2. Phase progression assumption (auto_advance)

> V1 assumes orchestrator-owned phase progression.
>
> If M1 proves phase execution is client-triggered, the CLI will add a thin
> `RunPhase` loop without taking ownership of orchestration decisions. This
> compatibility mode lets the CLI call `/phases/run` as a forward-progress
> signal, but it does not pick the next phase, does not interpret confidence,
> does not evaluate envelopes, and does not decide policy. The orchestrator
> remains the source of truth.

If the fallback is triggered, it is logged as **technical debt** to be removed
when the backend supports native auto_advance:

> **Debt**: phase-trigger compatibility mode must be removed when the
> orchestrator supports `auto_advance` natively.

Polling cadence (between phases when `current_phase_id` is `null`): 1s default,
exponential backoff to 5s. Cancelable by `Ctrl+C` or `Q`.

### 5.3. SSE event schema — tolerant parser

Wire format (standard SSE):

```
event: phase.started
id: 01HXYZ...
data: {"timestamp":"2026-05-05T14:23:01.234Z","payload":{...},"trace_id":"..."}

```

Parser rules in `adapters/outbound/ssestream/parser.go`:

1. The SSE `event:` field discriminates the type (maps to `domain.Event.Type`).
2. The SSE `id:` field is `Last-Event-ID` for reconnect.
3. `data:` is parsed as JSON; on parse failure, log warning and skip.
4. Unknown event types: log debug and skip (forward-compatible).
5. `heartbeat` updates the "last seen" timestamp but is not forwarded to the
   render path.

### 5.4. Known/expected V1 events

Verified against the orchestrator port comment block; final names are confirmed
during M1.

| Type | Meaning | Expected payload |
|---|---|---|
| `phase.started` | A phase started | `{ phase_type, phase_id, task_description }` |
| `phase.completed` | A phase ended | `{ phase_type, phase_id, status, confidence }` |
| `phase.blocked` | Phase blocked | `{ phase_type, phase_id, reason }` |
| `agent.spawned` | An AgentSession was spawned | `{ agent_role, agent_id, group_id?, task_id? }` |
| `agent.completed` | AgentSession finished | `{ agent_id, status }` |
| `task.started` | Apply task started | `{ group_id, task_id, files_pattern }` |
| `task.completed` | Apply task ended | `{ group_id, task_id, status }` |
| `approval.required` | Governance issued an approval gate | `{ gate_url, reason, risk, policy, phase }` |
| `approval.resolved` | Governance resolved the gate | `{ decision, resolved_by }` |
| `heartbeat` | Keep-alive | `{}` |

Tolerance policy:

- Unknown types do not break the CLI.
- Incomplete payload does not break the CLI; missing fields default to `""`.
- Corrupt event: warn and skip.
- `heartbeat`: updates `last_seen`, not rendered.
- `phase.*` and `approval.*`: never dropped.
- `agent.*` and `task.*`: may be visually compacted under sustained queue
  pressure.

### 5.5. ApprovalGate domain object

```go
type ApprovalGate struct {
    URL      string
    Reason   string
    Risk     string  // "low" | "medium" | "high" | "" if unknown
    Policy   string
    ChangeID ChangeID
    Phase    PhaseType
    TraceID  string
}
```

The TUI banner is **derived state**: it appears on `approval.required`, and
disappears when any of the following occurs:

- `approval.resolved` arrives.
- A snapshot refresh shows the phase is no longer blocked.
- Any forward-progress event arrives (`phase.started` for a later phase, etc.).

The snapshot is the source of truth.

### 5.6. Schema versioning

The CLI inspects header `X-Sophia-Schema-Version` on the SSE response:

- **Missing**: assume `v1`, log debug, continue.
- **Greater than supported**: log warning, continue in tolerant mode.
- **Incompatible**: V1 only warns. Hard blocking will be added when the first
  real breaking change ships.

### 5.7. Reconnect, backoff, heartbeat

| Situation | Behavior |
|---|---|
| Transient TCP/TLS error | Reconnect with `Last-Event-ID`, exponential backoff 1s → 30s, max 5 retries per phase |
| 5 retries fail | Emit `OnError` to `EventSink`; exit 4 in `--no-tui` |
| HTTP 404 (phase not found) | Probable race; refresh snapshot and re-subscribe |
| HTTP 401/403 | V1 has no auth; this indicates a backend bug; log error and abort |
| No heartbeat for 60s | Force reconnect |
| Server graceful close | Refresh snapshot to determine if the change is done or continues |

### 5.8. Timeouts

| Operation | Timeout |
|---|---|
| `POST /changes` | 30s |
| `GET /changes/{id}` | 10s |
| `GET /changes` (list) | 10s |
| SSE handshake | 15s |
| SSE total per phase | unbounded |
| Doctor healthz ping | 5s |
| Approval pending in `--no-tui` | 30 min default; `--approval-timeout=DURATION` overrides; exit 5 on expiry |

---

## 6. Doctor and security model

### 6.1. Doctor checks (V1)

`sophia doctor` is **read-only with respect to the user's repository, the
project and user config files, the materialized compose, and the running
services**. It may still write its own diagnostic and audit lines under
`<stateRoot>/logs/` (consistent with the filesystem boundary in §6.3).

With `--fix` it may additionally create missing XDG directories, materialize
the embedded compose, and create a default `config.yaml`. It never modifies
the user's repository, never edits `.sophia.yaml`, never writes credentials,
and never runs `docker compose up` (that is `sophia start`).

| # | Check | Pass criteria | Failure handling |
|---|---|---|---|
| 1 | Docker installed | `docker version` exit 0, daemon responds | fail |
| 2 | Docker Compose v2 | `docker compose version` major ≥ 2 | fail |
| 3 | Git installed | `git --version` exit 0 | fail |
| 4 | Repository detected | `git rev-parse --show-toplevel` exit 0 | warn |
| 5 | `.sophia.yaml` valid | parses, schema valid, project non-empty | warn if missing; fail if invalid |
| 6 | XDG paths | exist or `--fix` creates; perms `0700` | fail if perms wrong; warn if missing without `--fix` |
| 7 | Orchestrator reachable | `GET /api/v1/healthz` < 5s | fail |
| 8 | SSE handshake | open dummy SSE, await one frame, close | warn (V1: until backend dummy endpoint is documented) |
| 9 | Working tree status | `git status --porcelain` clean vs. dirty | info only |

XDG path probe: in default mode, the CLI verifies existence and permissions on
existing paths. It does not write probe files. Write/delete probing happens
only with `--fix` after creation, or implicitly when paths already exist and
are owned by the user.

Output formats:

- Default (TTY): pretty-printed with status icons.
- `--json`: machine-readable.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | No fails (warns/info OK) |
| `3` | At least one fail (preserved under `--json`) |
| `4` | Doctor itself crashed |

### 6.2. Security model V1

The slice is local-only, no auth, no network beyond localhost orchestrator,
no Context7. This narrows but does not eliminate the surface. OWASP LLM
Top 10 (2026) maps to V1 as follows:

| OWASP risk | Applies to V1? | Mitigation in V1 | Deferred |
|---|---|---|---|
| Prompt injection | Indirect: repo files travel to the orchestrator via agents | The CLI does not process repo contents; orchestrator/governance handle it | — |
| Insecure output handling | Yes: SSE payloads → TUI/JSONL | Secret redaction + ANSI escape in TUI rendering | V1 |
| Supply chain | Yes: Cobra, Bubble Tea v2, go-sse, yaml.v3 | Pinned `go.mod`, `go.sum` committed, dep review at M1 | V1 (partial) |
| Insecure plugin (MCP) | No | — | V1.1 |
| Excessive agency | The CLI executes only `docker compose` and `git` with fixed args | Subprocess invariant (§6.3) | V1 |

### 6.3. Hard invariants V1

These are **invariants**: enforced by tests and reviewed in code review.

1. **Filesystem boundary**. The CLI writes only to:
   - XDG paths (`configRoot`, `stateRoot`, `dataRoot`).
   - `.sophia.yaml` at the user's resolved repo root, **only** from
     `sophia init`.
   - `<stateRoot>/logs/`.
   - **Never** in `.git/`, source files, `/etc`, `/usr`, system paths.

2. **Network boundary**. V1 connects only to:
   - `config.orchestrator.url` (default `localhost:8080`).
   - **Nothing else**. Approval gate URLs are displayed textually; `[O]pen`
     opens them through the OS browser opener (`xdg-open`/`open`/`start`),
     never via the CLI's HTTP client.

3. **Subprocess boundary**. V1 executes only:
   - `docker` with fixed args (`compose -p sophia -f <data-path>/compose.yaml ...`).
   - `git` with fixed args (`rev-parse`, `config --get remote.origin.url`,
     `status --porcelain`, `symbolic-ref`).
   - Browser opener with a **validated URL** argument (must be `http(s)://...`,
     reject `javascript:`, `data:`, `file:`, `vbscript:`).
   - **Never** commands derived from orchestrator output or repo content.

4. **Secret redaction** (best-effort). Applied **before** the `EventSink`,
   inside the SSE pipeline `parser → normalizer → redactor → domain.Event`.
   Patterns:
   - `Bearer\s+[A-Za-z0-9._\-+/=]+`
   - JWT-shaped triplets
   - `AKIA[0-9A-Z]{16}` (AWS keys)
   - `gh[pousr]_[A-Za-z0-9]{36,}` (GitHub tokens)
   - High-entropy patterns **only when the surrounding field name suggests a
     secret** (`token`, `secret`, `key`, `authorization`, `password`,
     `credential`) **or** in free-form log messages. Not applied to technical
     payload fields.

   Replacement is `[REDACTED]`. Length is not preserved unless it aids
   debugging without exposing the secret.

5. **URL validation for `[O]pen`**:
   - Schema must be `http` or `https`.
   - Reject `javascript:`, `data:`, `file:`, `vbscript:`, etc.
   - On parse failure: log error, do not open, show error in TUI.

6. **YAML safety**:
   - `yaml.v3` decoder with `KnownFields(false)`.
   - Max input size for `.sophia.yaml`: 100 KB (reject larger).
   - Recursive aliases: `yaml.v3` detects them; reject.

7. **No execution of SSE payload content**. The TUI renders strings with
   strict ANSI escaping. `lipgloss` does not evaluate input.

### 6.4. Local audit log

Each `sophia run`/`attach`/`start`/`doctor` writes a JSONL line to
`<stateRoot>/logs/cli-YYYY-MM-DD.log`:

```json
{"ts":"2026-05-05T14:23:01-05:00","cmd":"run","change_id":"abc123","project":"ms-cotizacion","fingerprint":"a1b2c3d4...","exit":0,"duration_ms":1234567}
```

Not audit-grade (mutable, unsigned), but enables post-mortem when bugs are
reported. Rotation: one file per day, no compression, manual purge by the
user (V1.1 will add `sophia logs prune`).

Logging is also produced under `--no-tui --json`:

- `stdout`: machine-readable JSON lines (events).
- `stderr`: human-readable technical logs.
- `<stateRoot>/logs/cli-YYYY-MM-DD.log`: JSONL technical logs (always).

### 6.5. What is NOT in V1 (security)

- Auth / token storage (V1.2 with remote endpoints).
- TLS pinning (V1.2).
- Code signing of release binaries (V1.1).
- Auto-checksum verification on update (V1.1).
- MCP trust registry (V1.2).
- Permission profiles (`read-only`, `workspace-write`, `network-on-ask`) (V1.2).
- User-facing filesystem sandboxing (V2).

V1 ships with **SHA-256 checksums** for release artifacts; signing is V1.1.

---

## 7. Milestones and Definition of Done

Rule: **every milestone ends with something executable**, not just plumbing.

### 7.1. Milestone summary

| # | Milestone | Newly executable | Depends on |
|---|---|---|---|
| **M1** | Foundation | `sophia version`, `sophia doctor` (partial: docker/git/xdg) | — |
| **M2** | Provisioning | `sophia start`, `sophia stop`; `doctor` adds orchestrator + SSE handshake (warn) | M1 |
| **M3** | Project & state | `sophia init`, `sophia status` placeholder (local-only), state project-scoped | M1 |
| **M4** | Run via polling (scaffolding) | `sophia run "msg" --no-tui --json` with polling GET, no SSE yet | M2, M3 |
| **M5** | SSE upgrade | `run --no-tui --json` switches to SSE; reconnect + redaction | M4 |
| **M6** | TUI Timeline | `sophia run "msg"` opens Bubble Tea Timeline view, `Q` detaches | M5 |
| **M7** | ApplyBoard + approval | second view via `Tab`, passive approval banner with `[O]pen` | M6 |
| **M8** | Attach + changes | `sophia attach <id>`, `sophia changes` real, `sophia status` against orchestrator | M7 |
| **M9** | Hardening + release | binary v0.1.0, full e2e against orchestrator, security suite, GoReleaser, asciinema demo | M8 |

### 7.2. Per-milestone Definition of Done

**M1 — Foundation**

- Hexagonal layout fully created.
- `slog` JSONL handler writes to `<stateRoot>/logs/cli-YYYY-MM-DD.log`.
- Cobra root + `version` + `doctor` (other subcommands stubbed with
  "not implemented").
- Test harness with fakes for all outbound ports.
- `golangci-lint` and `go vet` green in CI.
- `Makefile`: `build`, `test`, `lint`, `run-doctor`.
- ≥ 70% coverage in `internal/domain` and `internal/application`.

**M2 — Provisioning**

- `compose.yaml` embedded, materialized with hash check, `.previous` backup,
  `--reset-compose`.
- `sophia start` / `sophia stop` operate the local stack with project name
  `sophia` regardless of CWD.
- Compose stub is labeled `sophia.stack: dev`, `sophia.profile: stub`.
- `doctor` adds orchestrator reachability (fail) and SSE handshake (warn in V1).
- Unit tests with `composeexec` fake; opt-in integration test
  (`make test-integration`) that uses Docker.

**M3 — Project & state**

- `sophia init` writes `.sophia.yaml` with `--project`, `--base-ref`,
  `--artifact-store`, `--force`. `init --force` repairs invalid YAML;
  `run`/`status` fail on invalid YAML; `changes` falls back to global.
- Fingerprint computation with `filepath.Abs`/`Clean`/tolerant `EvalSymlinks`.
- `filestate.Store` with atomic write.
- `yamlconfig.Project.Read/Write/Find` walks ancestors.
- `sophia status` resolves `last_change_id` (project-scoped → global → empty
  message).
- M3 `status` is a placeholder: it reads local state but does not yet contact
  the orchestrator. Real `status` ships in M8.
- Tests use `afero.MemMapFs`.

**M4 — Run via polling (scaffolding)**

- `orchestratorhttp.Client` implements `OrchestratorClient` (POST/GET/List).
- `application.RunChange` orchestrates: read `.sophia.yaml` → POST create →
  loop GET snapshot until terminal status.
- Polling interval default 1s, max 5s, configurable.
- JSON sink emits snapshots as JSONL (no SSE events yet).
- Exit codes 0/1/3/4 enforced.
- Unit tests with `fakeOrchestrator`.
- **First e2e test against a real orchestrator validates the `auto_advance`
  assumption**. If it fails, M4 adds the compatibility mode described in
  §5.2 (delegated `RunPhase` loop) without taking routing decisions in the CLI.
- Polling is explicitly scaffolding; the real V1 behavior arrives in M5.

**M5 — SSE upgrade**

- `ssestream.Client` with `tmaxmax/go-sse`.
- Reconnect + Last-Event-ID + exponential backoff (1s → 30s, max 5).
- 60s no-heartbeat → force reconnect.
- Tolerant parser.
- Redaction pipeline applied **before** events reach any sink.
- `fakeStream` in unit tests; e2e validates ordering against real orchestrator.

**M6 — TUI Timeline**

- Bubble Tea v2 + Lipgloss v2 with imports `charm.land/bubbletea/v2`,
  `charm.land/lipgloss/v2`. Versions pinned.
- Timeline view: 9 phases with status icons, current phase, duration,
  confidence (when known).
- SSE bridge with cap-256 buffer; drop policy honored (heartbeat first;
  phase/approval never).
- `Q` = detach immediately; `Ctrl+C` = first press confirms, second press
  detaches; never cancels in V1.
- Tests with `teatest` (v2 utilities).

**M7 — ApplyBoard + approval**

- Second view (`applyboard.go`): groups + tasks + team-leads visible,
  parallelism shown.
- Data sourced from `task.*` and `agent.*` events.
- Approval banner with `URL`, `Reason`, `Risk`, `Policy`, `ChangeID`, `Phase`,
  `TraceID`.
- `[O]` opens in browser via `osbrowser` after URL validation.
- Banner is derived state: hides on `approval.resolved`, snapshot
  refresh, or any forward-progress event.
- Golden snapshot tests for both views.

**M8 — Attach + changes**

- `application.AttachChange`: GET snapshot → render → SSE on current phase.
- `application.ListChanges` with filters (`project`, `status`, `limit`,
  `offset`).
- `sophia status` resolves project-scoped → global → empty.
- `run` and `attach` update both project-scoped and global `last_change_id`.
- e2e: `run` → `Q` → `attach` → completion.

**M9 — Hardening + release**

- GoReleaser config: builds for darwin/amd64, darwin/arm64, linux/amd64,
  linux/arm64.
- SHA-256 checksums on artifacts. (Code signing is V1.1.)
- GitHub Actions: `lint` + `test` + `e2e` + `release` (on tags).
- **Security suite** (`test/security/`):
  - Filesystem boundary: the CLI writes nowhere outside XDG + `.sophia.yaml`.
  - URL validation: vectors for `javascript:`, `data:`, `file:`, etc.
  - Redaction: corpus of known tokens.
  - YAML safety: bombs, recursive aliases, oversized files.
- **E2E suite** (`test/e2e/`):
  - `init → start → run → wait → Q → attach → wait completion`.
  - `--no-tui --json` produces a machine-parseable stream.
  - `doctor --json` parses correctly.
  - Approval flow: banner appears and disappears.
- README + CHANGELOG + LICENSE + ADR-0001 (architecture) + ADR-0002 (Bubble
  Tea v2 with breaking-changes notes).
- Asciinema demo (≤ 90 seconds): `init → doctor → run "..." → phase progress
  → Q detach → attach → completion`.

### 7.3. V1 done criteria

V1 is `v0.1.0` when:

1. M1–M9 DoD all met.
2. E2E suite passes against a real orchestrator (M9).
3. README documents installation + quickstart in under 10 minutes.
4. ADR-0001 and ADR-0002 written.
5. `sophia doctor` in a clean repo returns all green.
6. Asciinema demo recorded (≤ 90 s) and linked from README.

### 7.4. Risks per milestone

| Milestone | Risk | Plan B |
|---|---|---|
| M2 | Sibling services not yet release-ready | Stub them in compose with healthz mocks, labeled `sophia.profile: stub` |
| M4 | `auto_advance` assumption proves false | Prefer fixing the orchestrator. As temporary fallback, add compatibility-mode `RunPhase` loop without routing decisions, marked as debt |
| M5 | `tmaxmax/go-sse` incompatible with backend SSE shape | Fallback: `net/http` + `bufio.Scanner` (≈ 300 extra lines) |
| M6 | Bubble Tea v2 surprises | Versions pinned in M1; if v2 turns unstable, M6 pauses to resolve |
| M9 | Cross-compile fails on a platform | Release v0.1.0 with two platforms (darwin/arm64, linux/amd64); rest in v0.1.1 |

---

## 8. What is NOT in V1 (V1.1+)

- Context7 + MCP integration → V1.1
- `sophia profile` (model routing) → V1.2
- `sophia mem search` → V1.2
- `sophia gate open|status` separated from the TUI banner → V1.1
- `sophia cancel` → only when the backend exposes `/abort` and the user
  approves
- Auth / `sophia login` + remote endpoints → V1.2
- TLS pinning + code signing → V1.1
- Permission profiles (`read-only`, `workspace-write`, `network-on-ask`) → V1.2
- `sophia logs prune` → V1.1
- Additional TUI views (Diff, Logs, Summary, Agents) → V1.1+
- `sophia config get|set` → V1.1
- `sophia restart` → cosmetic, V1.2
- `sophia update` → V1.1 with optional auto-update

---

## 9. Open questions to verify at M1

These are tracked as M1 entry tasks. Each has a default behavior so V1 ships
even if the answer is delayed.

1. **Healthz endpoint name**. Spec assumes `GET /api/v1/healthz`; verify against
   orchestrator.
2. **Auto_advance semantics**. Spec assumes orchestrator-owned phase
   progression. M4 e2e test confirms; if false, compatibility mode applies
   (§5.2).
3. **Exact event type names**. Inferred from
   `sophia-orchestator/internal/ports/inbound/eventstream.go` comment block.
   Confirm the full list during M1.
4. **`approval.resolved` emission**. Spec accepts both worlds: explicit
   `approval.resolved` event, or implicit signal via snapshot refresh /
   forward-progress.
5. **`X-Sophia-Schema-Version` header**. Spec searches for it but tolerates
   absence.

---

## 10. Appendix — Decision log

These are the decisions cemented through brainstorming, recorded for future
reference.

| # | Decision | Rationale |
|---|---|---|
| 1 | Vertical slice scope for V1 | Tests contracts end-to-end before widening |
| 2 | Local-only orchestrator endpoint, no auth | Removes complexity that does not serve V1 |
| 3 | Passive approval gates (display URL, no API call) | Respects the boundary that governance owns the decision |
| 4 | `tmaxmax/go-sse` for SSE | Maintained, has reconnect / Last-Event-ID built in |
| 5 | `sophia init` mandatory + `.sophia.yaml` | Eliminates ambiguity about project identity |
| 6 | Timeline + ApplyBoard for V1 TUI | 80% of `run` value with manageable surface area |
| 7 | `--no-tui --json` from V1 | Forces clean separation between event source and rendering |
| 8 | `sophia attach` in V1 | Marginal cost, large UX value |
| 9 | Compose embedded via `//go:embed` | No network at provisioning time, deterministic |
| 10 | Project fingerprint scopes `last_change_id` | Avoids cross-repo contamination |
| 11 | Bubble Tea v2 with `charm.land/bubbletea/v2` | Verified via context7 against the official upgrade guide |
| 12 | JSONL logs in V1 | Critical for bug reports; cheap to implement |
| 13 | Doctor read-only by default; `--fix` for limited mutations | Predictable in CI |
| 14 | Redaction before EventSink | Single chokepoint for all sinks (TUI + JSONL + log file) |
| 15 | Compose stub for M2 to unblock CLI development | Avoids dependency on sibling release readiness |
| 16 | Demo asciinema (≤ 90 s) is part of V1 done | UX is product, not an extra |

---

*Spec authored 2026-05-05. Awaiting user review before transitioning to
implementation plan via `superpowers:writing-plans`.*
