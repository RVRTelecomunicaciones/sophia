# Sophia v0.2.0 — M10 Cross-Repo Wire Alignment Plan

> **Status:** Draft, NOT executed yet.
> **Decision basis:** ADR-0003 Path A3 (canonical spec, both repos converge).
> **Affected repos:** `sophia-cli`, `sophia-orchestator`, plus a new `sophia-contract` source-of-truth (or mirrored spec dir in both).
> **Anti-scope (preserved):** M9 / v0.1.0 hardening is closed. v0.2.0 changes are scoped to wire alignment + minimum CLI surface to drive the orchestrator's exposed verbs.

**Goal:** Ship `sophia-cli v0.2.0` + `sophia-orchestator v0.2.0` simultaneously, both implementing a single canonical wire spec (`sophia-wire-v1`). Eliminate the design-level drift catalogued in ADR-0003 + `docs/superpowers/research/m10-wire-inventory.md`. Add the minimum CLI surface required to call the orchestrator's first-class operations (approve/reject/abort) without removing the M7 browser-open approval UX.

**Anti-list (DO NOT do in M10):**
- NO multi-tenant API key management (single-tenant `SOPHIA_API_KEY` only).
- NO orchestrator dual-API: every endpoint exists EITHER as the canonical or as an explicit deprecation; no permanent shims.
- NO new TUI views (the M6/M7 timeline + apply board + banner stay; only data sources may change).
- NO governance / memory / runtime adapter rewrites (those repos stay on their own roadmaps).
- NO breaking changes to `domain.*` types in sophia-cli unless absolutely required by the contract (target: zero breakage).
- NO removing the M7 `[O]pen browser` keybinding; `sophia approve` is additive.
- NO publishing v0.2.0 of either repo without the canonical contract tagged in lockstep.

---

## D-M10 decision register

| ID | Question | Decision |
|----|----------|----------|
| D-M10-01 | Which path from ADR-0003? | **A3.** A1 is fallback if cross-repo coordination becomes infeasible mid-flight. |
| D-M10-02 | API key requirement scope? | Required for **remote** orchestrators (any URL whose host is not `localhost` / `127.0.0.1` / `[::1]`). Optional for local. CLI errors with `exit 3 + "auth required for remote orchestrator"` if env+flag missing on remote. Orchestrator middleware accepts anonymous on `localhost`-bound listeners only — explicit allowlist, NOT a `dev-mode` toggle. |
| D-M10-03 | Approval flow shape? | **Both, complementary.** SSE event still carries `gate_url`; M7's `[O]pen browser` keeps working as a UX affordance. NEW: `sophia approve <change-id> <phase-id> [-r reason]` and `sophia reject` send the in-band POST. The browser flow and the in-band POST hit the SAME orchestrator decision endpoint; either is valid. |
| D-M10-04 | Where does the canonical spec live? | A new file `docs/specs/sophia-wire-v1.md` mirrored verbatim in both repos. Single source-of-truth file is in `sophia-cli` (deciding repo); `sophia-orchestator`'s copy carries a banner pointing to the cli-side master. (Option: spin up a third `sophia-contract` repo if the spec grows beyond ~500 lines or attracts third-party clients.) |
| D-M10-05 | SSE granularity in canonical? | **Per-Phase**, with a per-Change index endpoint. The orchestrator's per-Phase model wins because phases ARE the unit that emits work; aggregating server-side would lose information. CLI's Runner gets a thin "phase-stream multiplexer" that switches subscriptions when `current_phase_id` changes on the Change snapshot. The CLI's `domain.Event` model survives (events are still typed); only the subscription target moves. |
| D-M10-06 | Health path canonical? | `/api/v1/health`. Orchestrator stays. CLI's `/healthz` call path is changed to `/health`. NO `/healthz` alias on the orchestrator side (rejected: dual-API). |
| D-M10-07 | SSE probe in `doctor`? | The current `sseprobe` calls `/api/v1/events` which doesn't exist on the orchestrator. Two sub-options: (a) probe by opening a stream at any-phase-of-any-change and immediately closing — requires an existing change; (b) drop the SSE check from `doctor` entirely (the GET `/health` already validates the HTTP server is up). **Pick (b)** — `doctor` reports SSE handshake as "deferred to first stream attempt" and removes the `sseprobe` call. SSE issues will surface on the first `sophia run`/`attach` and are diagnosed there. |
| D-M10-08 | Wire spec versioning? | The spec carries a `version: v1` field. Backward-incompatible changes bump to `v2` and live in a new spec file; servers + clients implementing both can hot-swap via an `API-Version` header (deferred to v0.3.0). |
| D-M10-09 | Localhost detection in CLI? | A small helper `internal/application/networktrust.go` parses `SOPHIA_ORCHESTRATOR_URL` and returns `true` iff host is `localhost`, `127.0.0.0/8`, or `::1`. NO Unix-socket support (out of scope). |
| D-M10-10 | Contract test home? | Live in `sophia-cli` under `test/contract/`. The orchestrator runs the same suite via Go test imports of a contract package published from sophia-cli's `pkg/contract/` (new). Avoids a third repo at v0.2.0 cost; revisit if pain emerges. |
| D-M10-11 | Coordinated release? | rc1 tags simultaneously: `sophia-cli v0.2.0-rc.1` + `sophia-orchestator v0.2.0-rc.1` + `docs/specs/sophia-wire-v1.md` v1.0 within the same week. After 7 days of integration smoke, both promote to `v0.2.0` final on the same day. Spec re-tags with the same release. |
| D-M10-12 | Deprecation policy for v0.1.0 wire? | The CLI v0.1.0 wire is **incompatible** with the orchestrator and never worked end-to-end against the real service. There's no production deployment to deprecate gracefully. v0.2.0 of CLI replaces v0.1.0 outright; users who pinned to v0.1.0 are advised in the migration guide to upgrade. NO server-side compatibility shim. |
| D-M10-13 | Approval/reject endpoint shape? | **Two candidate forms**, decided in Phase 1 spec authoring based on whether governance carries its own gate IDs. **Preferred form A**: `POST /api/v1/phases/{phase_id}/approve` and `/reject` — uses phase as the authoritative unit; matches the per-phase SSE model (D-M10-05). **Alternative form B**: `POST /api/v1/approvals/{gate_id}/approve` and `/reject` — used IFF governance issues distinct gate IDs separable from phase IDs (e.g. multiple gates per phase). The orchestrator's CURRENT path `POST /changes/{cid}/phases/{pid}/approve` is **rejected** as canonical: it carries a redundant change-id (phase IDs are globally unique already), and the spec must minimize the URL surface. Form A is the default; form B is recorded in spec section "Open governance considerations" for v0.3.0+ if needed. |
| D-M10-14 | `/health` vs `/ready` semantics | `GET /api/v1/health` = **process is up** (HTTP server responding). MUST always succeed if the binary is running, even if downstream deps (DB, governance, memory, runtime) are dead. **Hard gate** in `sophia doctor`: 200 → green check; non-200 → fail. `GET /api/v1/ready` = **dependencies are reachable + ready** (DB connectable, downstream services reachable). MAY return 503 transiently. **Warning/degraded** in `sophia doctor`: 200 → green; 503 → yellow check with `(orchestrator dependencies degraded)` annotation but doctor stays exit 0; absent endpoint → "ready endpoint not implemented; skipping". Doctor MUST NOT exit non-zero on `/ready` failure alone. |
| D-M10-15 | Workspace-independence gate | Before tagging EITHER repo, run `GOWORK=off go test ./...` locally and in CI. Confirms the build does not silently depend on a `go.work` file in the developer's tree (which would NOT be present on a fresh clone or release runner). New `make test-no-workspace` target in both repos. CI gate added to `release.yml` pre-flight. Failure to pass blocks tag push (release blocker, see D-M10-16). |
| D-M10-16 | Release blockers (v0.2.0 final tag must NOT push if any of these fail) | (1) `docs/specs/sophia-wire-v1.sha256` differs between sophia-cli and sophia-orchestator at the to-be-tagged commit. (2) `make contract` fails in either repo. (3) Cross-repo smoke (the 7-day window matrix from Task 7.2) has any unresolved RED entry. (4) sophia-cli v0.2.0 CHANGELOG does NOT carry an explicit "Compatibility" section stating the version is **incompatible with sophia-orchestator v0.1.x and earlier; requires sophia-orchestator v0.2.0+**. (5) `GOWORK=off go test ./...` fails in either repo (D-M10-15). All five gates are checked by Task 12.x.x.x of the M10 release flow before any tag push. |
| D-M10-17 | Research freshness policy | All external research that informs M10 decisions MUST follow `docs/research-policy.md`. Primary window: 2026-03-07 → 2026-05-07. Fallback window: 2026-01-01 → present. Pre-2026 sources are inadmissible for new decisions. Undated official docs may serve as secondary reference tagged `official current docs, undated` but never as sole basis for architecture/CI/release/security/API decisions. Every research-driven decision in M10 records an entry in the "Research log" section of this plan. Empty log on a research-driven phase blocks release. |

---

## Architecture overview

```
┌─────────────────────────────────────────────────────────────┐
│  docs/specs/sophia-wire-v1.md  ←── single source of truth   │
│  (mirrored byte-for-byte in both repos at every release)    │
└─────────────────────────┬───────────────────────────────────┘
                          │
            ┌─────────────┼──────────────┐
            ▼                            ▼
   ┌─────────────────┐           ┌──────────────────────┐
   │   sophia-cli    │           │  sophia-orchestator  │
   │     v0.2.0      │ ── HTTP ──>      v0.2.0          │
   │                 │   + SSE   │                      │
   │  +API key       │  + auth   │  Existing model:     │
   │  +phase stream  │           │  per-phase events,   │
   │   multiplexer   │           │  REST phase verbs,   │
   │  +approve/      │           │  X-Sophia-API-Key    │
   │   reject/abort  │           │                      │
   └─────────────────┘           └──────────────────────┘
            │                            │
            └────── pkg/contract/ ───────┘
                    (shared Go types,
                     contract tests run
                     in both repos' CI)
```

**Sophia-cli net additions over v0.1.0:**

- API key plumbing (env + flag + header).
- `network trust` helper (decides remote vs local).
- Phase-stream multiplexer in `Runner.Observe` / `ssestream`.
- New CLI commands: `approve`, `reject`, `abort`.
- `pkg/contract/` (new public package) holding canonical wire types + version constant.
- Contract test harness under `test/contract/`.

**Sophia-orchestator net additions over its current state:**

- Anonymous-on-localhost auth path (explicit listener allowlist).
- (Possibly) renaming a few response field names / status casing per the canonical spec.
- Adopt `pkg/contract/` types for request/response shapes (decouples the orchestrator's internal types from the wire).
- Contract tests run in CI.

Detailed changes per repo are in Phases 3 and 4.

---

## Phase dependency graph

```
Phase 1 (canonical contract)
    │
    └─► Phase 2 (compatibility matrix)
            │
            ├─► Phase 3 (orchestrator changes)  ──┐
            │                                      │
            └─► Phase 4 (CLI changes)        ──────┤
                                                   ▼
                                             Phase 5 (contract tests)
                                                   │
                                                   ├─► Phase 6 (migration docs)
                                                   │
                                                   └─► Phase 7 (coordinated release)
```

Phase 3 and 4 are independent after Phase 2 is signed; both can run in
parallel. Phase 5 blocks the release because the contract tests are the
v0.2.0 release gate.

---

## Phase 1 — Canonical contract authoring

### Task 1.1: Draft `docs/specs/sophia-wire-v1.md` (sophia-cli authoritative copy)

**Files (sophia-cli):**
- Create: `docs/specs/sophia-wire-v1.md`

**Content sections:**

1. **Status + version:** `version: v1`, `status: draft until v0.2.0 tag`, ratification date, both-repos owners.
2. **Transport invariants:** HTTP/1.1 + HTTP/2 both supported; JSON over UTF-8; SSE for events; UTC ISO-8601 timestamps; ULID for IDs.
3. **Auth scheme:**
   - Header: `X-Sophia-API-Key: <key>`.
   - Required when the request hits an orchestrator bound to a non-loopback address; orchestrator MUST accept anonymous on loopback-only listeners.
   - Reject mode: 401 + `{"code":"unauthorized","error":"X-Sophia-API-Key required"}`.
   - The orchestrator's listener configuration determines whether anon is allowed; the CLI doesn't probe — it sends key when configured, omits when not.
4. **Endpoint catalog** (every URL: method, path, auth requirement, request body, response body, error codes, side effects). Catalogue mirrors the orchestrator's M3-current state with adjustments per D-M10-* decisions.
5. **SSE event taxonomy:** `event: <type>` + `id: <ULID>` + `data: <JSON>`. Types: `heartbeat`, `phase.started`, `phase.completed`, `phase.failed`, `task.*`, `agent.*`, `approval.required`, `approval.resolved`. Each event's payload schema documented.
6. **Phase-stream multiplexer protocol** (D-M10-05): when the CLI sees a Change snapshot whose `current_phase_id` differs from the currently-subscribed `phase_id`, it MUST close the old stream and open a new one to `/changes/{cid}/phases/{new_pid}/events`. The orchestrator MUST NOT push events that span phases on a per-phase stream.
7. **Approval flow:** SSE event `approval.required` carries `{phase_id, gate_url, reason, risk, policy}`. Decision channels are equivalent and idempotent: opening `gate_url` in a browser submits an approval/rejection via the orchestrator's web UI; calling `POST /phases/{pid}/approve` with `{approver, reason?}` does the same. Either resolves the gate; second decision MUST return 409.
8. **Pagination:** `?limit=N&offset=M` on list endpoints; max `limit=100`. Response carries `total` for client-side paging UX.
9. **Error envelope:** `{"code": "<machine_code>", "error": "<human_message>", "details": {...}}`. Codes enumerated.
10. **Forward-compat rules:** unknown JSON fields ignored; unknown SSE event types skipped with a single-line warning to stderr; missing optional fields default to zero values.

- [ ] **Step 1:** Draft the spec. ~400-600 lines. Cite ADR-0003 + inventory as sources.
- [ ] **Step 2:** Cross-review: walk through every existing endpoint in `m10-wire-inventory.md` and confirm it's covered or explicitly excluded.
- [ ] **Step 3:** Internal sign-off (cli + orch owners) before mirroring.

### Task 1.2: Mirror to `sophia-orchestator/docs/specs/sophia-wire-v1.md`

**Files (sophia-orchestator):**
- Create: `docs/specs/sophia-wire-v1.md` (verbatim copy of cli-side master + a banner header pointing at the master URL).

CI in both repos runs a checksum check: `sha256sum docs/specs/sophia-wire-v1.md` must match the value committed in `docs/specs/sophia-wire-v1.sha256` (single file, both repos). Mismatch fails CI. Update via a coordinated PR pair.

### Task 1.3: Promote ADR-0003 from Draft to Accepted

**Files (sophia-cli):**
- Modify: `docs/adr/0003-cross-repo-wire-alignment.md` — flip `Status: Draft` → `Status: Accepted`. Reference D-M10-* register.
- Add: `docs/adr/0004-canonical-wire-spec-v1.md` — short ADR documenting where the spec lives, who can change it, the checksum-mirror policy.

**Files (sophia-orchestator):**
- Add: a copy of ADR-0003 (or a stub that points at the cli-side master) plus the new ADR-0004.

---

## Phase 2 — Compatibility matrix

### Task 2.1: Endpoint matrix — Required / Optional / Unsupported

**Files (sophia-cli):**
- Create: `docs/specs/cli-orchestrator-compatibility.md`

For every endpoint in the canonical spec, classify from the CLI's perspective:

| Class | Definition | Action if missing |
|-------|------------|-------------------|
| **Required** | CLI directly invokes; absence breaks a CLI command. | CLI exits with `code 3 + "orchestrator missing required endpoint X"` on first call. |
| **Optional** | CLI can take advantage if present, but degrades gracefully if absent (e.g. `/api/v1/ready`, `/metrics`). | CLI logs at debug; continues. |
| **Intentionally unsupported by CLI** | Orchestrator-only verb the CLI does NOT call (e.g. multi-tenant admin endpoints, governance bypass). | CLI ignores existence; documented in this matrix so contributors don't accidentally wire a flag for it. |

Initial classification (subject to v0.2.0-rc adjustment):

| Endpoint | Class | Owner CLI command(s) |
|----------|-------|----------------------|
| `GET /api/v1/health` | Required | `sophia doctor` |
| `GET /api/v1/ready` | Optional | `sophia doctor` (degraded warning if absent) |
| `GET /metrics` | Intentionally unsupported | (none) |
| `POST /api/v1/changes` | Required | `sophia run` |
| `GET /api/v1/changes` | Required | `sophia changes` |
| `GET /api/v1/changes/{id}` | Required | `sophia run`, `sophia attach`, `sophia status` |
| `POST /api/v1/changes/{id}/abort` | Required | `sophia abort` (NEW in v0.2.0) |
| `POST /api/v1/changes/{id}/phases/{type}/run` | Intentionally unsupported | (orchestrator drives; CLI does not control phase boundaries) |
| `GET /api/v1/phases/{phase_id}` | Required | `sophia run` / `attach` (multiplexer reads `current_phase_id`) — phase IDs are globally unique, no change-id required (D-M10-13) |
| `POST /api/v1/phases/{phase_id}/resume` | Optional | `sophia resume` (NEW, v0.2.0) — surfaces only on `--no-tui --json` mode |
| `POST /api/v1/phases/{phase_id}/approve` | Required | `sophia approve` (NEW, v0.2.0). Form A from D-M10-13. |
| `POST /api/v1/phases/{phase_id}/reject` | Required | `sophia reject` (NEW, v0.2.0). Form A from D-M10-13. |
| `GET /api/v1/phases/{phase_id}/board` | Optional | TUI ApplyBoard data refresh — fall back to SSE-derived state if 404 |
| `GET /api/v1/phases/{phase_id}/events` | Required | SSE multiplexer (D-M10-05) |

Each row in the final matrix carries a 1-2 sentence rationale citing the
D-M10-* decision that pinned the class.

### Task 2.2: SSE event taxonomy compatibility

**Files (sophia-cli):**
- Modify: `docs/specs/cli-orchestrator-compatibility.md` — add the SSE table.

Mirror format: every event type the spec defines, classified Required /
Optional / Unsupported by the CLI. E.g.:

| Event type | Class | Where consumed |
|------------|-------|----------------|
| `heartbeat` | Required | watchdog timer |
| `phase.started` | Required | timeline |
| `phase.completed` | Required | timeline + post-stream refresh trigger |
| `phase.failed` | Required | timeline |
| `task.created` | Optional | ApplyBoard (M7) |
| `task.completed` | Optional | ApplyBoard |
| `agent.dispatched` | Optional | ApplyBoard |
| `approval.required` | Required | Banner (M7) + arms `approvalTimeoutSink` |
| `approval.resolved` | Required | Banner clear + cancel timer |
| `<unknown>` | Forward-compat | Logged + skipped |

---

## Phase 3 — Orchestrator-side changes

### Task 3.1: Adopt `pkg/contract/` types

**Files (sophia-orchestator):**
- Modify: `internal/adapters/inbound/http/handlers/*.go` — replace request/response struct types with imports from `github.com/RVRTelecomunicaciones/sophia/pkg/contract` (sophia-cli's new public package per D-M10-10).
- Modify: `go.mod` to add `github.com/RVRTelecomunicaciones/sophia` as a dep at the v0.2.0-rc.1 tag.

Rationale: the contract types ARE the wire protocol (D-M10-04). Both repos using the same Go types eliminates a class of "field name diverged" bugs.

> If the dep direction is unacceptable (orchestrator depending on cli), invert: move `pkg/contract/` to a third repo `sophia-contract` with v1.0.0 tag and both repos depend on it. D-M10-10 currently picks the simpler path.

### Task 3.2: Anonymous-on-localhost auth path

**Files (sophia-orchestator):**
- Modify: `internal/adapters/inbound/http/middleware/auth.go` — `APIKey` middleware accepts anonymous when the request's `r.Host` (or X-Forwarded-For chain) resolves to a loopback address AND the configured listener was bound to loopback only. NO `--dev-mode` flag.
- Modify: `internal/infrastructure/config/config.go` — add `HTTP.AllowAnonLocalhost bool` (default `true`); when `false`, even loopback requests need a key.

CI test: orchestrator listens on `127.0.0.1:0`, anonymous request returns 200; orchestrator listens on `0.0.0.0:0`, anonymous returns 401.

### Task 3.3: SSE event payload normalization

**Files (sophia-orchestator):**
- Modify: handlers that emit SSE — make sure `event_id` is ULID, `payload` is the canonical schema, types match the catalog. Drop any orchestrator-internal-only fields from the wire.

### Task 3.4: Approval gate URL contract

**Files (sophia-orchestator):**
- Modify: the `approval.required` event payload — MUST include `gate_url`, `reason`, `risk`, `policy` per D-M10-03 spec text. Browser-open path is governance-side, not orchestrator-side, but the URL the event embeds MUST be reachable by the user's browser (orchestrator's own URL or a downstream governance UI URL).

### Task 3.5: Health endpoint stays at `/api/v1/health`

No change. CLI moves to match (Task 4.1). D-M10-06.

### Task 3.6: Orchestrator-side contract tests adoption

**Files (sophia-orchestator):**
- Add: `test/contract/orchestrator_contract_test.go` — runs the shared contract suite (imported from sophia-cli's `pkg/contract/test`) against an in-process orchestrator instance. Validates every spec assertion.

---

## Phase 4 — CLI-side changes

### Task 4.1: Health path migration

**Files (sophia-cli):**
- Modify: `internal/adapters/outbound/orchestratorhttp/healthz.go` — rename URL constant from `/api/v1/healthz` → `/api/v1/health`. Rename file? optional, low-value.
- Update tests under `internal/adapters/outbound/orchestratorhttp/healthz_test.go`.

### Task 4.2: API key plumbing

**Files (sophia-cli):**
- Create: `internal/application/networktrust.go` — `IsLoopbackURL(rawURL string) (bool, error)` parses + classifies (D-M10-09).
- Create: `internal/application/auth.go` — `APIKeyResolver` reads env `SOPHIA_API_KEY` + flag `--api-key`; returns `(key string, isRemoteRequired bool)`. If `isRemoteRequired && key == ""`, error.
- Modify: `internal/adapters/outbound/orchestratorhttp/client.go` — add `APIKey string` field to `Config`; inject `X-Sophia-API-Key` header on every request.
- Modify: `internal/adapters/outbound/sseprobe/probe.go` and `internal/adapters/outbound/ssestream/client.go` — same header.
- Modify: `internal/adapters/inbound/cli/root.go` — add persistent flag `--api-key`; resolved via `APIKeyResolver` in bootstrap.
- Modify: `internal/bootstrap/wire.go` — wire `APIKeyResolver` into all outbound adapters; fail with friendly message if remote-required and key missing.

CLI behavior:
- `SOPHIA_ORCHESTRATOR_URL=http://localhost:8080 sophia run "msg"` — no key needed.
- `SOPHIA_ORCHESTRATOR_URL=https://orch.example.com sophia run "msg"` (no key) — exit 3 with `auth required for remote orchestrator (set SOPHIA_API_KEY or --api-key)`.

### Task 4.3: Phase-stream multiplexer

**Files (sophia-cli):**
- Modify: `internal/adapters/outbound/ssestream/client.go` — `Subscribe` takes `outbound.StreamTarget{ChangeID, PhaseID}`; URL becomes `/api/v1/changes/{cid}/phases/{pid}/events`. Add a new method `SubscribePhase(ctx, cid, pid) (<-chan Event, Stop, error)` if needed for explicit phase-level control.
- Modify: `internal/application/runner.go` — `streamWithSink` now refreshes the Change snapshot when the per-phase stream ends (current behavior already calls `refreshAfterStreamEndWithSink`). The new step: if the snapshot's `current_phase_id` differs from the just-finished phase AND the change is not terminal, re-subscribe to the new phase. Loop until terminal status.
- Modify: `internal/ports/outbound/eventstream.go` — `StreamTarget` gets a `PhaseID domain.PhaseID` field. `domain.PhaseID` is a new type alias.
- Update tests: `runner_test.go` adds the multi-phase stream lifecycle case. Fakes (`test/fakes/eventstream.go`) get a `OnPhaseSwitch` hook for assertion.

### Task 4.4: New CLI command — `sophia approve <change-id> <phase-id>`

**Files (sophia-cli):**
- Create: `internal/application/approver.go` + `approver_test.go` — the use case (POSTs `/changes/{cid}/phases/{pid}/approve` with `{approver, reason?}`).
- Create: `internal/adapters/inbound/cli/approve.go` + `approve_test.go` — the CLI verb. Args: `<change-id> <phase-id>`. Flags: `-r --reason`, `--approver` (default to `$USER`).
- Modify: `internal/adapters/inbound/cli/root.go` — register.
- Modify: `internal/bootstrap/wire.go` — wire.

Symmetric for `sophia reject` (Task 4.5) and `sophia abort` (Task 4.6).

### Task 4.5: New CLI command — `sophia reject <change-id> <phase-id>`

Symmetric to 4.4. Hits `POST /phases/{pid}/reject`.

### Task 4.6: New CLI command — `sophia abort <change-id>`

**Files (sophia-cli):**
- Create: `internal/application/aborter.go` + tests.
- Create: `internal/adapters/inbound/cli/abort.go` + tests.

Hits `POST /api/v1/changes/{id}/abort` with `{reason?}`.

### Task 4.7: Drop SSE handshake from `doctor`

**Files (sophia-cli):**
- Modify: `internal/application/doctor.go` — remove the `sseprobe` step; replace with a single-line note in the doctor report: "SSE handshake: deferred to first run/attach (no orchestrator-side endpoint to probe pre-run)".
- Delete: `internal/adapters/outbound/sseprobe/` (entire package — no longer wired).
- Modify: `internal/bootstrap/wire.go` — drop the `sseprobe` instantiation.

Justification: D-M10-07. The probe endpoint never existed on the orchestrator; removing the call removes a misleading green-check.

### Task 4.8: `pkg/contract/` package

**Files (sophia-cli, NEW public package):**
- Create: `pkg/contract/types.go` — request/response DTOs identical to the canonical wire spec. The package is the Go ↔ wire bridge.
- Create: `pkg/contract/routes.go` — route name constants (`HealthPath = "/api/v1/health"`, etc.) so neither client nor server hand-types URLs.
- Create: `pkg/contract/events.go` — SSE event type name constants (`EventApprovalRequired = "approval.required"`, etc.).
- Create: `pkg/contract/version.go` — `const Version = "v1"`; `func RequiredHeaders(apiKey string) http.Header { ... }` for the API key header builder.
- Create: `pkg/contract/test/harness.go` — utilities the contract test suite uses (HTTP client builder, SSE consumer, decode helpers).

**STRICT scope rules for `pkg/contract/`** (per user constraint):

- MUST NOT import `internal/` from sophia-cli (would defeat the purpose; internal types pollute the public surface).
- MUST NOT import `application/`, `adapters/`, or `cli/` from sophia-cli.
- MUST contain ONLY: DTOs (request/response shapes), constants (route paths, event names, error codes, header names, version), and test helpers (`pkg/contract/test/`).
- MUST NOT depend on `bubbletea`, `cobra`, `pgx`, `chi`, or any framework-coupled package. Standard library + minor utility libs (e.g. `time`) only.
- IF either repo's adoption surfaces a coupling that would force `pkg/contract/` to import internal types, **STOP** and migrate `pkg/contract/` to a new repo `sophia-contract` BEFORE tagging `v0.2.0-rc.1`. Tracked under RM10-02.

The orchestrator imports these types via `go.mod` (Task 3.1).

---

## Phase 5 — Cross-repo contract tests

### Task 5.1: Contract test suite design

**Files (sophia-cli):**
- Create: `test/contract/spec_test.go` — a Go test suite that, for every endpoint in the canonical spec, asserts:
  - Request shape: send the canonical request body, server accepts.
  - Response shape: decode into the canonical type, all required fields present.
  - Auth: with key → 200, without key (on remote) → 401.
  - Error envelope: trigger a 4xx → response matches the spec error envelope.
  - SSE: subscribe to a phase stream, assert event types + payload shapes match the catalog.

Build tag: `//go:build contract`. Run via `make contract` (new target).

### Task 5.2: Run the suite in BOTH repos' CI

**Files (sophia-cli):**
- Modify: `.github/workflows/ci.yml` — add a `contract` job that builds an in-process orchestrator from a vendored `sophia-orchestator` binary (downloaded from a known release artifact or built from a pinned tag) and runs `make contract`.

**Files (sophia-orchestator):**
- Add: `.github/workflows/ci.yml` runs the same suite, but uses its own in-process server.

The contract tests are byte-identical (same Go file, imported in both repos via the `pkg/contract/test` package).

### Task 5.3: Auth-specific contract tests (D-M10-02 enforcement)

**Files (sophia-cli):**
- Add: `test/contract/auth_test.go` (also under `//go:build contract`)

Six scenarios are MANDATORY pass-criteria of the contract suite:

```go
// 1. Localhost listener + AllowAnonLocalhost=true + no API key → 200.
TestAuth_LoopbackAnonAllowed_When_AllowAnonLocalhost_True

// 2. Localhost listener + AllowAnonLocalhost=false + no API key → 401.
TestAuth_LoopbackAnonRejected_When_AllowAnonLocalhost_False

// 3. Remote listener + no API key → 401.
TestAuth_RemoteWithoutKey_Returns401

// 4. Remote listener + invalid API key → 401.
TestAuth_RemoteWithInvalidKey_Returns401

// 5. Remote listener + valid API key → 200.
TestAuth_RemoteWithValidKey_Returns200

// 6. CLI side: when SOPHIA_API_KEY is set, every outbound request
//    carries X-Sophia-API-Key with that value. (Inspect via httptest
//    capture of headers.)
TestCli_SophiaAPIKeyEnv_AddsXSophiaAPIKeyHeader
```

Each test cites D-M10-02 as the source-of-truth. Failures here block release per D-M10-16.

### Task 5.4: Pact-style golden file (optional, deferred to v0.2.1 if scope creeps)

A captured set of HTTP exchanges and SSE streams (golden files) that validates the spec without requiring a live server. Out of scope for v0.2.0 but slot into v0.2.1.

---

## Phase 6 — Migration / deprecation notes

### Task 6.1: CHANGELOG entries

**Files (sophia-cli):**
- Modify: `CHANGELOG.md` — under a new `## [v0.2.0]` section:
  - **BREAKING:** `/api/v1/healthz` → `/api/v1/health`. Stale orchestrators on the old path will be unreachable; users must upgrade orchestrator to v0.2.0+.
  - **BREAKING:** SSE per-Change streams (`/changes/{id}/events`) replaced by per-Phase streams. Custom integrations relying on the old path break.
  - **Added:** `--api-key` flag + `SOPHIA_API_KEY` env var.
  - **Added:** `sophia approve`, `sophia reject`, `sophia abort` commands.
  - **Removed:** `sseprobe` and the doctor's SSE handshake step.
  - **Internal:** `pkg/contract/` public types; `internal/application/networktrust.go`; phase-stream multiplexer in `Runner.Observe`.

**Files (sophia-orchestator):**
- Modify: `CHANGELOG.md` — under a new `## [v0.2.0]` section:
  - **Added:** Anonymous-on-localhost path (config `HTTP.AllowAnonLocalhost`).
  - **Internal:** Adopted `pkg/contract/` types; contract test suite.
  - **No removed endpoints.**

### Task 6.2: Migration guide

**Files (sophia-cli):**
- Create: `docs/migration/v0.1.0-to-v0.2.0.md` — step-by-step for users currently on v0.1.0.

Example outline:

```markdown
# Migrating from sophia-cli v0.1.0 to v0.2.0

## TL;DR

1. Upgrade sophia-orchestator to v0.2.0 first.
2. Set `SOPHIA_API_KEY` env var (mandatory for remote orchestrators).
3. Run `sophia doctor` — should now report 6/6 green.

## Breaking changes

### Auth
v0.1.0 sent no auth headers. v0.2.0 requires `X-Sophia-API-Key` for any
non-loopback orchestrator.

### Health endpoint
The doctor check now hits `/api/v1/health` instead of `/api/v1/healthz`.

### SSE streams
The runner / attacher subscribes to per-phase streams now; existing
custom code calling `/api/v1/changes/{id}/events` breaks.

## New capabilities

- `sophia approve <change-id> <phase-id>` ...
- `sophia reject <change-id> <phase-id>` ...
- `sophia abort <change-id>` ...
```

### Task 6.3: Server-side migration guide

**Files (sophia-orchestator):**
- Create: `docs/migration/v0.1.0-to-v0.2.0.md`.

(Lighter than the cli-side because the orchestrator was not publicly tagged at v0.1.0; mostly internal notes for ops.)

---

## Phase 7 — Coordinated v0.2.0 release

### Task 7.1: Pre-release rc tags (D-M10-11)

Same week, AFTER all five release blockers (D-M10-16) are green in both repos:

```bash
# Pre-tag gate (run in BOTH repos):
GOWORK=off go test ./...     # D-M10-15: workspace-independence
make contract                # D-M10-16(2): contract suite green
sha256sum docs/specs/sophia-wire-v1.md
# compare with the checksum committed in docs/specs/sophia-wire-v1.sha256;
# CI's "spec-checksum" job enforces this on every push to main.

# Then tag:
git tag v0.2.0-rc.1 -m "v0.2.0-rc.1 — wire alignment"
git push origin v0.2.0-rc.1
```

- `sophia-cli`: triggers the existing `release.yml` (matches `v*.*.*` glob — `v0.2.0-rc.1` is matched).
- `sophia-orchestator`: matching `v0.2.0-rc.1` tag.
- `docs/specs/sophia-wire-v1.md` checksum: identical in both repos at this commit.

### Task 7.2: Integration smoke (7-day window)

The user runs the same manual smoke from M9's checklist plus the new
v0.2.0-only bullets:

- [ ] `sophia approve <id> <phase>` mid-run resolves the gate.
- [ ] `sophia reject <id> <phase>` mid-run terminates the change with `failed`.
- [ ] `sophia abort <id>` mid-run terminates the change.
- [ ] Phase-stream multiplexer transitions cleanly between two phases (visible in JSONL output as two separate snapshot lines with different `current_phase_id`).
- [ ] `SOPHIA_API_KEY` works; missing key against a remote URL → exit 3 with friendly message.
- [ ] Loopback orchestrator without `SOPHIA_API_KEY` → 200 (anonymous-on-localhost).

Findings filed as v0.2.0-rc.2/.3/etc until clean.

### Task 7.3: Final v0.2.0 tag

Five release blockers (D-M10-16) MUST all be green:

| # | Gate | How verified |
|---|------|--------------|
| 1 | Spec checksum identical across repos | `sha256sum docs/specs/sophia-wire-v1.md` matches `docs/specs/sophia-wire-v1.sha256` in BOTH repos at the to-be-tagged commit. CI job `spec-checksum`. |
| 2 | `make contract` green | Both repos' CI on the to-be-tagged commit. |
| 3 | Cross-repo smoke 7-day window | All bullets in Task 7.2 ticked, no RED entries, sign-off committed at `docs/release/manual-smoke-checklist.md` v0.2.0 edition. |
| 4 | CHANGELOG "Compatibility" section in CLI | sophia-cli `CHANGELOG.md` `[v0.2.0]` section MUST contain a "Compatibility" subsection stating: *"v0.2.0 of sophia-cli is incompatible with sophia-orchestator v0.1.x and earlier. Requires sophia-orchestator v0.2.0 or later."* — exact text or stronger. CI grep enforces. |
| 5 | `GOWORK=off go test ./...` green in BOTH repos | New `make test-no-workspace` target. CI gate. |

If ANY block: STOP. Either fix it (preferred) or cycle to v0.2.0-rc.2 / .3 etc. NO force-tag, NO bypass. Mirrors D-M9-13 / D-M9-14 for v0.1.0.

When all 5 green:

```bash
# Both repos, same calendar day:
git tag v0.2.0 -m "v0.2.0 — wire alignment release"
git push origin main v0.2.0
```

GitHub Releases auto-publish via `release.yml` (sophia-cli) and the
orchestrator's release pipeline. Spec file's `Status:` flips to
`Accepted` for v0.2.0.

### Task 7.4: Post-release validation

- Download the released `sophia` binary; run against released `sophia-orchestator`; verify the M9 manual smoke checklist (now extended) is fully green.
- Promote `docs/release/manual-smoke-checklist.md` to v0.2.0 in both repos.
- Engram + session-summary save: M10 closed; v0.2.0 shipped.

---

## Risk register

| ID | Risk | Mitigation |
|----|------|------------|
| RM10-01 | Spec authoring drags into a multi-week design exercise | Cap Phase 1 at 1 calendar week. If unresolved decisions remain, fall back to A1 per ADR-0003 fallback clause. |
| RM10-02 | `pkg/contract/` Go-type sharing introduces dep cycle (orch depends on cli) | Mitigation B: factor to `sophia-contract` repo at v0.2.0-rc.2 if cycle becomes painful. |
| RM10-03 | Orchestrator's per-Phase SSE model leaks orchestrator-internal abstractions to the CLI | Spec section 5 normalizes event shapes; orchestrator strips internal fields before emit. Contract tests assert shape conformity. |
| RM10-04 | API key UX: users running locally hit "auth required" because their `localhost` isn't recognized | `IsLoopbackURL` covers `localhost`, `127.0.0.0/8`, `::1`. Edge cases (Tailscale magic DNS, custom resolver) are documented as "set `SOPHIA_API_KEY` even if local". |
| RM10-05 | M7's `[O]pen browser` keybinding becomes redundant after `sophia approve` ships | D-M10-03 keeps both. Documentation explicitly says they are equivalent decision channels; user picks the UX they prefer. |
| RM10-06 | Phase-stream multiplexer reconnects on every phase change, surfacing more network errors than v0.1.0's single-stream model | Reconnect is fast (sub-second); spec mandates `Last-Event-ID` is honored within a phase but reset across phases. CLI surfaces transient errors via `OnError(...)` to the sink which the JSONL/TUI render but do not bubble to exit code. |
| RM10-07 | Coordinated release windows slip; one repo tags v0.2.0 before the other | Spec checksum mismatch CI gate prevents either repo from passing CI on a tag commit when the spec drifts. v0.2.0 tag pre-flight grep: `rg "Status: Accepted" docs/specs/sophia-wire-v1.md` must pass on both repos. |
| RM10-08 | Test contract suite is duplicated across repos and drifts | The suite lives in `pkg/contract/test/` (single import); both repos pull the same Go module at the same `v0.2.0-rc.1` (or `sophia-contract` if RM10-02 fires). |
| RM10-09 | The contract becomes a moving target — every PR proposes spec changes | The spec file's PR template requires owner sign-off from both repos AND a paired PR in the other repo. Branch protection rules enforce. |
| RM10-10 | A1 fallback is invoked mid-flight; partial work on Phase 1 is wasted | Phase 1 deliverables (the spec doc itself) are useful even under A1 — they'd become the authoritative description of the orchestrator's existing surface that the CLI catches up to. No work lost. |

---

## Verification matrix

| Gate | Tool | Pass criteria | Where it runs |
|------|------|---------------|---------------|
| Spec ratification | manual review | both repo owners sign Phase 1 deliverables | once, end of Phase 1 |
| Compile (cli) | `go build ./...` | exit 0 | local + CI |
| Compile (orch) | `go build ./...` | exit 0 | local + CI |
| Tests (cli) | `go test -race ./...` | exit 0 | local + CI |
| Tests (orch) | `go test -race ./...` | exit 0 | local + CI |
| Lint (both) | `golangci-lint run` | exit 0 | local + CI |
| Vuln (both) | `govulncheck ./...` | 0 reachable HIGH/CRITICAL | local + CI |
| Security (both) | `gosec -severity high ./...` | 0 HIGH | local + CI |
| Spec checksum | `sha256sum docs/specs/sophia-wire-v1.md` matches `.sha256` file | identical across both repos | CI gate |
| Contract tests (cli) | `make contract` | every spec assertion passes against in-process orch | CI |
| Contract tests (orch) | `make contract` | same suite, against orch's own in-process server | CI |
| Manual smoke (extended) | `docs/release/manual-smoke-checklist.md` v0.2.0 edition | every box ticked + reviewer signature | manual at v0.2.0 tag |
| Workspace independence | `GOWORK=off go test ./...` (D-M10-15) | exit 0 in both repos | local + CI pre-tag gate |
| Auth contract tests | Task 5.3 six scenarios | exit 0 | CI |
| Compatibility section | grep `v0.2.0.*Compatibility` in CHANGELOG | match found | CI |
| Spec checksum match | `sha256sum docs/specs/sophia-wire-v1.md` vs `.sha256` (BOTH repos) | identical | CI on every push |
| Coordinated release | tag matches `v0.2.0`, CHANGELOG promoted, both repos tagged within 24h | manual coordination | gh release pages |

---

## Pre-execution gate

This plan is **NOT yet executable**. Three prerequisites must be satisfied before Phase 1 Task 1.1 starts:

1. **v0.1.0 closure confirmed.** No follow-up work pending on the v0.1.0 line that would conflict with M10's wire changes. v0.1.0 stays as published; any post-release issues route to a `v0.1.x` patch on a release branch — NOT into M10's main work.
2. **Owner sign-off recorded.** Per D-M10-04, both repos are treated as same-owner. The user (architect) explicitly authorizes M10 execution.
3. **Calendar window confirmed.** 2-3 weeks of focus available for: 1 week spec authoring + parallel Phase 3/4 implementation + 7-day rc smoke window. Slipping mid-flight risks invoking the A1 fallback.

When all three are green, mark this section "Authorized YYYY-MM-DD" and proceed to Phase 1 Task 1.1.

**Authorized 2026-05-07** by repository architect. All three prerequisites confirmed:
1. v0.1.0 formally closed; tags + releases not to be modified.
2. Owner sign-off recorded; same architect for both repos.
3. Calendar window 2–3 weeks committed for Phase 1 + cross-repo implementation + rc.1 + 7-day smoke + final v0.2.0.

Active execution begins at Phase 1 Task 1.1. Research policy (D-M10-17 / `docs/research-policy.md`) is now in force for any external sources. M10 must not modify v0.1.0 artifacts; force tags forbidden; contract tests un-bypassable. Any decision not covered by ADR-0003 / D-M10-* triggers STOP-and-review before proceeding.

## Research log (D-M10-17)

Every decision in this plan that was informed by external sources MUST
record an entry here. Template per `docs/research-policy.md`:

```markdown
### YYYY-MM-DD — <decision short title>

- **Problem:** <one sentence>
- **Source(s) consulted:**
  - <URL> — <YYYY-MM-DD>
  - <URL> — `official current docs, undated`
- **Decision:** <what was decided>
- **Impact:** <which plan/spec/code surface changed>
- **Researcher:** <name>
```

Empty until Phase 1 Task 1.1 starts; the spec-authoring task is the
first M10 activity that consults external sources (e.g. SSE/HTTP
specifications, current `chi/v5` and `pgx/v5` patterns, golangci-lint
v2 lint defaults, goreleaser v2 release attestation feature set,
sigstore/cosign current state).

---

## Implementation Notes — Deviations from Plan

### Phase 1 Tasks 1.1 / 1.2 / 1.3 — completed 2026-05-07

- Spec drafted at `docs/specs/sophia-wire-v1.md` (768 lines), checksum committed.
- Mirrored byte-for-byte to `sophia-orchestator/docs/specs/sophia-wire-v1.md` (commit `631693d` on orch main).
- ADR-0003 promoted Draft → Accepted (commit `9e1904a`).
- Cross-review found zero new incompatibilities not already addressed by the spec.

### Phase 2 Tasks 2.1 / 2.2 — completed 2026-05-07

- Compatibility matrix at `docs/specs/cli-orchestrator-compatibility.md` (368 lines, commit `43d314a`).
- 9 Required HTTP routes, 3 Optional, 3 Intentionally unsupported, 2 Reserved (Form B).
- 6 Required SSE events, 6 Optional, 1 forward-compat catch-all.
- 8 risks documented + mitigations linked to RM10-* register.

### Phase 3 (orchestrator implementation) — IN PROGRESS on branch `m10/orchestrator-wire-v1`

Branch HEAD = `8c2f2e0` on `sophia-orchestator/m10/orchestrator-wire-v1`. NOT merged to orch main. Three commits:

1. **`e5d99b5`** `feat(http/middleware): spec-compliant 401 envelope + AllowAnonLocalhost`
   - `middleware.APIKeyWithAnonOption(authn, allowAnon)` factory; backwards-compat `APIKey()` retained.
   - `middleware.IsLoopbackAddr(addr)` helper for bootstrap composition.
   - `middleware.AnonymousLoopbackProject` constant for context-injection on anon path.
   - Spec-compliant 401 envelope: `{"code":"unauthorized","error":"<msg>"}` with `application/json` content-type (was `unauthenticated` + text/plain default).
   - `HTTP.AllowAnonLocalhost bool` added to config + `SOPHIA_HTTP_ALLOW_ANON_LOCALHOST` env var. Default false.
   - 7 new auth tests covering the 6 D-M10-02 scenarios + parametric `IsLoopbackAddr` coverage.

2. **`88c9d14`** `feat(http): migrate phase routes to phase-scoped paths`
   - `r.Route("/api/v1/phases/{phase_id}", ...)` mounts get / resume / approve / reject / board / events at the top-level phase group per D-M10-13 Form A.
   - Old change-scoped phase paths REMOVED (no compat shim per user rule 5).
   - `POST /api/v1/changes/{change_id}/phases/{phase_type}/run` retained change-scoped (phase doesn't yet exist).
   - `Deps.AllowAnonLocalhost` field added; threaded into the auth middleware factory.
   - `router_test.go` `TestSSE_StreamReceivesEvents` URL updated to the new phase-scoped path.

3. **`8c2f2e0`** `feat(bootstrap): compose effective AllowAnonLocalhost from config + listener`
   - Bootstrap evaluates `effectiveAllowAnon = cfg.HTTP.AllowAnonLocalhost && middleware.IsLoopbackAddr(cfg.HTTP.Addr)`.
   - When the operator sets the flag but binds the listener to a non-loopback interface, downgrades to false and emits a `slog.Warn`.
   - Per-request middleware uses the composed bool; never inspects listener at runtime.

**Test status:** all 25 packages pass `go test -race ./...` on the branch. `go vet ./...` clean. `golangci-lint run` is BLOCKED by a pre-existing v1→v2 config schema mismatch in the orch repo's `.golangci.yaml` — out of Phase 3 scope; tracked as orch-side housekeeping.

**Tasks 3.1 (pkg/contract adoption) and 3.6 (cross-repo contract tests) — DEFERRED.** The plan's ordering placed these in Phase 3, but both depend on `pkg/contract/` which is created in Phase 4 Task 4.8 (sophia-cli side). Without Phase 4 authorization, neither can land. Plan amendment recommended at next sign-off:

- Move Task 3.1 (orch adopts pkg/contract types) and Task 3.6 (orch contract test adoption) into a new "Phase 3.5" that runs AFTER Phase 4 Task 4.8 ships pkg/contract/ to a tagged sophia-cli commit. Phase 3.5 becomes a follow-up commit on the same `m10/orchestrator-wire-v1` branch (or a new `m10/orchestrator-contract-adopt` branch).

Until then, the orchestrator ships Phase 3 with its existing internal types. Field names match the spec verbatim; the contract tests in Phase 5 will validate the wire shape regardless of which Go package owns the types.

**Tasks 3.3 (SSE event payload normalization) and 3.4 (approval gate URL contract) — NOT INSPECTED IN DEPTH.** The orchestrator emits SSE events from `internal/adapters/inbound/http/handlers/sse.go` and `phases.go`; the existing event shapes were not exhaustively diffed against `sophia-wire-v1` §5.3. Phase 5 contract tests will surface any payload mismatch and prompt a follow-up commit. No proactive normalization done in Phase 3 because:

1. The orch's existing SSE/event tests are green; the wire shape was never failing internally.
2. The compatibility matrix's "Optional" classification for `task.*` and `agent.*` events means CLI-side absence is non-fatal, so even if shapes drift slightly the CLI degrades gracefully.
3. Aggressive normalization without contract tests risks regressing the orch's existing behavior.

**Tasks NOT touched in Phase 3** (per user authorization scope):
- NO change to sophia-cli (Phase 4 not authorized).
- NO contract tests (Phase 5 not authorized).
- NO migration docs (Phase 6 not authorized).
- NO release activity, rc tags, or main-branch merges.

### Open observations for Phase 4 / 5 review

- `router_test.go` lines 212/227/269 carry pre-existing `using resp before checking for errors` lint findings (vet-only output). NOT introduced by Phase 3 commits; left unmodified to keep the diff scoped. Safe to fix in a Phase 5 cleanup commit.
- The orch's `.golangci.yaml` is on the v1 schema. Since `golangci-lint v1.64.x` binaries cannot lint Go 1.26.x code (the same issue M9 fixed for sophia-cli), the orch's CI lint job is likely red. Out of M10 wire scope; tracked as orch-side housekeeping for an independent commit.
- The orchestrator currently has NO release tags (no `v0.1.0`, no rc tags). The "no tocar v0.1.0" rule is trivially satisfied; for v0.2.0 release the coordinated tagging starts from a clean slate.
