# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

(empty — first changes after the v0.2.0-rc.1 cut land here)

---

## [v0.2.0-rc.1] — 2026-05-08

Coordinated release-candidate cut with `sophia-orchestator v0.2.0-rc.1`,
landing the M10 wire-alignment milestone. The two repos carry
byte-identical `docs/specs/sophia-wire-v1.md` mirrors (SHA256
`097be33907771e727fa1e4e834f5afc01d8c3f212bb503b2a4f2dc00d19fd6c5`)
gated by the cross-repo contract test suite.

7-day soak window per D-M10-11 begins on the rc.1 tag date. Both repos
promote to `v0.2.0` final on the same day after the soak window
closes, conditional on the soak matrix carrying zero unresolved RED
entries.

See M10 plan
`docs/superpowers/plans/2026-05-07-sophia-m10-wire-alignment-v0.2.0.md`
for the full rationale and decision log (D-M10-01 through D-M10-17).

### Compatibility

> **v0.2.0 is a coordinated cut-over.** Upgrade `sophia-cli` AND
> `sophia-orchestator` together. There is **no partial-upgrade path**.

- `sophia-cli v0.2.0` **requires** `sophia-orchestator v0.2.0+`.
- `sophia-cli v0.2.0` is **incompatible** with `sophia-orchestator v0.1.x`.
- `sophia-cli v0.1.0` is **incompatible** with `sophia-orchestator v0.2.0`.
- A **remote** orchestrator REQUIRES `SOPHIA_API_KEY` (env) or
  `--api-key` (flag). The cli refuses to call a remote orchestrator
  anonymously and exits `3` with `auth required for remote orchestrator`
  before any HTTP request.
- A **local loopback** orchestrator (bound to `localhost`,
  `127.0.0.0/8`, or `::1`) MAY accept anonymous calls **only if** the
  orchestrator was started with `AllowAnonLocalhost=true` (the flag is
  silently downgraded to `false` if the listener is bound to a
  non-loopback interface).

Migration guide: [`docs/migration/v0.1.0-to-v0.2.0.md`](docs/migration/v0.1.0-to-v0.2.0.md).
The same guide is mirrored byte-identically in the orchestrator repo.

### Wire-protocol changes (sophia-wire-v1)

- **Canonical wire spec** lives at `docs/specs/sophia-wire-v1.md`
  (mirrored byte-identically in `sophia-orchestator`; SHA256 cross-repo
  gate enforced by the contract test suite).
- **Health path:** `/api/v1/healthz` → **`/api/v1/health`**. No `/healthz`
  alias on the orchestrator side (D-M10-06).
- **Approval flow** is now **phase-scoped** (D-M10-13 Form A):
  - `POST /api/v1/phases/{phase_id}/approve`
  - `POST /api/v1/phases/{phase_id}/reject`
  Phase IDs are globally unique on the orchestrator; the redundant
  `change_id` is removed from the URL.
- **SSE streams are per-phase** (D-M10-05): subscribe to
  `GET /api/v1/phases/{phase_id}/events`. The legacy per-Change feed is
  gone; the cli's new multiplexer re-subscribes to the next phase when
  `current_phase_id` changes on the Change snapshot.
- **SSE event ids** are ULIDs (sophia-wire-v1 §5.1) — `Last-Event-ID`
  resume is now collision-safe.
- **`approval.resolved` replaces `phase.approved` / `phase.rejected`**
  with a `decision` field (`"approved" | "rejected"`).
- **`agent.dispatched`** is the canonical event name; the cli still
  accepts the legacy `agent.spawned` for backward-compat with prior
  fixtures.
- **`410 phase_terminal_no_events`**: subscribing to an SSE stream for
  a phase that is already terminal MUST fall back to
  `GET /api/v1/phases/{id}` for state. The cli's SSE client closes the
  channel without burning the retry budget.
- **`open` event** is sent first on every SSE connection (Phase 1.5
  amendment, Optional). Clients MAY use it for fast reconnect detection.
- **Optional `apply.*` diagnostic events** (Phase 1.5 amendment) are
  tolerated; clients MUST forward unknown events unchanged
  (sophia-wire-v1 §10).

### Authentication

- Header `X-Sophia-API-Key` is the canonical auth header (legacy
  `X-API-Key` is accepted by the orchestrator for migration but clients
  SHOULD use the canonical name).
- The cli resolves the key from `--api-key` flag → `SOPHIA_API_KEY`
  env → empty. The key value is **never logged or printed**.
- Loopback-only orchestrators MAY allow anonymous via the orchestrator's
  `AllowAnonLocalhost=true` flag, which is silently downgraded to
  `false` when the listener is bound to a non-loopback address (D-M10-02).

### Error envelope

- All non-2xx responses now carry the canonical envelope
  (sophia-wire-v1 §9.1): `{code, error, details?}`.
- 13 stable error codes (sophia-wire-v1 §9.2) round-trip via
  `errors.Is` to domain sentinels: `unauthorized`,
  `validation_failed`, `approver_required`, `limit_too_large`,
  `change_not_found`, `phase_not_found`, `change_already_exists`,
  `change_already_terminal`, `phase_not_resumable`, `phase_not_gated`,
  `gate_already_decided`, `phase_terminal_no_events`, `internal_error`.

### Added

- New CLI commands (Phase 4 Tasks 4.4–4.6 / D-M10-13):
  - `sophia approve <phase-id>` — POSTs the canonical approval. The
    M7 browser `[O]` keybinding is unchanged; either channel resolves
    the gate (D-M10-03).
  - `sophia reject <phase-id>` — symmetric to approve.
  - `sophia abort <change-id>` — POSTs `/api/v1/changes/{id}/abort`.
  Idempotency: `gate_already_decided` (approve/reject) and
  `change_already_terminal` (abort) exit `0` with informational text.
- `pkg/contract/` (Phase 3.6) — public Go package mirroring the wire
  surface (route paths, event names, error codes, request/response
  DTOs, header constants). Both repos depend on it.
- `--api-key` persistent flag on the root command. Resolved at
  `PersistentPreRunE`; commands that don't talk to the orchestrator
  (`version`, `doctor`, `init`, `start`, `stop`, `help`) skip the
  auth gate.
- Phase 5 contract test suite under `test/contract/` with build tag
  `contract`: 27 cross-repo wire-conformance tests covering the 9
  Required endpoints, 4 auth modes, the SSE event taxonomy, all 13
  error codes, and 8 CLI smoke commands. Plus a lightweight SHA256
  cross-repo gate that runs in default `go test`.
- `make contract` + `make test-no-workspace` Makefile targets
  (Phase 5; D-M10-15 / D-M10-16 release blockers).

### Changed

- The cli's outbound HTTP client now injects `X-Sophia-API-Key` on
  every authenticated request when a key is configured. Anonymous
  mode (no key) omits the header entirely — "absent ≠ empty" is
  enforced by a contract test.
- The cli's SSE consumer (`internal/adapters/outbound/ssestream`) is a
  per-phase client (was per-Change). The runner's observation loop is
  a phase-stream multiplexer that re-subscribes when
  `current_phase_id` changes on the Change snapshot.
- `StatusError` now parses the canonical error envelope and exposes
  `Code` / `Message` / `Details` fields. `errors.Is(err, domain.Err*)`
  maps every wire `code` to its domain sentinel.
- `internal/application/doctor.go` no longer probes SSE pre-run. The
  legacy probe targeted an endpoint the orchestrator never
  implemented; doctor now reports SSE handshake as
  `info: deferred to first run/attach` (D-M10-07).

### Removed

- `internal/adapters/outbound/sseprobe/` package (D-M10-07).
- `internal/ports/outbound/ssehandshake.go` and the `SSEProber` port.
- The deprecated `outbound.StreamTarget.PhaseID` semantic where the
  field was unused: it is now the authoritative subscription key.

### Documentation

- `docs/migration/v0.1.0-to-v0.2.0.md` — operator-facing migration
  guide with upgrade checklist and rollback procedure (mirrored
  byte-identically in the orchestrator repo).
- `docs/specs/cli-orchestrator-compatibility.md` §7 — Phase 5
  cross-repo compatibility report.
- `test/contract/HARNESS.md` — how to run the contract suite + how to
  extend it with a real-binary smoke (Phase 7).

---

## [v0.1.0] — 2026-05-07

First publicly tagged release. Bundles the entire M1-M8 feature surface plus
the M9 hardening + release infrastructure.

### BREAKING

- **Go module path renamed** from `github.com/RVRTelecomunicaciones/sophia-cli`
  to `github.com/RVRTelecomunicaciones/sophia` to match the GitHub repository
  (`RVRTelecomunicaciones/sophia`). This is the first publicly visible commit
  of the module — there are no downstream importers to migrate. `go install
  github.com/RVRTelecomunicaciones/sophia/cmd/sophia@latest` is the canonical
  install path going forward.

### Added

- **Release packaging** (M9): goreleaser v2 cross-platform builds (linux+darwin
  × amd64+arm64), SHA256 checksums, GitHub Actions release workflow triggered
  on `v*.*.*` tag push.
- **Documentation** (M9): Apache-2.0 LICENSE, README quickstart, full
  CHANGELOG covering M1-M8, ADR-0001 (hexagonal architecture), ADR-0002
  (Bubble Tea v2 on `charm.land`).
- **Security suite** (M9): `make vuln` (govulncheck reachable HIGH/CRITICAL
  gate), `make security` (gosec HIGH gate), `make licenses` (THIRD_PARTY_LICENSES
  inventory with go-licenses fallback for Go 1.26.x). CI runs all three.
- **Release gates** (M9): manual smoke checklist at
  `docs/release/manual-smoke-checklist.md`; security exception log at
  `docs/release/security-notes.md`.
- **Headless asciinema demo** at `assets/demo/sophia-quickstart.cast`.

### Changed

- CI workflow bumped to Go 1.26.x (matches `go.mod` toolchain).
- `bubbletea/v2` and `lipgloss/v2` promoted from indirect to direct deps via
  `go mod tidy` (they are imported directly by the TUI adapter).

### Internal

- `composeexec.writeFile0644` carries an inline `// #nosec G304,G306,G703 --`
  annotation for the gosec false positive (path is composed from XDG dataRoot
  + fixed filenames; 0o644 required for docker daemon uid mismatch).

### Known limitations

- **Wire-protocol drift vs `sophia-orchestator`**: integration testing on
  2026-05-07 against the real orchestrator service revealed three mismatches
  between sophia-cli's wire expectations and `sophia-orchestator`'s actual
  HTTP surface:

  | Endpoint | sophia-cli sends | orchestator implements |
  |---|---|---|
  | Health | `GET /api/v1/healthz` | `GET /api/v1/health` |
  | Auth | (no auth header) | requires `X-Sophia-API-Key` |
  | SSE event stream | `GET /api/v1/changes/{id}/events` | `GET /api/v1/changes/{cid}/phases/{pid}/events` (per-phase, not per-change) |

  v0.1.0 ships with the protocol it was specified against (M5–M8 design).
  The two repos evolved their HTTP surfaces in parallel without cross-repo
  integration tests. Aligning them is **scoped to v0.2.0** via a coordinated
  spec/ADR across both repositories. Until then, v0.1.0 is functional
  against any orchestrator that honors the wire protocol documented in
  the `internal/adapters/outbound/orchestratorhttp/` package and the M5
  SSE spec — including the in-process stub used during M9 smoke
  (`/tmp/orch-stub`, mirrors the same shape as `test/e2e/`'s test stubs).

- **Real-orchestrator manual smoke**: NOT executed at v0.1.0 cut. The
  `Sign-off` block in `docs/release/manual-smoke-checklist.md` records this
  explicitly. v0.1.1 / v0.2.0 will land that validation once the wire
  protocol is harmonized.

### Pre-existing milestone history

The full M1-M8 milestone history below is preserved verbatim from the M9
plan's CHANGELOG draft.

---

## [m8-attach-changes-status] — 2026-05-06

The third trio of read-side commands plus a refactor that unifies the run and
attach observation pipelines into a single engine.

### Added

- `application.Lister` — pure pass-through over `OrchestratorClient.ListChanges`.
- `application.Attacher` with two entry points: `Attach(ctx, AttachInput, sink)`
  (full pipeline) and `AttachFromSnapshot(ctx, snap, project, sink)` (skips
  `GetChange`; used by `cli.attachJSONL` for D-M8-13's eager-arm of
  `approvalTimeoutSink`).
- `application.Runner.Observe(ctx, RunResult, sink)` — exported observation loop
  shared by `Run` and `Attach`.
- `application.StatusReader` real implementation: `Resolve(ctx, ResolveInput) →
  StatusReport` with `*domain.Change` snapshot fetched via `OrchestratorClient.GetChange`,
  resolution order arg → project → global → empty per spec §2.5.
- `cli sophia attach <change-id>` — TUI by default, `--no-tui --json` for JSONL,
  `--approval-timeout` flag.
- `cli sophia changes [--limit N] [--status S] [--project P] [--json]` — table or
  JSON list of recent Changes.
- `cli sophia status [<change-id>] [--json]` — real fetch (was M3 placeholder).
- `cli/changeresponse.go` shared helper `changeResponseFromDomain` reused by
  `attach`, `changes`, `status`.
- `test/e2e/attach_workflow_test.go` (build-tag `e2e_smoke`) — end-to-end run →
  attach → done cycle.

### Changed

- `application.NewStatusReader` signature: `(StatusDeps, StatusOptions)`. Output
  type `StatusReport` replaces `StatusOutput`.
- `application.Lister` does NOT resolve project defaults; that lives in
  `cli/changes.go` per cambio 1.
- `cli.Deps` gains `Lister`, `Orch`, `AttacherFactory` fields.

### Fixed

- `cli.approvalTimeoutSink.startTimer` no longer resets the timer on re-emit
  (cambio 3 / D-M8-13). Eager-arm timestamp is preserved.
- `application.StatusReader.locate` surfaces `domain.ErrInvalidYAML` as
  `ExitError{Code: 3}` instead of silently falling through (cambio 4).
- `application.StatusReader.fetch` maps internal `FetchTimeout` to exit 4
  (transient) instead of conflating with config-stale exit 3 (cambio 5).
- `application.Attacher.Attach` no longer defers `sink.Close()` inside the
  `if err != nil` branch — direct call before each return.

### Internal

- Bootstrap wires `Lister`, `AttacherFactory`, and the new `StatusReader` shape.
- `Attacher.persistChangeID` extracted to a package-level helper shared with
  `Runner.persistChangeID`.
- `cli/stubs.go` deleted — every M1–M8 command is now real; `newStubCmd` had
  zero callers.
- `FakeOrchestrator` gains `OnGetChange func(domain.ChangeID)` hook for assertion
  in tests.

---

## [m7-applyboard-approval] — 2026-05-06

ApplyBoard view, approval gate banner, browser opener, and JSONL approval timeout.

### Added

- `internal/adapters/outbound/osbrowser` — `outbound.Browser` adapter with URL
  whitelist (http/https only) per spec §6.3 invariant 3.
- `tui.ApplyBoardState` — pure TUI-internal state derived from `task.*` /
  `agent.*` events. Renders the second view (Tab key toggles).
- `tui` Tab and `O` keybindings; `OpenBrowserMsg` / `BrowserOpenedMsg` plumbing.
- ApprovalGate banner overlay with `[O]pen browser` action.
- `cli --approval-timeout=DURATION` flag (default 30m). JSONL mode exits with
  code 5 when the timer fires per spec §5.8.
- `test/tui/applyboard_banner_test.go` — teatest coverage for Tab toggle + `[O]`
  open behavior.

### Changed

- Toolchain bumped to `go1.26.2` (added `toolchain` directive in `go.mod`;
  minimum stays at `go 1.25.0`).

### Fixed

- `tui.Update` `ApprovalGateMsg` handling no longer round-trips through
  `approvalGateAsEvent`, which was dropping `URL`, `Reason`, `Risk`, and
  `Policy` fields.
- `tui.Bridge` `Close` drains priority messages instead of dropping them.
- `tui.Bridge` snapshot deep-copies `Phases` to avoid aliasing.
- `ssestream` JWT regex tightened to require a 32-character third segment plus
  word boundaries — prevents false matches on dotted strings like
  `task_execution.phase_started.explore_mode`.

### Internal

- Bridge pressure tests refactored to deterministic worker-state sync (replaces
  scheduler-dependent timing).
- `cmd/sophia` instrumented via `go build -cover` for e2e coverage capture.
- Coverage push: `gitcli` and `composeexec` shell-out paths covered via
  fake-binary injection.

---

## [m6-tui-timeline] — 2026-05-06

Bubble Tea v2 TUI with timeline view, JSON-fallback CLI flow, and EventSink
bridge.

### Added

- `internal/adapters/inbound/tui` — Bubble Tea v2 program scaffolding pinned to
  `charm.land/bubbletea/v2 v2.0.6`, `bubbles/v2 v2.1.0`, `lipgloss/v2 v2.0.3`.
- `tui.Bridge` — `EventSink` adapter with cap-256 buffer + drop policy per spec
  §4.5; teaSender plumbing into the `tea.Program`.
- Immutable `tui.Model` with `ApplySnapshot` / `ApplyEvent` (no I/O).
- Pure `tui.Update` with `Q` (detach) and `Ctrl+C` (confirm-then-detach)
  keybindings.
- Lipgloss styles + Timeline `View()` with ANSI-safe rendering per spec §6.3
  invariant 7.
- `tui.Program` assembly with reattach hint emitted to stderr on Q detach.
- `cli.RunnerFactory` — sink injected at command time so the same Runner can
  serve TUI bridge or JSONL sink without rebuilding deps.
- `test/tui/timeline_test.go` — teatest golden integration coverage.

### Changed

- `cli sophia run` flag inversion: TUI is the default; `--no-tui --json` is the
  JSONL fallback (M5 was JSONL-only).

### Internal

- `bubbletea v2.0.6` requires Go ≥ 1.25; toolchain auto-bumped from 1.24.5.

---

## [m5-sse-upgrade] — 2026-05-06

Replace M4's polling loop with the production SSE pipeline plus secret
redaction.

### Added

- `internal/adapters/outbound/ssestream` package — `tmaxmax/go-sse` based
  SSE client with reconnect, retry budget, watchdog (60s heartbeat per spec
  §5.7), and Last-Event-ID resume.
- Secret redactor: pattern + field-name + allowlist redaction per spec §6.3
  (token, bearer, ghp_, AKIA, sk_*, password, JWT-shaped strings).
- Tolerant SSE parser per spec §5.3 / §5.4 (empty timestamps, unknown event
  types, missing fields all degrade gracefully).
- `application.Runner` rewrite: SSE consumption + approval translation
  (`approval.required` → `OnApprovalGate`); polling code removed.

### Changed

- Bootstrap wires `ssestream.Client` as `Runner.EventStream` (was the polling
  fake from M4).

### Fixed

- `Runner` fires `OnError` on ctx cancellation paths (was silently exiting).
- `FakeEventStream.Push` hardened against close races.

### Internal

- Per-attempt ctx cancel in `ssestream.Client.Subscribe`.
- Integration tests cover SSE blip recovery and heartbeat skip semantics.

---

## [m4-run-polling] — 2026-05-05

The first end-to-end run loop — orchestrator HTTP client, JSONL sink,
config resolver, and a polling Runner.

### Added

- `internal/adapters/outbound/orchestratorhttp` — DTOs (`ChangeResponse`,
  `PhaseDTO`), `StatusError` with sentinel mapping (404 → `ErrChangeNotFound`,
  5xx → `ErrUnreachable`), `CreateChange` / `GetChange` / `ListChanges`
  implementations.
- `internal/adapters/inbound/jsonsink` — JSONL sink that writes one event per
  line to stdout.
- `application.ConfigResolver` — flag/env/yaml layering per spec §3.4.
- `application.Runner` (M4 version) — polling loop with `ExitError` mapped per
  spec §2.3 exit codes.
- `cli sophia run "<message>"` — first end-to-end command (M4 ships JSONL only;
  M6 inverts to TUI by default).
- `test/e2e/run_polling_test.go` — e2e smoke that validates the auto-advance
  assumption.

### Fixed

- `omitzero` on time fields in DTOs (Go 1.24+ JSON tag).
- Oversized response body detection in `doJSON`.
- `--artifact-store` flag value validated.
- Env constants exported for cli use.

---

## [m3-project-state] — 2026-05-05

Per-project configuration and persistent local state.

### Added

- `internal/adapters/outbound/yamlconfig` — afero-backed `ProjectStore` and
  `UserStore` (0600 perms, 100KB cap) with `domain.ErrConfigMissing` /
  `ErrInvalidYAML` sentinels.
- `internal/adapters/outbound/filestate` — atomic-write state store with
  `ProjectMeta` keyed by fingerprint.
- `application.Initializer` — resolve repo root, write `.sophia.yaml`.
- `application.StatusReader` — local resolution: project → global → empty
  (M3 placeholder; M8 upgrades to live fetch).
- `cli sophia init` with `--project`, `--base-ref`, `--artifact-store`,
  `--force`.
- `cli sophia status` (M3 placeholder).
- Bootstrap wires `Initializer`, `StatusReader`, `yamlconfig`, `filestate`.

### Internal

- Integration tests for `init` + filestate atomicity.

---

## [m2-provisioning] — 2026-05-05

Local Sophia stack management — Docker compose materialization, doctor
extensions, SSE handshake.

### Added

- `domain.XDGPaths` + `outbound.PathResolver`.
- `internal/adapters/outbound/xdgpaths` — XDG-spec resolver with
  `Resolve` / `EnsureDirs` / `ValidateDirs`.
- `internal/infrastructure/httpclient` — shared HTTP client builder.
- `internal/adapters/outbound/orchestratorhttp.Client` — initial scaffold with
  `Healthz`.
- `outbound.SSEProber` + `internal/adapters/outbound/sseprobe` — one-shot
  SSE handshake validator.
- `application.DoctorService` extensions: paths, orchestrator, SSE checks.
- Embedded `compose.yaml` (V1 dev stack stub) + `composeexec.Materialize`
  with `.previous` backup.
- `application.Provisioner` (`Up` / `Down`) with project name `sophia`.
- `cli sophia start` (with `--reset-compose`) and `cli sophia stop`.

### Internal

- Argv-validation tests for compose `Up` / `Down`.
- Opt-in compose integration test.
- M2 final validation pass (`chore(m2)`).

---

## [m1-foundation] — 2026-05-05

Repo skeleton — module, hexagonal layout, domain types, ports, fakes, CI,
and the first command (`doctor`).

### Added

- Go module `github.com/RVRTelecomunicaciones/sophia-cli` with hexagonal
  package layout (`domain`, `ports/{inbound,outbound}`, `application`,
  `adapters/{inbound,outbound}`, `bootstrap`).
- Domain types: `ChangeID`, `ChangeStatus`, `Change`, `PhaseType`,
  `PhaseStatus`, `Phase`, `Event`, `ApprovalGate`, `ArtifactStoreMode`,
  `ProjectConfig`, `UserConfig`, `Fingerprint` + `ComputeFingerprint`,
  sentinel errors.
- Inbound port: `EventSink`.
- Outbound ports: `OrchestratorClient`, `EventStream`, `StateStore`,
  `ProjectConfigStore`, `UserConfigStore`, `GitInspector`, `Compose`,
  `Clock`.
- `test/fakes` package — every port's fake (`FakeOrchestrator`,
  `FakeEventStream`, `FakeStateStore`, `FakeProjectConfigStore`,
  `FakeUserConfigStore`, `FakeGitInspector`, `FakeComposeRunner`,
  `FakeClock`).
- `application.DoctorService` — first use case (Docker / git / orchestrator
  reachability).
- `cli sophia doctor` + `cli sophia version` + Cobra root scaffolding.
- `Makefile` (`build`, `test`, `vet`, `lint`, `coverage`).
- `golangci.yml` config.
- `.github/workflows/ci.yml` — build + test + lint + coverage on push/PR.
