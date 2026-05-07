# M10 — Cross-repo HTTP surface inventory

> Read-only inventory captured 2026-05-07 by the M10 Task 1 Explore agent.
> Source for ADR-0003 ("Cross-repo wire alignment with sophia-orchestator").

## sophia-cli outbound HTTP calls

| # | Use case | Method | Path | Auth header | Request shape | Response shape |
|---|----------|--------|------|-------------|---------------|----------------|
| 1 | `DoctorService.checkOrchestrator` | GET | `/api/v1/healthz` | (none) | n/a | JSON: `{status, uptime_s?, checked_at?}` |
| 2 | `Runner.Run`, `Attacher.Attach` | POST | `/api/v1/changes` | (none) | JSON: `{name, project, base_ref, artifact_store_mode}` | JSON: `ChangeResponse` |
| 3 | `Runner.refreshAfterStreamEnd`, `Attacher.Attach`, `StatusReader.fetch` | GET | `/api/v1/changes/{id}` | (none) | n/a | JSON: `ChangeResponse` |
| 4 | `Lister.List` | GET | `/api/v1/changes?project=…&status=…&limit=…&offset=…` | (none) | n/a | JSON: `{items: ChangeResponse[], total?}` |
| 5 | `Runner.streamWithSink`, `Attacher.Attach` | GET | `/api/v1/changes/{change_id}/events` | (none) | n/a (`Last-Event-ID`, `Accept: text/event-stream`) | SSE: heartbeat, phase events, approval events |
| 6 | `DoctorService.checkSSE` | GET | `/api/v1/events` | (none) | n/a (`Accept: text/event-stream`) | SSE handshake (header check only) |

`ChangeResponse` shape (from `internal/adapters/outbound/orchestratorhttp/dto.go`):

```go
type ChangeResponse struct {
    ChangeID          string     `json:"change_id"`
    Name              string     `json:"name,omitempty"`
    Project           string     `json:"project,omitempty"`
    BaseRef           string     `json:"base_ref,omitempty"`
    ArtifactStoreMode string     `json:"artifact_store_mode,omitempty"`
    Status            string     `json:"status,omitempty"`
    CurrentPhaseID    string     `json:"current_phase_id,omitempty"`
    Phases            []PhaseDTO `json:"phases,omitempty"`
    CreatedAt         time.Time  `json:"created_at,omitzero"`
    UpdatedAt         time.Time  `json:"updated_at,omitzero"`
}
```

**Auth pattern:** sophia-cli sends NO auth headers on any request. There
is no API-key code path, no env var, no flag.

## sophia-orchestator inbound HTTP routes

| # | Handler | Method | Path | Auth | Request body | Response body |
|---|---------|--------|------|------|--------------|---------------|
| 1 | `HealthHandler.Check` | GET | `/api/v1/health` | none | n/a | `{status:"ok", uptime_s, checked_at}` |
| 2 | `HealthHandler.Ready` | GET | `/api/v1/ready` | none | n/a | `{status:"ready"}` or 503 |
| 3 | (metrics) | GET | `/metrics` | none | n/a | Prometheus text |
| 4 | `ChangesHandler.Create` | POST | `/api/v1/changes` | **X-Sophia-API-Key** | `{name, project, artifact_store_mode?, base_ref?}` | 201 + change snapshot |
| 5 | `ChangesHandler.List` | GET | `/api/v1/changes?…` | **X-Sophia-API-Key** | n/a | `{items: changeDTO[]}` |
| 6 | `ChangesHandler.Get` | GET | `/api/v1/changes/{change_id}` | **X-Sophia-API-Key** | n/a | change snapshot |
| 7 | `ChangesHandler.Abort` | POST | `/api/v1/changes/{change_id}/abort` | **X-Sophia-API-Key** | `{reason?}` | `{status:"aborted"}` |
| 8 | `PhasesHandler.Run` | POST | `/api/v1/changes/{change_id}/phases/{phase_type}/run` | **X-Sophia-API-Key** | `{task_description?, context_overrides?, retry_budget?}` | 202 + `{phase_id, status, events_url, started_at}` |
| 9 | `PhasesHandler.Get` | GET | `/api/v1/changes/{change_id}/phases/{phase_id}` | **X-Sophia-API-Key** | n/a | phase snapshot |
| 10 | `PhasesHandler.Resume` | POST | `/api/v1/changes/{change_id}/phases/{phase_id}/resume` | **X-Sophia-API-Key** | n/a | 202 + phase snapshot |
| 11 | `PhasesHandler.Approve` | POST | `/api/v1/changes/{change_id}/phases/{phase_id}/approve` | **X-Sophia-API-Key** | `{approver, reason?}` | `{status:"approved"}` |
| 12 | `PhasesHandler.Reject` | POST | `/api/v1/changes/{change_id}/phases/{phase_id}/reject` | **X-Sophia-API-Key** | `{approver, reason?}` | `{status:"rejected"}` |
| 13 | `ApplyHandler.GetBoard` | GET | `/api/v1/changes/{change_id}/phases/{phase_id}/board` | **X-Sophia-API-Key** | n/a | `{board_id, phase_id, status, groups: groupDTO[]}` |
| 14 | `SSEHandler.Stream` | GET | `/api/v1/changes/{change_id}/phases/{phase_id}/events` | **X-Sophia-API-Key** | n/a (`Accept: text/event-stream`) | SSE: open, heartbeat, task events |

## Mismatches identified

| # | Concern | sophia-cli expects | sophia-orchestator implements | Severity |
|---|---------|-------------------|-------------------------------|----------|
| 1 | Health endpoint path | `/api/v1/healthz` | `/api/v1/health` | **P** (path) |
| 2 | Auth on `/changes/*` | (no headers) | `X-Sophia-API-Key` required | **M** (middleware) |
| 3 | Auth on SSE | (no headers) | `X-Sophia-API-Key` required | **M** |
| 4 | SSE probe path | `/api/v1/events` | (no top-level events route) | **P** (likely never wired) |
| 5 | SSE stream scope | per-Change `/changes/{id}/events` | per-Phase `/changes/{cid}/phases/{pid}/events` | **D** (design) |
| 6 | Phase lifecycle | implicit (read-only on Change) | first-class REST verbs | **D** |
| 7 | Approval flow | OOB browser open via `gate_url` event | in-band `POST /phases/{id}/approve` | **D** |
| 8 | ApplyBoard state | derived from `task.*`/`agent.*` SSE events | REST `GET /phases/{id}/board` | **D** |

**Severity codes:**
- **P** — path-only mismatch (rename/alias either side)
- **M** — middleware mismatch (auth header)
- **D** — design-level mismatch (different model / shape / semantics)

## Endpoints with no counterpart

### sophia-cli endpoints with NO orchestrator handler (likely sophia-cli specced wrong)

- `GET /api/v1/healthz` (sophia-cli's `healthz.go`) — orchestrator only has `/health`.
- `GET /api/v1/events` (sophia-cli's `sseprobe.DefaultPath`) — orchestrator has no top-level events endpoint.

### sophia-orchestator endpoints the CLI never calls (CLI gap)

- `POST /api/v1/changes/{change_id}/phases/{phase_type}/run` (`PhasesHandler.Run`)
- `GET /api/v1/changes/{change_id}/phases/{phase_id}` (`PhasesHandler.Get`)
- `POST /api/v1/changes/{change_id}/phases/{phase_id}/resume` (`PhasesHandler.Resume`)
- `POST /api/v1/changes/{change_id}/phases/{phase_id}/approve` (`PhasesHandler.Approve`)
- `POST /api/v1/changes/{change_id}/phases/{phase_id}/reject` (`PhasesHandler.Reject`)
- `GET /api/v1/changes/{change_id}/phases/{phase_id}/board` (`ApplyHandler.GetBoard`)
- `POST /api/v1/changes/{change_id}/abort` (`ChangesHandler.Abort`)
- `GET /api/v1/ready` (`HealthHandler.Ready`)
- `GET /metrics`

This means the CLI today is a **partial client**: it can create and observe
Changes, but it cannot drive phase-level operations (run a specific phase,
approve/reject, fetch the apply board, abort). Those are only
accessible by hitting the orchestrator's REST API directly.

## File citations

- `sophia-cli/internal/adapters/outbound/orchestratorhttp/healthz.go:11`
  — `c.base+"/api/v1/healthz"`
- `sophia-cli/internal/adapters/outbound/orchestratorhttp/changes.go:31`
  — `c.base+"/api/v1/changes"` (POST)
- `sophia-cli/internal/adapters/outbound/orchestratorhttp/changes.go:50`
  — `c.base + "/api/v1/changes/" + url.PathEscape(string(id))` (GET)
- `sophia-cli/internal/adapters/outbound/orchestratorhttp/changes.go:79`
  — `c.base + "/api/v1/changes"` (LIST with query)
- `sophia-cli/internal/adapters/outbound/ssestream/client.go:20`
  — `const DefaultStreamPath = "/api/v1/changes/%s/events"`
- `sophia-cli/internal/adapters/outbound/sseprobe/probe.go:17`
  — `const DefaultPath = "/api/v1/events"`
- `sophia-orchestator/internal/adapters/inbound/http/router.go:51`
  — `r.Get("/api/v1/health", hh.Check)`
- `sophia-orchestator/internal/adapters/inbound/http/router.go:52`
  — `r.Get("/api/v1/ready", hh.Ready)`
- `sophia-orchestator/internal/adapters/inbound/http/router.go:60`
  — `r.Use(middleware.APIKey(d.Auth))` (gates /changes/*)
- `sophia-orchestator/internal/adapters/inbound/http/router.go:68`
  — `r.Route("/api/v1/changes", …)`
- `sophia-orchestator/internal/adapters/inbound/http/router.go:82`
  — `r.Get("/events", sh.Stream)` (mounted under phase route)
- `sophia-orchestator/internal/adapters/inbound/http/middleware/auth.go:24-29`
  — `APIKey(authn) middleware; expects X-Sophia-API-Key`
- `sophia-orchestator/internal/adapters/inbound/http/handlers/apply_health.go:79`
  — `GetBoard handles GET /api/v1/changes/{cid}/phases/{pid}/board`
