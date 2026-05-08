# Contract test harness — sophia-wire-v1

This directory holds the cross-repo wire-conformance test suite that
gates a coordinated `sophia-cli` + `sophia-orchestator` release.

The suite has two tiers:

| Tier | When it runs | What it validates |
|---|---|---|
| **Lightweight** (`*_test.go` without build tag) | every `make test` | spec checksum invariant: local & cross-repo `sophia-wire-v1.sha256` match |
| **Contract** (`-tags=contract`) | opt-in via `make contract` | the cli's outbound HTTP + SSE clients consume a spec-conformant orchestrator correctly |

The orchestrator side's "implements the spec" gate lives in the
orchestrator repo's own test suite (Phase 3.8). When BOTH suites pass
AND `sophia-wire-v1.sha256` matches across the two repos, cross-repo
compatibility is established at the wire level.

## Running

```bash
# Lightweight (always-on): SHA256 cross-repo gate.
go test ./test/contract/...

# Full contract suite (synthetic spec server + cli e2e through application services).
make contract
# or:
go test -tags=contract ./test/contract/...

# Phase 5 / D-M10-15 release-blocker gate: must pass without a go.work file.
make test-no-workspace
```

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `SOPHIA_ORCHESTRATOR_REPO` | `../sophia-orchestator` | absolute or relative path to the orchestrator repo. The cross-repo checksum test reads `<repo>/docs/specs/sophia-wire-v1.sha256` from this location. Skips with an explanatory message when not found. |

## What's covered

### Required endpoints (sophia-wire-v1 §4)
- `GET /api/v1/health` (un-authenticated)
- `POST /api/v1/changes`
- `GET /api/v1/changes`
- `GET /api/v1/changes/{id}`
- `POST /api/v1/changes/{id}/abort`
- `GET /api/v1/phases/{id}`
- `POST /api/v1/phases/{id}/approve`
- `POST /api/v1/phases/{id}/reject`
- `GET /api/v1/phases/{id}/events`

### Auth modes (sophia-wire-v1 §3, D-M10-02)
- loopback anonymous accepted
- remote anonymous rejected with `unauthorized` envelope
- valid key accepted (header `X-Sophia-API-Key`)
- invalid key rejected

### SSE events (sophia-wire-v1 §5)
- `open` (Optional, Phase 1.5 amendment) — emitted on connection
- `phase.started` / `phase.completed` / `phase.failed`
- `approval.required` / `approval.resolved`
- unknown event types (e.g. `apply.tx.committed`) are tolerated and surfaced unchanged
- `phase_terminal_no_events` 410 closes the channel without a retry storm

### Error envelope (sophia-wire-v1 §9)
- canonical `{code, error, details?}` shape
- all 13 stable codes round-trip through `errors.Is` to domain sentinels:
  `unauthorized`, `validation_failed`, `approver_required`,
  `limit_too_large`, `change_not_found`, `phase_not_found`,
  `change_already_exists`, `change_already_terminal`,
  `phase_not_resumable`, `phase_not_gated`, `gate_already_decided`,
  `phase_terminal_no_events`, `internal_error`

### CLI smoke (Phase 5 scope item 8)
- `doctor`: orchestrator check passes against the spec server
- `changes`: list returns canonical shape
- `approve` / `reject`: hit the right endpoints; idempotent on `gate_already_decided`
- `abort`: hits `/abort`; idempotent on `change_already_terminal`
- `run`: full multiplexer flow — CreateChange → SSE subscribe → drain → snapshot → done

## Test architecture

The contract suite uses a **synthetic spec server** (`spec_server_test.go`)
that responds with `pkg/contract` types. The synthetic server is
programmable — each test sets the behaviors it needs (auth required,
approve_already_decided, terminal phase, SSE event queue, …) via the
`specServer` struct fields.

This is honest about what we can validate at this stage:

- **The cli is spec-correct** ← validated here.
- **The orchestrator is spec-correct** ← validated by the orch's own
  suite (Phase 3.8) which builds an `httptest.Server` from the orch's
  router and asserts the same wire shapes.
- **Both sides agree on what "spec" means** ← validated by the SHA256
  cross-repo gate (`TestSpecChecksum_CrossRepoMatchesOrchestrator`).

Together these three gates establish cross-repo compatibility without
requiring both binaries to be co-resident in CI. A binary smoke that
boots the real orchestrator (Postgres-backed) is documented below as
the next step but not currently blocking.

## Future: real-binary smoke (post-Phase 5)

The recommended path for a full E2E against the real orchestrator
binary:

1. CI runs Postgres in a service container (or testcontainers).
2. CI clones both repos at the to-be-tagged SHAs.
3. CI builds `cmd/sophia-orchestator` from the orch repo and starts
   the binary on a free port.
4. The cli's contract suite is re-run with
   `SOPHIA_CONTRACT_BASE_URL=http://localhost:NNNN` to swap the
   synthetic server for the real one.

This test mode is intentionally NOT gated on Phase 5 — Phase 7
(coordinated release) is where the binary smoke + tag flow lands.
