# Sophia CLI — V1 Implementation Roadmap (M1–M9)

> **For agentic workers:** This is the **roadmap**, not an executable plan. Each
> milestone has its own detailed plan generated only when its predecessor's
> Definition of Done is met. The next plan in line is
> `2026-05-05-sophia-cli-m1-foundation.md`.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md`
**Status:** Roadmap drafted 2026-05-05; M1 plan ready.
**Rule:** every milestone must end with something executable.

---

## Cross-cutting decisions (apply to every milestone)

| # | Decision | Rationale | Source |
|---|---|---|---|
| 1 | Go module path: `github.com/RVRTelecomunicaciones/sophia-cli` | Mirrors sibling services in `2026/` org | Sibling repos `agent-governance-core`, `sophia-orchestator`, `sophia-runtime-adapters`, `sophia-memory-engine` |
| 2 | Go version: 1.26.x (track `go.mod` toolchain) | Spec §0 cites 1.26.2; release line is current | go.dev/doc/go1.26 |
| 3 | Vocabulary: `Change`, never `Mission` | Backend dominio — verified at `sophia-orchestator/internal/domain/change` | Spec §1 + ports/inbound/change.go |
| 4 | Endpoints: only `POST /api/v1/changes`, `GET /api/v1/changes/{id}`, `GET /api/v1/changes`, plus a healthz path TBD M1 | Spec §5.1 | Spec |
| 5 | Out of V1: Context7, MCP, profiles, memory search, auth, remote endpoints | Spec §8 | Spec |
| 6 | Hexagonal layout: `cmd/`, `internal/{domain,application,ports,adapters,bootstrap,infrastructure}`, `test/` | Sibling convention | Spec §4.1 |
| 7 | Logging: stdlib `log/slog` + custom JSONL handler at `<stateRoot>/logs/cli-YYYY-MM-DD.log` | Spec §6.4 | Spec |
| 8 | Tests: `testing` stdlib + `github.com/stretchr/testify/require` + `github.com/spf13/afero` (test-only) | Spec §4.6 | Spec |
| 9 | TUI: `charm.land/bubbletea/v2` + `charm.land/lipgloss/v2` + `charm.land/bubbles/v2` (added at M6) | Verified via context7 against `charmbracelet/bubbles/UPGRADE_GUIDE_V2.md` | Engram memory `sophia-cli/decisions/bubbletea-v2` |
| 10 | SSE: `github.com/tmaxmax/go-sse` (added at M5) | Spec §4.6 | Spec |
| 11 | Cobra: `github.com/spf13/cobra` (added at M1) | Spec §4.6 | Spec |
| 12 | YAML: `gopkg.in/yaml.v3` (added at M3) | Spec §6.3 | Spec |
| 13 | Tests target ≥ 70% coverage in `internal/domain` and `internal/application` | Spec §7 M1 DoD | Spec |
| 14 | No code outside the project tree at `/Users/russell/Documents/2026/sophia-cli/` | User constraint + spec §6.3 | User instructions |
| 15 | No destructive operations (no `git push --force`, no `rm -rf`, no auto-commit on user behalf) | User constraint | User instructions |

---

## Milestone summary table

| # | Milestone | Newly executable | Net new files (approx) | Go deps added | Plan filename |
|---|---|---|---|---|---|
| **M1** | Foundation | `sophia version`, partial `sophia doctor` | ~45 | cobra, testify, afero | `2026-05-05-sophia-cli-m1-foundation.md` |
| **M2** | Provisioning | `sophia start/stop`, doctor adds orchestrator + SSE checks | ~8 | (none) | TBD after M1 done |
| **M3** | Project & state | `sophia init`, `sophia status` (placeholder), state project-scoped | ~12 | yaml.v3 | TBD after M2 done |
| **M4** | Run polling | `sophia run --no-tui --json` via polling GET | ~10 | (none) | TBD after M3 done |
| **M5** | SSE upgrade | `run` switches to SSE, reconnect, redaction | ~8 | tmaxmax/go-sse | TBD after M4 done |
| **M6** | TUI Timeline | `sophia run` with Bubble Tea Timeline + Q detach | ~10 | charm.land/bubbletea/v2, lipgloss/v2, bubbles/v2 | TBD after M5 done |
| **M7** | ApplyBoard + Approval | second view, passive approval banner | ~6 | (none; uses existing TUI) | TBD after M6 done |
| **M8** | Attach + changes | `sophia attach`, `sophia changes`, `sophia status` real | ~6 | (none) | TBD after M7 done |
| **M9** | Hardening + release | binary v0.1.0, e2e suite, security suite, demo | ~10 | (none; build-time only) | TBD after M8 done |

---

## M1 — Foundation

**Goal.** Establish the hexagonal Go skeleton, all outbound port definitions
and their fakes, the `slog` JSONL handler, and a Cobra CLI exposing two real
commands (`version`, `doctor` with three working checks: docker, git, XDG)
plus stubs for the rest. End state: running `sophia doctor` in a clean repo
returns a structured report; running `go test ./...`, `go vet ./...` and
`golangci-lint run` all pass.

**Files** (categorized):

- *cmd*: `cmd/sophia/main.go`
- *bootstrap*: `internal/bootstrap/{wire,version,logger}.go`
- *domain*: `internal/domain/{errors,phase,change,event,approval,fingerprint,config}.go` (+ tests)
- *ports/inbound*: `internal/ports/inbound/eventsink.go`
- *ports/outbound*: `internal/ports/outbound/{orchestrator,eventstream,compose,git,projectconfig,userconfig,statestore,browser,clock}.go`
- *application*: `internal/application/doctor.go` (+ tests)
- *adapters/inbound/cli*: `internal/adapters/inbound/cli/{root,version,doctor,stubs}.go`
- *adapters/outbound*: `internal/adapters/outbound/{stdclock,composeexec,gitcli}/*.go`
- *infrastructure*: `internal/infrastructure/logging/jsonl_handler.go`
- *test/fakes*: `test/fakes/{clock,git,compose,orchestrator,eventstream,projectconfig,userconfig,statestore,browser}.go`
- *root*: `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, `.gitignore`, `.editorconfig`, `.github/workflows/ci.yml`

**Validation criteria** (binding for M1 done):

- `go test ./...` exits 0.
- `go vet ./...` exits 0.
- `golangci-lint run` exits 0 (config in `.golangci.yml`).
- `go build ./...` exits 0.
- `make build` produces a binary at `./bin/sophia`.
- `./bin/sophia version` prints version + commit + build date.
- `./bin/sophia doctor` runs the three implemented checks and reports a
  pretty table; with `--json` produces a machine-readable report.
- `./bin/sophia init|start|stop|run|attach|status|changes` print
  `not implemented yet (M<n>)` and exit 0.
- Coverage in `internal/domain` ≥ 70% (verify with
  `go test -coverprofile=cover.out ./internal/domain/... && go tool cover -func=cover.out`).
- Coverage in `internal/application` ≥ 70%.
- Logs written to `<stateRoot>/logs/cli-YYYY-MM-DD.log` are valid JSONL.

**Test commands**:

```bash
make build                                   # compile
make test                                    # go test ./...
make lint                                    # golangci-lint run
make vet                                     # go vet ./...
make coverage                                # coverage for domain + application
./bin/sophia version
./bin/sophia doctor
./bin/sophia doctor --json | jq .
```

**Go deps added at M1**:

| Module | Pinned | Purpose |
|---|---|---|
| `github.com/spf13/cobra` | latest minor | CLI framework |
| `github.com/stretchr/testify` | latest minor | Test assertions |
| `github.com/spf13/afero` | latest minor (test-only) | In-memory FS for tests |

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | `golangci-lint` config drift between sibling services | Copy baseline `.golangci.yml` from `sophia-orchestator`, then trim |
| R2 | Coverage threshold hard to reach with stub commands | Move stub-command logic out of cobra wiring so it remains untested by design; concentrate tests in domain + doctor service |
| R3 | XDG path resolution differs between platforms (macOS/Linux) | Spec §3.1 default to Linux-style paths on macOS; honor XDG vars when set; tests cover both branches via fakes |

**Pending decisions** (require user review before M1 end):

| ID | Question | Default if user silent |
|---|---|---|
| D-M1-01 | Exact `golangci-lint` ruleset (which linters enabled) | Use `errcheck`, `govet`, `staticcheck`, `revive`, `gofmt`, `goimports`, `gosec`, `unused`, `unparam`, `misspell` |
| D-M1-02 | License header on every Go file? | No header in M1; ADR-0001 may add later |
| D-M1-03 | `cobra` cmd structure: persistent vs local flags | Use persistent flags only on `root` for global concerns (`--orchestrator-url`, `--log-level`, `--no-color`) |

**Checkpoint recommendation**: review with stronger model after Phase 6
(application/doctor service tests) — that is where the test harness pattern
cements; if it's wrong, every later milestone inherits the mistake.

---

## M2 — Provisioning

**Goal.** Embed a `compose.yaml` stub via `//go:embed`, materialize it
deterministically into `<dataRoot>/compose/`, expose `sophia start/stop`
operating with project name `sophia` regardless of CWD. Doctor adds checks
7 (orchestrator reachable) and 8 (SSE handshake, warn).

**Files**:

- *root*: `compose.yaml` (the embedded source of truth)
- *adapters/outbound*: `internal/adapters/outbound/composeexec/{embed,materialize,runner}.go`
- *application*: `internal/application/provisioner.go`
- *adapters/inbound/cli*: `internal/adapters/inbound/cli/{start,stop}.go` (replace stubs)
- *infrastructure*: `internal/infrastructure/httpclient/builder.go`
- *adapters/outbound*: `internal/adapters/outbound/orchestratorhttp/healthz.go`
- *test/integration*: `test/integration/compose_test.go` (opt-in, skipped without Docker)

**Validation criteria**:

- `sophia start` from any CWD: orchestrator container becomes `healthy`
  within 30 s.
- `sophia start --reset-compose` works on a manually edited
  `<dataRoot>/compose/compose.yaml`; previous version saved as
  `compose.yaml.previous`.
- `sophia stop` returns 0 and removes containers; volumes preserved.
- `sophia doctor` adds the orchestrator check (fail) and SSE handshake (warn);
  in a non-running stack the exit code is 3.
- `make test-integration` (opt-in) passes against a real Docker daemon.

**Test commands**:

```bash
make build && ./bin/sophia start
docker compose -p sophia ps
./bin/sophia doctor
./bin/sophia stop
make test-integration                        # requires Docker
```

**Go deps added**: none (uses stdlib `os/exec`, embedded compose).

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | Sibling services not yet released; compose stub is empty | Stub minimal compose: only `sophia-orchestator` (real if released, else mock-server stub container with `/api/v1/healthz`) |
| R2 | `docker compose` v1 vs v2 detection | M1 doctor already validates v2; M2 fails fast if v1 |
| R3 | Hash-check of materialized compose mis-flags edits | Store both current-embedded hash and last-materialized hash in a `compose.meta.json` so we distinguish "user edited" vs "binary upgraded" |

**Pending decisions**:

| ID | Question |
|---|---|
| D-M2-01 | Does the compose stub include placeholders for governance/memory/runtime, or only orchestrator? |
| D-M2-02 | Healthz endpoint exact path — confirm against `sophia-orchestator/internal/adapters/inbound/http/...` during M2 |

**Checkpoint**: review with stronger model before locking healthz path.

---

## M3 — Project & state

**Goal.** `sophia init` writes `.sophia.yaml` at the resolved repo root;
`sophia status` reads project-scoped `last_change_id` (placeholder, no
orchestrator call); state writer is atomic and project-fingerprinted.

**Files**:

- *adapters/outbound*: `internal/adapters/outbound/yamlconfig/{project,user,dto}.go`
- *adapters/outbound*: `internal/adapters/outbound/filestate/store.go`
- *adapters/outbound*: `internal/adapters/outbound/gitcli/inspector.go` (extend with `RemoteURL`, `CurrentBranch`, `Status`)
- *application*: `internal/application/{initializer,status}.go`
- *adapters/inbound/cli*: `internal/adapters/inbound/cli/{init,status}.go` (replace stubs)
- *test/integration*: `test/integration/init_test.go`

**Validation criteria**:

- `sophia init` outside a git repo fails with exit 3 and a clear message.
- `sophia init` inside a git repo writes `.sophia.yaml` at the repo root, not
  the CWD.
- `sophia init --force` overwrites an invalid `.sophia.yaml`.
- `sophia status` with no local change prints the empty-state message and
  exit 0.
- Fingerprint computation is stable across runs in the same repo.
- Atomic write: kill -9 mid-write does not corrupt `last_change_id`.

**Test commands**:

```bash
mkdir /tmp/repo && cd /tmp/repo && git init -q
../path/to/sophia init
cat .sophia.yaml
../path/to/sophia status
```

**Go deps added**: `gopkg.in/yaml.v3`.

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | `EvalSymlinks` fails in sandboxed FS | Tolerant fallback to `Clean`-only path |
| R2 | Submodules / worktrees produce unexpected `repo_root` | Document behavior, tests cover the case |

**Pending decisions**:

| ID | Question |
|---|---|
| D-M3-01 | If `repo_root` resolves to a path with no `remote.origin.url`, is the project name from `.sophia.yaml` enough to identify uniqueness? |

**Checkpoint**: none required.

---

## M4 — Run via polling (scaffolding)

**Goal.** End-to-end create-and-observe loop using HTTP polling only (no SSE
yet). Validates the `auto_advance` assumption against a real orchestrator.

**Files**:

- *adapters/outbound/orchestratorhttp*: `client.go`, `dto.go`, `errors.go`
- *application*: `internal/application/runner.go`
- *adapters/inbound/cli*: `internal/adapters/inbound/cli/run.go` (replace stub; only `--no-tui --json` works in M4)
- *adapters/inbound/jsonsink*: `internal/adapters/inbound/jsonsink/sink.go`
- *test/e2e*: `test/e2e/run_polling_test.go`

**Validation criteria**:

- `sophia run "..." --no-tui --json` against a real orchestrator emits
  snapshot JSON lines until terminal status.
- Exit codes 0/1/3/4 enforced.
- The first M4 e2e test confirms `auto_advance` (orchestrator advances
  phases by itself) **or** documents the fallback path.

**Test commands**:

```bash
sophia start
sophia run "test change" --no-tui --json | tee out.jsonl
echo "exit code: $?"
sophia stop
```

**Go deps added**: none.

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | `auto_advance` proves false | Compatibility-mode `RunPhase` loop without routing decisions; documented as tech debt (spec §5.2) |
| R2 | Polling backpressure under heavy load | Default 1 s, exponential to 5 s; cancelable on Ctrl+C |

**Pending decisions**: D-M4-01 — exact response shape of `POST /api/v1/changes`
(verify with first e2e).

**Checkpoint**: **stronger model review required** if `auto_advance` test
fails — the fallback path touches the mantra and needs careful framing.

---

## M5 — SSE upgrade

**Goal.** Replace polling with SSE in `run --no-tui --json`. Add reconnect,
`Last-Event-ID`, redaction pipeline, tolerant parser.

**Files**:

- *adapters/outbound/ssestream*: `client.go`, `reconnect.go`, `parser.go`,
  `redactor.go`
- *application*: `internal/application/runner.go` (refactor to consume SSE
  via the port instead of polling)
- *test/integration*: `test/integration/sse_reconnect_test.go`

**Validation criteria**:

- `sophia run --no-tui --json` against the orchestrator emits both snapshot
  lines and event lines.
- Network blip mid-stream → reconnect via `Last-Event-ID` without losing
  the in-flight phase progress.
- Unknown event types do not break the parser.
- Heartbeat handling: 60 s without heartbeat → force reconnect.
- Secret redaction: corpus of known token shapes is replaced by
  `[REDACTED]` before reaching any sink.

**Test commands**:

```bash
sophia run "..." --no-tui --json
# in another terminal: simulate network blip via toxiproxy or by stopping/starting orchestrator briefly
```

**Go deps added**: `github.com/tmaxmax/go-sse`.

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | `tmaxmax/go-sse` does not interoperate with backend SSE shape | Fallback: `net/http` + `bufio.Scanner` (≈ 300 extra lines) |
| R2 | Redactor over-redacts technical fields | Conservative scope: fields named `token`, `secret`, `key`, `authorization`, `password`, `credential`, plus free-form log messages (spec §6.3 #4) |

**Pending decisions**: confirm event-type names against the orchestrator's
event publisher (not the comment block in `eventstream.go`).

**Checkpoint**: review reconnect/backoff strategy with stronger model — the
discard policy under pressure (heartbeat first, never `phase.*`) must hold.

---

## M6 — TUI Timeline

**Goal.** Bubble Tea v2 program with a single Timeline view; `Q` detaches;
`Ctrl+C` confirms then detaches.

**Files**:

- *adapters/inbound/tui*: `program.go`, `model.go`, `update.go`,
  `view_timeline.go`, `keybindings.go`, `styles.go`
- *adapters/inbound/tui*: `bridge.go` (cap-256 buffer, drop policy)
- *test*: `test/tui/timeline_test.go` (using `teatest` v2 utilities)

**Validation criteria**:

- `sophia run "..."` opens Bubble Tea Timeline with 9 phases.
- `Q` exits with code 0 and prints the reattach hint.
- `Ctrl+C` first prompts; second `Ctrl+C` exits 2.
- Drop policy honored: heartbeat dropped first; `phase.*` and `approval.*`
  never dropped.

**Test commands**:

```bash
make build && ./bin/sophia run "test"          # interactive; Q to exit
make test                                      # tui golden tests
```

**Go deps added**: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`,
`charm.land/bubbles/v2`. **Pin exact versions in `go.mod`**.

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | `tea.KeyMsg` → `tea.KeyPressMsg` rename not caught | Engram memory `sophia-cli/decisions/bubbletea-v2` lists breaking changes; `make lint` will flag deprecated names |
| R2 | `program.Send` blocking under sustained pressure | Custom buffer with cap 256 and explicit drop policy; never assert non-blocking semantics |

**Pending decisions**: D-M6-01 — final keybindings beyond `Q`/`Ctrl+C`/`Tab`
(Tab arrives in M7).

**Checkpoint**: review the bridge implementation with stronger model — it is
the boundary between concurrent goroutines (SSE) and the Bubble Tea program;
race conditions here are silent.

---

## M7 — ApplyBoard + Approval banner

**Goal.** Second TUI view (`Tab` toggles), passive approval banner with
`[O]pen` shortcut.

**Files**:

- *adapters/inbound/tui*: `view_applyboard.go`, `view_approval_banner.go`
- *adapters/outbound/osbrowser*: `browser.go`
- *adapters/inbound/tui*: extend `model.go`, `update.go` for view switching
  and banner state

**Validation criteria**:

- `Tab` toggles Timeline ↔ ApplyBoard.
- ApplyBoard shows groups + tasks + team-leads sourced from `task.*` and
  `agent.*` events.
- `approval.required` event displays banner; `[O]` opens validated URL via
  OS browser.
- Banner disappears on `approval.resolved`, snapshot refresh, or any
  forward-progress event.
- URL validation rejects `javascript:`, `data:`, `file:`, `vbscript:`.

**Test commands**:

```bash
make test                                      # golden snapshots for both views
```

**Go deps added**: none.

**Risks**: low; views are pure render of received state.

**Pending decisions**: D-M7-01 — visual treatment of paralelism in
ApplyBoard (table vs. nested tree).

**Checkpoint**: optional design review on ApplyBoard layout.

---

## M8 — Attach + changes + status

**Goal.** Snapshot+stream attach; changes listing with project filter; full
`sophia status` against the orchestrator.

**Files**:

- *application*: `internal/application/{attacher,lister,status}.go` (status
  upgraded from M3 placeholder)
- *adapters/inbound/cli*: `internal/adapters/inbound/cli/{attach,changes,status}.go`
  (replace stubs / extend)
- *test/e2e*: `test/e2e/attach_workflow_test.go`

**Validation criteria**:

- `sophia changes` returns recent changes (defaults: limit=10, project from
  `.sophia.yaml`).
- `sophia attach <id>` pulls snapshot, renders, then subscribes to current
  phase SSE.
- `sophia status [<id>]` resolves to the right `last_change_id`
  (project-scoped → global → empty).
- E2E: `run` → `Q` → `attach` → completion produces consistent terminal
  state.

**Test commands**:

```bash
sophia run "..."
# Q
sophia attach <change-id>
# wait completion
sophia status                                  # reflects DONE/BLOCKED
sophia changes
```

**Go deps added**: none.

**Risks**: race between snapshot fetch and SSE subscription — mitigated by
applying events on top of snapshot only when their phase matches.

**Pending decisions**: none.

**Checkpoint**: none required.

---

## M9 — Hardening + release

**Goal.** Cross-compile binaries, security suite, full e2e suite, GoReleaser
pipeline, asciinema demo, ADR-0001/0002.

**Files**:

- *root*: `.goreleaser.yaml`
- *.github/workflows*: `release.yml`, `e2e.yml`, `security.yml`
- *test/security*: `boundary_test.go`, `url_validation_test.go`,
  `redactor_corpus_test.go`, `yaml_safety_test.go`
- *test/e2e*: `full_workflow_test.go`, `json_mode_test.go`, `doctor_json_test.go`,
  `approval_flow_test.go`
- *docs*: `docs/adr/0001-architecture.md`, `docs/adr/0002-bubbletea-v2.md`,
  `README.md`, `CHANGELOG.md`, `LICENSE`
- *root*: `demo/sophia-quickstart.cast` (asciinema)

**Validation criteria**:

- GoReleaser produces darwin/amd64, darwin/arm64, linux/amd64, linux/arm64
  artifacts with SHA-256 checksums.
- All security tests pass.
- All e2e tests pass against a real orchestrator.
- README quickstart works end-to-end in under 10 minutes.
- Asciinema demo ≤ 90 s.
- Tagged `v0.1.0`.

**Test commands**:

```bash
goreleaser release --snapshot --clean
make test-security
make test-e2e
```

**Go deps added**: none (build-time only: GoReleaser, asciinema-cli).

**Risks**:

| # | Risk | Mitigation |
|---|---|---|
| R1 | Cross-compile fails on a platform | Release v0.1.0 with darwin/arm64 + linux/amd64; rest in v0.1.1 |
| R2 | Demo recording leaks dev environment paths | Record from a fresh container; redact paths in post |

**Pending decisions**: D-M9-01 — code signing path. **Spec defers signing
to V1.1**; M9 ships SHA-256 checksums only.

**Checkpoint**: **stronger model review required** on:
- ADR-0001 (architecture summary)
- security suite coverage matrix
- GoReleaser config

---

## Cross-milestone risk index

| ID | Risk | First impact | Status |
|---|---|---|---|
| RX-01 | `auto_advance` assumption proves false | M4 | Documented in spec §5.2; fallback ready |
| RX-02 | `tmaxmax/go-sse` incompatible with backend SSE | M5 | Fallback to `net/http`+`bufio` |
| RX-03 | Bubble Tea v2 surprise breaking changes | M6 | Engram memory tracks known breaks; pin versions |
| RX-04 | Sibling services not yet released | M2 | Compose stub strategy approved |
| RX-05 | Cross-compile failure | M9 | Two-platform release acceptable |
| RX-06 | Coverage threshold ≥ 70% hard to meet | M1 | Concentrate tests in domain + application; stub-only code does not count |

---

## Cross-milestone pending-decisions index

| ID | Question | Resolves at | Default |
|---|---|---|---|
| D-M1-01 | golangci-lint ruleset | M1 | Baseline copied from `sophia-orchestator` |
| D-M1-02 | License header on Go files | M1 | None for V1 |
| D-M1-03 | Persistent vs local cobra flags | M1 | Persistent only at root for `--orchestrator-url`, `--log-level`, `--no-color` |
| D-M2-01 | Compose stub coverage (governance/memory/runtime) | M2 | Only orchestrator; rest mocked at HTTP level |
| D-M2-02 | Exact `/healthz` path | M2 | `GET /api/v1/healthz` |
| D-M3-01 | Project uniqueness without `remote.origin.url` | M3 | Acceptable — global fallback rescues |
| D-M4-01 | Response shape of `POST /api/v1/changes` | M4 | Verified by first e2e test |
| D-M6-01 | Keybindings beyond Q/Ctrl+C/Tab | M6 | Defer to M7 |
| D-M7-01 | ApplyBoard layout | M7 | Table |
| D-M9-01 | Code signing | M9 | Defer to V1.1 |

---

## Cross-milestone checkpoints (stronger model review)

| Phase | Checkpoint topic | Why |
|---|---|---|
| M1 | application/doctor service test harness | Pattern propagates to every later milestone |
| M2 | healthz path lock-in | Affects every doctor invocation downstream |
| M4 | `auto_advance` validation result | If false, mantra wording must be defended |
| M5 | SSE reconnect/backoff/discard policy | Concurrency + correctness |
| M6 | TUI bridge concurrency model | Race conditions silent under load |
| M9 | ADR-0001 + security suite + GoReleaser | Public-facing artifacts |

---

## What this roadmap does NOT include

- Build-time tooling for releases (covered in M9).
- Specific compose YAML contents (M2 will produce one).
- Actual event-type names confirmation (M5 first e2e).
- Final TUI keybindings beyond V1 minimum (deferred to M7).
- Profile/MCP/Context7/auth — outside V1 by spec §8.

---

*Roadmap drafted 2026-05-05. M1 detailed plan ready at
`2026-05-05-sophia-cli-m1-foundation.md`.*
