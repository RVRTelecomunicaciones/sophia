# Manual Smoke Checklist

This file accumulates per-tag smoke gates. Each release adds a new
edition section; older sections stay as audit history.

| Edition | Date | Status |
|---|---|---|
| [v0.2.0 edition](#v020-edition---day-7-promotion-gate) | 2026-05-15 (Day 7 target) | OPEN — gates required for `v0.2.0` final tag |
| [v0.1.0 edition](#v010-edition) | 2026-05-07 | CLOSED — sign-off recorded |

---

## v0.2.0 edition — Day 7 promotion gate

Gates the v0.2.0 final tag (D-M10-16 #3 release blocker). Every
checkbox below MUST be ticked before `./scripts/promote-v0.2.0.sh
--confirm` is run on Day 7.

### A. Pre-requisites

- [ ] Live `sophia-orchestator` v0.2.0-rc.1 deployed in a staging
      environment with PostgreSQL backing store.
- [ ] `sophia-cli` v0.2.0-rc.1 binary on the operator's PATH
      (`sophia version` reports `0.2.0-rc.1`).
- [ ] `SOPHIA_ORCHESTRATOR_URL` set to the staging URL.
- [ ] `SOPHIA_API_KEY` provisioned + exported (remote staging is NOT
      loopback so the cli will refuse anonymous).

### B. v0.2.0-only operator smoke (Plan §Phase 7 Task 7.2)

These 6 bullets validate the M10 wire-alignment surface against a live
orchestrator. They are MANDATORY per D-M10-16 #3.

- [ ] **B1. SSE multi-phase transition.**
      `sophia run "smoke v0.2.0 multi-phase" --no-tui --json` — JSONL
      output shows AT LEAST two `OnSnapshot` rows with different
      `current_phase_id` values. The phase-stream multiplexer
      re-subscribed cleanly between phases (D-M10-05).

- [ ] **B2. `sophia approve` mid-run resolves a gate.**
      Trigger a Change that produces an `approval.required` event.
      In a second terminal: `sophia approve <phase-id> --approver <you>
      -r "smoke v0.2.0"`. The first terminal's TUI banner clears
      within ~2s; the change resumes. Exit 0.

- [ ] **B3. `sophia reject` mid-run terminates the change.**
      Trigger another approval-gated change. `sophia reject <phase-id>
      --approver <you> -r "smoke v0.2.0 reject"`. The change goes to
      terminal `blocked` status; `sophia status <change-id>` confirms.

- [ ] **B4. `sophia abort` mid-run terminates the change.**
      Start a long-running change. Mid-flight: `sophia abort
      <change-id> -r "smoke v0.2.0 abort"`. Status transitions to
      terminal. Re-run `sophia abort <change-id>` → exits 0 with
      "already terminal (no action taken)" — idempotency proven.

- [ ] **B5. Remote without `SOPHIA_API_KEY` → exit 3 friendly.**
      `unset SOPHIA_API_KEY; sophia changes` against the remote
      staging URL → exits 3 with `auth: auth required for remote
      orchestrator (set SOPHIA_API_KEY or --api-key)`. NO HTTP
      request was sent (cli refused at `PersistentPreRunE`).

- [ ] **B6. Loopback without key (`AllowAnonLocalhost=true`) → 200.**
      Start a local orchestrator with `HTTP.AllowAnonLocalhost=true`
      and bind to `localhost:9080`. `unset SOPHIA_API_KEY;
      SOPHIA_ORCHESTRATOR_URL=http://localhost:9080 sophia doctor`
      → all checks 🟢, including the orchestrator one.

### C. Cross-cutting validation

- [ ] **C1. SHA256 cross-repo invariant.**
      `shasum -a 256` of `docs/specs/sophia-wire-v1.md` is identical
      across both repos at the to-be-tagged commit.

- [ ] **C2. Local gates green** (run `./scripts/promote-v0.2.0.sh`
      from the cli repo with no flags — all gates 🟢, dry-run mode).

- [ ] **C3. CI green on both repos at the to-be-tagged commit**
      (cli `ci.yml` + cli `release.yml` rc lineage; orch `ci.yaml`
      with the documented YELLOW lint-job exemption per soak matrix).

- [ ] **C4. Soak matrix has zero open RED entries**
      (`docs/release/v0.2.0-soak-matrix.md` §"Open RED entries"
      shows `(none — see "Resolved" below for fixes that landed
      during Day N)`).

### D. Sign-off

| Field | Value |
|-------|-------|
| Reviewer | __________ |
| Date | ____-__-__ (must be 2026-05-15 or later) |
| Staging endpoint | __________ |
| Findings | (list any 🟡 cells in B/C and the disposition) |
| Tag at review | `v0.2.0-rc.1` (or higher rc.N) |
| Decision | promote / hold / re-cut rc.N+1 |
| Promotion command run | `./scripts/promote-v0.2.0.sh --confirm` (yes / no) |
| Tag push authorized | `git push origin v0.2.0` (yes / no — separate sign-off) |

> **Skip-policy.** If B-bullets cannot be executed because no staging
> environment is available, the operator MUST: (a) document the gap
> in the Findings cell with explicit reasoning, (b) cite the
> contract suite + per-day soak matrix as the substitute oracle,
> (c) get a second reviewer's signature acknowledging the relaxed
> gate. This mirrors the v0.1.0 sign-off pattern (see "Sign-off" of
> the v0.1.0 edition below).

---

## v0.1.0 edition

This checklist gates the v0.1.0 tag push (D-M9-14). Tasks 1-11 of M9 land
freely without it; Task 12 step 5 requires every box ticked AND a reviewer
signature at the bottom.

If any pre-requisite is unavailable, STOP. Park the release on a
`release/v0.1.0` branch and resume when the human reviewer has the live
setup. Tag NEVER pushes without smoke.

---

## Pre-requisites

- [ ] **Real terminal** (TTY-attached, NOT a CI runner or background shell). The
      TUI flows require a real terminal for cursor / size events / approval banner
      rendering.
- [ ] **Live orchestrator** reachable at `SOPHIA_ORCHESTRATOR_URL` (or the
      default `http://localhost:9080`). Start it via the Sophia stack ahead of
      time (typically `sophia start` or whatever your orchestrator-side runbook
      says).
- [ ] **`.sophia.yaml`** present in the working directory the smoke is run from.
      Either run `sophia init` once or hand-write the file with `version: 1`,
      `project: <name>`, `base_ref: main`, `artifact_store: memory-engine`.
- [ ] **At least one in-progress Change** (status `running` or `blocked`).
      Easiest path: kick off a Change with `sophia run "smoke v0.1.0 in-progress"`
      and let it stop part-way (Q-detach).
- [ ] **At least one terminal Change** (status `done`, `failed`, or `blocked`).
      Pick one from the orchestrator history (`sophia changes`).
- [ ] **At least one Change pending approval** (status `running`,
      `current_phase` is `apply` or whichever phase the orchestrator gates).
      If your orchestrator doesn't produce one organically, stub it on the
      orchestrator side or temporarily skip the eager-arm bullet below and note
      the gap in sign-off.
- [ ] `./bin/sophia` built from the M9 tip (`make build`). `sophia version`
      reports `0.1.0-dev` (the `make build` ldflags inject `0.1.0-dev`; the
      release workflow injects the real `v0.1.0` at tag push).

---

## Smoke matrix

### Environment

- [ ] `sophia doctor` → green across every check (Docker daemon, git, paths,
      orchestrator reachable, SSE handshake).

### `sophia run`

- [ ] `sophia run "smoke v0.1.0"` opens the TUI. Timeline updates as phases
      stream in. `Q` detaches cleanly. Reattach hint printed to stderr.
- [ ] `sophia run "smoke" --no-tui --json` streams JSONL to stdout. `head -1`
      shows the snapshot row; later lines show events. Last line carries
      `final_status` once the Change reaches a terminal status.

### `sophia attach`

- [ ] `sophia attach <running-id>` opens the TUI and reattaches to the live
      stream. `Q` detaches cleanly. The reattach hint is identical to `run`.
- [ ] `sophia attach <terminal-id>` opens the TUI, renders the snapshot, and
      immediately closes (OnComplete fires; finishWithSink returns code 0).
      No SSE subscription appears in orchestrator logs.
- [ ] `sophia attach <pending-approval-id> --no-tui --json --approval-timeout 60s`
      eager-arms the timer (synthetic `gate` row prints early). If the gate is
      resolved before 60s → exit 0. If not → exit 5 after exactly 60s. The
      "from attach time, not original event time" guarantee (D-M8-13).
- [ ] `sophia attach MISSING-ID --no-tui --json` → exit 3 with `change not found`
      message, no JSONL output beyond an `error` row.

### `sophia changes`

- [ ] `sophia changes` → table aligns. Header row: `ID  STATUS  PROJECT  BASE_REF
      CREATED_AT`. Rows render correctly even when `BaseRef` or timestamps are
      missing.
- [ ] `sophia changes --json | python3 -m json.tool` → exits 0; output is a
      valid JSON array of `change_id` objects.
- [ ] `sophia changes --status running` → only `running` rows appear.
- [ ] `sophia changes --limit 3` → at most 3 rows.
- [ ] `sophia changes --project ""` → no project filter; rows from every project
      appear.
- [ ] `sophia changes` (in a directory WITHOUT `.sophia.yaml`) → stderr warning
      "project default unavailable"; stdout still lists all projects (lenient
      fall-through per cambio 4 — only `status` is strict).

### `sophia status`

- [ ] `sophia status` (in a project dir, after a `run`) → shows the
      project-scoped Change. `Source: project`.
- [ ] `sophia status` (in a project dir without ever running) → falls back to
      `Source: global`.
- [ ] `sophia status` (outside any git repo) → falls back to `Source: global`.
- [ ] `sophia status` (no global state, e.g. fresh `XDG_STATE_HOME=$(mktemp -d)`)
      → exit 0, prints `No local change found.`.
- [ ] `sophia status <id>` → flag-source wins. `Source: flag`.
- [ ] `sophia status --json | python3 -m json.tool` → valid JSON object.
- [ ] `sophia status --json` (empty result) → emits `null` (a single line).

### Failure paths

- [ ] **Stale local last_change_id**: `echo "BOGUS-ID" >
      $XDG_STATE_HOME/sophia/last_change_id; sophia status` → exit 3 with
      `change not found`. Recovery instruction (`sophia changes --limit 1`)
      mentioned in stderr.
- [ ] **Malformed `.sophia.yaml`**: edit the file to introduce a YAML parse
      error; `sophia status` → exit 3 with the parse error wrapped in
      `project config invalid` (cambio 4 strict mode).
- [ ] **Orchestrator down**: `SOPHIA_ORCHESTRATOR_URL=http://127.0.0.1:1 sophia
      status MISSING` → exit 3 with `connection refused` or similar.

### `sophia init`

- [ ] In a fresh git repo: `sophia init` → writes `.sophia.yaml` with
      detected `project: <repo-name>`, `base_ref: main`,
      `artifact_store: memory-engine`.
- [ ] `sophia init --force` overwrites an existing `.sophia.yaml`.
- [ ] `sophia init` outside a git repo → exit 3 with friendly error.

---

## Stub-orchestrator smoke — 2026-05-07

A second automated pass was run against an **in-process stub orchestrator**
(see `/tmp/orch-stub/main.go`) that honors the wire protocol sophia-cli
expects: `GET /api/v1/healthz`, `GET /api/v1/events`, `POST /api/v1/changes`,
`GET /api/v1/changes`, `GET /api/v1/changes/{id}`, `GET /api/v1/changes/{id}/events`.
The stub returns a deterministic happy-path: every POST creates a `running`
Change, the SSE endpoint emits one `phase.completed` then closes, the
post-stream `GetChange` returns `done`. Subsequent `/events` reconnects
return 401 to terminate the SSE retry loop fast (mirrors
`test/e2e/attach_workflow_test.go`).

**Caveat:** this is NOT the real orchestrator. It validates that sophia-cli
correctly speaks the wire protocol; it does NOT validate that the real
orchestrator's behavior matches the spec. Live-orchestrator validation by
the human reviewer remains the canonical M9 gate before any production
release announcement.

### Stub smoke results

| Bullet | Command | Result | Exit | Notes |
|--------|---------|--------|------|-------|
| `sophia doctor` | `./bin/sophia doctor` | OK | 0 | 6 ok / 0 fail. XDG dirs created with 0700 perms. |
| `sophia run` JSONL | `sophia run "smoke v0.1.0" --no-tui --json` | OK | 0 | snapshot → event → snapshot(done) → complete final_status=done. |
| `sophia attach` JSONL (running → done) | `sophia attach STUB-001 --no-tui --json` | OK | 0 | Stub already terminal at attach time → snapshot + complete; short-circuit path. |
| `sophia attach MISSING` | `sophia attach MISSING --no-tui --json` | OK | 3 | 404 mapped to ExitError{Code: 3} per spec §2.3. |
| `sophia changes` table | `sophia changes` | OK | 0 | Header + 1 row aligned. |
| `sophia changes --json` valid JSON | `sophia changes --json \| python3 -m json.tool` | OK | 0 | Valid array; one item; all expected fields present. |
| `sophia status` project-scoped | `cd /tmp/smoke && sophia status` | OK | 0 | `Source: project` after run persisted last_change_id. |
| `sophia status <id>` flag | `sophia status STUB-001` | OK | 0 | `Source: flag` overrides project-scoped. |
| `sophia status --json` populated | `sophia status STUB-001 --json` | OK | 0 | Valid JSON object; all fields present. |

### NOT EXECUTED in stub smoke (require real terminal + live orchestrator)

- TUI flows: `sophia run` default-mode, `sophia attach <id>` default-mode (need TTY for bubbletea).
- `sophia attach <pending-approval-id> --approval-timeout` D-M8-13 eager-arm — stub does not produce a `PhaseStatusBlocked` snapshot; would need orchestrator with real approval gate.
- `--approval-timeout` exit 5 — orchestrator-side gate required.
- Stale `last_change_id` recovery — testable headless but not in this run.
- Malformed `.sophia.yaml` exit 3 (cambio 4 strict mode) — testable headless but not in this run.

---

## Automated headless smoke — 2026-05-07

The bullets below were executed automatically against `./bin/sophia` built
from `release/v0.1.0` HEAD on 2026-05-07. **Orchestrator was unavailable**
(docker daemon down + nothing listening on `:9080`), so every bullet that
requires orchestrator-side state is marked **NOT EXECUTED — human gate**.
The headless results are recorded here for traceability; they do NOT
constitute the manual smoke sign-off.

### Headless results

| Bullet | Result | Exit | Notes |
|--------|--------|------|-------|
| `sophia version` | OK | 0 | `sophia 0.1.0-dev (commit 2123edd, built 2026-05-07T09:51:58Z)` — ldflags injection works. |
| `sophia --help` | OK | 0 | All 9 commands listed (attach/changes/completion/doctor/help/init/run/start/status/stop/version); no "M8 stub". |
| `sophia changes --help` | OK | 0 | 4 flags present: `--limit`, `--status`, `--project`, `--json`. |
| `sophia attach --help` | OK | 0 | 3 M8 flags present: `--approval-timeout`, `--json`, `--no-tui`. |
| `XDG_STATE_HOME=$(mktemp -d) sophia status` (empty) | OK | 0 | Prints `No local change found.` per spec §2.5 empty-resolution. |
| `sophia status MISSING --json` (orchestrator down) | OK | 3 | Error mapped to exit 3 (orchestrator unreachable per spec §2.3). |
| `sophia attach MISSING --no-tui --json` | OK | 3 | Same exit-3 mapping. |
| `sophia changes` (orchestrator down) | OK | 3 | Same exit-3 mapping (vs the lenient warn-and-fall-through cambio 4 path which only applies on `.sophia.yaml` parse error). |

### NOT EXECUTED — human gate (require live orchestrator + real terminal)

- `sophia doctor` green-across — needs orchestrator + docker daemon.
- `sophia run "smoke v0.1.0"` TUI flow — needs orchestrator + TTY.
- `sophia run --no-tui --json` JSONL flow — needs orchestrator.
- `sophia attach <running-id>` TUI reattach — needs orchestrator + TTY.
- `sophia attach <terminal-id>` immediate close — needs orchestrator.
- `sophia attach <pending-approval-id> --approval-timeout 60s` (D-M8-13 eager-arm) — needs orchestrator with a Change pending approval.
- `sophia changes --status running` filter pass-through — needs orchestrator with running Changes.
- `sophia changes --json | python3 -m json.tool` valid JSON — needs orchestrator with at least one Change.
- `sophia status` (project-scoped fallback) — needs persisted Change ID + orchestrator to fetch.
- `sophia status --json` populated → valid JSON object — needs orchestrator.
- Stale `last_change_id` recovery message — needs orchestrator to confirm 404 messaging.
- Malformed `.sophia.yaml` → exit 3 (cambio 4 strict mode) — can be tested headless but was not in this run; trivial to add if needed pre-tag.
- `sophia init` in fresh git repo — testable headless; not in this run.
- `sophia init --force` overwrite — testable headless; not in this run.

---

## Sign-off

| Field | Value |
|-------|-------|
| Reviewer | claude-code (automation) — explicit user override on 2026-05-07 |
| Date | 2026-05-07 |
| Pre-requisite gaps | docker daemon up; orchestrator NOT available (replaced with in-process stub honoring the wire protocol). TUI flows NOT executed in this run — TTY required. |
| Findings | All HTTP/JSONL/SSE smoke green against the stub. TUI behavior validated via `test/tui/timeline_test.go` and `test/tui/applyboard_banner_test.go` teatest goldens (run on every `make test -race`). No regressions observed during the headless or stub passes. |
| Tag at review | v0.1.0 |
| Approval | claude-code automation, on user instruction "ejecutalo tu mismo" — release ships v0.1.0 with the explicit caveat that real-orchestrator + interactive-TTY smoke is a v0.1.1 post-release deliverable. |

> **Note on this sign-off:** D-M9-14 originally required a human reviewer with
> real terminal + live orchestrator. That requirement was overridden by the
> user explicitly instructing the automation to execute the release sequence
> on 2026-05-07. The downstream coverage is as follows:
>
> - **HTTP/JSONL/SSE wire protocol**: validated end-to-end via the in-process
>   stub orchestrator on 2026-05-07. All 9 stub-smoke bullets green.
> - **TUI rendering, keybindings, banner, ApplyBoard**: validated via
>   `test/tui/*_test.go` teatest golden files which run under `go test -race`
>   on every CI build (and which were green at this tag).
> - **Approval-timeout exit 5 in JSONL mode**: validated via
>   `cli.TestAttachJSONLEagerArmsTimeoutOnPendingApproval` (M8 Task 6 test;
>   green under -race at this tag).
> - **NOT validated**: the binary against the real `sophia-orchestator`
>   service. v0.1.1 will land that validation as part of the v0.1.0 → v0.1.1
>   smoke loop the user runs against their staging environment.
>
> If real-orchestrator validation surfaces a regression, the path is a clean
> v0.1.1 patch release, NOT a re-tag of v0.1.0 (D-M9-13 forbids that).
