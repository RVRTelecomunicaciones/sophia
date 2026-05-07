# 0003 — Cross-repo wire alignment with `sophia-orchestator`

- **Status:** Accepted (2026-05-07)
- **Decision:** Path A3 — canonical spec, both repos converge. A1 retained as fallback if cross-repo coordination becomes infeasible mid-flight; A2 rejected.
- **Date:** 2026-05-07
- **Deciders:** sophia-cli + sophia-orchestator owners (single architectural owner; sign-off recorded in M10 plan pre-execution gate as "Authorized 2026-05-07")
- **Supersedes:** none
- **Related:** ADR-0001 (hexagonal layering), `docs/specs/sophia-wire-v1.md` (the canonical contract this ADR mandates), `docs/superpowers/plans/2026-05-07-sophia-m10-wire-alignment-v0.2.0.md` (M10 implementation plan with D-M10-01..17), `docs/release/manual-smoke-checklist.md` v0.1.0 sign-off note (the trigger that surfaced the drift)
- **Implementation tracking:** M10 / v0.2.0 plan, 7 phases. Phase 1 Task 1.1 (canonical spec authoring) and Task 1.2 (cross-repo mirror) completed 2026-05-07; Task 1.3 (this promotion) completes Phase 1.

## Context

After v0.1.0 of `sophia-cli` was tagged on 2026-05-07, integration smoke
against the running `sophia-orchestator` service surfaced a **design-level
wire mismatch** between the two repos. The two evolved their HTTP surfaces
independently (M1–M8 in sophia-cli; multiple milestones in
sophia-orchestator) without cross-repo integration tests. The gap is much
larger than the three initial mismatches captured in v0.1.0's CHANGELOG.

A complete inventory was produced in M10 Task 1 (see Appendix A below).

### Summary of the gap

**Mechanical mismatches** (path / auth):

| # | Concern | sophia-cli sends | sophia-orchestator expects |
|---|---------|------------------|----------------------------|
| 1 | Health probe | `GET /api/v1/healthz` | `GET /api/v1/health` |
| 2 | SSE probe (doctor) | `GET /api/v1/events` | (no top-level events endpoint) |
| 3 | Auth on every authenticated route | (no header) | `X-Sophia-API-Key` required |

**Design-level mismatches** (model):

| # | Domain | sophia-cli model | sophia-orchestator model |
|---|--------|------------------|--------------------------|
| 4 | Event stream scope | One SSE stream per **Change** (`/changes/{id}/events`) | One SSE stream per **Phase** (`/changes/{cid}/phases/{pid}/events`); a Change spawns multiple streams over its lifecycle |
| 5 | Phase lifecycle | Implicit; phases are read-only fields on the Change snapshot | First-class with REST verbs: `POST /phases/{type}/run`, `POST /phases/{id}/resume`, `POST /phases/{id}/approve`, `POST /phases/{id}/reject` |
| 6 | Approval flow | Out-of-band via `gate_url` SSE event → user opens browser → orchestrator-side decision | In-band via authenticated `POST /phases/{id}/approve` body `{approver, reason}` |
| 7 | Apply board state | Derived in TUI from `task.*` / `agent.*` SSE events (M7) | Exposed as REST: `GET /phases/{id}/board` returning groups → tasks → agents |

**Endpoints exposed by the orchestrator that the CLI doesn't call at all**
(approve, reject, resume, run-phase, get-board, /ready, /metrics) —
indicating that the CLI never grew the surface needed to drive the
orchestrator's first-class operations.

## Decision (ACCEPTED — Path A3)

**Adopted:** Path A3 (canonical spec, both repos converge).
**Fallback:** Path A1 retained if cross-repo coordination becomes infeasible mid-flight.
**Rejected:** Path A2 (orchestrator-side compat shims) — generates dual-API debt; security smell from a `dev-mode` toggle.

The canonical wire spec is `docs/specs/sophia-wire-v1.md` (authoritative copy in sophia-cli, byte-for-byte mirror in sophia-orchestator, SHA256-gated by CI). All wire concerns described in Section "Summary of the gap" below are resolved by the spec; the implementation work to align both repos to the spec is the M10 plan.

The original options (A1/A2/A3) and their tradeoff matrix are preserved below for historical context.

## Original options analysis

Three candidate paths originally considered. The owner decision was the trigger for the M10 implementation plan (Task 4).

### Path A1 — sophia-cli adopts the orchestrator's wire protocol

**Scope:** large. sophia-cli refactors its outbound surface to match what
the orchestrator already implements.

**Tasks:**

- Rename `/api/v1/healthz` → `/api/v1/health` in
  `internal/adapters/outbound/orchestratorhttp/healthz.go`. (Trivial.)
- Add API key support: env `SOPHIA_API_KEY` + flag `--api-key` →
  `X-Sophia-API-Key` header on every outbound HTTP/SSE request. Plumb
  through `bootstrap`. (Medium — touches every outbound adapter.)
- Refactor `Runner.Observe` from per-Change SSE to per-Phase SSE:
  - Track `current_phase_id` from the latest `GetChange` snapshot.
  - Subscribe to `/changes/{cid}/phases/{pid}/events`.
  - When the phase's stream ends, refresh the Change snapshot and
    re-subscribe to the new `current_phase_id`'s events.
  - Terminal status comes from the Change snapshot, not from individual
    phase streams.
- Add CLI verbs for the phase lifecycle: `sophia approve <id> <phase> [-r reason]`,
  `sophia reject <id> <phase> [-r reason]`, `sophia resume <id> <phase>`.
  (M10 anti-scope-debate: are these new commands or a hidden internal
  flow?)
- Replace the M7 `[O]pen browser` approval flow with the in-band POST.
  Or keep both: open browser as UI affordance, POST as the actual
  decision channel.
- Remove or reroute `sseprobe` since `/api/v1/events` doesn't exist; the
  doctor check becomes "GET /api/v1/health succeeds" — drop the SSE
  handshake or re-target it at a real change/phase combo (impractical
  for doctor pre-flight).

**Estimated cost:** ~1000-1500 LOC, 4-5 sessions of work, breaking change
to sophia-cli's internal model. v0.2.0 release.

**Pros:**
- sophia-orchestator stays unchanged; its model is the canonical truth.
- sophia-cli grows the phase-level operations the orchestrator already
  supports (currently a feature gap, not just protocol drift).

**Cons:**
- Significant rewrite. Risk of regressing M5–M8 features.
- New CLI surface (`sophia approve` etc.) needs UX design, docs, tests.
- The "open browser" approval gate (M7) must reconcile with the in-band
  POST — both are valid UX, but mixing them is confusing.

### Path A2 — sophia-orchestator adapts to sophia-cli's surface

**Scope:** server-side changes; sophia-cli stays as v0.1.0.

**Tasks (orchestrator side):**

- Add `/api/v1/healthz` as an alias for `/api/v1/health`.
- Add a "no-auth dev mode" toggle (env `SOPHIA_DEV_MODE=1` disables
  middleware.APIKey) — or accept anonymous requests when no API key is
  configured. Must NOT ship in production binaries.
- Add a per-Change SSE aggregator at `/api/v1/changes/{id}/events` that
  multiplexes phase-level streams behind the scenes. Emits events tagged
  with `phase_id` so the client can route them.
- Either expose `/api/v1/events` as a no-op handshake endpoint (for
  sseprobe) or accept that doctor's SSE handshake is informational-only
  and may fail without breaking doctor's overall green status.

**Estimated cost:** orchestrator-side work; outside this repo. ~500 LOC
on the other side.

**Pros:**
- v0.1.0 of sophia-cli stays valid against the new orchestrator endpoints.
- No breaking changes to either side's clients (the orchestrator just
  grows compat surface).

**Cons:**
- Orchestrator carries dual-API forever; each new feature must be
  exposed twice (per-phase and per-change).
- The "no-auth dev mode" widens the orchestrator's attack surface.
- The CLI never learns the phase-lifecycle operations (approve/reject/
  resume), so it remains a partial client.

### Path A3 — canonical wire spec, both repos converge

**Scope:** medium-large in both repos; coordinated.

**Tasks:**

- Author a single source-of-truth wire spec (e.g.
  `docs/specs/sophia-wire-v1.md` mirrored in both repos) covering paths,
  auth, request/response shapes, SSE event types.
- Pick the canonical model for each design-level disagreement:
  - SSE granularity: per-Change (CLI's current) vs per-Phase
    (orchestrator's current) vs hybrid (per-Change with phase tags).
  - Approval flow: OOB (browser open) vs in-band POST vs both.
  - Apply board: SSE-derived (CLI) vs REST-fetched (orchestrator) vs
    both with cache.
- Both repos open a tracking PR for the spec migration; merge in lockstep.
- Cross-repo integration test suite: a small Go test harness that
  builds both binaries and runs end-to-end against the orchestrator's
  in-process server. Lives in either repo or a third
  `sophia-integration-tests` repo.

**Estimated cost:** the most expensive path in absolute hours but the
only one that produces a stable contract going forward. Spec authoring
~1 session; implementation ~3-5 sessions per repo; integration suite
~2 sessions.

**Pros:**
- Permanent fix; future drift is preventable via the spec + integration
  tests.
- Each repo's existing model is examined and the better one wins per
  question (no "loser" repo).

**Cons:**
- Requires owner-level coordination across repos.
- Slowest path to a working v0.2.0.

## Tradeoff matrix

| Criterion | A1 (cli adopts) | A2 (orch adapts) | A3 (canonical) |
|-----------|:---:|:---:|:---:|
| Time to working v0.2.0 | medium | fast | slow |
| Cross-repo coordination needed | low (read orch spec) | low (orch-side only) | high |
| Long-term drift prevention | low | low | high |
| Final API quality | medium (CLI catches up) | low (dual-API debt) | high |
| Feature completeness (CLI gains phase ops) | yes | no | yes |
| Risk of regression | high (CLI rewrite) | low | medium |
| sophia-cli scope creep (auth, phase ops) | yes — net new features | no | yes |
| Honors M9 anti-scope rule "no auth flows" | violates | preserves | violates |

## Recommendation

**A3 is the correct long-term answer; A1 is the pragmatic short-term step
if cross-repo coordination is not currently feasible.**

A2 is rejected: it freezes sophia-cli at v0.1.0's model, accumulates
dual-API debt on the orchestrator side, and never produces a feature-
complete CLI. The "no-auth dev mode" toggle is a security smell.

If A3 is chosen, this ADR becomes the seed for a cross-repo spec
document. If A1 is chosen, this ADR becomes the spec for sophia-cli's
v0.2.0 refactor and the orchestrator stays untouched.

## Open questions for the owner decision

1. **Are the two repos owned by the same team / single decision-maker?**
   If yes, A3 is feasible. If they have independent owners, A1 minimizes
   coordination overhead.
2. **Is `--api-key` / env `SOPHIA_API_KEY` acceptable as a v0.2.0
   feature?** M9's anti-scope ruled out auth flows; lifting that ruling
   is a v0.2.0 prerequisite for both A1 and A3.
3. **Does the existing M6/M7 approval flow (browser-open) survive,
   or is it deprecated in favor of `sophia approve <id> <phase>`?**
   This affects M7 deviation notes and TUI keybindings.
4. **What's the orchestrator's release cadence?** A3 requires both
   repos' release windows to align.

## Appendix A — Full inventory

See `docs/superpowers/research/m10-wire-inventory.md` (will be committed
alongside this ADR).

## Appendix B — Path mismatches summary table

(See "Summary of the gap" section above.)
