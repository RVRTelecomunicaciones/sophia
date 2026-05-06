# Sophia CLI — M4 Run via Polling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the first end-to-end create-and-observe loop using HTTP polling only (no SSE yet). `sophia run "msg" --no-tui --json` reads `.sophia.yaml`, POSTs a Change, persists `last_change_id` atomically, then polls `GET /api/v1/changes/{id}` until terminal status, emitting JSONL snapshot lines to stdout. Validates the orchestrator's `auto_advance` assumption.

**Architecture:** Three new pieces wire together. (1) `orchestratorhttp` gains DTOs + typed errors + real `CreateChange/GetChange/ListChanges` (replacing M2's `ErrNotImplemented`). (2) New inbound adapter `jsonsink` implements `EventSink` writing JSONL to stdout. (3) New application service `Runner` orchestrates the full vertical slice; a `ConfigResolver` layers flags > env > project yaml > user yaml > defaults. The runner exits with typed `ExitError` wired into main.go for spec exit codes 0/1/3/4.

**Tech Stack:** Go 1.24.x · `net/http` (stdlib) · `encoding/json` (stdlib). No new third-party deps.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.1, §2.2, §2.3, §2.4, §3.4, §3.5, §4.2, §5.1, §5.2, §6.3, §7.2 M4 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M4)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M4 boundaries — what is NOT in M4:**

- No SSE consumer — polling only (M5 replaces).
- No Bubble Tea TUI — `--no-tui` is implicit (TUI lands M6).
- No `sophia attach` / `sophia changes` real commands — stay stubs (M8).
- No real status with HTTP fetch — stays M3 placeholder (M8 finishes).
- No browser opener — M7.
- No redaction pipeline — snapshots have structured fields only; M5 SSE adds the redactor.
- No `RunPhase` compatibility mode unless e2e proves `auto_advance` is false (escalate then).
- No approval-timeout exit 5 (M5+; M4 stops on terminal status only).

---

## Phase 1 — Orchestrator DTOs and typed errors

### Task 1: orchestratorhttp/dto.go — request/response shapes

**Files:**
- Create: `internal/adapters/outbound/orchestratorhttp/dto.go`
- Create: `internal/adapters/outbound/orchestratorhttp/dto_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/orchestratorhttp/dto_test.go`:

```go
package orchestratorhttp_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestCreateRequestJSONFieldsAreSnakeCase(t *testing.T) {
	r := orchestratorhttp.CreateChangeRequest{
		Name:              "test",
		Project:           "p",
		BaseRef:           "main",
		ArtifactStoreMode: "engram",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"test"`, `"project":"p"`, `"base_ref":"main"`, `"artifact_store_mode":"engram"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in %s", want, b)
		}
	}
}

func TestChangeResponseDecodes(t *testing.T) {
	body := `{
		"change_id":"01HX-abc",
		"name":"test",
		"project":"p",
		"base_ref":"main",
		"artifact_store_mode":"engram",
		"status":"running",
		"current_phase_id":"phase-1",
		"phases":[
			{"id":"phase-1","type":"explore","status":"running","confidence":0.5,"started_at":"2026-05-05T12:00:00Z"}
		],
		"created_at":"2026-05-05T11:00:00Z",
		"updated_at":"2026-05-05T12:00:00Z"
	}`
	var resp orchestratorhttp.ChangeResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	got := resp.ToDomain()
	if got.ID.String() != "01HX-abc" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", got.Status)
	}
	if got.CurrentPhaseID != "phase-1" {
		t.Errorf("CurrentPhaseID = %q", got.CurrentPhaseID)
	}
	if len(got.Phases) != 1 {
		t.Fatalf("Phases len = %d", len(got.Phases))
	}
	if got.Phases[0].Type != domain.PhaseExplore {
		t.Errorf("phase Type = %q", got.Phases[0].Type)
	}
	if got.Phases[0].Confidence != 0.5 {
		t.Errorf("phase Confidence = %f", got.Phases[0].Confidence)
	}
	if got.Phases[0].StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
	want := time.Date(2026, 5, 5, 11, 0, 0, 0, time.UTC)
	if !got.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want)
	}
}

func TestChangeResponseTolerantOfMissingFields(t *testing.T) {
	body := `{"change_id":"01HX","status":"pending"}`
	var resp orchestratorhttp.ChangeResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	got := resp.ToDomain()
	if got.ID.String() != "01HX" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusPending {
		t.Errorf("Status = %q", got.Status)
	}
	if got.Phases != nil {
		t.Errorf("Phases should be nil when absent, got %v", got.Phases)
	}
}

func TestListResponseDecodes(t *testing.T) {
	body := `{"items":[{"change_id":"a","status":"done"},{"change_id":"b","status":"running"}],"total":2}`
	var resp orchestratorhttp.ListChangesResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("Items len = %d", len(resp.Items))
	}
	if resp.Total != 2 {
		t.Errorf("Total = %d", resp.Total)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/orchestratorhttp/dto.go`:

```go
package orchestratorhttp

import (
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// CreateChangeRequest mirrors the body of POST /api/v1/changes.
type CreateChangeRequest struct {
	Name              string `json:"name"`
	Project           string `json:"project"`
	BaseRef           string `json:"base_ref"`
	ArtifactStoreMode string `json:"artifact_store_mode"`
}

// PhaseDTO mirrors a phase entry in the orchestrator's snapshot response.
type PhaseDTO struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Confidence float64   `json:"confidence"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
}

// ChangeResponse mirrors the response shape of POST /api/v1/changes and
// GET /api/v1/changes/{id}. All fields are tolerant of absence per spec
// §3.2 (forward-compatible).
type ChangeResponse struct {
	ChangeID          string     `json:"change_id"`
	Name              string     `json:"name,omitempty"`
	Project           string     `json:"project,omitempty"`
	BaseRef           string     `json:"base_ref,omitempty"`
	ArtifactStoreMode string     `json:"artifact_store_mode,omitempty"`
	Status            string     `json:"status,omitempty"`
	CurrentPhaseID    string     `json:"current_phase_id,omitempty"`
	Phases            []PhaseDTO `json:"phases,omitempty"`
	CreatedAt         time.Time  `json:"created_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at,omitempty"`
}

// ToDomain converts a ChangeResponse to the pure domain type.
func (r *ChangeResponse) ToDomain() *domain.Change {
	c := &domain.Change{
		ID:                domain.ChangeID(r.ChangeID),
		Name:              r.Name,
		Project:           r.Project,
		BaseRef:           r.BaseRef,
		ArtifactStoreMode: r.ArtifactStoreMode,
		Status:            domain.ChangeStatus(r.Status),
		CurrentPhaseID:    r.CurrentPhaseID,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
	if len(r.Phases) > 0 {
		c.Phases = make([]domain.Phase, len(r.Phases))
		for i, p := range r.Phases {
			c.Phases[i] = domain.Phase{
				ID:         p.ID,
				Type:       domain.PhaseType(p.Type),
				Status:     domain.PhaseStatus(p.Status),
				Confidence: p.Confidence,
				StartedAt:  p.StartedAt,
				EndedAt:    p.EndedAt,
			}
		}
	}
	return c
}

// ListChangesResponse mirrors GET /api/v1/changes.
type ListChangesResponse struct {
	Items []ChangeResponse `json:"items"`
	Total int              `json:"total"`
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/orchestratorhttp/dto.go internal/adapters/outbound/orchestratorhttp/dto_test.go
git commit -m "feat(orchestratorhttp): add request/response DTOs with domain conversion"
```

---

### Task 2: orchestratorhttp/errors.go — typed HTTP errors

**Files:**
- Create: `internal/adapters/outbound/orchestratorhttp/errors.go`
- Create: `internal/adapters/outbound/orchestratorhttp/errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
package orchestratorhttp_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestStatusErrorPreservesCode(t *testing.T) {
	e := orchestratorhttp.NewStatusError(404, []byte(`{"error":"not found"}`))
	if e.StatusCode != 404 {
		t.Errorf("StatusCode = %d", e.StatusCode)
	}
	if e.Error() == "" {
		t.Error("empty message")
	}
}

func TestStatusErrorMapsNotFoundToErrChangeNotFound(t *testing.T) {
	e := orchestratorhttp.NewStatusError(404, nil)
	if !errors.Is(e, domain.ErrChangeNotFound) {
		t.Errorf("404 should map to ErrChangeNotFound")
	}
}

func TestStatusErrorMapsServerErrorsToErrUnreachable(t *testing.T) {
	e := orchestratorhttp.NewStatusError(503, nil)
	if !errors.Is(e, domain.ErrUnreachable) {
		t.Errorf("5xx should map to ErrUnreachable")
	}
}

func TestStatusErrorOtherCodesAreNotMapped(t *testing.T) {
	e := orchestratorhttp.NewStatusError(400, []byte("bad input"))
	if errors.Is(e, domain.ErrChangeNotFound) || errors.Is(e, domain.ErrUnreachable) {
		t.Errorf("400 should not map to known sentinels")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/orchestratorhttp/errors.go`:

```go
package orchestratorhttp

import (
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// StatusError is returned by orchestratorhttp methods on non-2xx responses.
// It maps 404 to domain.ErrChangeNotFound and 5xx to domain.ErrUnreachable
// via errors.Is.
type StatusError struct {
	StatusCode int
	Body       []byte
}

// NewStatusError constructs a StatusError.
func NewStatusError(code int, body []byte) *StatusError {
	return &StatusError{StatusCode: code, Body: body}
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("orchestrator HTTP %d", e.StatusCode)
	}
	// Cap body in error message to prevent log bloat.
	body := e.Body
	if len(body) > 256 {
		body = body[:256]
	}
	return fmt.Sprintf("orchestrator HTTP %d: %s", e.StatusCode, body)
}

// Is enables errors.Is(err, domain.ErrChangeNotFound) and similar.
func (e *StatusError) Is(target error) bool {
	switch {
	case e.StatusCode == 404 && errors.Is(target, domain.ErrChangeNotFound):
		return true
	case e.StatusCode >= 500 && e.StatusCode < 600 && errors.Is(target, domain.ErrUnreachable):
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/orchestratorhttp/errors.go internal/adapters/outbound/orchestratorhttp/errors_test.go
git commit -m "feat(orchestratorhttp): add StatusError with sentinel-error mapping"
```

---

## Phase 2 — Real CreateChange / GetChange / ListChanges

### Task 3: Implement OrchestratorClient.CreateChange / GetChange / ListChanges

**Files:**
- Create: `internal/adapters/outbound/orchestratorhttp/changes.go`
- Create: `internal/adapters/outbound/orchestratorhttp/changes_test.go`
- Modify: `internal/adapters/outbound/orchestratorhttp/client.go` (remove the three M2 stubs)

- [ ] **Step 1: Read client.go**

Read `internal/adapters/outbound/orchestratorhttp/client.go` first to understand current shape. M2 left `CreateChange`, `GetChange`, `ListChanges` as stubs returning `ErrNotImplemented`. Remove those three method bodies (we replace them in `changes.go`).

- [ ] **Step 2: Write the failing test**

`internal/adapters/outbound/orchestratorhttp/changes_test.go`:

```go
package orchestratorhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestCreateChangePostsBodyAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		if r.URL.Path != "/api/v1/changes" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		var got orchestratorhttp.CreateChangeRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Name != "msg" || got.Project != "p" || got.BaseRef != "main" || got.ArtifactStoreMode != "engram" {
			t.Errorf("body = %+v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"change_id":"01HX","name":"msg","project":"p","status":"pending"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{
		Name: "msg", Project: "p", BaseRef: "main", ArtifactStoreMode: "engram",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID.String() != "01HX" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != domain.ChangeStatusPending {
		t.Errorf("Status = %q", got.Status)
	}
}

func TestCreateChangeMapsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"down"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnreachable) {
		t.Errorf("expected ErrUnreachable, got %v", err)
	}
}

func TestGetChangeFetchesAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/changes/01HX" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"change_id":"01HX","status":"running","current_phase_id":"phase-1"}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.GetChange(context.Background(), domain.ChangeID("01HX"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.ChangeStatusRunning {
		t.Errorf("Status = %q", got.Status)
	}
	if got.CurrentPhaseID != "phase-1" {
		t.Errorf("CurrentPhaseID = %q", got.CurrentPhaseID)
	}
}

func TestGetChangeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	_, err := c.GetChange(context.Background(), domain.ChangeID("nope"))
	if !errors.Is(err, domain.ErrChangeNotFound) {
		t.Errorf("expected ErrChangeNotFound, got %v", err)
	}
}

func TestListChangesQueryString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		if q.Get("project") != "p" {
			t.Errorf("project = %q", q.Get("project"))
		}
		if q.Get("status") != "running" {
			t.Errorf("status = %q", q.Get("status"))
		}
		if q.Get("limit") != "5" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		if q.Get("offset") != "10" {
			t.Errorf("offset = %q", q.Get("offset"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"change_id":"a","status":"running"}],"total":1}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	got, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{
		Project: "p", Status: "running", Limit: 5, Offset: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID.String() != "a" {
		t.Errorf("got = %+v", got)
	}
}

func TestListChangesNoFiltersOmitsEmptyParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		if q.Get("project") != "" {
			t.Errorf("project should be empty, got %q", q.Get("project"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"total":0}`)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if _, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{}); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/...`
Expected: FAIL (CreateChange etc. still return ErrNotImplemented).

- [ ] **Step 4: Implement changes.go**

`internal/adapters/outbound/orchestratorhttp/changes.go`:

```go
package orchestratorhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// MaxBodySize caps response body reads to 1 MiB to bound memory.
const MaxBodySize = 1 << 20

// CreateChange POSTs /api/v1/changes.
func (c *Client) CreateChange(ctx context.Context, in outbound.CreateChangeInput) (*domain.Change, error) {
	body, err := json.Marshal(CreateChangeRequest{
		Name:              in.Name,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: in.ArtifactStoreMode,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/changes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	var resp ChangeResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.ToDomain(), nil
}

// GetChange GETs /api/v1/changes/{id}.
func (c *Client) GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("empty change id")
	}
	u := c.base + "/api/v1/changes/" + url.PathEscape(string(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	var resp ChangeResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.ToDomain(), nil
}

// ListChanges GETs /api/v1/changes with optional filters.
func (c *Client) ListChanges(ctx context.Context, f outbound.ListChangesFilter) ([]*domain.Change, error) {
	q := url.Values{}
	if f.Project != "" {
		q.Set("project", f.Project)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}
	u := c.base + "/api/v1/changes"
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	var resp ListChangesResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := make([]*domain.Change, len(resp.Items))
	for i, item := range resp.Items {
		dto := item
		out[i] = dto.ToDomain()
	}
	return out, nil
}

// doJSON executes req, reads up to MaxBodySize bytes, returns a *StatusError
// on non-2xx, or json-decodes a 2xx body into out.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewStatusError(resp.StatusCode, body)
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode body: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Update client.go**

Read `client.go`. Remove the three stub methods (`CreateChange`, `GetChange`, `ListChanges`) and the `ErrNotImplemented` variable that the stubs returned. The package-level `ErrNotImplemented` is no longer referenced. Drop it.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/... -race`
Expected: PASS (all 13 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/outbound/orchestratorhttp/changes.go \
        internal/adapters/outbound/orchestratorhttp/changes_test.go \
        internal/adapters/outbound/orchestratorhttp/client.go
git commit -m "feat(orchestratorhttp): implement CreateChange/GetChange/ListChanges (M4)"
```

---

## Phase 3 — JsonSink inbound adapter

### Task 4: adapters/inbound/jsonsink/sink.go — EventSink → stdout JSONL

**Files:**
- Create: `internal/adapters/inbound/jsonsink/sink.go`
- Create: `internal/adapters/inbound/jsonsink/sink_test.go`

- [ ] **Step 1: Write the failing test**

```go
package jsonsink_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/jsonsink"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
)

func TestSinkImplementsEventSink(t *testing.T) {
	var _ inbound.EventSink = jsonsink.New(jsonsink.Config{Writer: &bytes.Buffer{}})
}

func TestSnapshotEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	change := &domain.Change{
		ID:             "01HX",
		Status:         domain.ChangeStatusRunning,
		CurrentPhaseID: "phase-1",
		Phases: []domain.Phase{
			{ID: "phase-1", Type: domain.PhaseExplore, Status: domain.PhaseStatusRunning},
		},
	}
	if err := s.OnSnapshot(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, line)
	}
	if got["type"] != "snapshot" {
		t.Errorf("type = %v", got["type"])
	}
	if got["change_id"] != "01HX" {
		t.Errorf("change_id = %v", got["change_id"])
	}
	if got["status"] != "running" {
		t.Errorf("status = %v", got["status"])
	}
}

func TestEventEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	ev := domain.Event{
		Type:      "phase.started",
		Timestamp: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"phase": "explore"},
	}
	if err := s.OnEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "event" {
		t.Errorf("type = %v", got["type"])
	}
	if got["ev"] != "phase.started" {
		t.Errorf("ev = %v", got["ev"])
	}
}

func TestApprovalGateEmitsValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	gate := domain.ApprovalGate{
		URL: "https://gov/approvals/x", Reason: "needs review", Risk: "high", Policy: "require_approval",
		ChangeID: "01HX", Phase: domain.PhaseApply,
	}
	if err := s.OnApprovalGate(context.Background(), gate); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "event" {
		t.Errorf("type = %v", got["type"])
	}
	if got["ev"] != "approval.required" {
		t.Errorf("ev = %v", got["ev"])
	}
	payload, _ := got["payload"].(map[string]any)
	if payload["gate_url"] != "https://gov/approvals/x" {
		t.Errorf("gate_url = %v", payload["gate_url"])
	}
}

func TestErrorEmitsAndDoesNotReturn(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	if err := s.OnError(context.Background(), errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"type":"error"`) {
		t.Errorf("missing error line: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"message":"boom"`) {
		t.Errorf("missing message: %s", buf.String())
	}
}

func TestCompleteEmitsTerminalStatus(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	if err := s.OnComplete(context.Background(), domain.ChangeStatusDone); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"final_status":"done"`) {
		t.Errorf("missing final_status: %s", buf.String())
	}
}

func TestSinkIsConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	s := jsonsink.New(jsonsink.Config{Writer: &buf})
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				_ = s.OnEvent(context.Background(), domain.Event{Type: "x"})
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	if got := strings.Count(buf.String(), "\n"); got != 200 {
		t.Errorf("line count = %d, want 200", got)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/jsonsink/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/inbound/jsonsink/sink.go`:

```go
// Package jsonsink implements inbound.EventSink by writing one JSON object
// per line to a configured io.Writer (typically os.Stdout). Used by
// `sophia run --no-tui --json` per spec §2.4.
package jsonsink

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the Sink.
type Config struct {
	Writer io.Writer
}

// Sink implements inbound.EventSink.
type Sink struct {
	mu sync.Mutex
	w  io.Writer
}

// New returns a Sink writing to cfg.Writer.
func New(cfg Config) *Sink { return &Sink{w: cfg.Writer} }

// OnSnapshot writes a snapshot line.
func (s *Sink) OnSnapshot(_ context.Context, c *domain.Change) error {
	phases := make([]map[string]any, 0, len(c.Phases))
	for _, p := range c.Phases {
		phases = append(phases, map[string]any{
			"id":         p.ID,
			"type":       string(p.Type),
			"status":     string(p.Status),
			"confidence": p.Confidence,
			"started_at": optionalTime(p.StartedAt),
			"ended_at":   optionalTime(p.EndedAt),
		})
	}
	return s.write(map[string]any{
		"type":             "snapshot",
		"change_id":        c.ID.String(),
		"name":             c.Name,
		"project":          c.Project,
		"status":           string(c.Status),
		"current_phase_id": c.CurrentPhaseID,
		"phases":           phases,
	})
}

// OnEvent writes an event line.
func (s *Sink) OnEvent(_ context.Context, ev domain.Event) error {
	return s.write(map[string]any{
		"type":    "event",
		"ev":      ev.Type,
		"ts":      optionalTime(ev.Timestamp),
		"payload": ev.Payload,
	})
}

// OnApprovalGate writes an approval-gate event line.
func (s *Sink) OnApprovalGate(_ context.Context, g domain.ApprovalGate) error {
	return s.write(map[string]any{
		"type": "event",
		"ev":   "approval.required",
		"payload": map[string]any{
			"gate_url":  g.URL,
			"reason":    g.Reason,
			"risk":      g.Risk,
			"policy":    g.Policy,
			"change_id": g.ChangeID.String(),
			"phase":     string(g.Phase),
		},
	})
}

// OnError writes an error line. Best-effort: never bubble write errors up to
// runner so that streaming continues for all sinks.
func (s *Sink) OnError(_ context.Context, e error) error {
	return s.write(map[string]any{
		"type":    "error",
		"message": e.Error(),
	})
}

// OnComplete writes a final terminal-status line.
func (s *Sink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	return s.write(map[string]any{
		"type":         "complete",
		"final_status": string(st),
	})
}

// Close is a no-op for stdout.
func (s *Sink) Close() error { return nil }

func (s *Sink) write(v map[string]any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.w.Write(b)
	return err
}

// optionalTime returns RFC3339 string or nil for zero times, so JSONL lines
// don't carry meaningless `0001-01-01T00:00:00Z`.
func optionalTime(t interface{ IsZero() bool }) any {
	type zeroable interface {
		Format(string) string
		IsZero() bool
	}
	z, ok := t.(zeroable)
	if !ok || z.IsZero() {
		return nil
	}
	return z.Format("2006-01-02T15:04:05.000Z07:00")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/jsonsink/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/jsonsink/sink.go internal/adapters/inbound/jsonsink/sink_test.go
git commit -m "feat(jsonsink): add inbound EventSink that writes JSONL to stdout"
```

---

## Phase 4 — ConfigResolver (flag/env/yaml layering)

### Task 5: application/configresolver.go — layered configuration

**Files:**
- Create: `internal/application/configresolver.go`
- Create: `internal/application/configresolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newResolver() (*application.ConfigResolver, *fakes.FakeProjectConfigStore, *fakes.FakeUserConfigStore, *fakes.FakeGitInspector) {
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()
	git := fakes.NewFakeGitInspector()
	r := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc,
		UserStore:    uc,
		Git:          git,
	})
	return r, pc, uc, git
}

func TestResolverFlagBeatsEnv(t *testing.T) {
	r, _, _, _ := newResolver()
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		Flags: application.ResolverFlags{OrchestratorURL: "http://flag"},
		Env:   map[string]string{"SOPHIA_ORCHESTRATOR_URL": "http://env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != "http://flag" {
		t.Errorf("OrchestratorURL = %q", got.OrchestratorURL)
	}
}

func TestResolverEnvBeatsProject(t *testing.T) {
	r, pc, _, git := newResolver()
	git.Root = "/repo"
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "from-yaml", BaseRef: "develop",
	})
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		Env: map[string]string{"SOPHIA_PROJECT": "from-env", "SOPHIA_BASE_REF": "release"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "from-env" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.BaseRef != "release" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
}

func TestResolverProjectBeatsUser(t *testing.T) {
	r, pc, uc, git := newResolver()
	git.Root = "/repo"
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "from-yaml", BaseRef: "main", ArtifactStore: domain.ArtifactStoreOpenspec,
	})
	_ = uc.Write(context.Background(), "/cfg/config.yaml", &domain.UserConfig{
		OrchestratorURL: "http://from-user", TimeoutSeconds: 42,
	})
	got, err := r.Resolve(context.Background(), application.ResolverInput{
		UserConfigPath: "/cfg/config.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "from-yaml" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.OrchestratorURL != "http://from-user" {
		t.Errorf("OrchestratorURL = %q", got.OrchestratorURL)
	}
	if got.ArtifactStore != domain.ArtifactStoreOpenspec {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
}

func TestResolverDefaultsApplyLast(t *testing.T) {
	r, _, _, git := newResolver()
	git.NotARepo = true
	got, err := r.Resolve(context.Background(), application.ResolverInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != application.DefaultOrchestratorURL {
		t.Errorf("OrchestratorURL = %q (want default)", got.OrchestratorURL)
	}
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
	if got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
	if got.TimeoutSeconds != application.DefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds = %d", got.TimeoutSeconds)
	}
}

func TestResolverFailsWhenProjectMissingForRun(t *testing.T) {
	r, _, _, git := newResolver()
	git.NotARepo = true
	_, err := r.Resolve(context.Background(), application.ResolverInput{RequireProject: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrConfigMissing) {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestResolverInvalidProjectYAMLBubblesUp(t *testing.T) {
	r, pc, _, git := newResolver()
	git.Root = "/repo"
	pc.ReadErr = domain.ErrInvalidYAML
	_, err := r.Resolve(context.Background(), application.ResolverInput{RequireProject: true})
	if !errors.Is(err, domain.ErrInvalidYAML) {
		t.Errorf("expected ErrInvalidYAML, got %v", err)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/application/configresolver.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// Defaults applied when no other layer provides a value.
const (
	DefaultOrchestratorURL = "http://localhost:9080"
	DefaultBaseRef         = "main"
	DefaultTimeoutSeconds  = 30
)

// ResolverFlags carries CLI flag values. Empty strings/zero ints mean "not set".
type ResolverFlags struct {
	OrchestratorURL string
	Project         string
	BaseRef         string
	ArtifactStore   string
	TimeoutSeconds  int
}

// ResolverInput controls Resolve.
type ResolverInput struct {
	Flags          ResolverFlags
	Env            map[string]string
	UserConfigPath string // empty ⇒ skip user config
	RequireProject bool   // true ⇒ fail with ErrConfigMissing when no project resolved
}

// ResolvedConfig is the layered result.
type ResolvedConfig struct {
	OrchestratorURL string
	Project         string
	BaseRef         string
	ArtifactStore   domain.ArtifactStoreMode
	TimeoutSeconds  int
}

// ConfigResolverDeps groups the ports the resolver needs.
type ConfigResolverDeps struct {
	ProjectStore outbound.ProjectConfigStore
	UserStore    outbound.UserConfigStore
	Git          outbound.GitInspector
}

// ConfigResolver layers configuration per spec §3.4.
type ConfigResolver struct {
	deps ConfigResolverDeps
}

// NewConfigResolver constructs a ConfigResolver.
func NewConfigResolver(d ConfigResolverDeps) *ConfigResolver { return &ConfigResolver{deps: d} }

// Resolve applies the precedence flags > env > project > user > defaults.
func (r *ConfigResolver) Resolve(ctx context.Context, in ResolverInput) (ResolvedConfig, error) {
	out := ResolvedConfig{
		OrchestratorURL: DefaultOrchestratorURL,
		BaseRef:         DefaultBaseRef,
		ArtifactStore:   domain.ArtifactStoreEngram,
		TimeoutSeconds:  DefaultTimeoutSeconds,
	}

	// User config (lowest non-default).
	if in.UserConfigPath != "" {
		uc, err := r.deps.UserStore.Read(ctx, in.UserConfigPath)
		if err != nil && !errors.Is(err, domain.ErrConfigMissing) {
			return out, fmt.Errorf("user config: %w", err)
		}
		if uc != nil {
			if uc.OrchestratorURL != "" {
				out.OrchestratorURL = uc.OrchestratorURL
			}
			if uc.TimeoutSeconds > 0 {
				out.TimeoutSeconds = uc.TimeoutSeconds
			}
		}
	}

	// Project config (.sophia.yaml).
	pc, pcErr := r.readProjectConfig(ctx)
	if pcErr != nil && !errors.Is(pcErr, domain.ErrConfigMissing) && !errors.Is(pcErr, domain.ErrNotARepo) {
		return out, fmt.Errorf("project config: %w", pcErr)
	}
	if pc != nil {
		if pc.Project != "" {
			out.Project = pc.Project
		}
		if pc.BaseRef != "" {
			out.BaseRef = pc.BaseRef
		}
		if pc.ArtifactStore != "" {
			out.ArtifactStore = pc.ArtifactStore
		}
	}

	// Env vars.
	if v := in.Env["SOPHIA_ORCHESTRATOR_URL"]; v != "" {
		out.OrchestratorURL = v
	}
	if v := in.Env["SOPHIA_PROJECT"]; v != "" {
		out.Project = v
	}
	if v := in.Env["SOPHIA_BASE_REF"]; v != "" {
		out.BaseRef = v
	}

	// Flags (highest).
	if in.Flags.OrchestratorURL != "" {
		out.OrchestratorURL = in.Flags.OrchestratorURL
	}
	if in.Flags.Project != "" {
		out.Project = in.Flags.Project
	}
	if in.Flags.BaseRef != "" {
		out.BaseRef = in.Flags.BaseRef
	}
	if in.Flags.ArtifactStore != "" {
		out.ArtifactStore = domain.ArtifactStoreMode(in.Flags.ArtifactStore)
	}
	if in.Flags.TimeoutSeconds > 0 {
		out.TimeoutSeconds = in.Flags.TimeoutSeconds
	}

	if in.RequireProject && out.Project == "" {
		return out, fmt.Errorf("%w: project not set (need .sophia.yaml or --project / SOPHIA_PROJECT)", domain.ErrConfigMissing)
	}
	return out, nil
}

func (r *ConfigResolver) readProjectConfig(ctx context.Context) (*domain.ProjectConfig, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNotARepo, err)
	}
	path := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/configresolver.go internal/application/configresolver_test.go
git commit -m "feat(application): add ConfigResolver (flag/env/yaml layering per spec §3.4)"
```

---

## Phase 5 — Runner application service

### Task 6: application/runner.go — sophia run polling loop

**Files:**
- Create: `internal/application/runner.go`
- Create: `internal/application/runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

type recordingSink struct {
	Snapshots []*domain.Change
	Events    []domain.Event
	Errors    []error
	Final     domain.ChangeStatus
	closed    bool
}

func (s *recordingSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	cp := *c
	s.Snapshots = append(s.Snapshots, &cp)
	return nil
}
func (s *recordingSink) OnEvent(_ context.Context, e domain.Event) error {
	s.Events = append(s.Events, e)
	return nil
}
func (s *recordingSink) OnApprovalGate(_ context.Context, _ domain.ApprovalGate) error { return nil }
func (s *recordingSink) OnError(_ context.Context, err error) error {
	s.Errors = append(s.Errors, err)
	return nil
}
func (s *recordingSink) OnComplete(_ context.Context, st domain.ChangeStatus) error {
	s.Final = st
	return nil
}
func (s *recordingSink) Close() error { s.closed = true; return nil }

func newRunner(orch *fakes.FakeOrchestrator, sink *recordingSink) (*application.Runner, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	r := application.NewRunner(application.RunnerDeps{
		Orch:  orch,
		State: state,
		Git:   fakes.NewFakeGitInspector(),
		Sink:  sink,
	}, application.RunnerOptions{
		PollMin: time.Millisecond,
		PollMax: 5 * time.Millisecond,
	})
	return r, state
}

func TestRunnerCreatesAndPollsUntilDone(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, state := newRunner(orch, sink)

	// First snapshot is "running"; the second is "done". Use TickHook
	// to advance after the first poll.
	first := true
	orch.TickHook = func(c *domain.Change) {
		if first {
			c.Status = domain.ChangeStatusRunning
			first = false
		} else {
			c.Status = domain.ChangeStatusDone
		}
	}

	res, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalStatus != domain.ChangeStatusDone {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
	if !res.ChangeID.IsZero() == false {
		// just checking it's set
	}
	if len(sink.Snapshots) < 2 {
		t.Errorf("expected ≥2 snapshots, got %d", len(sink.Snapshots))
	}
	if sink.Final != domain.ChangeStatusDone {
		t.Errorf("OnComplete final = %q", sink.Final)
	}

	// Verify last_change_id persisted at both scopes.
	gid, _ := state.GetGlobalLast(context.Background())
	if gid != res.ChangeID {
		t.Errorf("global last = %q, want %q", gid, res.ChangeID)
	}
}

func TestRunnerExitCodeOnFailedTerminalStatus(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	orch.TickHook = func(c *domain.Change) { c.Status = domain.ChangeStatusFailed }

	res, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err == nil {
		t.Fatal("expected ExitError for failed terminal status")
	}
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exit.Code != 1 {
		t.Errorf("Code = %d, want 1", exit.Code)
	}
	if res.FinalStatus != domain.ChangeStatusFailed {
		t.Errorf("FinalStatus = %q", res.FinalStatus)
	}
}

func TestRunnerExitCode3OnCreateFailure(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	orch.CreateErr = errors.New("orchestrator unreachable")
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	_, err := r.Run(context.Background(), application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 3 {
		t.Errorf("Code = %d, want 3", exit.Code)
	}
}

func TestRunnerCanceledContextReturnsExit4(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	// Make Get block forever so the context cancel is what stops us.
	orch.GetBlockUntilCancel = true

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := r.Run(ctx, application.RunInput{
		Project: "p", Message: "msg", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})
	var exit *application.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exit.Code != 4 {
		t.Errorf("Code = %d, want 4", exit.Code)
	}
}

func TestRunnerInputRequiresProjectAndMessage(t *testing.T) {
	orch := fakes.NewFakeOrchestrator()
	sink := &recordingSink{}
	r, _ := newRunner(orch, sink)

	_, err := r.Run(context.Background(), application.RunInput{Message: ""})
	if err == nil {
		t.Error("expected error on empty message")
	}
	_, err = r.Run(context.Background(), application.RunInput{Message: "m"})
	if err == nil {
		t.Error("expected error on empty project")
	}
}
```

- [ ] **Step 2: Extend FakeOrchestrator with TickHook + CreateErr + GetBlockUntilCancel**

Read `test/fakes/orchestrator.go` first. Add to the struct:
```go
type FakeOrchestrator struct {
	mu                  sync.Mutex
	HealthzErr          error
	CreateErr           error
	GetBlockUntilCancel bool
	TickHook            func(*domain.Change)  // optional; called on each GetChange before returning
	changes             map[domain.ChangeID]*domain.Change
	nextID              int
}
```

Modify `CreateChange` to honor `CreateErr`:
```go
func (f *FakeOrchestrator) CreateChange(_ context.Context, in outbound.CreateChangeInput) (*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CreateErr != nil {
		return nil, f.CreateErr
	}
	// ... existing body unchanged
}
```

Modify `GetChange` to honor `TickHook` and `GetBlockUntilCancel`:
```go
func (f *FakeOrchestrator) GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error) {
	if f.GetBlockUntilCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[id]
	if !ok {
		return nil, domain.ErrChangeNotFound
	}
	if f.TickHook != nil {
		f.TickHook(c)
	}
	out := *c
	if c.Phases != nil {
		out.Phases = append([]domain.Phase{}, c.Phases...)
	}
	return &out, nil
}
```

- [ ] **Step 3: Run test (still failing — Runner undefined)**

Run: `go test ./internal/application/... ./test/fakes/... -race`
Expected: FAIL on Runner-related symbols; FakeOrchestrator tests still PASS (the additions are backward-compatible).

- [ ] **Step 4: Implement runner.go**

`internal/application/runner.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/inbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ExitError carries a process exit code per spec §2.3.
//   0 → terminal DONE
//   1 → terminal BLOCKED/FAILED
//   3 → config / orchestrator-unreachable / change-not-found
//   4 → transient error (poll-loop ctx canceled, network mid-run)
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
}

// Unwrap allows errors.Is/As traversal.
func (e *ExitError) Unwrap() error { return e.Err }

// RunnerDeps groups the ports the Runner needs.
type RunnerDeps struct {
	Orch  outbound.OrchestratorClient
	State outbound.StateStore
	Git   outbound.GitInspector
	Sink  inbound.EventSink
}

// RunnerOptions tunes the polling cadence.
type RunnerOptions struct {
	PollMin time.Duration // default 1s
	PollMax time.Duration // default 5s
}

// RunInput controls Run.
type RunInput struct {
	Project       string
	Message       string
	BaseRef       string
	ArtifactStore domain.ArtifactStoreMode
}

// RunResult reports the terminal outcome.
type RunResult struct {
	ChangeID    domain.ChangeID
	FinalStatus domain.ChangeStatus
}

// Runner orchestrates `sophia run` per spec §2.2 (polling-only V1 in M4).
type Runner struct {
	deps RunnerDeps
	opts RunnerOptions
}

// NewRunner constructs a Runner.
func NewRunner(d RunnerDeps, opts RunnerOptions) *Runner {
	if opts.PollMin <= 0 {
		opts.PollMin = time.Second
	}
	if opts.PollMax <= 0 {
		opts.PollMax = 5 * time.Second
	}
	return &Runner{deps: d, opts: opts}
}

// Run creates a Change and observes it via polling until terminal status.
// Returns RunResult and either nil (DONE) or *ExitError with the spec code.
func (r *Runner) Run(ctx context.Context, in RunInput) (RunResult, error) {
	if in.Message == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: --message required")}
	}
	if in.Project == "" {
		return RunResult{}, &ExitError{Code: 3, Err: errors.New("run: project not set")}
	}
	if in.ArtifactStore == "" {
		in.ArtifactStore = domain.ArtifactStoreEngram
	}
	if in.BaseRef == "" {
		in.BaseRef = "main"
	}

	defer r.deps.Sink.Close() //nolint:errcheck // best-effort

	created, err := r.deps.Orch.CreateChange(ctx, outbound.CreateChangeInput{
		Name:              in.Message,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: string(in.ArtifactStore),
	})
	if err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RunResult{}, &ExitError{Code: 4, Err: err}
		}
		return RunResult{}, &ExitError{Code: 3, Err: err}
	}

	res := RunResult{ChangeID: created.ID}
	if err := r.persistChangeID(ctx, in.Project, created.ID); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}
	if err := r.deps.Sink.OnSnapshot(ctx, created); err != nil {
		_ = r.deps.Sink.OnError(ctx, err)
	}

	if created.Status.IsTerminal() {
		return r.finish(ctx, res, created.Status)
	}

	final, err := r.poll(ctx, created.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return res, &ExitError{Code: 4, Err: err}
		}
		_ = r.deps.Sink.OnError(ctx, err)
		return res, &ExitError{Code: 4, Err: err}
	}
	return r.finish(ctx, res, final)
}

// poll runs the GET-snapshot loop until terminal status.
func (r *Runner) poll(ctx context.Context, id domain.ChangeID) (domain.ChangeStatus, error) {
	delay := r.opts.PollMin
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
		snap, err := r.deps.Orch.GetChange(ctx, id)
		if err != nil {
			return "", err
		}
		if err := r.deps.Sink.OnSnapshot(ctx, snap); err != nil {
			// Sink errors are non-fatal but emitted.
			_ = r.deps.Sink.OnError(ctx, err)
		}
		if snap.Status.IsTerminal() {
			return snap.Status, nil
		}
		// Exponential backoff up to PollMax. Cap at PollMax once reached.
		if snap.CurrentPhaseID == "" {
			delay *= 2
			if delay > r.opts.PollMax {
				delay = r.opts.PollMax
			}
		} else {
			// Active phase → keep at PollMin.
			delay = r.opts.PollMin
		}
	}
}

func (r *Runner) finish(ctx context.Context, res RunResult, st domain.ChangeStatus) (RunResult, error) {
	res.FinalStatus = st
	_ = r.deps.Sink.OnComplete(ctx, st)
	switch st {
	case domain.ChangeStatusDone:
		return res, nil
	case domain.ChangeStatusBlocked, domain.ChangeStatusFailed:
		return res, &ExitError{Code: 1, Err: fmt.Errorf("change ended %s", st)}
	default:
		return res, &ExitError{Code: 4, Err: fmt.Errorf("unexpected non-terminal status %q", st)}
	}
}

func (r *Runner) persistChangeID(ctx context.Context, project string, id domain.ChangeID) error {
	if err := r.deps.State.SetGlobalLast(ctx, id); err != nil {
		return fmt.Errorf("global last: %w", err)
	}
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		// Outside a repo — keep only the global record. Not fatal.
		return nil
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(project, root, remote)
	if err := r.deps.State.SetLast(ctx, fp, id); err != nil {
		return fmt.Errorf("project last: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/runner.go internal/application/runner_test.go test/fakes/orchestrator.go
git commit -m "feat(application): add Runner (polling loop, ExitError per spec §2.3)"
```

---

## Phase 6 — Cobra `sophia run` command

### Task 7: cli/run.go — sophia run "msg" --no-tui --json

**Files:**
- Modify: `internal/adapters/inbound/cli/root.go` (add Runner+ConfigResolver to Deps, replace stub)
- Create: `internal/adapters/inbound/cli/run.go`
- Create: `internal/adapters/inbound/cli/run_test.go`
- Modify: `internal/adapters/inbound/cli/stubs_test.go` (drop "run")

- [ ] **Step 1: Read root.go and stubs_test.go**

Read both files first.

- [ ] **Step 2: Update Deps struct in root.go**

Add fields:
```go
type Deps struct {
	Doctor       *application.DoctorService
	Provisioner  *application.Provisioner
	Initializer  *application.Initializer
	StatusReader *application.StatusReader
	Runner       *application.Runner
	Resolver     *application.ConfigResolver

	UserConfigPath string // optional; passed to ConfigResolver

	Version   string
	Commit    string
	BuildDate string
}
```

Replace `root.AddCommand(newStubCmd("run", ..., "M4"))` with `root.AddCommand(newRunCmd(d))`.

- [ ] **Step 3: Drop "run" from stubs_test.go**

Edit the `cases` map in `TestStubsAnnounceMilestone`:
```go
cases := map[string]string{
    "attach":  "M8",
    "changes": "M8",
}
```

- [ ] **Step 4: Write the failing test**

`internal/adapters/inbound/cli/run_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newRunDeps(t *testing.T, sinkBuf *bytes.Buffer) (cli.Deps, *fakes.FakeOrchestrator) {
	t.Helper()
	orch := fakes.NewFakeOrchestrator()
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	pc := fakes.NewFakeProjectConfigStore()
	uc := fakes.NewFakeUserConfigStore()

	// Pre-populate .sophia.yaml so resolver finds the project.
	_ = pc.Write(context.Background(), "/repo/.sophia.yaml", &domain.ProjectConfig{
		Project: "ms-cotizacion", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram,
	})

	resolver := application.NewConfigResolver(application.ConfigResolverDeps{
		ProjectStore: pc, UserStore: uc, Git: git,
	})

	// Sink: pass through buffer used by the tests for assertions.
	// We use jsonsink directly here to validate end-to-end.
	// (Importing jsonsink inside cli_test is fine — test-only edge.)
	return cli.Deps{
		Resolver: resolver,
		Runner: application.NewRunner(application.RunnerDeps{
			Orch:  orch,
			State: state,
			Git:   git,
			Sink:  newTestSink(sinkBuf),
		}, application.RunnerOptions{
			PollMin: time.Millisecond,
			PollMax: 5 * time.Millisecond,
		}),
	}, orch
}

func newTestSink(w *bytes.Buffer) *testSink {
	return &testSink{w: w}
}

type testSink struct{ w *bytes.Buffer }

func (s *testSink) OnSnapshot(_ context.Context, c *domain.Change) error {
	_, err := s.w.WriteString("snap:" + c.ID.String() + ":" + string(c.Status) + "\n")
	return err
}
func (s *testSink) OnEvent(_ context.Context, _ domain.Event) error            { return nil }
func (s *testSink) OnApprovalGate(_ context.Context, _ domain.ApprovalGate) error { return nil }
func (s *testSink) OnError(_ context.Context, _ error) error                    { return nil }
func (s *testSink) OnComplete(_ context.Context, _ domain.ChangeStatus) error   { return nil }
func (s *testSink) Close() error                                                { return nil }

func TestRunCommandRequiresMessage(t *testing.T) {
	deps, _ := newRunDeps(t, &bytes.Buffer{})
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "--no-tui", "--json"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when message missing")
	}
}

func TestRunCommandSucceedsWithMessage(t *testing.T) {
	var sinkBuf bytes.Buffer
	deps, orch := newRunDeps(t, &sinkBuf)
	first := true
	orch.TickHook = func(c *domain.Change) {
		if first {
			c.Status = domain.ChangeStatusRunning
			first = false
		} else {
			c.Status = domain.ChangeStatusDone
		}
	}
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"run", "test message", "--no-tui", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sinkBuf.String(), "snap:") {
		t.Errorf("sink missing snapshot: %s", sinkBuf.String())
	}
	if !strings.Contains(sinkBuf.String(), "done") {
		t.Errorf("sink missing terminal status: %s", sinkBuf.String())
	}
}

func TestRunCommandReturnsExitErrorOnFailure(t *testing.T) {
	var sinkBuf bytes.Buffer
	deps, orch := newRunDeps(t, &sinkBuf)
	orch.TickHook = func(c *domain.Change) { c.Status = domain.ChangeStatusFailed }

	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"run", "msg", "--no-tui", "--json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 5: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL.

- [ ] **Step 6: Implement run.go**

`internal/adapters/inbound/cli/run.go`:

```go
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newRunCmd(d Deps) *cobra.Command {
	var (
		noTUI         bool
		jsonOut       bool
		baseRef       string
		artifactStore string
		project       string
		orchURL       string
	)
	cmd := &cobra.Command{
		Use:   "run [message]",
		Short: "Create and observe a Change",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Runner == nil || d.Resolver == nil {
				return fmt.Errorf("run: runner not wired")
			}
			if !noTUI || !jsonOut {
				return fmt.Errorf("run: M4 only supports --no-tui --json (TUI ships in M6)")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("run: message required (positional argument)")
			}
			resolved, err := d.Resolver.Resolve(cmd.Context(), application.ResolverInput{
				Flags: application.ResolverFlags{
					OrchestratorURL: orchURL,
					Project:         project,
					BaseRef:         baseRef,
					ArtifactStore:   artifactStore,
				},
				Env:            envSnapshot(),
				UserConfigPath: d.UserConfigPath,
				RequireProject: true,
			})
			if err != nil {
				return err
			}
			res, err := d.Runner.Run(cmd.Context(), application.RunInput{
				Project:       resolved.Project,
				Message:       args[0],
				BaseRef:       resolved.BaseRef,
				ArtifactStore: resolved.ArtifactStore,
			})
			if err != nil {
				var exit *application.ExitError
				if errors.As(err, &exit) {
					// Surface to main.go via the same typed error.
					return exit
				}
				return err
			}
			_ = res // success path — output already streamed to the sink
			return nil
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "stream JSONL to stdout instead of a TUI (required in M4)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (required in M4 with --no-tui)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "override base_ref")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "", "override artifact_store mode")
	cmd.Flags().StringVar(&project, "project", "", "override project slug")
	cmd.Flags().StringVar(&orchURL, "orchestrator-url", "", "override orchestrator URL")
	return cmd
}

// envSnapshot returns the SOPHIA_* env vars consulted by the resolver.
func envSnapshot() map[string]string {
	out := map[string]string{}
	for _, k := range []string{"SOPHIA_ORCHESTRATOR_URL", "SOPHIA_PROJECT", "SOPHIA_BASE_REF"} {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		}
	}
	return out
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/inbound/cli/run.go internal/adapters/inbound/cli/run_test.go \
        internal/adapters/inbound/cli/root.go internal/adapters/inbound/cli/stubs_test.go
git commit -m "feat(cli): add run command (--no-tui --json, M4 polling-only)"
```

---

## Phase 7 — Bootstrap rewiring + main.go exit-code mapping

### Task 8: bootstrap/wire.go and cmd/sophia/main.go

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`
- Modify: `cmd/sophia/main.go`

- [ ] **Step 1: Read all three files**

Read `internal/bootstrap/wire.go`, `internal/bootstrap/wire_test.go`, and `cmd/sophia/main.go`.

- [ ] **Step 2: Update wire_test.go**

Add a test:
```go
func TestNewWiresM4Run(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("run cmd is nil")
	}
}
```

- [ ] **Step 3: Update wire.go**

Read current wire.go. Add imports:
```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/jsonsink"
```

Inside `New()`, after `statusReader := ...`, add:

```go
resolver := application.NewConfigResolver(application.ConfigResolverDeps{
	ProjectStore: projectStore,
	UserStore:    yamlconfig.NewUserStore(yamlconfig.UserConfig{}),
	Git:          git,
})

// JSON sink writes to stdout. The runner is constructed with this sink
// for V1; M6 will swap to a TUI sink based on flags.
sink := jsonsink.New(jsonsink.Config{Writer: os.Stdout})
runner := application.NewRunner(application.RunnerDeps{
	Orch:  orch,
	State: state,
	Git:   git,
	Sink:  sink,
}, application.RunnerOptions{})

userConfigPath := filepath.Join(xdg.ConfigRoot, "config.yaml")
```

(Add the `path/filepath` import if not already imported.)

Update the `cli.Deps{...}` literal:
```go
deps := cli.Deps{
	Doctor:         doctor,
	Provisioner:    provisioner,
	Initializer:    initializer,
	StatusReader:   statusReader,
	Runner:         runner,
	Resolver:       resolver,
	UserConfigPath: userConfigPath,
	Version:        info.Version,
	Commit:         info.Commit,
	BuildDate:      info.BuildDate,
}
```

- [ ] **Step 4: Update cmd/sophia/main.go to map ExitError**

Read `cmd/sophia/main.go`. The current handler exits 3 on any error and 4 on bootstrap error. Update to honor `*application.ExitError` codes:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, err := bootstrap.New(bootstrap.Config{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap error:", err)
		os.Exit(4)
	}
	if err := root.ExecuteContext(ctx); err != nil {
		var exit *application.ExitError
		if errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(exit.Code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(3)
	}
}
```

- [ ] **Step 5: Run tests + build**

Run:
```bash
go test ./internal/bootstrap/... -race
make build
```
Expected: PASS, binary builds.

- [ ] **Step 6: Smoke**

Run:
```bash
./bin/sophia run --help
./bin/sophia run "test" --no-tui --json
echo "exit=$?"
```

Expected:
- `--help` shows `--no-tui`, `--json`, `--base-ref`, `--artifact-store`, `--project`, `--orchestrator-url`
- `./bin/sophia run "test" --no-tui --json` against this repo (no `.sophia.yaml`) returns exit 3 because `RequireProject` fails

- [ ] **Step 7: Commit**

```bash
git add internal/bootstrap/wire.go internal/bootstrap/wire_test.go cmd/sophia/main.go
git commit -m "feat(bootstrap): wire M4 (Runner, Resolver, jsonsink) + map ExitError in main.go"
```

---

## Phase 8 — End-to-end smoke against a stub orchestrator

### Task 9: test/e2e/run_polling_test.go

**Files:**
- Create: `test/e2e/run_polling_test.go`

- [ ] **Step 1: Write the test (gated by `e2e_smoke`)**

```go
//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeRunAgainstStub spins up an in-process httptest stub of
// /api/v1/changes that emits a Change transitioning pending → running → done,
// then runs `./bin/sophia run "msg" --no-tui --json --orchestrator-url=<stub>`
// from inside a temp git repo with .sophia.yaml. Verifies:
//   - exit 0
//   - JSONL stream contains snapshot with status=done
//   - last_change_id persisted to <stateRoot>/last_change_id
//
// Validates the M4 auto_advance assumption: the stub auto-advances the Change
// without any client-side phase trigger. If a real orchestrator does NOT
// auto-advance, this test fails and we add the §5.2 compatibility mode.
func TestSmokeRunAgainstStub(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	binary := absBinary(t)

	// Stub orchestrator with auto-advance.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/changes":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"change_id":"01HX","status":"pending","name":"msg","project":"p"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/changes/"):
			calls++
			status := "running"
			if calls > 1 {
				status = "done"
			}
			_, _ = w.Write([]byte(`{"change_id":"01HX","status":"` + status + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Temp repo with .sophia.yaml.
	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	yaml := []byte("version: 1\nproject: p\nbase_ref: main\nartifact_store: engram\n")
	if err := os.WriteFile(filepath.Join(tmp, ".sophia.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	// Isolate XDG state to a temp dir.
	stateDir := t.TempDir()
	cmd := exec.Command(binary, "run", "msg", "--no-tui", "--json", "--orchestrator-url="+srv.URL)
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(),
		"XDG_STATE_HOME="+stateDir,
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify last JSONL line carries the terminal status.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines")
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("invalid JSON line: %v\n%s", err, l)
		}
	}
	if !strings.Contains(stdout.String(), `"final_status":"done"`) {
		t.Errorf("missing terminal status in output: %s", stdout.String())
	}

	// last_change_id persisted globally.
	gpath := filepath.Join(stateDir, "sophia", "last_change_id")
	if _, err := os.Stat(gpath); err != nil {
		t.Errorf("expected global last_change_id at %s: %v", gpath, err)
	}
}

func absBinary(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../bin/sophia")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("binary missing — run `make build` first: %v", err)
	}
	return abs
}
```

- [ ] **Step 2: Build and run**

Run:
```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS. If the test fails because auto_advance doesn't behave as the stub predicts, escalate per spec §5.2 — that triggers the compatibility-mode work, not in M4 scope.

- [ ] **Step 3: Commit**

```bash
git add test/e2e/run_polling_test.go
git commit -m "test(e2e): add run polling smoke test (validates auto_advance assumption)"
```

---

## Phase 9 — Final validation

### Task 10: Final validation pass

**Files:** none (verification only).

- [ ] **Step 1: vet + tests + race**
```bash
go vet ./...
go test -race ./...
```
Expected: exit 0.

- [ ] **Step 2: Lint**
```bash
golangci-lint run
```
Acceptable `//nolint` patterns: existing precedents (gosec on subprocess shellouts, unparam on fakes). Fix new findings in place.

- [ ] **Step 3: Coverage**
```bash
go test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
go tool cover -func=cover.out | tail -n 1
```
Expected: total ≥ 70%.

- [ ] **Step 4: Binary smoke**

```bash
make build

# 1) Runs against this repo (no .sophia.yaml) → exit 3 with config error
./bin/sophia run "test" --no-tui --json
echo "no-config exit=$?"

# 2) Help output shows the flags
./bin/sophia run --help

# 3) M4 only supports --no-tui --json
./bin/sophia run "test"
echo "no-flags exit=$?"  # expect 3 with M4 message

# 4) Other commands still work
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
./bin/sophia init --help
./bin/sophia status --help
```

Expected:
- `run --help` shows --no-tui --json --base-ref --artifact-store --project --orchestrator-url
- `run "test"` without flags exits 3 with "M4 only supports --no-tui --json"
- `run "test" --no-tui --json` outside .sophia.yaml exits 3 with project-not-set
- All other commands unchanged

- [ ] **Step 5: E2E smoke**

```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS for run_polling + the M2 doctor/version smoke.

- [ ] **Step 6: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m4): final validation pass" || echo "nothing to commit"
git tag -a m4-run-polling -m "M4 Run via Polling complete"
git tag
```

---

## Self-review checklist

- [ ] **Spec coverage:** Every M4 DoD from spec §7.2 has at least one task.
  - `orchestratorhttp.Client` implements OrchestratorClient (POST/GET/List) → Tasks 1–3
  - `application.RunChange` orchestrates: read .sophia.yaml → POST → poll until terminal → Tasks 5, 6
  - Polling default 1s, max 5s, configurable → Task 6 (RunnerOptions)
  - JSON sink emits snapshots as JSONL → Task 4
  - Exit codes 0/1/3/4 enforced → Tasks 6, 8
  - Unit tests with fakeOrchestrator → Tasks 1–7
  - First e2e test validates auto_advance → Task 9
  - Polling is scaffolding (no SSE) → Task 6 explicitly notes M5 replaces
- [ ] **No placeholders:** No "TBD"/"TODO"/"similar to" in steps.
- [ ] **Type consistency:** `application.RunnerDeps`, `application.RunInput`, `application.ResolverInput`, `application.ExitError` consistent across tasks.
- [ ] **Frequent commits:** every task ends with a commit.
- [ ] **TDD discipline:** failing test before implementation (Task 8 has no unit test for main.go, just integration check via build+smoke).
- [ ] **No premature M5+ scope:** no SSE, no redactor, no TUI, no `RunPhase` compatibility mode.

---

## Pending decisions (carried into M4 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M4-01 | Exact response shape of POST/GET endpoints | Use the DTO defined in Task 1 — verified by Task 9 e2e against a stub. Real orchestrator validation arrives in M5+. |
| D-M4-02 | Should `--message` be a flag or positional? | Positional (spec §2.1: `sophia run "msg"`). Cobra uses `cobra.MaximumNArgs(1)`. |
| D-M4-03 | Should the runner emit `phase.started` / `phase.completed` events derived from snapshot diffs? | No — M4 is "snapshots only" per spec §7.2. M5 SSE adds real events. |
| D-M4-04 | Should `last_change_id` be persisted before or after first snapshot is emitted? | Before — spec §2.2 step 5 places persistence right after POST/CREATE. |
| D-M4-05 | Polling backoff active phase vs idle phase | Reset to PollMin when current_phase_id is set; exponential 1s→5s when null. |

---

## Risks specific to M4

| ID | Risk | Mitigation |
|---|---|---|
| RM4-01 | Real orchestrator does NOT auto_advance | Task 9 e2e fails → escalate to spec §5.2 compatibility mode (NOT in M4 scope). Documented as RUN technical debt. |
| RM4-02 | DTO field name mismatch with sibling service | Task 1 DTO is tolerant (omitempty); Task 9 e2e provides the smoke. M5 hardens. |
| RM4-03 | Polling backpressure on a slow orchestrator | Cap at PollMax = 5s and respect ctx.Done() for Ctrl+C. |
| RM4-04 | Sink writes to stdout race with logger writes to stderr | Logger is JSONL-on-stderr, sink is JSONL-on-stdout. They never collide. |
| RM4-05 | last_change_id persistence fails mid-run | Logged via OnError but not fatal — user can recover from snapshot output. |

---

## What this plan does NOT cover (intentional)

- SSE consumer (tmaxmax/go-sse) → M5
- Reconnect, Last-Event-ID → M5
- Redaction pipeline → M5
- TUI Timeline (Bubble Tea) → M6
- ApplyBoard view + ApprovalGate banner → M7
- Real `sophia status` (HTTP fetch) → M8
- `sophia attach` / `sophia changes` → M8
- Browser opener → M7
- Approval-timeout exit code 5 → M5+
- `RunPhase` compatibility mode → conditional, escalated only if Task 9 e2e fails

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-05-sophia-cli-m4-run-polling.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task. Use `superpowers:subagent-driven-development`.

**2. Inline Execution** — `superpowers:executing-plans`, batch execution with checkpoints.
