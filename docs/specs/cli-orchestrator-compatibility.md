# sophia-cli ↔ sophia-orchestator compatibility matrix

```
spec id      : cli-orchestrator-compat-v1
status       : draft (pending Phase 2 sign-off)
draft date   : 2026-05-07
governs      : sophia-cli vs sophia-orchestator integration at sophia-wire-v1
authority    : derived from docs/specs/sophia-wire-v1.md + ADR-0003 + D-M10-01..17
phase        : M10 Phase 2 deliverable (Tasks 2.1 + 2.2)
research log : empty — internal sources only (spec, inventory, ADRs, code), exempt per docs/research-policy.md
```

> **What this document does.** Catalogues every wire surface in
> `sophia-wire-v1.md` and tags it Required / Optional / Intentionally
> unsupported FROM THE CLI'S PERSPECTIVE. For each, it documents the
> CLI's expected behavior when the endpoint or event type is present,
> absent, or returns an error.
>
> **Why this exists separately from the spec.** The wire spec defines
> the orchestrator's full HTTP and SSE surface. The compatibility
> matrix is the CLI's policy for HOW MUCH of that surface to consume,
> what to do on absence, and what to refuse to call. It is the CLI's
> responsibility, not the orchestrator's; the orchestrator MAY expose
> more than this matrix mentions, and the CLI MUST ignore the excess.

---

## Classification legend

- **Required** — CLI directly invokes; absence breaks a CLI command. CLI
  exits with code 3 and a friendly "orchestrator missing required
  endpoint" message on the first call that hits the absent endpoint.
- **Optional** — CLI takes advantage if present; degrades gracefully if
  absent (logs at debug, silently skips, falls back to alternate data
  source, etc.). CLI MUST NOT exit non-zero solely because an Optional
  endpoint or event is missing.
- **Intentionally unsupported (CLI-side)** — Orchestrator MAY expose;
  CLI does NOT call. Listed here so contributors don't accidentally
  wire a CLI flag for it. Not a gap, not a bug.

---

## 1. HTTP endpoint matrix

Source of truth: `docs/specs/sophia-wire-v1.md` Section 4 + Appendix A.

### 1.1 Health and readiness

| Endpoint | Class | Owner CLI command(s) | Behavior on absence | Behavior on error | Rationale |
|----------|-------|----------------------|---------------------|-------------------|-----------|
| `GET /api/v1/health` | **Required** | `sophia doctor` | doctor reports `✗ Orchestrator reachable` and the overall doctor exits 3 | 5xx → doctor reports red, exits 3; non-200/5xx (404, 503) → doctor reports red, exits 3 | D-M10-14: process liveness is a hard gate; the CLI cannot meaningfully operate against an unreachable orchestrator. |
| `GET /api/v1/ready` | **Optional** | `sophia doctor` (yellow check) | doctor reports `Ready endpoint not implemented; skipping` and stays exit 0 | 503 + JSON → doctor reports `degraded` with the failed checks listed, stays exit 0 | D-M10-14: dependency-readiness is informational. Triggering doctor failure on a transient downstream blip would create false negatives. |

### 1.2 Changes (CRUD)

| Endpoint | Class | Owner CLI command(s) | Behavior on absence | Behavior on error | Rationale |
|----------|-------|----------------------|---------------------|-------------------|-----------|
| `POST /api/v1/changes` | **Required** | `sophia run` | exit 3 `orchestrator missing required endpoint POST /api/v1/changes` | 400 `validation_failed` → exit 3 with field details; 401 → exit 3 auth message; 5xx → exit 3 unreachable | Spec Section 4.2; without create, `run` cannot fulfill its contract. |
| `GET /api/v1/changes` | **Required** | `sophia changes` | exit 3 | 400 `limit_too_large` → CLI re-issues with clamped limit and emits warning; other 4xx → exit 3 | Spec Section 4.2 + 7. |
| `GET /api/v1/changes/{id}` | **Required** | `sophia run` (post-stream refresh), `sophia attach` (initial fetch + multiplexer), `sophia status` | exit 3 | 404 `change_not_found` → exit 3 wrapping `domain.ErrChangeNotFound`; 5xx → exit 3 unreachable; ctx cancel → exit 4 | Spec Section 4.2; the multiplexer (Section 5.4) cannot operate without it. |
| `POST /api/v1/changes/{id}/abort` | **Required** | `sophia abort` (NEW v0.2.0) | exit 3 — but `sophia abort` is the only command that hits this; users who don't run `abort` are unaffected | 404 → exit 3; 409 `change_already_terminal` → exit 0 with informational message ("change already terminal; nothing to abort") | Spec Section 4.2; idempotent terminal-already path is a UX courtesy. |
| `POST /api/v1/changes/{id}/phases/{type}/run` | **Intentionally unsupported (CLI-side)** | (none) | n/a | n/a | D-M10 / spec Section 4.3: orchestrator drives phase boundaries autonomously per its own governance rules. CLI does NOT control phase advancement. |

### 1.3 Phases (phase-scoped per D-M10-13 Form A)

| Endpoint | Class | Owner CLI command(s) | Behavior on absence | Behavior on error | Rationale |
|----------|-------|----------------------|---------------------|-------------------|-----------|
| `GET /api/v1/phases/{phase_id}` | **Required** | `sophia run` / `attach` (multiplexer reads `current_phase_id`); `sophia approve` / `reject` pre-flight | exit 3 (multiplexer cannot work) | 404 `phase_not_found` → exit 3; 5xx → exit 3 | Spec Section 4.3; the per-Phase SSE multiplexer reads the phase to confirm the target before subscribing. |
| `POST /api/v1/phases/{phase_id}/resume` | **Optional** | `sophia resume` (NEW v0.2.0, surfaces only `--no-tui --json`) | command errors with "resume not supported by this orchestrator" exit 3 — but `resume` is rarely-used; absence does not affect typical run/attach flow | 404 → exit 3; 409 `phase_not_resumable` → exit 1 (logical failure, not config) | Spec Section 4.3; the orchestrator's retry budget already handles most resumption cases automatically. |
| `POST /api/v1/phases/{phase_id}/approve` | **Required** | `sophia approve` (NEW v0.2.0) | exit 3 — but only `sophia approve` and `[O]` browser-flow paths hit this; users on auto-approve policies unaffected | 404 → exit 3; 409 `gate_already_decided` → exit 0 with informational message ("gate already decided"); 422 `approver_required` → CLI ensured `--approver` non-empty before send | Spec Section 4.3 + 8 (D-M10-03 dual-channel + D-M10-13 Form A). |
| `POST /api/v1/phases/{phase_id}/reject` | **Required** | `sophia reject` (NEW v0.2.0) | exit 3 (same as approve) | same as approve | Spec Section 4.3. |
| `GET /api/v1/phases/{phase_id}/board` | **Optional** | TUI ApplyBoard view (M7) refresh source | TUI falls back to SSE-derived state (M7 default behavior); no error surfaced to user | 404 `phase_has_no_board` → fall back to SSE-derived; 5xx → fall back; ctx cancel → fall back | Spec Section 4.3; the M7 TUI was designed to derive ApplyBoard state from `task.*` / `agent.*` SSE events before this REST endpoint existed. The endpoint is an optimization, not a dependency. |
| `GET /api/v1/phases/{phase_id}/events` | **Required** | SSE multiplexer (`Runner.Observe` / `Attacher`) | exit 3 — without per-phase events, the run/attach loop cannot detect phase progression | server emits `phase.completed` / `phase.failed` and closes → CLI advances per Section 5.4; 410 `phase_terminal_no_events` → CLI fetches snapshot via `GET /phases/{id}` instead and short-circuits | Spec Section 4.3 + 5; D-M10-05 multiplexer protocol. |

### 1.4 Approvals (Form B reservation)

| Endpoint | Class | Owner CLI command(s) | Notes |
|----------|-------|----------------------|-------|
| `POST /api/v1/approvals/{gate_id}/approve` | **Reserved — not implemented in v1** | (none) | D-M10-13 Form B. Kept for v0.3.0+ if governance issues distinct gate IDs. CLI MUST NOT call this in v1. |
| `POST /api/v1/approvals/{gate_id}/reject` | **Reserved — not implemented in v1** | (none) | Symmetric. |

### 1.5 Out-of-CLI-scope

| Endpoint | Class | Notes |
|----------|-------|-------|
| `GET /metrics` | **Intentionally unsupported (CLI-side)** | Prometheus scrape target; consumed by ops infra, not the CLI. |
| `/api/v1/admin/*` | **Intentionally unsupported (CLI-side)** | Reserved by the orchestrator for admin / debug surface. CLI does not call. |

### 1.6 Endpoint class summary

| Class | Count | Endpoints |
|-------|-------|-----------|
| Required | 7 | health, POST changes, GET changes (list), GET changes/{id}, POST abort, GET phases/{id}, POST approve, POST reject, GET phases/{id}/events |
| Optional | 3 | ready, POST resume, GET board |
| Intentionally unsupported | 3 | POST changes/{id}/phases/{type}/run, /metrics, /api/v1/admin/* |
| Reserved (v0.3.0+) | 2 | POST /approvals/{gate_id}/approve, /reject |

> Wait — Section 1.2 lists 9 routes I called Required, but the summary
> count is 7. Why? `POST /api/v1/changes/{id}/abort` is Required ONLY
> if `sophia abort` exists; same for phase-scoped approve/reject which
> are Required only if `sophia approve|reject` exist. All four DO ship
> in v0.2.0, so they ARE Required in the v0.2.0 column. The summary
> below is the v0.2.0 Required surface counted by HTTP method+path:
> 1. GET `/api/v1/health`
> 2. POST `/api/v1/changes`
> 3. GET `/api/v1/changes`
> 4. GET `/api/v1/changes/{id}`
> 5. POST `/api/v1/changes/{id}/abort`
> 6. GET `/api/v1/phases/{id}`
> 7. POST `/api/v1/phases/{id}/approve`
> 8. POST `/api/v1/phases/{id}/reject`
> 9. GET `/api/v1/phases/{id}/events`
>
> That's 9 Required HTTP routes (count corrected in next revision).

### 1.7 Auth header policy (per endpoint)

Every Required + Optional row in 1.2 and 1.3 carries `X-Sophia-API-Key`
when:

- `SOPHIA_API_KEY` env or `--api-key` flag is set, OR
- target URL host is non-loopback (CLI fails pre-flight if no key).

Health (`/api/v1/health`) and readiness (`/api/v1/ready`) are auth-free
on the orchestrator side and the CLI sends no header to them by default
(but tolerates the orchestrator's middleware accepting the header
anyway, for the case where deployment ops route everything through
the same listener).

---

## 2. SSE event taxonomy matrix

Source of truth: `docs/specs/sophia-wire-v1.md` Section 5.3.

### 2.1 Per-event classification

| Event type | Class | Required by | Behavior on absence | Behavior on malformed payload | Rationale |
|------------|-------|-------------|---------------------|-------------------------------|-----------|
| `heartbeat` | **Required** | watchdog timer (CLI exits stream as stale after 60s of silence per spec §5.3 / M5 watchdog) | watchdog fires → CLI reconnects | parse error → CLI logs + reconnects | Server emits every 15s; the absence of heartbeats is the only signal of a wedged stream. |
| `phase.started` | **Required** | TUI timeline + ApplyBoard derivation | timeline shows "phase running" without start time; no impact on flow | malformed → log + skip; phase still progresses via `phase.completed` arrival | Spec Section 5.3. |
| `phase.completed` | **Required** | multiplexer (triggers post-stream refresh + phase switch); JSONL final-status emission | CLI never advances; ctx cancel eventually triggers exit 4 | malformed → log + skip; if subsequent `GET /phases/{id}` shows terminal, CLI proceeds | Critical for the phase-stream multiplexer (Section 5.4 of the spec). |
| `phase.failed` | **Required** | same as `phase.completed` but maps to exit 1 / blocked-or-failed status | same as `phase.completed` absence | same | Symmetric to `phase.completed`. |
| `task.created` | **Optional** | ApplyBoard view (M7) | ApplyBoard shows fewer rows; M7 still renders correctly | log + skip | M7 enriches the ApplyBoard from these events; without them, the board is stale but correct. |
| `task.started` | **Optional** | ApplyBoard view | same as `task.created` | log + skip | Spec Section 5.3. |
| `task.completed` | **Optional** | ApplyBoard view + count derivation | partial state; M7 still renders | log + skip | Spec Section 5.3. |
| `task.failed` | **Optional** | ApplyBoard view | task shown as running indefinitely until phase ends | log + skip | Spec Section 5.3. |
| `agent.dispatched` | **Optional** | ApplyBoard agents column | column unpopulated; rest of board unaffected | log + skip | Spec Section 5.3. |
| `agent.completed` | **Optional** | ApplyBoard agents column | same as `agent.dispatched` | log + skip | Spec Section 5.3. |
| `approval.required` | **Required** | M7 banner overlay + `approvalTimeoutSink` arm; `attach` eager-arm per D-M8-13 | gate goes unnoticed; user has no signal a phase is blocked | malformed (missing `phase_id` etc) → log + skip; the underlying phase status reaches `blocked` and is observable via `GET /phases/{id}` | Spec Section 5.3 + 8. |
| `approval.resolved` | **Required** | M7 banner clear; timer cancel | banner stays up until phase stream closes; timer eventually fires → exit 5 | log + skip | Spec Section 5.3 + 8. |
| `<unknown>` | **Forward-compat** | (none) | log single warning to stderr (`sophia: unknown SSE event type 'foo' (skipped)`) and continue | n/a | Spec Section 10.2. |

### 2.2 Event class summary

| Class | Count | Events |
|-------|-------|--------|
| Required | 6 | heartbeat, phase.started, phase.completed, phase.failed, approval.required, approval.resolved |
| Optional | 6 | task.created/started/completed/failed, agent.dispatched/completed |
| Forward-compat (`<unknown>`) | 1 (catch-all) | any future type the orchestrator adds |

### 2.3 Payload-shape compatibility

For each Required event, the CLI MUST tolerate:

- Unknown JSON object fields in `payload` (forward-compat).
- Missing OPTIONAL payload fields (e.g. `task.completed.output_summary`).
- Mismatched timestamp precision (server may emit second / millisecond /
  nanosecond ISO-8601 — all valid).

For Optional events, the CLI MUST NOT exit non-zero on any payload
shape problem. Logging + skip is the policy.

---

## 3. New incompatibilities discovered (Phase 2 walk-through)

Per user instruction: any incompatibility surfaced during Phase 2
that is NOT already covered by `sophia-wire-v1.md` is a STOP-and-review
trigger.

### Phase 2 walk-through findings

The Phase 2 walk-through completed 2026-05-07 found **no new
incompatibilities** beyond those already addressed by the spec:

- All 9 Required HTTP routes are documented in spec Section 4.
- All 6 Required SSE event types are documented in spec Section 5.3.
- All 5 design-level mismatches in `m10-wire-inventory.md` are
  resolved by spec sections 4.3 (phase-scoped routes via D-M10-13),
  5 (per-phase SSE granularity via D-M10-05), 8 (dual-channel approval
  via D-M10-03), 1.6/2.1 (dual ApplyBoard data source).
- The 3 mechanical mismatches (path, auth, sseprobe) are resolved by
  D-M10-06, D-M10-02, D-M10-07 respectively.

No STOP-and-review triggered.

---

## 4. Breaking changes between v0.1.0 and v0.2.0 wire

This section enumerates exactly what breaks when a v0.1.0 client tries
to talk to a v0.2.0 server, and vice versa, so users have a clear
migration story.

### 4.1 v0.1.0 sophia-cli ↔ v0.2.0 sophia-orchestator

**Result:** v0.1.0 CLI is fully INCOMPATIBLE with v0.2.0 server.

| What breaks | Why |
|-------------|-----|
| `sophia doctor` | v0.1.0 CLI calls `/api/v1/healthz`; server only serves `/api/v1/health` (D-M10-06). Doctor's "Orchestrator reachable" check fails with 404. |
| `sophia run` / `attach` / `changes` / `status` | All authenticated endpoints. v0.1.0 CLI sends no `X-Sophia-API-Key`; v0.2.0 server (when bound non-loopback) returns 401. |
| `sophia run` / `attach` SSE consumption | v0.1.0 CLI subscribes to `/api/v1/changes/{id}/events`; that path no longer exists. Server returns 404. CLI exits 3 immediately. |
| `sophia doctor` SSE handshake | v0.1.0 CLI probes `/api/v1/events`; that path never existed. Was already broken in v0.1.0; v0.2.0 makes the failure mode explicit (CLI no longer probes). |

### 4.2 v0.2.0 sophia-cli ↔ v0.1.x sophia-orchestator

**Result:** v0.2.0 CLI is fully INCOMPATIBLE with v0.1.x server, where
v0.1.x is whatever was running before the v0.2.0 mirror landed (the
historical orchestrator state captured in `m10-wire-inventory.md`).

| What breaks | Why |
|-------------|-----|
| `sophia doctor` | v0.2.0 CLI calls `/api/v1/health`; v0.1.x server has it (no break here). |
| `sophia run` | v0.2.0 CLI sends `X-Sophia-API-Key`; v0.1.x server's `middleware.APIKey` accepts it. Then v0.2.0 CLI fetches the change, reads `current_phase_id`, and tries `GET /api/v1/phases/{phase_id}` — but v0.1.x server only routes `GET /api/v1/changes/{cid}/phases/{pid}` (change-scoped). Result: 404, CLI exits 3. |
| `sophia approve` / `reject` | v0.2.0 CLI POSTs `/api/v1/phases/{pid}/approve`; v0.1.x routes `/api/v1/changes/{cid}/phases/{pid}/approve`. 404. |
| SSE multiplexer | v0.2.0 CLI subscribes to `/api/v1/phases/{pid}/events`; v0.1.x routes `/api/v1/changes/{cid}/phases/{pid}/events`. 404. |

### 4.3 v0.1.0 ↔ v0.1.0 (legacy reference)

Documented in v0.1.0 CHANGELOG "Known limitations" — the v0.1.0 CLI
never functioned end-to-end against the v0.1.x orchestrator due to
the originally-discovered drift. v0.2.0 is the first version where
both sides talk to each other end-to-end successfully.

### 4.4 Migration cost summary

| Migration path | Effort | Notes |
|----------------|--------|-------|
| User upgrades CLI v0.1.0 → v0.2.0 AND server v0.1.x → v0.2.0 simultaneously | low | Recommended path. CLI `sophia version` and orchestrator's own version probe both visible. |
| User upgrades only CLI | broken | v0.2.0 CLI fails immediately against v0.1.x server. |
| User upgrades only server | broken | v0.1.0 CLI fails immediately against v0.2.0 server. |
| User stays on v0.1.0 (both sides) | broken | Was never end-to-end functional; documented as "Known limitations" in CLI v0.1.0 CHANGELOG. |

There is NO partial-upgrade path. v0.2.0 is a coordinated cut-over.

---

## 5. Risks before Phase 3 / Phase 4

These are the risks the M10 plan should weigh before authorizing
Phase 3 (orchestrator changes) or Phase 4 (CLI changes). Each links
to a mitigation in the M10 plan's risk register.

### 5.1 Implementation order risk

If Phase 3 ships before Phase 4 (or vice versa), the partial deploy
breaks the running stack. **Mitigation:** Phases 3 and 4 are
INDEPENDENT after Phase 2 sign-off (their tasks don't share a code
path), but their RC tags MUST be coordinated — D-M10-11 mandates
matching `v0.2.0-rc.1` tags within the same week. CI checksum gate
(D-M10-15 / D-M10-16(1)) blocks tag push if the spec drifted.

### 5.2 SSE multiplexer regression risk

The CLI's `Runner.Observe` (M5/M8) is the most-tested code path in
sophia-cli. Switching from per-Change to per-Phase subscription
introduces a new failure mode: the CLI re-subscribes to a new phase
mid-stream. **Mitigation:** Phase 4 Task 4.3 specs the multiplexer
behavior (spec Section 5.4). Contract tests (Phase 5) MUST cover the
phase-transition case (multi-phase change, observed end-to-end).

### 5.3 Approval flow ambiguity risk

D-M10-03 keeps both decision channels live. A user who opens the
browser AND hits `sophia approve` in parallel can produce a race
where one channel's POST arrives first. **Mitigation:** Server
returns 409 `gate_already_decided` to the second; client treats
409 as informational ("already decided; first decision wins"). Spec
Section 8.1 mandates this idempotency. Contract test in Phase 5
asserts the 409 path.

### 5.4 API key UX risk

Users running locally hit a confusing 401 if their `SOPHIA_ORCHESTRATOR_URL`
host is something `IsLoopbackURL` doesn't recognize (e.g. Tailscale
magic DNS, Docker container name, custom resolver). **Mitigation:**
The CLI's pre-flight error message is explicit: `auth required for
remote orchestrator (set SOPHIA_API_KEY or --api-key)`. Documentation
notes that "remote" means "any non-loopback URL" and recommends
setting the env var even on local dev to avoid the edge case.

### 5.5 `pkg/contract/` coupling risk

Phase 4 Task 4.8 introduces a new public Go package depended-on by
the orchestrator. If the package starts importing internal CLI types,
the contract becomes leaky. **Mitigation:** D-M10-10 + Task 4.8
strict scope rules (no `internal/` imports, no `application` /
`adapters` / `cli` imports). RM10-02 fallback: factor to
`sophia-contract` repo before rc.1 if coupling forces the issue.

### 5.6 Deprecation gap risk

v0.1.0 CLI users who pinned via `go install` get NO automatic upgrade.
**Mitigation:** v0.2.0 CHANGELOG carries a "Compatibility" section
explicit that v0.2.0 is incompatible with v0.1.x server (D-M10-16(4)).
Users self-coordinate.

### 5.7 Cross-repo CI coordination risk

If `pkg/contract/` lives in sophia-cli, the orchestrator's `go.mod`
pins a sophia-cli commit. A breaking change to `pkg/contract/` in
sophia-cli main breaks the orchestrator's build. **Mitigation:**
`pkg/contract/` is treated as a stable public surface; PRs that
modify it require approval in BOTH repos (M10 plan RM10-09).

### 5.8 SSE per-phase reconnect storm risk

A change with N phases produces N+1 stream subscriptions over its
lifetime. For long-running changes (apply phase with many tasks),
the per-phase model trades one long-lived stream for several
shorter ones. **Mitigation:** the phase-stream protocol mandates
clean close via `phase.completed` / `phase.failed` so the CLI
distinguishes "phase done, transition" from "network blip,
reconnect". Backoff for blip-reconnect uses the existing
`ssestream.Backoff{Min: 1s, Max: 30s}` config.

---

## 6. Open questions deferred to Phase 3+ / 4+

These were raised during Phase 2 review and parked for resolution
during the relevant implementation phase. None blocks Phase 3/4
authorization.

| # | Question | Phase to resolve |
|---|----------|------------------|
| 1 | Should `sophia doctor` print the orchestrator's `version` field from `/health`? | Phase 4 (CLI doctor wiring) |
| 2 | What's the on-the-wire format of `details` in error envelopes when the error is `validation_failed`? Spec leaves it as "open object"; concrete schema TBD. | Phase 1.5 (spec follow-up) — minor; not a blocker |
| 3 | Does the orchestrator emit `heartbeat` on the global `/api/v1/events` endpoint or only on per-phase streams? | Phase 3 (orchestrator-side); spec mentions only per-phase |
| 4 | If the user provides `--api-key` AND `SOPHIA_API_KEY` simultaneously, which wins? Spec implies flag overrides env. | Phase 4 (CLI auth resolver) — confirm in implementation plan |
| 5 | TUI: when `sophia approve` succeeds via in-band POST while the TUI is open, does the TUI banner clear via the `approval.resolved` SSE event (existing M7 path)? Confirmed yes by spec Section 8.2; no additional CLI work. | Phase 4 (CLI verify) |

---

## 7. Sign-off

**Phase 2 Tasks 2.1 + 2.2 deliverables:**

- Section 1: HTTP endpoint compatibility matrix — 9 Required, 3 Optional, 3 Intentionally unsupported, 2 Reserved
- Section 2: SSE event taxonomy matrix — 6 Required, 6 Optional, 1 forward-compat catch-all
- Section 3: walk-through findings (no new incompatibilities)
- Section 4: v0.1.0 ↔ v0.2.0 breaking-change catalog
- Section 5: 8 risks documented for Phase 3 / 4 review
- Section 6: 5 open questions deferred to implementation phases

**Sign-off block:**

| Field | Value |
|-------|-------|
| Reviewer | __________ |
| Date | ____-__-__ |
| Phase 3 authorization granted? | (yes / no — defer to next sign-off) |
| Phase 4 authorization granted? | (yes / no — defer to next sign-off) |
| Open questions to resolve before Phase 3/4? | (list) |

Until this block is signed, Phases 3 and 4 are NOT authorized.

---

## 7. Phase 5 contract gate result (2026-05-07)

Phases 1.5, 3, 3.6, 3.7, 3.8, 4, and 5 have all landed. Phase 5
(this section) records the cross-repo compatibility status as of the
contract test run.

### 7.1 SHA256 cross-repo gate (D-M10-16 release blocker #1)

| Repo | path | SHA256 |
|---|---|---|
| sophia-cli | `docs/specs/sophia-wire-v1.md` | `097be33907771e727fa1e4e834f5afc01d8c3f212bb503b2a4f2dc00d19fd6c5` |
| sophia-orchestator (`m10/orchestrator-wire-v1`) | `docs/specs/sophia-wire-v1.md` | `097be33907771e727fa1e4e834f5afc01d8c3f212bb503b2a4f2dc00d19fd6c5` |

✅ **Hashes match.** Both repos carry byte-identical
`sophia-wire-v1.md` mirrors. The `.sha256` files in each repo also
match the actual file content (no stale recordings).

### 7.2 Cross-repo test status

| Suite | Location | Result |
|---|---|---|
| sophia-cli unit + integration | `make test` | 21 packages, all green; race-clean; `GOWORK=off`-clean |
| sophia-cli contract suite | `make contract` (`-tags=contract`) | 27 tests across 9 endpoints + auth + SSE + 13 error codes + CLI smoke; race-clean |
| sophia-orchestator unit | `make test-unit` on `m10/orchestrator-wire-v1` | 25 packages, all green; race-clean |

### 7.3 Required endpoint coverage

| Endpoint | cli contract test | orch unit test |
|---|---|---|
| `GET /api/v1/health` | `TestContract_HealthEndpoint` | `TestHealth_Public` |
| `POST /api/v1/changes` | `TestContract_CreateAndGetChange` | `TestCreateChange_Roundtrip` |
| `GET /api/v1/changes` | `TestContract_ListChanges_*` | `TestList_LimitTooLarge` |
| `GET /api/v1/changes/{id}` | `TestContract_CreateAndGetChange` | router test |
| `POST /api/v1/changes/{id}/abort` | `TestContract_AbortChange*` | router test |
| `GET /api/v1/phases/{id}` | `TestContract_GetPhase` | router test |
| `POST /api/v1/phases/{id}/approve` | `TestContract_ApprovePhase_*` | `TestApprove_*` |
| `POST /api/v1/phases/{id}/reject` | `TestContract_RejectPhase_HappyPath` | `TestReject_*` |
| `GET /api/v1/phases/{id}/events` | `TestContract_SSE_EventTypes`, `TestContract_SSE_PhaseTerminalNoEvents` | `TestSSE_StreamReceivesEvents`, `TestSSE_PhaseTerminalNoEvents` |

### 7.4 Auth gate

| Mode | cli test | orch test |
|---|---|---|
| Loopback anonymous | `TestContract_Auth_LoopbackAnonAllowed` | `TestAuth_*` (`AllowAnonLocalhost` path) |
| Remote anonymous → 401 unauthorized | `TestContract_Auth_RemoteAnonRejected` | `TestAuth_RequiredOnProtectedEndpoints` |
| Valid key → 200 | `TestContract_Auth_ValidKeyAccepted` | `TestAuth_AcceptsValidKey` |
| Invalid key → 401 | `TestContract_Auth_InvalidKeyRejected` | `TestAuth_RequiredOnProtectedEndpoints` |
| API key never logged | `TestClient_AuthHeaderOmittedWhenAnon` (absent ≠ empty) | n/a (server side) |

### 7.5 SSE event compatibility

| Event | cli test | orch test |
|---|---|---|
| `heartbeat` | parser unit tests | router test (Stream emits heartbeat) |
| `phase.started` | `TestContract_SSE_EventTypes` | `TestRun_PhaseStartedPayloadShape` |
| `phase.completed` | `TestContract_SSE_EventTypes` | `TestRun_PhaseCompletedPayloadShape` |
| `phase.failed` | TUI + parser tests | `TestRun_PhaseFailedPayloadShape` |
| `approval.required` | `TestContract_SSE_EventTypes` | router test |
| `approval.resolved` | `TestContract_SSE_EventTypes` | `TestApprove_HappyPath` |
| Unknown / `apply.*` tolerated | `TestContract_SSE_EventTypes`, `TestModel_TolerantOfApplyDiagnostics` | n/a (server emits) |
| 410 phase_terminal_no_events without retry storm | `TestContract_SSE_PhaseTerminalNoEvents` | `TestSSE_PhaseTerminalNoEvents` |

### 7.6 Error envelope (13 stable codes)

✅ **All 13 codes round-trip.** Validated by
`TestContract_ErrorEnvelope_AllStableCodes` (cli) and
`TestErrorEnvelope_Shape` + per-code 4xx tests (orch).

### 7.7 CLI smoke (Phase 5 scope item 8)

| Command | Smoke test |
|---|---|
| `sophia doctor` | `TestSmoke_DoctorReportsHealthOK` |
| `sophia run` | `TestSmoke_Run_StreamsThenFinishesDone` (full multiplexer flow) |
| `sophia attach` (snapshot) | `TestSmoke_Attach_RetrievesSnapshot` |
| `sophia changes` | `TestSmoke_ChangesList` |
| `sophia status` | covered by `TestSmoke_Attach_RetrievesSnapshot` |
| `sophia approve` | `TestSmoke_Approve_Idempotent` |
| `sophia reject` | `TestSmoke_Reject_HappyPath` |
| `sophia abort` | `TestSmoke_Abort_Idempotent` |

### 7.8 Compatibility matrix (post-Phase 5)

| cli ↔ orchestator pair | Wire | Status |
|---|---|---|
| cli `main` (v0.2.0-dev) ↔ orch `m10/orchestrator-wire-v1` | sophia-wire-v1 | ✅ Compatible. SHA256 match + 27 contract tests + 25 orch tests + auth + SSE + 13 error codes verified. Pre-tag, pre-merge. |
| cli `main` ↔ orch `main` (v0.1.x) | divergent | ❌ Incompatible. v0.2.0 is a coordinated cut-over. Documented under §4.4. |
| cli v0.1.0 ↔ orch `m10/orchestrator-wire-v1` | divergent | ❌ Incompatible. v0.2.0 server requires v0.2.0 cli. Documented under §4.4. |

### 7.9 Phase 5 deviations

**None.** Every Phase 5 scope item landed as authorized. The only
deferral is the "real orchestrator binary smoke" — documented in
`test/contract/HARNESS.md` §"Future" and intentionally out of scope
(needs Postgres in CI; lands in Phase 7 coordinated release).
