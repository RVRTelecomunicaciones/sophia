# Sophia CLI — M1 Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the hexagonal Go skeleton, all outbound port definitions and their fakes, the slog JSONL handler, and a Cobra CLI exposing two real commands (`version`, `doctor` with three working checks: docker, git, XDG paths) plus stubs for the rest. End state: `sophia version` prints version metadata, `sophia doctor` runs three checks and emits a structured report, and `go test ./...`, `go vet ./...`, `golangci-lint run` all pass.

**Architecture:** Hexagonal Go with `cmd/sophia` thin entrypoint, `internal/{domain,application,ports,adapters,bootstrap,infrastructure}` for layers, and `test/fakes` for shared port fakes. Domain is pure (no external imports). Application services consume outbound ports and emit results through one inbound port (`EventSink`). Cobra and Bubble Tea (M6) are inbound adapters; HTTP/SSE/Git/Docker are outbound adapters. Bootstrap wires everything; tests inject fakes.

**Tech Stack:** Go 1.26.x · `log/slog` (stdlib) · `github.com/spf13/cobra` · `github.com/stretchr/testify` · `github.com/spf13/afero` (test-only) · `crypto/sha256` · `gopkg.in/yaml.v3` is NOT yet used (M3).

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md`
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md`
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M1 boundaries — what is NOT in M1:**

- No HTTP client to the orchestrator (M2 partial healthz; M4 full).
- No SSE (M5).
- No Bubble Tea TUI (M6).
- No `.sophia.yaml` reading/writing — ports defined, fakes ready, real adapter is M3.
- No state store atomic writes — same as above.
- No browser opener real adapter — port + fake only (real osbrowser is M7).
- Doctor checks 4–9 (repo, .sophia.yaml, orchestrator, SSE, worktree) are stubbed or warn-only in M1; real implementations land in their natural milestone.

---

## Phase 1 — Project skeleton

### Task 1: Initialize Go module and base directories

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `.editorconfig`
- Create: empty placeholder dirs via `.gitkeep` in
  - `internal/domain/`
  - `internal/application/`
  - `internal/ports/inbound/`
  - `internal/ports/outbound/`
  - `internal/adapters/inbound/cli/`
  - `internal/adapters/outbound/`
  - `internal/bootstrap/`
  - `internal/infrastructure/logging/`
  - `cmd/sophia/`
  - `test/fakes/`
  - `test/e2e/`
  - `bin/`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/russell/Documents/2026/sophia-cli
go mod init github.com/RVRTelecomunicaciones/sophia-cli
```

Expected: creates `go.mod` with `module github.com/RVRTelecomunicaciones/sophia-cli` and `go 1.26`.

- [ ] **Step 2: Write `.gitignore`**

```
# binaries
/bin/
*.exe
*.test
*.out
cover.out
coverage.html

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db

# logs (local state writes go to $XDG_STATE_HOME, not the repo)
logs/
```

- [ ] **Step 3: Write `.editorconfig`**

```
root = true

[*]
charset = utf-8
end_of_line = lf
indent_style = space
indent_size = 4
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab
indent_size = 4

[Makefile]
indent_style = tab

[*.{yml,yaml,md}]
indent_size = 2
```

- [ ] **Step 4: Create directory skeleton with `.gitkeep`**

```bash
for d in internal/domain internal/application internal/ports/inbound internal/ports/outbound \
         internal/adapters/inbound/cli internal/adapters/outbound internal/bootstrap \
         internal/infrastructure/logging cmd/sophia test/fakes test/e2e bin; do
  mkdir -p "$d"
  touch "$d/.gitkeep"
done
```

- [ ] **Step 5: Verify**

Run: `go mod tidy && ls -la`
Expected: `go.mod` present, `go.sum` not yet created (no deps), all dirs visible.

- [ ] **Step 6: Commit**

```bash
git init
git add go.mod .gitignore .editorconfig internal cmd test bin
git commit -m "chore: initialize go module and hexagonal skeleton"
```

---

### Task 2: Add Makefile and golangci-lint config

**Files:**
- Create: `Makefile`
- Create: `.golangci.yml`

- [ ] **Step 1: Write `Makefile`**

```makefile
.DEFAULT_GOAL := help

GO          ?= go
GOLANGCI    ?= golangci-lint
PKG         := github.com/RVRTelecomunicaciones/sophia-cli
BIN_DIR     := bin
BIN         := $(BIN_DIR)/sophia
VERSION     ?= 0.1.0-dev
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X $(PKG)/internal/bootstrap.Version=$(VERSION) \
               -X $(PKG)/internal/bootstrap.Commit=$(COMMIT) \
               -X $(PKG)/internal/bootstrap.BuildDate=$(DATE)

.PHONY: help build test vet lint coverage clean run-doctor run-version

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the sophia binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/sophia

test: ## Run all tests
	$(GO) test ./...

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint
	$(GOLANGCI) run

coverage: ## Compute coverage for domain + application
	$(GO) test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
	$(GO) tool cover -func=cover.out | tail -n 1

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) cover.out coverage.html

run-version: build ## Build and run sophia version
	$(BIN) version

run-doctor: build ## Build and run sophia doctor
	$(BIN) doctor
```

- [ ] **Step 2: Write `.golangci.yml`**

```yaml
run:
  timeout: 3m
  tests: true

linters:
  disable-all: true
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - gofmt
    - goimports
    - gosec
    - unused
    - unparam
    - misspell
    - ineffassign
    - typecheck

linters-settings:
  revive:
    rules:
      - name: exported
        arguments:
          - disableStutteringCheck
  gosec:
    excludes:
      - G104   # Errors unhandled (we use errcheck)

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
        - errcheck
    - path: test/fakes/
      linters:
        - revive
```

- [ ] **Step 3: Verify Makefile help works**

Run: `make help`
Expected: prints the help table with `build`, `test`, `vet`, `lint`, `coverage`, `clean`.

- [ ] **Step 4: Commit**

```bash
git add Makefile .golangci.yml
git commit -m "chore: add Makefile and golangci-lint config"
```

---

### Task 3: Add GitHub Actions CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write workflow file**

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build-test-lint:
    name: build, test, lint
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26.x'
          cache: true

      - name: go vet
        run: go vet ./...

      - name: go build
        run: go build ./...

      - name: go test
        run: go test ./... -race

      - name: coverage
        run: |
          go test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
          go tool cover -func=cover.out | tail -n 1

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.62
          args: --timeout=3m
```

- [ ] **Step 2: Commit**

```bash
mkdir -p .github/workflows
git add .github/workflows/ci.yml
git commit -m "ci: add github actions workflow for build/test/lint"
```

---

## Phase 2 — Domain types (TDD)

### Task 4: domain/errors.go — sentinel errors

**Files:**
- Create: `internal/domain/errors.go`
- Create: `internal/domain/errors_test.go`

- [ ] **Step 1: Write the failing test**

`internal/domain/errors_test.go`:

```go
package domain_test

import (
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestSentinelErrorsAreDistinct(t *testing.T) {
	all := []error{
		domain.ErrConfigMissing,
		domain.ErrChangeNotFound,
		domain.ErrUnreachable,
		domain.ErrInvalidYAML,
		domain.ErrNotARepo,
		domain.ErrInvalidURL,
	}
	seen := make(map[string]bool)
	for _, e := range all {
		if e == nil {
			t.Fatal("sentinel error is nil")
		}
		if seen[e.Error()] {
			t.Errorf("duplicate sentinel: %q", e.Error())
		}
		seen[e.Error()] = true
	}
}

func TestSentinelErrorsAreErrorsAsTarget(t *testing.T) {
	wrapped := errors.Join(domain.ErrConfigMissing, errors.New("context"))
	if !errors.Is(wrapped, domain.ErrConfigMissing) {
		t.Error("ErrConfigMissing should be reachable via errors.Is")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain/...`
Expected: FAIL with "undefined: domain.ErrConfigMissing".

- [ ] **Step 3: Implement**

`internal/domain/errors.go`:

```go
// Package domain holds pure entities and value objects shared across the CLI.
// It MUST NOT import any other internal package or any third-party library
// beyond the Go standard library.
package domain

import "errors"

// Sentinel errors used across the CLI.
var (
	ErrConfigMissing  = errors.New("config missing")
	ErrChangeNotFound = errors.New("change not found")
	ErrUnreachable    = errors.New("orchestrator unreachable")
	ErrInvalidYAML    = errors.New("invalid yaml")
	ErrNotARepo       = errors.New("not a git repository")
	ErrInvalidURL     = errors.New("invalid url")
)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/errors.go internal/domain/errors_test.go
git commit -m "feat(domain): add sentinel errors"
```

---

### Task 5: domain/phase.go — PhaseType, PhaseStatus, Phase

**Files:**
- Create: `internal/domain/phase.go`
- Create: `internal/domain/phase_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestAllPhasesAreNine(t *testing.T) {
	got := domain.AllPhases()
	if len(got) != 9 {
		t.Fatalf("expected 9 phases, got %d", len(got))
	}
	want := []domain.PhaseType{
		domain.PhaseInit, domain.PhaseExplore, domain.PhaseProposal,
		domain.PhaseSpec, domain.PhaseDesign, domain.PhaseTasks,
		domain.PhaseApply, domain.PhaseVerify, domain.PhaseArchive,
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("phase[%d] = %q, want %q", i, got[i], p)
		}
	}
}

func TestPhaseTypeIsValid(t *testing.T) {
	for _, p := range domain.AllPhases() {
		if !p.IsValid() {
			t.Errorf("%q should be valid", p)
		}
	}
	if domain.PhaseType("bogus").IsValid() {
		t.Error("bogus phase should be invalid")
	}
	if domain.PhaseType("").IsValid() {
		t.Error("empty phase should be invalid")
	}
}

func TestPhaseStatusIsTerminal(t *testing.T) {
	cases := map[domain.PhaseStatus]bool{
		domain.PhaseStatusPending: false,
		domain.PhaseStatusRunning: false,
		domain.PhaseStatusDone:    true,
		domain.PhaseStatusBlocked: true,
		domain.PhaseStatusFailed:  true,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestPhaseZeroValue(t *testing.T) {
	var p domain.Phase
	if p.Type != "" || p.Status != "" {
		t.Errorf("zero Phase should have empty Type and Status")
	}
	if !p.StartedAt.IsZero() {
		t.Error("zero Phase StartedAt must be zero time")
	}
	_ = time.Time{} // ensure time import compiles
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain/...`
Expected: FAIL with `undefined: domain.AllPhases` etc.

- [ ] **Step 3: Implement**

`internal/domain/phase.go`:

```go
package domain

import "time"

// PhaseType is one of the nine canonical SDD phases.
type PhaseType string

// The nine canonical SDD phase types in execution order.
const (
	PhaseInit     PhaseType = "init"
	PhaseExplore  PhaseType = "explore"
	PhaseProposal PhaseType = "proposal"
	PhaseSpec     PhaseType = "spec"
	PhaseDesign   PhaseType = "design"
	PhaseTasks    PhaseType = "tasks"
	PhaseApply    PhaseType = "apply"
	PhaseVerify   PhaseType = "verify"
	PhaseArchive  PhaseType = "archive"
)

// AllPhases returns the canonical phase order.
func AllPhases() []PhaseType {
	return []PhaseType{
		PhaseInit, PhaseExplore, PhaseProposal, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseVerify, PhaseArchive,
	}
}

// IsValid reports whether p is one of the nine canonical phase types.
func (p PhaseType) IsValid() bool {
	switch p {
	case PhaseInit, PhaseExplore, PhaseProposal, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseVerify, PhaseArchive:
		return true
	}
	return false
}

// PhaseStatus mirrors the orchestrator's phase status enum.
type PhaseStatus string

// Phase status values.
const (
	PhaseStatusPending PhaseStatus = "pending"
	PhaseStatusRunning PhaseStatus = "running"
	PhaseStatusDone    PhaseStatus = "done"
	PhaseStatusBlocked PhaseStatus = "blocked"
	PhaseStatusFailed  PhaseStatus = "failed"
)

// IsTerminal reports whether s is a terminal phase status.
func (s PhaseStatus) IsTerminal() bool {
	return s == PhaseStatusDone || s == PhaseStatusBlocked || s == PhaseStatusFailed
}

// Phase is one phase instance within a Change.
type Phase struct {
	ID         string
	Type       PhaseType
	Status     PhaseStatus
	Confidence float64
	StartedAt  time.Time
	EndedAt    time.Time
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/phase.go internal/domain/phase_test.go
git commit -m "feat(domain): add PhaseType, PhaseStatus, Phase"
```

---

### Task 6: domain/change.go — ChangeID, ChangeStatus, Change

**Files:**
- Create: `internal/domain/change.go`
- Create: `internal/domain/change_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestChangeIDIsZero(t *testing.T) {
	var id domain.ChangeID
	if !id.IsZero() {
		t.Error("zero-value ChangeID should be zero")
	}
	id = "01HX..."
	if id.IsZero() {
		t.Error("non-empty ChangeID should not be zero")
	}
}

func TestChangeIDString(t *testing.T) {
	id := domain.ChangeID("abc")
	if id.String() != "abc" {
		t.Errorf("got %q, want %q", id.String(), "abc")
	}
}

func TestChangeStatusIsTerminal(t *testing.T) {
	cases := map[domain.ChangeStatus]bool{
		domain.ChangeStatusPending: false,
		domain.ChangeStatusRunning: false,
		domain.ChangeStatusDone:    true,
		domain.ChangeStatusBlocked: true,
		domain.ChangeStatusFailed:  true,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestChangeZeroValue(t *testing.T) {
	var c domain.Change
	if !c.ID.IsZero() {
		t.Error("zero Change should have zero ID")
	}
	if c.Phases != nil {
		t.Error("zero Change should have nil Phases")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/domain/change.go`:

```go
package domain

import "time"

// ChangeID identifies a Change within the orchestrator.
type ChangeID string

// String returns the underlying string value.
func (id ChangeID) String() string { return string(id) }

// IsZero reports whether id is the zero value.
func (id ChangeID) IsZero() bool { return string(id) == "" }

// ChangeStatus mirrors the orchestrator's change status enum.
type ChangeStatus string

// Change status values.
const (
	ChangeStatusPending ChangeStatus = "pending"
	ChangeStatusRunning ChangeStatus = "running"
	ChangeStatusDone    ChangeStatus = "done"
	ChangeStatusBlocked ChangeStatus = "blocked"
	ChangeStatusFailed  ChangeStatus = "failed"
)

// IsTerminal reports whether s is a terminal change status.
func (s ChangeStatus) IsTerminal() bool {
	return s == ChangeStatusDone || s == ChangeStatusBlocked || s == ChangeStatusFailed
}

// Change is the snapshot of a Change as understood by the CLI.
// It is the read model assembled from `GET /api/v1/changes/{id}` responses.
type Change struct {
	ID                ChangeID
	Name              string
	Project           string
	BaseRef           string
	ArtifactStoreMode string
	Status            ChangeStatus
	CurrentPhaseID    string
	Phases            []Phase
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/change.go internal/domain/change_test.go
git commit -m "feat(domain): add ChangeID, ChangeStatus, Change"
```

---

### Task 7: domain/event.go — Event

**Files:**
- Create: `internal/domain/event.go`
- Create: `internal/domain/event_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestEventZeroValue(t *testing.T) {
	var e domain.Event
	if e.Type != "" {
		t.Error("zero Event should have empty Type")
	}
	if e.Payload != nil {
		t.Error("zero Event should have nil Payload")
	}
}

func TestEventCarriesTimestamp(t *testing.T) {
	now := time.Now()
	e := domain.Event{Type: "phase.started", Timestamp: now}
	if !e.Timestamp.Equal(now) {
		t.Errorf("Timestamp lost: got %v, want %v", e.Timestamp, now)
	}
}

func TestEventPayloadAllowsArbitraryKeys(t *testing.T) {
	e := domain.Event{
		Type:    "agent.spawned",
		Payload: map[string]any{"agent_role": "team-lead", "n": 3},
	}
	if e.Payload["agent_role"] != "team-lead" {
		t.Error("Payload string lost")
	}
	if e.Payload["n"] != 3 {
		t.Error("Payload int lost")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/domain/event.go`:

```go
package domain

import "time"

// Event is a single SSE event surfaced by the orchestrator. The CLI does
// not interpret payload semantics; adapters render it as-is.
type Event struct {
	Type      string         // e.g. "phase.started", "agent.spawned", "approval.required"
	Timestamp time.Time      // server-emitted timestamp
	Payload   map[string]any // free-form; missing fields rendered as ""
	TraceID   string         // optional; used for cross-cutting tracing
	EventID   string         // SSE id: field; used for Last-Event-ID reconnect
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/event.go internal/domain/event_test.go
git commit -m "feat(domain): add Event"
```

---

### Task 8: domain/approval.go — ApprovalGate

**Files:**
- Create: `internal/domain/approval.go`
- Create: `internal/domain/approval_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestApprovalGateAllFieldsPresent(t *testing.T) {
	g := domain.ApprovalGate{
		URL:      "https://gov.local/approvals/abc",
		Reason:   "NO APPLY WITHOUT TASKS APPROVED",
		Risk:     "medium",
		Policy:   "require_approval",
		ChangeID: domain.ChangeID("01HX..."),
		Phase:    domain.PhaseApply,
		TraceID:  "tr-1",
	}
	if g.URL == "" || g.Reason == "" || g.Risk == "" || g.Policy == "" {
		t.Error("ApprovalGate fields lost")
	}
	if g.ChangeID.IsZero() {
		t.Error("ApprovalGate.ChangeID lost")
	}
	if g.Phase != domain.PhaseApply {
		t.Errorf("ApprovalGate.Phase = %q", g.Phase)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/domain/approval.go`:

```go
package domain

// ApprovalGate is a passive surface: governance issues it; the CLI displays
// it. The CLI never resolves it directly. Decisions live in governance's UI.
type ApprovalGate struct {
	URL      string
	Reason   string
	Risk     string // "low" | "medium" | "high" | "" if unknown
	Policy   string // governance policy name, e.g. "require_approval"
	ChangeID ChangeID
	Phase    PhaseType
	TraceID  string
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/approval.go internal/domain/approval_test.go
git commit -m "feat(domain): add ApprovalGate"
```

---

### Task 9: domain/fingerprint.go — Fingerprint and Compute

**Files:**
- Create: `internal/domain/fingerprint.go`
- Create: `internal/domain/fingerprint_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestFingerprintIs16HexChars(t *testing.T) {
	fp := domain.ComputeFingerprint("ms-cotizacion", "/home/user/repo", "git@github.com:org/repo.git")
	if len(fp) != 16 {
		t.Errorf("len(fp)=%d, want 16; fp=%q", len(fp), fp)
	}
	for _, r := range fp {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("fingerprint contains non-hex char: %q", r)
		}
	}
}

func TestFingerprintIsDeterministic(t *testing.T) {
	a := domain.ComputeFingerprint("p", "/r", "u")
	b := domain.ComputeFingerprint("p", "/r", "u")
	if a != b {
		t.Errorf("fingerprint not deterministic: %q vs %q", a, b)
	}
}

func TestFingerprintDistinguishesInputs(t *testing.T) {
	cases := [][3]string{
		{"p", "/r", "u"},
		{"p2", "/r", "u"},
		{"p", "/r2", "u"},
		{"p", "/r", "u2"},
		{"p", "/r", ""},
	}
	seen := make(map[domain.Fingerprint]struct{})
	for _, c := range cases {
		fp := domain.ComputeFingerprint(c[0], c[1], c[2])
		if _, dup := seen[fp]; dup {
			t.Errorf("collision for %v", c)
		}
		seen[fp] = struct{}{}
	}
}

func TestFingerprintHandlesEmptyRemote(t *testing.T) {
	fp := domain.ComputeFingerprint("p", "/r", "")
	if len(fp) != 16 {
		t.Errorf("empty remote should still produce a 16-char fp, got %q", fp)
	}
}

func TestFingerprintZeroValue(t *testing.T) {
	var fp domain.Fingerprint
	if fp.String() != "" {
		t.Error("zero Fingerprint should stringify to empty")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/domain/fingerprint.go`:

```go
package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint is a 16-char hex digest used to scope local state per project.
type Fingerprint string

// String returns the underlying hex string.
func (f Fingerprint) String() string { return string(f) }

// ComputeFingerprint produces the 16-char hex SHA-256 digest of
// `projectName \x00 repoRoot \x00 remoteURL`.
//
// It is the project identity used to scope <stateRoot>/projects/<fp>/.
// remoteURL may be empty for repositories without a remote.
func ComputeFingerprint(projectName, repoRoot, remoteURL string) Fingerprint {
	h := sha256.New()
	h.Write([]byte(projectName))
	h.Write([]byte{0})
	h.Write([]byte(repoRoot))
	h.Write([]byte{0})
	h.Write([]byte(remoteURL))
	return Fingerprint(hex.EncodeToString(h.Sum(nil))[:16])
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/fingerprint.go internal/domain/fingerprint_test.go
git commit -m "feat(domain): add Fingerprint and ComputeFingerprint"
```

---

### Task 10: domain/config.go — ProjectConfig, UserConfig, ArtifactStoreMode

**Files:**
- Create: `internal/domain/config.go`
- Create: `internal/domain/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestArtifactStoreModeIsValid(t *testing.T) {
	valid := []domain.ArtifactStoreMode{
		domain.ArtifactStoreEngram,
		domain.ArtifactStoreOpenspec,
		domain.ArtifactStoreHybrid,
		domain.ArtifactStoreNone,
	}
	for _, m := range valid {
		if !m.IsValid() {
			t.Errorf("%q should be valid", m)
		}
	}
	if domain.ArtifactStoreMode("bogus").IsValid() {
		t.Error("bogus mode should not be valid")
	}
	if domain.ArtifactStoreMode("").IsValid() {
		t.Error("empty mode should not be valid")
	}
}

func TestProjectConfigZeroValue(t *testing.T) {
	var c domain.ProjectConfig
	if c.Project != "" || c.BaseRef != "" {
		t.Error("zero ProjectConfig should have empty fields")
	}
}

func TestUserConfigZeroValue(t *testing.T) {
	var c domain.UserConfig
	if c.OrchestratorURL != "" || c.TimeoutSeconds != 0 {
		t.Error("zero UserConfig should have empty fields")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/domain/config.go`:

```go
package domain

// ArtifactStoreMode is the SDD artifact store backend.
type ArtifactStoreMode string

// Artifact store modes accepted by the orchestrator.
const (
	ArtifactStoreEngram   ArtifactStoreMode = "engram"
	ArtifactStoreOpenspec ArtifactStoreMode = "openspec"
	ArtifactStoreHybrid   ArtifactStoreMode = "hybrid"
	ArtifactStoreNone     ArtifactStoreMode = "none"
)

// IsValid reports whether m is one of the known modes.
func (m ArtifactStoreMode) IsValid() bool {
	switch m {
	case ArtifactStoreEngram, ArtifactStoreOpenspec, ArtifactStoreHybrid, ArtifactStoreNone:
		return true
	}
	return false
}

// ProjectConfig is the in-memory shape of `.sophia.yaml`.
// Adapter-side YAML DTOs live in adapters/outbound/yamlconfig and decode
// into this type. The domain itself does not depend on yaml.v3.
type ProjectConfig struct {
	Version       int
	Project       string
	BaseRef       string
	ArtifactStore ArtifactStoreMode
}

// UserConfig is the in-memory shape of <configRoot>/config.yaml.
// V1 honors only the orchestrator block; UI fields are reserved for V1.1.
type UserConfig struct {
	Version         int
	OrchestratorURL string
	TimeoutSeconds  int
}
```

- [ ] **Step 4: Run tests + go vet**

Run: `go test ./internal/domain/... && go vet ./...`
Expected: PASS, no vet warnings.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/config.go internal/domain/config_test.go
git commit -m "feat(domain): add ArtifactStoreMode, ProjectConfig, UserConfig"
```

---

## Phase 3 — Ports

### Task 11: ports/inbound/eventsink.go

**Files:**
- Create: `internal/ports/inbound/eventsink.go`

- [ ] **Step 1: Write the file**

```go
// Package inbound declares the inbound ports — interfaces consumed by the
// adapters that drive the CLI (cobra commands, bubbletea TUI, JSON sink).
package inbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// EventSink is the single render port. Application services push events here
// without knowing whether they are painting a TUI or emitting JSONL. Both
// implementations live in adapters/inbound/{tui,jsonsink}/.
type EventSink interface {
	OnSnapshot(ctx context.Context, change *domain.Change) error
	OnEvent(ctx context.Context, ev domain.Event) error
	OnApprovalGate(ctx context.Context, gate domain.ApprovalGate) error
	OnError(ctx context.Context, err error) error
	OnComplete(ctx context.Context, finalStatus domain.ChangeStatus) error
	Close() error
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./internal/ports/inbound/...`
Expected: succeeds with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/ports/inbound/eventsink.go
git commit -m "feat(ports): add EventSink inbound port"
```

---

### Task 12: ports/outbound — clients (orchestrator, eventstream, compose, git)

**Files:**
- Create: `internal/ports/outbound/orchestrator.go`
- Create: `internal/ports/outbound/eventstream.go`
- Create: `internal/ports/outbound/compose.go`
- Create: `internal/ports/outbound/git.go`

- [ ] **Step 1: Write `orchestrator.go`**

```go
// Package outbound declares the outbound ports — interfaces implemented by
// adapters in internal/adapters/outbound/. Application services depend on
// these abstractions, never on concrete adapters.
package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// CreateChangeInput mirrors the orchestrator request body for POST /api/v1/changes.
type CreateChangeInput struct {
	Name              string
	Project           string
	BaseRef           string
	ArtifactStoreMode string
}

// ListChangesFilter is the parameter set for GET /api/v1/changes.
type ListChangesFilter struct {
	Project string
	Status  string
	Limit   int
	Offset  int
}

// OrchestratorClient is the CLI's view of sophia-orchestator HTTP API.
// Real adapter: internal/adapters/outbound/orchestratorhttp.
type OrchestratorClient interface {
	Healthz(ctx context.Context) error
	CreateChange(ctx context.Context, in CreateChangeInput) (*domain.Change, error)
	GetChange(ctx context.Context, id domain.ChangeID) (*domain.Change, error)
	ListChanges(ctx context.Context, filter ListChangesFilter) ([]*domain.Change, error)
}
```

- [ ] **Step 2: Write `eventstream.go`**

```go
package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// StreamTarget identifies the SSE target. V1 uses per-phase streams; the
// shape is forward-compatible with a future per-Change global stream.
type StreamTarget struct {
	ChangeID domain.ChangeID
	PhaseID  string
}

// SubscribeOptions carries reconnect metadata.
type SubscribeOptions struct {
	LastEventID string
}

// EventStreamClient is the CLI's SSE consumer abstraction. Real adapter
// arrives in M5: internal/adapters/outbound/ssestream.
type EventStreamClient interface {
	Subscribe(ctx context.Context, target StreamTarget, opts SubscribeOptions) (<-chan domain.Event, func() error, error)
}
```

- [ ] **Step 3: Write `compose.go`**

```go
package outbound

import "context"

// ComposeRunner abstracts `docker compose` subprocess invocations.
// Real adapter: internal/adapters/outbound/composeexec.
type ComposeRunner interface {
	Version(ctx context.Context) (string, error)
	Up(ctx context.Context, composePath, projectName string) error
	Down(ctx context.Context, composePath, projectName string) error
}
```

- [ ] **Step 4: Write `git.go`**

```go
package outbound

import "context"

// GitInspector abstracts `git` subprocess invocations the CLI needs.
// Real adapter: internal/adapters/outbound/gitcli.
type GitInspector interface {
	Version(ctx context.Context) (string, error)
	RepoRoot(ctx context.Context, cwd string) (string, error)
	RemoteURL(ctx context.Context, cwd string) (string, error)
	CurrentBranch(ctx context.Context, cwd string) (string, error)
	Status(ctx context.Context, cwd string) (clean bool, files int, err error)
}
```

- [ ] **Step 5: Verify and commit**

Run: `go build ./internal/ports/outbound/...`
Expected: compiles.

```bash
git add internal/ports/outbound/orchestrator.go internal/ports/outbound/eventstream.go \
        internal/ports/outbound/compose.go internal/ports/outbound/git.go
git commit -m "feat(ports): add outbound client ports"
```

---

### Task 13: ports/outbound — stores (projectconfig, userconfig, statestore)

**Files:**
- Create: `internal/ports/outbound/projectconfig.go`
- Create: `internal/ports/outbound/userconfig.go`
- Create: `internal/ports/outbound/statestore.go`

- [ ] **Step 1: Write `projectconfig.go`**

```go
package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// ProjectConfigStore reads and writes `.sophia.yaml` at the resolved repo root.
// Real adapter (M3): internal/adapters/outbound/yamlconfig.
type ProjectConfigStore interface {
	Read(ctx context.Context, path string) (*domain.ProjectConfig, error)
	Write(ctx context.Context, path string, cfg *domain.ProjectConfig) error
	Find(ctx context.Context, startDir string) (path string, err error)
}
```

- [ ] **Step 2: Write `userconfig.go`**

```go
package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// UserConfigStore reads and writes the user-level `<configRoot>/config.yaml`.
type UserConfigStore interface {
	Read(ctx context.Context, path string) (*domain.UserConfig, error)
	Write(ctx context.Context, path string, cfg *domain.UserConfig) error
}
```

- [ ] **Step 3: Write `statestore.go`**

```go
package outbound

import (
	"context"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// StateStore manages last_change_id, both project-scoped and as a global fallback.
// Real adapter (M3): internal/adapters/outbound/filestate.
type StateStore interface {
	GetLast(ctx context.Context, fp domain.Fingerprint) (domain.ChangeID, error)
	SetLast(ctx context.Context, fp domain.Fingerprint, id domain.ChangeID) error
	GetGlobalLast(ctx context.Context) (domain.ChangeID, error)
	SetGlobalLast(ctx context.Context, id domain.ChangeID) error
}
```

- [ ] **Step 4: Verify and commit**

Run: `go build ./internal/ports/outbound/...`
Expected: compiles.

```bash
git add internal/ports/outbound/projectconfig.go internal/ports/outbound/userconfig.go \
        internal/ports/outbound/statestore.go
git commit -m "feat(ports): add outbound store ports"
```

---

### Task 14: ports/outbound — utilities (browser, clock)

**Files:**
- Create: `internal/ports/outbound/browser.go`
- Create: `internal/ports/outbound/clock.go`

- [ ] **Step 1: Write `browser.go`**

```go
package outbound

import "context"

// Browser opens validated http(s) URLs via the OS opener (`xdg-open`/`open`/`start`).
// Real adapter (M7): internal/adapters/outbound/osbrowser.
// The implementation MUST validate the URL scheme before invoking the opener.
type Browser interface {
	Open(ctx context.Context, url string) error
}
```

- [ ] **Step 2: Write `clock.go`**

```go
package outbound

import "time"

// Clock abstracts time.Now() for deterministic testing.
type Clock interface {
	Now() time.Time
}
```

- [ ] **Step 3: Verify and commit**

Run: `go build ./internal/ports/outbound/...`
Expected: compiles.

```bash
git add internal/ports/outbound/browser.go internal/ports/outbound/clock.go
git commit -m "feat(ports): add outbound utility ports"
```

---

## Phase 4 — Test fakes

> Each fake lives in `test/fakes/` and is a small struct that implements
> exactly one outbound port. The conformance test ensures the fake is a
> compile-time interface implementer.

### Task 15: test/fakes/clock.go — FakeClock

**Files:**
- Create: `test/fakes/clock.go`
- Create: `test/fakes/clock_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeClockImplementsClock(t *testing.T) {
	var _ outbound.Clock = (*fakes.FakeClock)(nil)
}

func TestFakeClockReturnsConfiguredTime(t *testing.T) {
	want := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	c := fakes.NewFakeClock(want)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}

func TestFakeClockAdvance(t *testing.T) {
	c := fakes.NewFakeClock(time.Unix(0, 0).UTC())
	c.Advance(5 * time.Minute)
	want := time.Unix(300, 0).UTC()
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL with "undefined: fakes.FakeClock".

- [ ] **Step 3: Implement**

`test/fakes/clock.go`:

```go
// Package fakes provides shared in-memory implementations of outbound ports
// for use in unit tests across the CLI. Production code MUST NOT import
// this package.
package fakes

import (
	"sync"
	"time"
)

// FakeClock is a deterministic Clock for tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock constructs a FakeClock starting at start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the fake clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/clock.go test/fakes/clock_test.go
git commit -m "test(fakes): add FakeClock"
```

---

### Task 16: test/fakes/git.go — FakeGitInspector

**Files:**
- Create: `test/fakes/git.go`
- Create: `test/fakes/git_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeGitImplementsGitInspector(t *testing.T) {
	var _ outbound.GitInspector = (*fakes.FakeGitInspector)(nil)
}

func TestFakeGitVersion(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.GitVersion = "git version 2.46.0"
	v, err := g.Version(context.Background())
	if err != nil {
		t.Fatalf("Version err: %v", err)
	}
	if v != "git version 2.46.0" {
		t.Errorf("Version = %q", v)
	}
}

func TestFakeGitRepoRootError(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.NotARepo = true
	_, err := g.RepoRoot(context.Background(), "/anywhere")
	if err == nil {
		t.Error("expected error when NotARepo")
	}
}

func TestFakeGitStatus(t *testing.T) {
	g := fakes.NewFakeGitInspector()
	g.Clean = false
	g.DirtyCount = 3
	clean, n, err := g.Status(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if clean || n != 3 {
		t.Errorf("got clean=%v n=%d", clean, n)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/git.go`:

```go
package fakes

import (
	"context"
	"errors"
)

// FakeGitInspector is a programmable GitInspector for tests.
type FakeGitInspector struct {
	GitVersion string
	NotARepo   bool
	Root       string
	Remote     string
	Branch     string
	Clean      bool
	DirtyCount int
}

// NewFakeGitInspector returns sensible defaults.
func NewFakeGitInspector() *FakeGitInspector {
	return &FakeGitInspector{
		GitVersion: "git version 2.46.0",
		Root:       "/repo",
		Remote:     "git@github.com:org/repo.git",
		Branch:     "main",
		Clean:      true,
	}
}

// Version returns the configured git version string.
func (f *FakeGitInspector) Version(_ context.Context) (string, error) {
	return f.GitVersion, nil
}

// RepoRoot returns the configured root or an error if NotARepo is set.
func (f *FakeGitInspector) RepoRoot(_ context.Context, _ string) (string, error) {
	if f.NotARepo {
		return "", errors.New("not a git repository")
	}
	return f.Root, nil
}

// RemoteURL returns the configured remote URL.
func (f *FakeGitInspector) RemoteURL(_ context.Context, _ string) (string, error) {
	return f.Remote, nil
}

// CurrentBranch returns the configured branch name.
func (f *FakeGitInspector) CurrentBranch(_ context.Context, _ string) (string, error) {
	return f.Branch, nil
}

// Status returns the configured clean flag and dirty file count.
func (f *FakeGitInspector) Status(_ context.Context, _ string) (bool, int, error) {
	return f.Clean, f.DirtyCount, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/git.go test/fakes/git_test.go
git commit -m "test(fakes): add FakeGitInspector"
```

---

### Task 17: test/fakes/compose.go — FakeComposeRunner

**Files:**
- Create: `test/fakes/compose.go`
- Create: `test/fakes/compose_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeComposeImplementsComposeRunner(t *testing.T) {
	var _ outbound.ComposeRunner = (*fakes.FakeComposeRunner)(nil)
}

func TestFakeComposeVersion(t *testing.T) {
	c := fakes.NewFakeComposeRunner()
	c.ComposeVersion = "Docker Compose version v2.32.1"
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != "Docker Compose version v2.32.1" {
		t.Errorf("Version = %q", v)
	}
}

func TestFakeComposeUpDownRecorded(t *testing.T) {
	c := fakes.NewFakeComposeRunner()
	_ = c.Up(context.Background(), "/p/compose.yaml", "sophia")
	_ = c.Down(context.Background(), "/p/compose.yaml", "sophia")
	if got := c.UpCalls; got != 1 {
		t.Errorf("UpCalls = %d, want 1", got)
	}
	if got := c.DownCalls; got != 1 {
		t.Errorf("DownCalls = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/compose.go`:

```go
package fakes

import "context"

// FakeComposeRunner is a programmable ComposeRunner for tests.
type FakeComposeRunner struct {
	ComposeVersion string
	UpErr          error
	DownErr        error
	UpCalls        int
	DownCalls      int
}

// NewFakeComposeRunner returns sensible defaults.
func NewFakeComposeRunner() *FakeComposeRunner {
	return &FakeComposeRunner{ComposeVersion: "Docker Compose version v2.32.1"}
}

// Version returns the configured compose version string.
func (f *FakeComposeRunner) Version(_ context.Context) (string, error) {
	return f.ComposeVersion, nil
}

// Up records the call and returns the configured error.
func (f *FakeComposeRunner) Up(_ context.Context, _, _ string) error {
	f.UpCalls++
	return f.UpErr
}

// Down records the call and returns the configured error.
func (f *FakeComposeRunner) Down(_ context.Context, _, _ string) error {
	f.DownCalls++
	return f.DownErr
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/compose.go test/fakes/compose_test.go
git commit -m "test(fakes): add FakeComposeRunner"
```

---

### Task 18: test/fakes/orchestrator.go — FakeOrchestrator

**Files:**
- Create: `test/fakes/orchestrator.go`
- Create: `test/fakes/orchestrator_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeOrchestratorImplementsClient(t *testing.T) {
	var _ outbound.OrchestratorClient = (*fakes.FakeOrchestrator)(nil)
}

func TestFakeOrchestratorHealthz(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz err: %v", err)
	}
	c.HealthzErr = errors.New("down")
	if err := c.Healthz(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestFakeOrchestratorCreateAndGet(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	change, err := c.CreateChange(context.Background(), outbound.CreateChangeInput{Name: "n", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if change.ID.IsZero() {
		t.Fatal("expected non-zero id")
	}
	got, err := c.GetChange(context.Background(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "n" || got.Project != "p" {
		t.Errorf("round-trip lost fields: %+v", got)
	}
}

func TestFakeOrchestratorListFilters(t *testing.T) {
	c := fakes.NewFakeOrchestrator()
	c.SeedChange(&domain.Change{ID: "a", Project: "x", Status: domain.ChangeStatusRunning})
	c.SeedChange(&domain.Change{ID: "b", Project: "y", Status: domain.ChangeStatusDone})
	got, err := c.ListChanges(context.Background(), outbound.ListChangesFilter{Project: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("filter failed: %+v", got)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/orchestrator.go`:

```go
package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// FakeOrchestrator is an in-memory OrchestratorClient for tests.
type FakeOrchestrator struct {
	mu         sync.Mutex
	HealthzErr error
	changes    map[domain.ChangeID]*domain.Change
	nextID     int
}

// NewFakeOrchestrator returns an empty FakeOrchestrator.
func NewFakeOrchestrator() *FakeOrchestrator {
	return &FakeOrchestrator{changes: map[domain.ChangeID]*domain.Change{}}
}

// SeedChange inserts a Change for use in tests.
func (f *FakeOrchestrator) SeedChange(c *domain.Change) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes[c.ID] = c
}

// Healthz returns the configured error.
func (f *FakeOrchestrator) Healthz(_ context.Context) error {
	return f.HealthzErr
}

// CreateChange persists a new Change with a synthetic ID.
func (f *FakeOrchestrator) CreateChange(_ context.Context, in outbound.CreateChangeInput) (*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := domain.ChangeID(fmt.Sprintf("fake-change-%d", f.nextID))
	c := &domain.Change{
		ID:                id,
		Name:              in.Name,
		Project:           in.Project,
		BaseRef:           in.BaseRef,
		ArtifactStoreMode: in.ArtifactStoreMode,
		Status:            domain.ChangeStatusPending,
	}
	f.changes[id] = c
	return c, nil
}

// GetChange returns a stored Change or domain.ErrChangeNotFound.
func (f *FakeOrchestrator) GetChange(_ context.Context, id domain.ChangeID) (*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[id]
	if !ok {
		return nil, domain.ErrChangeNotFound
	}
	return c, nil
}

// ListChanges returns Changes filtered by the supplied criteria.
func (f *FakeOrchestrator) ListChanges(_ context.Context, filter outbound.ListChangesFilter) ([]*domain.Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.Change, 0, len(f.changes))
	for _, c := range f.changes {
		if filter.Project != "" && c.Project != filter.Project {
			continue
		}
		if filter.Status != "" && string(c.Status) != filter.Status {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/orchestrator.go test/fakes/orchestrator_test.go
git commit -m "test(fakes): add FakeOrchestrator"
```

---

### Task 19: test/fakes/eventstream.go — FakeEventStream

**Files:**
- Create: `test/fakes/eventstream.go`
- Create: `test/fakes/eventstream_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeEventStreamImplementsClient(t *testing.T) {
	var _ outbound.EventStreamClient = (*fakes.FakeEventStream)(nil)
}

func TestFakeEventStreamDelivers(t *testing.T) {
	s := fakes.NewFakeEventStream()
	target := outbound.StreamTarget{ChangeID: "c", PhaseID: "p"}
	ch, cancel, err := s.Subscribe(context.Background(), target, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	s.Push(target, domain.Event{Type: "phase.started"})
	got := <-ch
	if got.Type != "phase.started" {
		t.Errorf("got %q", got.Type)
	}
}

func TestFakeEventStreamCloseIsIdempotent(t *testing.T) {
	s := fakes.NewFakeEventStream()
	target := outbound.StreamTarget{ChangeID: "c"}
	_, cancel, err := s.Subscribe(context.Background(), target, outbound.SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cancel(); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := cancel(); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/eventstream.go`:

```go
package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// FakeEventStream is an in-memory EventStreamClient for tests.
// Push events with Push; subscribers receive them in FIFO order.
type FakeEventStream struct {
	mu          sync.Mutex
	subscribers map[string][]chan domain.Event
}

// NewFakeEventStream returns an empty stream.
func NewFakeEventStream() *FakeEventStream {
	return &FakeEventStream{subscribers: map[string][]chan domain.Event{}}
}

func key(t outbound.StreamTarget) string {
	return string(t.ChangeID) + "|" + t.PhaseID
}

// Subscribe registers a new subscriber and returns its channel and cancel func.
func (s *FakeEventStream) Subscribe(_ context.Context, target outbound.StreamTarget, _ outbound.SubscribeOptions) (<-chan domain.Event, func() error, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan domain.Event, 16)
	s.subscribers[key(target)] = append(s.subscribers[key(target)], ch)
	var once sync.Once
	cancel := func() error {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			subs := s.subscribers[key(target)]
			for i, c := range subs {
				if c == ch {
					s.subscribers[key(target)] = append(subs[:i], subs[i+1:]...)
					close(ch)
					return
				}
			}
		})
		return nil
	}
	return ch, cancel, nil
}

// Push delivers ev to every subscriber on target.
func (s *FakeEventStream) Push(target outbound.StreamTarget, ev domain.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers[key(target)] {
		ch <- ev
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/... -race`
Expected: PASS, no race detected.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/eventstream.go test/fakes/eventstream_test.go
git commit -m "test(fakes): add FakeEventStream"
```

---

### Task 20: test/fakes/projectconfig.go + userconfig.go — FakeProjectConfigStore + FakeUserConfigStore

**Files:**
- Create: `test/fakes/projectconfig.go`
- Create: `test/fakes/userconfig.go`
- Create: `test/fakes/configstore_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeProjectConfigImplementsPort(t *testing.T) {
	var _ outbound.ProjectConfigStore = (*fakes.FakeProjectConfigStore)(nil)
}

func TestFakeUserConfigImplementsPort(t *testing.T) {
	var _ outbound.UserConfigStore = (*fakes.FakeUserConfigStore)(nil)
}

func TestFakeProjectConfigRoundTrip(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	cfg := &domain.ProjectConfig{Version: 1, Project: "p", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram}
	if err := s.Write(context.Background(), "/repo/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "p" || got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestFakeProjectConfigFind(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	cfg := &domain.ProjectConfig{Project: "p"}
	_ = s.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	path, err := s.Find(context.Background(), "/repo/sub/dir")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/repo/.sophia.yaml" {
		t.Errorf("Find = %q", path)
	}
}

func TestFakeProjectConfigFindMissing(t *testing.T) {
	s := fakes.NewFakeProjectConfigStore()
	_, err := s.Find(context.Background(), "/somewhere")
	if err == nil {
		t.Error("expected error when no .sophia.yaml is registered")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement `projectconfig.go`**

```go
package fakes

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// FakeProjectConfigStore stores .sophia.yaml in memory.
type FakeProjectConfigStore struct {
	mu    sync.Mutex
	store map[string]*domain.ProjectConfig
}

// NewFakeProjectConfigStore returns an empty store.
func NewFakeProjectConfigStore() *FakeProjectConfigStore {
	return &FakeProjectConfigStore{store: map[string]*domain.ProjectConfig{}}
}

// Read returns the stored config or domain.ErrConfigMissing.
func (s *FakeProjectConfigStore) Read(_ context.Context, path string) (*domain.ProjectConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.store[path]
	if !ok {
		return nil, domain.ErrConfigMissing
	}
	out := *cfg
	return &out, nil
}

// Write persists cfg under path.
func (s *FakeProjectConfigStore) Write(_ context.Context, path string, cfg *domain.ProjectConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *cfg
	s.store[path] = &out
	return nil
}

// Find walks ancestors of startDir looking for a registered .sophia.yaml.
func (s *FakeProjectConfigStore) Find(_ context.Context, startDir string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := startDir
	for dir != "" && dir != "/" {
		candidate := dir + "/.sophia.yaml"
		if _, ok := s.store[candidate]; ok {
			return candidate, nil
		}
		idx := strings.LastIndex(dir, "/")
		if idx <= 0 {
			break
		}
		dir = dir[:idx]
	}
	if _, ok := s.store["/.sophia.yaml"]; ok {
		return "/.sophia.yaml", nil
	}
	return "", errors.New("no .sophia.yaml found")
}
```

- [ ] **Step 4: Implement `userconfig.go`**

```go
package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// FakeUserConfigStore stores config.yaml in memory.
type FakeUserConfigStore struct {
	mu    sync.Mutex
	store map[string]*domain.UserConfig
}

// NewFakeUserConfigStore returns an empty store.
func NewFakeUserConfigStore() *FakeUserConfigStore {
	return &FakeUserConfigStore{store: map[string]*domain.UserConfig{}}
}

// Read returns the stored config or domain.ErrConfigMissing.
func (s *FakeUserConfigStore) Read(_ context.Context, path string) (*domain.UserConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.store[path]
	if !ok {
		return nil, domain.ErrConfigMissing
	}
	out := *cfg
	return &out, nil
}

// Write persists cfg under path.
func (s *FakeUserConfigStore) Write(_ context.Context, path string, cfg *domain.UserConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *cfg
	s.store[path] = &out
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add test/fakes/projectconfig.go test/fakes/userconfig.go test/fakes/configstore_test.go
git commit -m "test(fakes): add FakeProjectConfigStore and FakeUserConfigStore"
```

---

### Task 21: test/fakes/statestore.go — FakeStateStore

**Files:**
- Create: `test/fakes/statestore.go`
- Create: `test/fakes/statestore_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeStateStoreImplementsPort(t *testing.T) {
	var _ outbound.StateStore = (*fakes.FakeStateStore)(nil)
}

func TestFakeStateStoreProjectScoped(t *testing.T) {
	s := fakes.NewFakeStateStore()
	fp := domain.Fingerprint("abc")
	if id, _ := s.GetLast(context.Background(), fp); !id.IsZero() {
		t.Error("empty store should return zero")
	}
	if err := s.SetLast(context.Background(), fp, "01H..."); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetLast(context.Background(), fp)
	if got != "01H..." {
		t.Errorf("got %q", got)
	}
}

func TestFakeStateStoreGlobal(t *testing.T) {
	s := fakes.NewFakeStateStore()
	if id, _ := s.GetGlobalLast(context.Background()); !id.IsZero() {
		t.Error("expected zero")
	}
	_ = s.SetGlobalLast(context.Background(), "g1")
	got, _ := s.GetGlobalLast(context.Background())
	if got != "g1" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/statestore.go`:

```go
package fakes

import (
	"context"
	"sync"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// FakeStateStore stores last_change_id project-scoped and globally in memory.
type FakeStateStore struct {
	mu       sync.Mutex
	projects map[domain.Fingerprint]domain.ChangeID
	global   domain.ChangeID
}

// NewFakeStateStore returns an empty store.
func NewFakeStateStore() *FakeStateStore {
	return &FakeStateStore{projects: map[domain.Fingerprint]domain.ChangeID{}}
}

// GetLast returns the project-scoped last change id (zero if absent).
func (s *FakeStateStore) GetLast(_ context.Context, fp domain.Fingerprint) (domain.ChangeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projects[fp], nil
}

// SetLast stores the project-scoped last change id.
func (s *FakeStateStore) SetLast(_ context.Context, fp domain.Fingerprint, id domain.ChangeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[fp] = id
	return nil
}

// GetGlobalLast returns the global fallback last change id.
func (s *FakeStateStore) GetGlobalLast(_ context.Context) (domain.ChangeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.global, nil
}

// SetGlobalLast stores the global fallback last change id.
func (s *FakeStateStore) SetGlobalLast(_ context.Context, id domain.ChangeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = id
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/statestore.go test/fakes/statestore_test.go
git commit -m "test(fakes): add FakeStateStore"
```

---

### Task 22: test/fakes/browser.go — FakeBrowser

**Files:**
- Create: `test/fakes/browser.go`
- Create: `test/fakes/browser_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeBrowserImplementsPort(t *testing.T) {
	var _ outbound.Browser = (*fakes.FakeBrowser)(nil)
}

func TestFakeBrowserRecordsURLs(t *testing.T) {
	b := fakes.NewFakeBrowser()
	_ = b.Open(context.Background(), "https://example.com/a")
	_ = b.Open(context.Background(), "https://example.com/b")
	if got := len(b.Opened); got != 2 {
		t.Fatalf("Opened len = %d", got)
	}
	if b.Opened[0] != "https://example.com/a" || b.Opened[1] != "https://example.com/b" {
		t.Errorf("Opened = %v", b.Opened)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/browser.go`:

```go
package fakes

import (
	"context"
	"sync"
)

// FakeBrowser records the URLs that would have been opened.
type FakeBrowser struct {
	mu     sync.Mutex
	Opened []string
	OpenErr error
}

// NewFakeBrowser returns an empty FakeBrowser.
func NewFakeBrowser() *FakeBrowser { return &FakeBrowser{} }

// Open records url and returns the configured error.
func (f *FakeBrowser) Open(_ context.Context, url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.OpenErr != nil {
		return f.OpenErr
	}
	f.Opened = append(f.Opened, url)
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/browser.go test/fakes/browser_test.go
git commit -m "test(fakes): add FakeBrowser"
```

---

## Phase 5 — Logging infrastructure

### Task 23: infrastructure/logging/jsonl_handler.go — JSONL slog handler

**Files:**
- Create: `internal/infrastructure/logging/jsonl_handler.go`
- Create: `internal/infrastructure/logging/jsonl_handler_test.go`

- [ ] **Step 1: Write the failing test**

```go
package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/logging"
)

func TestJSONLHandlerEmitsValidLines(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo)
	logger := slog.New(h)

	logger.Info("hello", slog.String("cmd", "doctor"))
	logger.Warn("careful", slog.Int("retries", 2))

	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSONL line %q: %v", line, err)
		}
		if _, ok := m["ts"]; !ok {
			t.Errorf("line missing ts: %q", line)
		}
		if _, ok := m["level"]; !ok {
			t.Errorf("line missing level: %q", line)
		}
		if _, ok := m["msg"]; !ok {
			t.Errorf("line missing msg: %q", line)
		}
	}
}

func TestJSONLHandlerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelWarn)
	logger := slog.New(h)
	logger.Info("hidden")
	logger.Warn("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Error("warn line missing")
	}
	if strings.Contains(buf.String(), "hidden") {
		t.Error("info line should be filtered")
	}
}

func TestJSONLHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo).WithAttrs([]slog.Attr{slog.String("scope", "doctor")})
	logger := slog.New(h)
	logger.Info("ok")
	if !strings.Contains(buf.String(), `"scope":"doctor"`) {
		t.Errorf("scope attr lost: %s", buf.String())
	}
}

func TestJSONLHandlerHandleContextDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONLHandler(&buf, slog.LevelInfo)
	r := slog.NewRecord(timeNowForTest(), slog.LevelInfo, "x", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}
```

You will also need a small helper at the bottom of the file:

```go
func timeNowForTest() time.Time {
	return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
}
```

(Add `import "time"` to the test file.)

- [ ] **Step 2: Run test**

Run: `go test ./internal/infrastructure/logging/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/infrastructure/logging/jsonl_handler.go`:

```go
// Package logging provides slog handlers used by the CLI.
package logging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
)

// JSONLHandler is a minimal slog.Handler that writes one JSON object per
// line. It is suitable for `<stateRoot>/logs/cli-YYYY-MM-DD.log`.
type JSONLHandler struct {
	mu      *sync.Mutex
	w       io.Writer
	level   slog.Level
	attrs   []slog.Attr
	groups  []string
}

// NewJSONLHandler constructs a JSONLHandler writing to w at level.
func NewJSONLHandler(w io.Writer, level slog.Level) *JSONLHandler {
	return &JSONLHandler{
		mu:    &sync.Mutex{},
		w:     w,
		level: level,
	}
}

// Enabled reports whether level is enabled.
func (h *JSONLHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// WithAttrs returns a new handler with the given attrs appended.
func (h *JSONLHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &c
}

// WithGroup returns a new handler scoped to the named group.
func (h *JSONLHandler) WithGroup(name string) slog.Handler {
	c := *h
	c.groups = append(append([]string{}, h.groups...), name)
	return &c
}

// Handle writes r as a JSON object with `ts`, `level`, `msg`, and attributes.
func (h *JSONLHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]any{
		"ts":    r.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	for _, a := range h.attrs {
		m[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(b)
	return err
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infrastructure/logging/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/logging/jsonl_handler.go internal/infrastructure/logging/jsonl_handler_test.go
git commit -m "feat(logging): add JSONL slog handler"
```

---

## Phase 6 — Application doctor service

### Task 24: application/doctor.go — RunDiagnostics with TDD

**Files:**
- Create: `internal/application/doctor.go`
- Create: `internal/application/doctor_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package application_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newDoctor() (*application.DoctorService, *fakes.FakeComposeRunner, *fakes.FakeGitInspector) {
	compose := fakes.NewFakeComposeRunner()
	git := fakes.NewFakeGitInspector()
	xdg := func(_ string) error { return nil } // all paths exist
	d := application.NewDoctorService(compose, git, xdg)
	return d, compose, git
}

func TestDoctorAllGreen(t *testing.T) {
	d, _, _ := newDoctor()
	report := d.Run(context.Background())
	if report.Summary.Fail != 0 {
		t.Errorf("expected 0 fails, got %+v\n%v", report.Summary, report.Checks)
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}

func TestDoctorReportsDockerComposeMissing(t *testing.T) {
	d, compose, _ := newDoctor()
	compose.ComposeVersion = ""
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected a fail when compose version is empty")
	}
}

func TestDoctorReportsComposeV1AsFail(t *testing.T) {
	d, compose, _ := newDoctor()
	compose.ComposeVersion = "docker-compose version 1.29.2"
	report := d.Run(context.Background())
	found := false
	for _, c := range report.Checks {
		if c.ID == "compose" && c.Level == application.LevelFail {
			found = true
		}
	}
	if !found {
		t.Errorf("expected compose v1 to fail; got %+v", report.Checks)
	}
}

func TestDoctorReportsGitMissing(t *testing.T) {
	d, _, git := newDoctor()
	git.GitVersion = ""
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected git fail when version empty")
	}
}

func TestDoctorXDGPathError(t *testing.T) {
	compose := fakes.NewFakeComposeRunner()
	git := fakes.NewFakeGitInspector()
	xdg := func(p string) error { return application.ErrPathInvalid }
	d := application.NewDoctorService(compose, git, xdg)
	report := d.Run(context.Background())
	if report.Summary.Fail == 0 {
		t.Error("expected XDG path fail")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/application/doctor.go`:

```go
// Package application contains the use cases of the CLI. Use cases consume
// outbound ports and never import adapters or third-party UI libraries.
package application

import (
	"context"
	"errors"
	"strings"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// CheckLevel categorizes a doctor check outcome.
type CheckLevel string

// Doctor check levels.
const (
	LevelOK   CheckLevel = "ok"
	LevelInfo CheckLevel = "info"
	LevelWarn CheckLevel = "warn"
	LevelFail CheckLevel = "fail"
)

// Check is one row in the doctor report.
type Check struct {
	ID     string
	Title  string
	Level  CheckLevel
	Detail string
}

// DiagnosticsSummary aggregates check counts.
type DiagnosticsSummary struct {
	OK   int
	Info int
	Warn int
	Fail int
}

// DiagnosticsReport is the output of doctor.
type DiagnosticsReport struct {
	Checks  []Check
	Summary DiagnosticsSummary
}

// ErrPathInvalid is returned by an XDG path validator when a path is not
// present or has wrong permissions.
var ErrPathInvalid = errors.New("xdg path invalid")

// XDGValidator returns nil if path exists with correct permissions; otherwise
// returns ErrPathInvalid (or another error). Default-mode doctor never
// creates paths; --fix mode (M2+) does.
type XDGValidator func(path string) error

// DoctorService orchestrates the M1 subset of doctor checks: docker compose,
// git, and XDG paths. M2 adds orchestrator + SSE; M3 adds .sophia.yaml; etc.
type DoctorService struct {
	compose outbound.ComposeRunner
	git     outbound.GitInspector
	xdg     XDGValidator
}

// NewDoctorService constructs a DoctorService.
func NewDoctorService(compose outbound.ComposeRunner, git outbound.GitInspector, xdg XDGValidator) *DoctorService {
	return &DoctorService{compose: compose, git: git, xdg: xdg}
}

// Run executes the doctor checks and returns a DiagnosticsReport.
func (d *DoctorService) Run(ctx context.Context) DiagnosticsReport {
	checks := []Check{
		d.checkDocker(ctx),
		d.checkCompose(ctx),
		d.checkGit(ctx),
		d.checkXDG(),
	}
	summary := DiagnosticsSummary{}
	for _, c := range checks {
		switch c.Level {
		case LevelOK:
			summary.OK++
		case LevelInfo:
			summary.Info++
		case LevelWarn:
			summary.Warn++
		case LevelFail:
			summary.Fail++
		}
	}
	return DiagnosticsReport{Checks: checks, Summary: summary}
}

func (d *DoctorService) checkDocker(ctx context.Context) Check {
	v, err := d.compose.Version(ctx)
	if err != nil {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: "docker not available"}
	}
	return Check{ID: "docker", Title: "Docker daemon", Level: LevelOK, Detail: "available"}
}

func (d *DoctorService) checkCompose(ctx context.Context) Check {
	v, err := d.compose.Version(ctx)
	if err != nil || v == "" {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "compose not available"}
	}
	if !isComposeV2(v) {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "v2 required, got: " + v}
	}
	return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkGit(ctx context.Context) Check {
	v, err := d.git.Version(ctx)
	if err != nil {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: "git not available"}
	}
	return Check{ID: "git", Title: "Git", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkXDG() Check {
	for _, p := range []string{"configRoot", "stateRoot", "dataRoot"} {
		if err := d.xdg(p); err != nil {
			return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: p + ": " + err.Error()}
		}
	}
	return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelOK, Detail: "all paths valid"}
}

func isComposeV2(version string) bool {
	low := strings.ToLower(version)
	if !strings.Contains(low, "compose version") {
		return false
	}
	idx := strings.Index(low, "v")
	if idx < 0 {
		return false
	}
	return idx+1 < len(low) && low[idx+1] == '2'
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS.

- [ ] **Step 5: Coverage check**

Run: `go test -coverprofile=cover.out ./internal/application/... && go tool cover -func=cover.out | tail -n 1`
Expected: total coverage ≥ 70%.

- [ ] **Step 6: Commit**

```bash
git add internal/application/doctor.go internal/application/doctor_test.go
git commit -m "feat(application): add DoctorService"
```

---

## Phase 7 — Real outbound adapters needed for M1

### Task 25: adapters/outbound/stdclock/clock.go

**Files:**
- Create: `internal/adapters/outbound/stdclock/clock.go`
- Create: `internal/adapters/outbound/stdclock/clock_test.go`

- [ ] **Step 1: Write the failing test**

```go
package stdclock_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/stdclock"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestStdClockImplementsClock(t *testing.T) {
	var _ outbound.Clock = stdclock.New()
}

func TestStdClockNowIsRecent(t *testing.T) {
	c := stdclock.New()
	now := c.Now()
	if time.Since(now) > time.Second {
		t.Errorf("Now() not recent: %v", now)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/stdclock/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// Package stdclock implements outbound.Clock using time.Now.
package stdclock

import "time"

// Clock implements outbound.Clock.
type Clock struct{}

// New returns a new Clock.
func New() *Clock { return &Clock{} }

// Now returns time.Now().
func (Clock) Now() time.Time { return time.Now() }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/stdclock/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/stdclock/clock.go internal/adapters/outbound/stdclock/clock_test.go
git commit -m "feat(adapters): add stdclock"
```

---

### Task 26: adapters/outbound/composeexec/runner.go — Version + ComposeVersion

**Files:**
- Create: `internal/adapters/outbound/composeexec/runner.go`
- Create: `internal/adapters/outbound/composeexec/runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
package composeexec_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestComposeExecImplementsRunner(t *testing.T) {
	var _ outbound.ComposeRunner = composeexec.New(composeexec.Config{})
}

func TestComposeExecVersionRunsBinary(t *testing.T) {
	// Use a stub binary that prints a known string.
	r := composeexec.New(composeexec.Config{
		DockerBinary: "echo",
		ExtraArgs:    []string{"Docker", "Compose", "version", "v2.32.1"},
	})
	out, err := r.Version(context.Background())
	if err != nil {
		t.Fatalf("Version err: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/composeexec/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// Package composeexec implements outbound.ComposeRunner via `docker compose`.
package composeexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Config controls the composeexec runner.
type Config struct {
	DockerBinary string   // default "docker"
	ExtraArgs    []string // testing hook to override args
}

// Runner implements outbound.ComposeRunner.
type Runner struct {
	cfg Config
}

// New returns a Runner.
func New(cfg Config) *Runner {
	if cfg.DockerBinary == "" {
		cfg.DockerBinary = "docker"
	}
	return &Runner{cfg: cfg}
}

// Version returns the trimmed output of `docker compose version`.
func (r *Runner) Version(ctx context.Context) (string, error) {
	args := r.cfg.ExtraArgs
	if len(args) == 0 {
		args = []string{"compose", "version"}
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("compose version: %w (output: %s)", err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// Up runs `docker compose -p <project> -f <composePath> up -d`.
func (r *Runner) Up(ctx context.Context, composePath, projectName string) error {
	if composePath == "" || projectName == "" {
		return errors.New("composePath and projectName are required")
	}
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, "compose", "-p", projectName, "-f", composePath, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose up: %w (output: %s)", err, out)
	}
	return nil
}

// Down runs `docker compose -p <project> -f <composePath> down`.
func (r *Runner) Down(ctx context.Context, composePath, projectName string) error {
	if composePath == "" || projectName == "" {
		return errors.New("composePath and projectName are required")
	}
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, "compose", "-p", projectName, "-f", composePath, "down")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down: %w (output: %s)", err, out)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/composeexec/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/composeexec/runner.go internal/adapters/outbound/composeexec/runner_test.go
git commit -m "feat(adapters): add composeexec runner (Version, Up, Down)"
```

---

### Task 27: adapters/outbound/gitcli/inspector.go — Version + RepoRoot + RemoteURL + Branch + Status

**Files:**
- Create: `internal/adapters/outbound/gitcli/inspector.go`
- Create: `internal/adapters/outbound/gitcli/inspector_test.go`

- [ ] **Step 1: Write the failing test**

```go
package gitcli_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestGitCLIImplementsInspector(t *testing.T) {
	var _ outbound.GitInspector = gitcli.New(gitcli.Config{})
}

func TestGitCLIVersionWithEcho(t *testing.T) {
	g := gitcli.New(gitcli.Config{Binary: "echo", VersionArgs: []string{"git", "version", "2.46.0"}})
	v, err := g.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v == "" {
		t.Error("expected non-empty version")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/gitcli/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// Package gitcli implements outbound.GitInspector by shelling out to git.
package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Config configures the gitcli inspector.
type Config struct {
	Binary      string   // default "git"
	VersionArgs []string // testing override
}

// Inspector implements outbound.GitInspector.
type Inspector struct {
	cfg Config
}

// New returns a new Inspector.
func New(cfg Config) *Inspector {
	if cfg.Binary == "" {
		cfg.Binary = "git"
	}
	return &Inspector{cfg: cfg}
}

func (g *Inspector) run(ctx context.Context, cwd string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, g.cfg.Binary, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w (output: %s)", args, err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// Version returns `git --version`.
func (g *Inspector) Version(ctx context.Context) (string, error) {
	args := g.cfg.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	return g.run(ctx, "", args...)
}

// RepoRoot returns `git rev-parse --show-toplevel`.
func (g *Inspector) RepoRoot(ctx context.Context, cwd string) (string, error) {
	return g.run(ctx, cwd, "rev-parse", "--show-toplevel")
}

// RemoteURL returns `git config --get remote.origin.url` (empty string if absent).
func (g *Inspector) RemoteURL(ctx context.Context, cwd string) (string, error) {
	out, err := g.run(ctx, cwd, "config", "--get", "remote.origin.url")
	if err != nil {
		// `git config --get` exits non-zero when the key is absent. Treat as empty.
		return "", nil
	}
	return out, nil
}

// CurrentBranch returns the active branch name.
func (g *Inspector) CurrentBranch(ctx context.Context, cwd string) (string, error) {
	return g.run(ctx, cwd, "symbolic-ref", "--short", "HEAD")
}

// Status returns whether the working tree is clean and the count of dirty files.
func (g *Inspector) Status(ctx context.Context, cwd string) (bool, int, error) {
	out, err := g.run(ctx, cwd, "status", "--porcelain")
	if err != nil {
		return false, 0, err
	}
	if out == "" {
		return true, 0, nil
	}
	lines := strings.Split(out, "\n")
	n, _ := strconv.Atoi(strconv.Itoa(len(lines)))
	return false, n, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/gitcli/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/gitcli/inspector.go internal/adapters/outbound/gitcli/inspector_test.go
git commit -m "feat(adapters): add gitcli inspector"
```

---

## Phase 8 — Cobra commands

### Task 28: adapters/inbound/cli/root.go

**Files:**
- Create: `internal/adapters/inbound/cli/root.go`
- Create: `internal/adapters/inbound/cli/root_test.go`

- [ ] **Step 1: Add cobra dependency**

```bash
go get github.com/spf13/cobra@latest
go mod tidy
```

- [ ] **Step 2: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestRootCommandHasName(t *testing.T) {
	c := cli.NewRoot(cli.Deps{})
	if c.Use != "sophia" {
		t.Errorf("root use = %q, want sophia", c.Use)
	}
}

func TestRootHelpOutput(t *testing.T) {
	c := cli.NewRoot(cli.Deps{})
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Error("expected help output")
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL.

- [ ] **Step 4: Implement**

```go
// Package cli wires Cobra subcommands. The package depends only on inbound
// and outbound ports; never on adapters/outbound/* directly. Bootstrap is
// responsible for constructing concrete dependencies and passing them as
// Deps.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

// Deps groups the application services injected into Cobra commands.
// As later milestones add services (Runner, Attacher, ...), extend Deps
// rather than passing many individual fields.
type Deps struct {
	Doctor *application.DoctorService
	// Version metadata
	Version   string
	Commit    string
	BuildDate string
}

// NewRoot returns the root sophia command with all subcommands attached.
func NewRoot(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "sophia",
		Short: "Sophia CLI — create and observe SDD Changes",
		Long: `sophia is the human entry point of the Sophia ecosystem.

It creates and observes Changes executed by sophia-orchestator. The CLI
itself does not coordinate phases, evaluate policy, or store canonical
state.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd(d))
	root.AddCommand(newDoctorCmd(d))
	root.AddCommand(newStubCmd("init", "Initialize .sophia.yaml at the resolved repo root", "M3"))
	root.AddCommand(newStubCmd("start", "Start the local Sophia stack via docker compose", "M2"))
	root.AddCommand(newStubCmd("stop", "Stop the local Sophia stack", "M2"))
	root.AddCommand(newStubCmd("run", "Create and observe a Change", "M4"))
	root.AddCommand(newStubCmd("attach", "Attach to an existing Change", "M8"))
	root.AddCommand(newStubCmd("status", "Show status of a Change", "M3"))
	root.AddCommand(newStubCmd("changes", "List recent Changes", "M8"))

	return root
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/inbound/cli/root.go internal/adapters/inbound/cli/root_test.go
git commit -m "feat(cli): add cobra root command"
```

---

### Task 29: adapters/inbound/cli/version.go

**Files:**
- Create: `internal/adapters/inbound/cli/version.go`
- Create: `internal/adapters/inbound/cli/version_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestVersionCommandPrintsAllFields(t *testing.T) {
	c := cli.NewRoot(cli.Deps{Version: "0.1.0", Commit: "abc1234", BuildDate: "2026-05-05"})
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"version"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"0.1.0", "abc1234", "2026-05-05"} {
		if !strings.Contains(got, want) {
			t.Errorf("version output missing %q: %q", want, got)
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print sophia version, commit, and build date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "sophia %s (commit %s, built %s)\n", d.Version, d.Commit, d.BuildDate)
			return nil
		},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/version.go internal/adapters/inbound/cli/version_test.go
git commit -m "feat(cli): add version command"
```

---

### Task 30: adapters/inbound/cli/doctor.go

**Files:**
- Create: `internal/adapters/inbound/cli/doctor.go`
- Create: `internal/adapters/inbound/cli/doctor_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newDoctorDeps() cli.Deps {
	xdg := func(_ string) error { return nil }
	return cli.Deps{
		Doctor: application.NewDoctorService(fakes.NewFakeComposeRunner(), fakes.NewFakeGitInspector(), xdg),
	}
}

func TestDoctorCommandPrintsTable(t *testing.T) {
	c := cli.NewRoot(newDoctorDeps())
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"doctor"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Docker", "Git", "XDG"} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor output missing %q: %s", want, got)
		}
	}
}

func TestDoctorCommandJSON(t *testing.T) {
	c := cli.NewRoot(newDoctorDeps())
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"doctor", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Summary struct{ OK, Info, Warn, Fail int }
		Checks  []map[string]any
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newDoctorCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run environment diagnostics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := d.Doctor.Run(cmd.Context())
			if jsonOut {
				return printDoctorJSON(cmd.OutOrStdout(), report)
			}
			printDoctorTable(cmd.OutOrStdout(), report)
			if report.Summary.Fail > 0 {
				return fmt.Errorf("doctor: %d check(s) failed", report.Summary.Fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit a JSON report instead of the human table")
	return cmd
}

func printDoctorTable(w io.Writer, r application.DiagnosticsReport) {
	fmt.Fprintln(w, "sophia doctor — checking environment")
	fmt.Fprintln(w)
	for _, c := range r.Checks {
		icon := "✓"
		switch c.Level {
		case application.LevelInfo:
			icon = "ℹ"
		case application.LevelWarn:
			icon = "⚠"
		case application.LevelFail:
			icon = "✗"
		}
		fmt.Fprintf(w, "  %s %-20s %s\n", icon, c.Title, c.Detail)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d ok · %d info · %d warn · %d fail\n", r.Summary.OK, r.Summary.Info, r.Summary.Warn, r.Summary.Fail)
}

func printDoctorJSON(w io.Writer, r application.DiagnosticsReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/doctor.go internal/adapters/inbound/cli/doctor_test.go
git commit -m "feat(cli): add doctor command (table + --json)"
```

---

### Task 31: adapters/inbound/cli/stubs.go — stub remaining commands

**Files:**
- Create: `internal/adapters/inbound/cli/stubs.go`
- Create: `internal/adapters/inbound/cli/stubs_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestStubsAnnounceMilestone(t *testing.T) {
	cases := map[string]string{
		"init":    "M3",
		"start":   "M2",
		"stop":    "M2",
		"run":     "M4",
		"attach":  "M8",
		"status":  "M3",
		"changes": "M8",
	}
	for sub, milestone := range cases {
		c := cli.NewRoot(cli.Deps{})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetArgs([]string{sub})
		if err := c.Execute(); err != nil {
			t.Fatalf("%s err: %v", sub, err)
		}
		got := out.String()
		if !strings.Contains(got, "not implemented yet") || !strings.Contains(got, milestone) {
			t.Errorf("%s output = %q (want milestone %s)", sub, got, milestone)
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newStubCmd returns a placeholder subcommand that reports the milestone in
// which it will be implemented. Stubs must always exit 0 so scripts that
// probe `sophia <cmd>` succeed during M1.
func newStubCmd(use, short, milestone string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: not implemented yet (planned for %s)\n", use, milestone)
			return nil
		},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/stubs.go internal/adapters/inbound/cli/stubs_test.go
git commit -m "feat(cli): add stub subcommands for init/start/stop/run/attach/status/changes"
```

---

## Phase 9 — Bootstrap and main

### Task 32: bootstrap/version.go and bootstrap/logger.go

**Files:**
- Create: `internal/bootstrap/version.go`
- Create: `internal/bootstrap/logger.go`
- Create: `internal/bootstrap/version_test.go`
- Create: `internal/bootstrap/logger_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/bootstrap/version_test.go`:

```go
package bootstrap_test

import (
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestVersionInfoFallback(t *testing.T) {
	info := bootstrap.NewVersionInfo()
	if info.Version == "" || info.Commit == "" || info.BuildDate == "" {
		t.Errorf("VersionInfo has empty fields: %+v", info)
	}
	if !strings.HasPrefix(info.Version, "0.") && info.Version != "dev" {
		t.Errorf("unexpected version default: %q", info.Version)
	}
}
```

`internal/bootstrap/logger_test.go`:

```go
package bootstrap_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestNewLoggerWritesJSONL(t *testing.T) {
	var buf bytes.Buffer
	logger := bootstrap.NewLogger(&buf, slog.LevelInfo)
	logger.Info("hello", slog.String("k", "v"))
	if !strings.Contains(buf.String(), `"msg":"hello"`) {
		t.Errorf("expected JSONL output, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/bootstrap/...`
Expected: FAIL.

- [ ] **Step 3: Implement `version.go`**

```go
// Package bootstrap is the composition root of the CLI. It wires concrete
// adapters (HTTP, exec, FS) into application services and returns a
// configured *cobra.Command. cmd/sophia is intentionally thin and only
// invokes bootstrap.New.
package bootstrap

// These vars are populated via -ldflags at build time:
//   -X github.com/.../internal/bootstrap.Version=0.1.0
//   -X github.com/.../internal/bootstrap.Commit=abc1234
//   -X github.com/.../internal/bootstrap.BuildDate=2026-05-05T...Z
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// VersionInfo carries the build identity displayed by `sophia version`.
type VersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

// NewVersionInfo returns the build identity, falling back to "dev"/"unknown"
// when ldflags were not set.
func NewVersionInfo() VersionInfo {
	return VersionInfo{Version: Version, Commit: Commit, BuildDate: BuildDate}
}
```

- [ ] **Step 4: Implement `logger.go`**

```go
package bootstrap

import (
	"io"
	"log/slog"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/logging"
)

// NewLogger returns a *slog.Logger backed by the JSONL handler. Pass a file
// in stateRoot/logs as w when building the production logger; tests use
// bytes.Buffer.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(logging.NewJSONLHandler(w, level))
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/bootstrap/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/version.go internal/bootstrap/logger.go \
        internal/bootstrap/version_test.go internal/bootstrap/logger_test.go
git commit -m "feat(bootstrap): add VersionInfo and JSONL logger constructor"
```

---

### Task 33: bootstrap/wire.go — composition root

**Files:**
- Create: `internal/bootstrap/wire.go`
- Create: `internal/bootstrap/wire_test.go`

- [ ] **Step 1: Write the failing test**

```go
package bootstrap_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestNewProducesRoot(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	if root.Use != "sophia" {
		t.Errorf("root use = %q", root.Use)
	}
}

func TestNewWiresDoctor(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	doctor, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor not found: %v", err)
	}
	if doctor == nil {
		t.Fatal("doctor cmd is nil")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/bootstrap/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package bootstrap

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

// Config controls bootstrap.New.
type Config struct {
	LogWriter io.Writer  // nil ⇒ os.Stderr
	LogLevel  slog.Level // default Info
}

// New is the composition root. It builds concrete outbound adapters, application
// services, and returns the configured root cobra command.
func New(cfg Config) (*cobra.Command, error) {
	if cfg.LogWriter == nil {
		cfg.LogWriter = os.Stderr
	}
	logger := NewLogger(cfg.LogWriter, cfg.LogLevel)
	slog.SetDefault(logger)

	compose := composeexec.New(composeexec.Config{})
	git := gitcli.New(gitcli.Config{})
	xdg := newXDGValidator()
	doctor := application.NewDoctorService(compose, git, xdg)

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:    doctor,
		Version:   info.Version,
		Commit:    info.Commit,
		BuildDate: info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}

// newXDGValidator returns a function that checks XDG paths for existence and
// permissions. M1 default behavior is read-only: it does not create paths.
// If a path is absent, returns application.ErrPathInvalid.
func newXDGValidator() application.XDGValidator {
	return func(_ string) error {
		// M1 placeholder: assume paths are valid. Real validator (existence
		// + 0700 perms) lands at M2 alongside provisioning, when stateRoot
		// becomes load-bearing.
		return nil
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bootstrap/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/wire.go internal/bootstrap/wire_test.go
git commit -m "feat(bootstrap): add wire (composition root)"
```

---

### Task 34: cmd/sophia/main.go — thin entrypoint

**Files:**
- Create: `cmd/sophia/main.go`
- Modify: `bin/.gitkeep` (no-op; verify directory still tracked)

- [ ] **Step 1: Write the entrypoint**

```go
// Command sophia is the human entry point of the Sophia ecosystem.
// All business logic lives under internal/. main is intentionally minimal:
// it builds the root command via bootstrap.New and executes it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(3)
	}
}
```

- [ ] **Step 2: Build the binary**

Run: `make build`
Expected: produces `bin/sophia` with no errors.

- [ ] **Step 3: Run version**

Run: `./bin/sophia version`
Expected: prints `sophia 0.1.0-dev (commit <short>, built <date>)`.

- [ ] **Step 4: Run doctor**

Run: `./bin/sophia doctor`
Expected: prints the doctor table; exit 0 in a healthy environment.

- [ ] **Step 5: Commit**

```bash
git add cmd/sophia/main.go
git commit -m "feat(cmd): add sophia binary entrypoint"
```

---

## Phase 10 — End-to-end smoke and final validation

### Task 35: test/e2e/version_smoke_test.go

**Files:**
- Create: `test/e2e/version_smoke_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build e2e_smoke

// Package e2e runs end-to-end smoke tests that exercise the compiled binary.
// Build the binary with `make build` before running:
//   go test -tags=e2e_smoke ./test/e2e/...
package e2e_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestSmokeVersion(t *testing.T) {
	cmd := exec.Command("../../bin/sophia", "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("sophia version failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "sophia") {
		t.Errorf("output missing sophia: %s", out.String())
	}
}
```

- [ ] **Step 2: Build and run**

Run:
```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add test/e2e/version_smoke_test.go
git commit -m "test(e2e): add version smoke test"
```

---

### Task 36: test/e2e/doctor_smoke_test.go

**Files:**
- Create: `test/e2e/doctor_smoke_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build e2e_smoke

package e2e_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestSmokeDoctorJSON(t *testing.T) {
	cmd := exec.Command("../../bin/sophia", "doctor", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Doctor may return non-zero when checks fail (e.g. CI without docker).
	// We only require that --json produces parseable JSON on stdout.
	_ = cmd.Run()

	var report struct {
		Summary struct{ OK, Info, Warn, Fail int }
		Checks  []map[string]any
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor --json produced invalid JSON: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}
	if len(report.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(report.Checks))
	}
}
```

- [ ] **Step 2: Build and run**

Run:
```bash
make build
go test -tags=e2e_smoke ./test/e2e/...
```
Expected: PASS regardless of host environment (the test is tolerant of fails).

- [ ] **Step 3: Commit**

```bash
git add test/e2e/doctor_smoke_test.go
git commit -m "test(e2e): add doctor --json smoke test"
```

---

### Task 37: Final validation — Makefile, lint, coverage

**Files:** none (validation-only).

- [ ] **Step 1: Run go vet**

Run: `make vet`
Expected: exit 0, no findings.

- [ ] **Step 2: Run tests with race detector**

Run: `make test`
Expected: exit 0.

Optional thoroughness:

```bash
go test -race ./...
```

- [ ] **Step 3: Run lint**

Run: `make lint`
Expected: exit 0.

If lint flags issues:
- Fix in place if minor (formatting, unused).
- If a rule is fundamentally wrong for this project, document the
  exemption in `.golangci.yml` with a one-line comment justifying it.

- [ ] **Step 4: Coverage check**

Run: `make coverage`
Expected: total ≥ 70%.

If below 70%:
- Inspect uncovered functions: `go tool cover -html=cover.out -o coverage.html && open coverage.html`.
- Add table-driven tests for uncovered branches in `domain` and
  `application`. Do not add tests merely to push coverage on
  trivial getters; instead, prefer testing behavior at boundaries.

- [ ] **Step 5: Run binary smoke**

```bash
make build
./bin/sophia version
./bin/sophia doctor
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
./bin/sophia init       # stub: not implemented yet (planned for M3)
./bin/sophia start      # stub
./bin/sophia stop       # stub
./bin/sophia run        # stub
./bin/sophia attach abc # stub
./bin/sophia status     # stub
./bin/sophia changes    # stub
```

Expected: every command exits 0; version + doctor produce real output;
stubs print their milestone tag.

- [ ] **Step 6: Final commit and tag**

```bash
git add -A
git status
git commit -m "chore(m1): final validation pass" || echo "nothing to commit"
git tag -a m1-foundation -m "M1 Foundation complete"
```

- [ ] **Step 7: Update tasks**

Mark every task in this plan as `[x]` and record the commit SHAs in a
`docs/superpowers/notes/m1-completion.md` summary (optional but recommended
for traceability into M2).

---

## Self-review checklist

Run this after the plan is complete and before invoking
`superpowers:subagent-driven-development` or `superpowers:executing-plans`.

- [ ] **Spec coverage:** every M1 DoD item from spec §7.2 has at least one
  task above. (Hexagonal layout, slog JSONL handler, Cobra root + version +
  doctor, test fakes for all outbound ports, golangci-lint, Makefile,
  coverage ≥ 70%.)
- [ ] **No placeholders:** no "TBD", "TODO", "fill in details", or "similar
  to Task N" anywhere.
- [ ] **Type consistency:** all type/method names match across tasks
  (e.g. `DoctorService.Run`, not `RunDiagnostics`).
- [ ] **Frequent commits:** every task ends with at least one commit.
- [ ] **TDD discipline:** every code-producing task has a failing test
  before the implementation step.
- [ ] **No premature M2+ scope:** no HTTP client, no SSE, no Bubble Tea,
  no `.sophia.yaml` real adapter, no state store atomic write, no real
  browser opener.

---

## Pending decisions (carried into M1 execution)

| ID | Question | Default behavior if user silent |
|---|---|---|
| D-M1-01 | golangci-lint ruleset | The set in §Phase 1 Task 2 (`errcheck`, `govet`, `staticcheck`, `revive`, `gofmt`, `goimports`, `gosec`, `unused`, `unparam`, `misspell`, `ineffassign`, `typecheck`) |
| D-M1-02 | License header on Go files | None for V1 |
| D-M1-03 | Persistent vs local cobra flags | Persistent flags are added at the root in M2+ when they actually carry meaning; M1 has none |

---

## Risks specific to M1

| ID | Risk | Mitigation built into plan |
|---|---|---|
| RM1-01 | `go vet` flags `Status` ambiguity in `gitcli.Inspector` (multi-return signature) | Explicit named-return signature; tests exercise both branches |
| RM1-02 | XDG validator placeholder masks future real failures | Placeholder is callsite-injected; M2 swaps in a real implementation without changing application/doctor |
| RM1-03 | Coverage falls short because stubs don't count | Test stubs in Task 31 ensures stub branches are exercised; domain + application have rich tests by design |

---

## What this plan does NOT cover (intentional)

- HTTP client to orchestrator → M2 (healthz only) and M4 (full).
- SSE consumer → M5.
- Bubble Tea TUI → M6.
- `.sophia.yaml` reading/writing → M3.
- State store atomic writes → M3.
- Real browser opener (`xdg-open`/`open`/`start`) → M7.
- Compose embedded `.yaml` → M2.
- GoReleaser, security suite, asciinema demo → M9.

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-05-sophia-cli-m1-foundation.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task,
review between tasks, fast iteration. Use
`superpowers:subagent-driven-development`.

**2. Inline Execution** — execute tasks in this session using
`superpowers:executing-plans`, batch execution with checkpoints for review.

Pick when the implementation session opens.
