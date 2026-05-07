# Manual Smoke Checklist — v0.1.0

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
      `project: <name>`, `base_ref: main`, `artifact_store: engram`.
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
      `artifact_store: engram`.
- [ ] `sophia init --force` overwrites an existing `.sophia.yaml`.
- [ ] `sophia init` outside a git repo → exit 3 with friendly error.

---

## Sign-off

| Field | Value |
|-------|-------|
| Reviewer | __________ |
| Date | ____-__-__ |
| Pre-requisite gaps | (none / list any skipped bullets above) |
| Findings | (none / list any unexpected behaviors observed) |
| Tag at review | v0.1.0 |
| Approval | __________ (sign here once every applicable bullet is ticked) |

Once signed, commit this file with the smoke results captured in the
"Pre-requisite gaps" / "Findings" rows. The release workflow's tag push is
authorized by the presence of a signed checklist on the same commit lineage
as the tag.
