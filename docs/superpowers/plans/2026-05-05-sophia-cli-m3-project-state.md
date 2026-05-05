# Sophia CLI — M3 Project & State Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `.sophia.yaml` reading/writing via `gopkg.in/yaml.v3`, project-fingerprinted state with atomic writes, `sophia init` and a placeholder `sophia status` (no orchestrator call yet — that lands in M8). End state: a fresh repo can run `sophia init`, `cat .sophia.yaml`, `sophia status` (empty-state), and `sophia init --force` to repair invalid YAML.

**Architecture:** Three new outbound adapters: `yamlconfig` (project + user YAML I/O via afero), `filestate` (atomic temp+rename writer keyed by fingerprint), and `gitcli.RepoRoot` already exists from M1 (verify only). Two new application services: `Initializer` (resolves repo root, computes fingerprint, writes config + meta) and `StatusReader` (resolves last_change_id from project → global → empty). Two cobra commands replace the M3 stubs.

**Tech Stack:** Go 1.24.x · `gopkg.in/yaml.v3` (new) · `github.com/spf13/afero` (test-only, new) · `crypto/sha256` (stdlib, already used) · `path/filepath` (Abs/Clean/EvalSymlinks).

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§2.1, §2.5, §3.1, §3.2, §3.3, §3.4, §3.5, §6.3, §7.2 M3 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M3)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M3 boundaries — what is NOT in M3:**

- No HTTP call to orchestrator from `status` — placeholder only (real M8).
- No `sophia run` / `attach` / `changes` — those are M4 / M5 / M8.
- No SSE — M5.
- No TUI — M6.
- No `--orchestrator-url` flag plumbing into commands beyond bootstrap — M4.
- No environment variable layering (`SOPHIA_PROJECT` etc.) — that lands when there's a use case (M4 minimum).

---

## Phase 1 — Dependencies

### Task 1: Add yaml.v3 and afero dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add deps**

```bash
cd /Users/russell/Documents/2026/sophia-cli
go get gopkg.in/yaml.v3@latest
go get github.com/spf13/afero@latest
go mod tidy
```

Expected: `go.mod` lists `gopkg.in/yaml.v3` and `github.com/spf13/afero` with their pinned versions; `go.sum` updated.

- [ ] **Step 2: Verify compile**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add yaml.v3 and afero deps for M3"
```

---

## Phase 2 — yamlconfig adapter (project + user)

### Task 2: yamlconfig DTOs

**Files:**
- Create: `internal/adapters/outbound/yamlconfig/dto.go`
- Create: `internal/adapters/outbound/yamlconfig/dto_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/outbound/yamlconfig/dto_test.go`:
```go
package yamlconfig_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestProjectDTORoundTrip(t *testing.T) {
	in := &domain.ProjectConfig{
		Version:       1,
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	}
	dto := yamlconfig.ToProjectDTO(in)
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back yamlconfig.ProjectDTO
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	got := yamlconfig.FromProjectDTO(&back)
	if got.Project != in.Project || got.BaseRef != in.BaseRef || got.ArtifactStore != in.ArtifactStore {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestUserDTORoundTrip(t *testing.T) {
	in := &domain.UserConfig{
		Version:         1,
		OrchestratorURL: "http://localhost:9080",
		TimeoutSeconds:  30,
	}
	dto := yamlconfig.ToUserDTO(in)
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back yamlconfig.UserDTO
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	got := yamlconfig.FromUserDTO(&back)
	if got.OrchestratorURL != in.OrchestratorURL || got.TimeoutSeconds != in.TimeoutSeconds {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestProjectDTOArtifactStoreYAMLKey(t *testing.T) {
	dto := yamlconfig.ProjectDTO{
		Version:       1,
		Project:       "p",
		BaseRef:       "main",
		ArtifactStore: "engram",
	}
	out, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) == "" {
		t.Fatal("empty yaml")
	}
	// Verify the YAML key is `artifact_store` (snake_case), not `artifactstore`.
	if !contains(string(out), "artifact_store: engram") {
		t.Errorf("yaml does not use artifact_store key: %s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/yamlconfig/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/yamlconfig/dto.go`:
```go
// Package yamlconfig implements outbound.ProjectConfigStore and
// outbound.UserConfigStore using gopkg.in/yaml.v3 over an afero filesystem.
//
// DTOs are kept private to this package: the domain layer never sees yaml
// tags. Conversion functions translate between the wire format and the
// pure domain types.
package yamlconfig

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// ProjectDTO is the on-disk shape of `.sophia.yaml`.
type ProjectDTO struct {
	Version       int    `yaml:"version"`
	Project       string `yaml:"project"`
	BaseRef       string `yaml:"base_ref"`
	ArtifactStore string `yaml:"artifact_store"`
}

// UserDTO is the on-disk shape of `<configRoot>/config.yaml`.
type UserDTO struct {
	Version      int            `yaml:"version"`
	Orchestrator OrchestratorBlock `yaml:"orchestrator"`
}

// OrchestratorBlock is the orchestrator subtree of UserDTO.
type OrchestratorBlock struct {
	URL            string `yaml:"url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// ToProjectDTO converts a domain config to its DTO.
func ToProjectDTO(c *domain.ProjectConfig) ProjectDTO {
	return ProjectDTO{
		Version:       c.Version,
		Project:       c.Project,
		BaseRef:       c.BaseRef,
		ArtifactStore: string(c.ArtifactStore),
	}
}

// FromProjectDTO converts a DTO to the domain type.
func FromProjectDTO(d *ProjectDTO) *domain.ProjectConfig {
	return &domain.ProjectConfig{
		Version:       d.Version,
		Project:       d.Project,
		BaseRef:       d.BaseRef,
		ArtifactStore: domain.ArtifactStoreMode(d.ArtifactStore),
	}
}

// ToUserDTO converts a domain UserConfig to DTO.
func ToUserDTO(c *domain.UserConfig) UserDTO {
	return UserDTO{
		Version: c.Version,
		Orchestrator: OrchestratorBlock{
			URL:            c.OrchestratorURL,
			TimeoutSeconds: c.TimeoutSeconds,
		},
	}
}

// FromUserDTO converts a DTO to UserConfig.
func FromUserDTO(d *UserDTO) *domain.UserConfig {
	return &domain.UserConfig{
		Version:         d.Version,
		OrchestratorURL: d.Orchestrator.URL,
		TimeoutSeconds:  d.Orchestrator.TimeoutSeconds,
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/yamlconfig/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/yamlconfig/dto.go internal/adapters/outbound/yamlconfig/dto_test.go
git commit -m "feat(yamlconfig): add ProjectDTO/UserDTO with domain conversions"
```

---

### Task 3: yamlconfig.ProjectStore — Read/Write/Find with afero

**Files:**
- Create: `internal/adapters/outbound/yamlconfig/project.go`
- Create: `internal/adapters/outbound/yamlconfig/project_test.go`

- [ ] **Step 1: Write the failing test**

```go
package yamlconfig_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/spf13/afero"
)

func TestProjectStoreImplementsPort(t *testing.T) {
	var _ outbound.ProjectConfigStore = yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: afero.NewMemMapFs()})
}

func TestProjectStoreWriteThenRead(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})

	cfg := &domain.ProjectConfig{
		Version:       1,
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	}
	path := "/repo/.sophia.yaml"
	if err := s.Write(context.Background(), path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "ms-cotizacion" || got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestProjectStoreReadMissingReturnsErrConfigMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != domain.ErrConfigMissing {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestProjectStoreReadInvalidYAMLReturnsErrInvalidYAML(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/repo/.sophia.yaml", []byte("::not yaml::"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Read(context.Background(), "/repo/.sophia.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != domain.ErrInvalidYAML {
		t.Errorf("expected ErrInvalidYAML, got %v", err)
	}
}

func TestProjectStoreReadRejectsOversizedFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	big := make([]byte, 101*1024) // 101KB > 100KB cap
	for i := range big {
		big[i] = 'a'
	}
	if err := afero.WriteFile(fs, "/repo/.sophia.yaml", big, 0o644); err != nil {
		t.Fatal(err)
	}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	if _, err := s.Read(context.Background(), "/repo/.sophia.yaml"); err == nil {
		t.Error("expected size cap error")
	}
}

func TestProjectStoreFindWalksAncestors(t *testing.T) {
	fs := afero.NewMemMapFs()
	cfg := &domain.ProjectConfig{Version: 1, Project: "p", BaseRef: "main", ArtifactStore: domain.ArtifactStoreEngram}
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	if err := s.Write(context.Background(), "/repo/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	startDir := filepath.Join("/repo", "sub", "deeper")
	if err := fs.MkdirAll(startDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find(context.Background(), startDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/repo/.sophia.yaml" {
		t.Errorf("Find = %q", got)
	}
}

func TestProjectStoreFindReturnsErrNotFound(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	_, err := s.Find(context.Background(), "/nowhere/sub")
	if err == nil {
		t.Error("expected error")
	}
}

func TestProjectStoreWriteCreatesParentDirs(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{FS: fs})
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	if err := s.Write(context.Background(), "/new/path/.sophia.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	exists, _ := afero.Exists(fs, "/new/path/.sophia.yaml")
	if !exists {
		t.Error("expected file written under /new/path")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/yamlconfig/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/yamlconfig/project.go`:
```go
package yamlconfig

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// MaxProjectYAMLSize is the spec §6.3 cap (100 KB) for `.sophia.yaml`.
const MaxProjectYAMLSize = 100 * 1024

// ProjectConfig configures the ProjectStore.
type ProjectConfig struct {
	FS afero.Fs // optional; defaults to OS FS
}

// ProjectStore implements outbound.ProjectConfigStore.
type ProjectStore struct {
	fs afero.Fs
}

// NewProjectStore returns a ProjectStore.
func NewProjectStore(cfg ProjectConfig) *ProjectStore {
	fs := cfg.FS
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &ProjectStore{fs: fs}
}

// Read returns the parsed `.sophia.yaml`. Maps not-found to ErrConfigMissing
// and parse errors to ErrInvalidYAML.
func (s *ProjectStore) Read(_ context.Context, path string) (*domain.ProjectConfig, error) {
	exists, err := afero.Exists(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !exists {
		return nil, domain.ErrConfigMissing
	}
	f, err := s.fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	limited := io.LimitReader(f, MaxProjectYAMLSize+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	if len(b) > MaxProjectYAMLSize {
		return nil, fmt.Errorf("file %q exceeds %d bytes", path, MaxProjectYAMLSize)
	}

	var dto ProjectDTO
	dec := yaml.NewDecoder(bytesReader(b))
	dec.KnownFields(false)
	if err := dec.Decode(&dto); err != nil {
		return nil, domain.ErrInvalidYAML
	}
	return FromProjectDTO(&dto), nil
}

// Write persists cfg to path with mode 0644 (non-sensitive). Parent dirs
// are created with 0700 if missing.
func (s *ProjectStore) Write(_ context.Context, path string, cfg *domain.ProjectConfig) error {
	dto := ToProjectDTO(cfg)
	b, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." && dir != "/" {
		if err := s.fs.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("mkdir %q: %w", dir, err)
		}
	}
	return afero.WriteFile(s.fs, path, b, 0o644)
}

// Find walks ancestors of startDir looking for a `.sophia.yaml`.
func (s *ProjectStore) Find(_ context.Context, startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		candidate := filepath.Join(dir, ".sophia.yaml")
		exists, err := afero.Exists(s.fs, candidate)
		if err == nil && exists {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .sophia.yaml found above %q", startDir)
}

func bytesReader(b []byte) *bytesReaderImpl {
	return &bytesReaderImpl{b: b}
}

type bytesReaderImpl struct {
	b []byte
	i int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/yamlconfig/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/yamlconfig/project.go internal/adapters/outbound/yamlconfig/project_test.go
git commit -m "feat(yamlconfig): add ProjectStore with afero (Read/Write/Find, 100KB cap)"
```

---

### Task 4: yamlconfig.UserStore — Read/Write

**Files:**
- Create: `internal/adapters/outbound/yamlconfig/user.go`
- Create: `internal/adapters/outbound/yamlconfig/user_test.go`

- [ ] **Step 1: Write the failing test**

```go
package yamlconfig_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/spf13/afero"
)

func TestUserStoreImplementsPort(t *testing.T) {
	var _ outbound.UserConfigStore = yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: afero.NewMemMapFs()})
}

func TestUserStoreRoundTrip(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	cfg := &domain.UserConfig{Version: 1, OrchestratorURL: "http://localhost:9080", TimeoutSeconds: 30}
	if err := s.Write(context.Background(), "/cfg/config.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(context.Background(), "/cfg/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.OrchestratorURL != "http://localhost:9080" || got.TimeoutSeconds != 30 {
		t.Errorf("round-trip lost: %+v", got)
	}
}

func TestUserStoreReadMissing(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	_, err := s.Read(context.Background(), "/cfg/config.yaml")
	if err != domain.ErrConfigMissing {
		t.Errorf("expected ErrConfigMissing, got %v", err)
	}
}

func TestUserStoreWriteUses0600(t *testing.T) {
	fs := afero.NewMemMapFs()
	s := yamlconfig.NewUserStore(yamlconfig.UserConfig{FS: fs})
	cfg := &domain.UserConfig{Version: 1, OrchestratorURL: "http://x", TimeoutSeconds: 5}
	if err := s.Write(context.Background(), "/cfg/config.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	fi, err := fs.Stat("/cfg/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 0600", perm)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/yamlconfig/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/yamlconfig/user.go`:
```go
package yamlconfig

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// UserConfig configures the UserStore.
type UserConfig struct {
	FS afero.Fs
}

// UserStore implements outbound.UserConfigStore.
//
// Per spec §3.1: user config is sensitive (may contain orchestrator URL with
// credentials in V1.1+) and is written 0600.
type UserStore struct {
	fs afero.Fs
}

// NewUserStore returns a UserStore.
func NewUserStore(cfg UserConfig) *UserStore {
	fs := cfg.FS
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &UserStore{fs: fs}
}

// Read returns the parsed user config or ErrConfigMissing.
func (s *UserStore) Read(_ context.Context, path string) (*domain.UserConfig, error) {
	exists, err := afero.Exists(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !exists {
		return nil, domain.ErrConfigMissing
	}
	b, err := afero.ReadFile(s.fs, path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var dto UserDTO
	if err := yaml.Unmarshal(b, &dto); err != nil {
		return nil, domain.ErrInvalidYAML
	}
	return FromUserDTO(&dto), nil
}

// Write persists cfg to path with mode 0600.
func (s *UserStore) Write(_ context.Context, path string, cfg *domain.UserConfig) error {
	dto := ToUserDTO(cfg)
	b, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." && dir != "/" {
		if err := s.fs.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("mkdir %q: %w", dir, err)
		}
	}
	return afero.WriteFile(s.fs, path, b, 0o600)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/yamlconfig/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/yamlconfig/user.go internal/adapters/outbound/yamlconfig/user_test.go
git commit -m "feat(yamlconfig): add UserStore (0600 perms)"
```

---

## Phase 3 — filestate (atomic state writer)

### Task 5: filestate.Store — atomic temp+rename

**Files:**
- Create: `internal/adapters/outbound/filestate/store.go`
- Create: `internal/adapters/outbound/filestate/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
package filestate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func newStore(t *testing.T) (*filestate.Store, string) {
	t.Helper()
	tmp := t.TempDir()
	return filestate.New(filestate.Config{StateRoot: tmp}), tmp
}

func TestStoreImplementsPort(t *testing.T) {
	var _ outbound.StateStore = filestate.New(filestate.Config{StateRoot: "/tmp"})
}

func TestProjectScopedRoundTrip(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("abc123def456")

	id, err := s.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if !id.IsZero() {
		t.Error("fresh store should return zero id")
	}

	if err := s.SetLast(context.Background(), fp, "01HX..."); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got != "01HX..." {
		t.Errorf("GetLast = %q", got)
	}

	// Verify file location.
	want := filepath.Join(root, "projects", string(fp), "last_change_id")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %q: %v", want, err)
	}
}

func TestGlobalRoundTrip(t *testing.T) {
	s, root := newStore(t)

	if err := s.SetGlobalLast(context.Background(), "g1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGlobalLast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "g1" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "last_change_id")); err != nil {
		t.Errorf("expected global file: %v", err)
	}
}

func TestSetLastIsAtomic(t *testing.T) {
	// We can't easily simulate kill -9, but we can verify the implementation
	// uses a temp file in the same directory and renames into place.
	s, root := newStore(t)
	fp := domain.Fingerprint("fp1")
	if err := s.SetLast(context.Background(), fp, "01H..."); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "projects", string(fp))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "last_change_id.tmp" {
			t.Error("temp file leftover after SetLast")
		}
	}
}

func TestDirectoriesUse0700(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("fp700")
	_ = s.SetLast(context.Background(), fp, "01H")
	dir := filepath.Join(root, "projects", string(fp))
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

func TestMetaJSONWritten(t *testing.T) {
	s, root := newStore(t)
	fp := domain.Fingerprint("fpmeta")
	meta := filestate.ProjectMeta{
		Project:   "ms-cotizacion",
		RepoRoot:  "/home/u/repo",
		RemoteURL: "git@github.com:org/repo.git",
	}
	if err := s.WriteProjectMeta(context.Background(), fp, meta); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadProjectMeta(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != meta.Project || got.RemoteURL != meta.RemoteURL {
		t.Errorf("meta round-trip lost: %+v", got)
	}
	// Verify file exists.
	if _, err := os.Stat(filepath.Join(root, "projects", string(fp), "meta.json")); err != nil {
		t.Errorf("expected meta.json: %v", err)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/filestate/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/filestate/store.go`:
```go
// Package filestate implements outbound.StateStore using `<stateRoot>/`
// with atomic writes (temp file + os.Rename) per spec §3.5.
package filestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the Store.
type Config struct {
	// StateRoot is the resolved <stateRoot> path (typically XDGPaths.StateRoot).
	StateRoot string
}

// Store implements outbound.StateStore.
type Store struct {
	root string
}

// New returns a Store rooted at cfg.StateRoot.
func New(cfg Config) *Store { return &Store{root: cfg.StateRoot} }

// ProjectMeta is the JSON payload at <stateRoot>/projects/<fp>/meta.json.
type ProjectMeta struct {
	Project   string    `json:"project"`
	RepoRoot  string    `json:"repo_root"`
	RemoteURL string    `json:"remote_url"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) projectDir(fp domain.Fingerprint) string {
	return filepath.Join(s.root, "projects", string(fp))
}

func (s *Store) projectFile(fp domain.Fingerprint) string {
	return filepath.Join(s.projectDir(fp), "last_change_id")
}

func (s *Store) globalFile() string {
	return filepath.Join(s.root, "last_change_id")
}

func (s *Store) metaFile(fp domain.Fingerprint) string {
	return filepath.Join(s.projectDir(fp), "meta.json")
}

// GetLast returns the project-scoped last change id (zero if absent).
func (s *Store) GetLast(_ context.Context, fp domain.Fingerprint) (domain.ChangeID, error) {
	return readChangeID(s.projectFile(fp))
}

// SetLast atomically persists id under the project scope.
func (s *Store) SetLast(_ context.Context, fp domain.Fingerprint, id domain.ChangeID) error {
	if err := os.MkdirAll(s.projectDir(fp), 0o700); err != nil {
		return fmt.Errorf("mkdir project: %w", err)
	}
	return atomicWrite(s.projectFile(fp), []byte(id), 0o644)
}

// GetGlobalLast returns the global fallback last change id.
func (s *Store) GetGlobalLast(_ context.Context) (domain.ChangeID, error) {
	return readChangeID(s.globalFile())
}

// SetGlobalLast atomically persists id at the global scope.
func (s *Store) SetGlobalLast(_ context.Context, id domain.ChangeID) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("mkdir state root: %w", err)
	}
	return atomicWrite(s.globalFile(), []byte(id), 0o644)
}

// WriteProjectMeta persists meta.json under projects/<fp>/.
func (s *Store) WriteProjectMeta(_ context.Context, fp domain.Fingerprint, m ProjectMeta) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(s.projectDir(fp), 0o700); err != nil {
		return fmt.Errorf("mkdir project: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return atomicWrite(s.metaFile(fp), b, 0o644)
}

// ReadProjectMeta loads meta.json under projects/<fp>/.
func (s *Store) ReadProjectMeta(_ context.Context, fp domain.Fingerprint) (ProjectMeta, error) {
	b, err := os.ReadFile(s.metaFile(fp))
	if err != nil {
		return ProjectMeta{}, fmt.Errorf("read meta: %w", err)
	}
	var m ProjectMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return ProjectMeta{}, fmt.Errorf("unmarshal meta: %w", err)
	}
	return m, nil
}

func readChangeID(path string) (domain.ChangeID, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return domain.ChangeID(strings.TrimSpace(string(b))), nil
}

// atomicWrite writes b to path via a temp file in the same directory plus
// os.Rename. This is durable on POSIX FS for power loss within rename.
func atomicWrite(path string, b []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("temp create: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("temp write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("temp sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("temp close: %w", err)
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return fmt.Errorf("temp chmod: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/filestate/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/filestate/store.go internal/adapters/outbound/filestate/store_test.go
git commit -m "feat(filestate): add Store with atomic write + ProjectMeta"
```

---

## Phase 4 — Application services

### Task 6: application/initializer.go — Initializer use case

**Files:**
- Create: `internal/application/initializer.go`
- Create: `internal/application/initializer_test.go`

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

func newInit() (*application.Initializer, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	init := application.NewInitializer(application.InitializerDeps{
		Git:           git,
		ProjectStore:  store,
	})
	return init, git, store
}

func TestInitWritesAtRepoRoot(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/home/u/myrepo"

	res, err := init.Run(context.Background(), application.InitInput{
		Project:       "ms-cotizacion",
		BaseRef:       "main",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "/home/u/myrepo/.sophia.yaml" {
		t.Errorf("Path = %q", res.Path)
	}
	got, err := store.Read(context.Background(), res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "ms-cotizacion" {
		t.Errorf("project lost: %+v", got)
	}
}

func TestInitFailsOutsideGitRepo(t *testing.T) {
	init, git, _ := newInit()
	git.NotARepo = true
	_, err := init.Run(context.Background(), application.InitInput{Project: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotARepo) {
		t.Errorf("expected ErrNotARepo, got %v", err)
	}
}

func TestInitAbortsWhenConfigExistsWithoutForce(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/r"
	_ = store.Write(context.Background(), "/r/.sophia.yaml", &domain.ProjectConfig{Project: "old"})
	_, err := init.Run(context.Background(), application.InitInput{Project: "new"})
	if err == nil {
		t.Error("expected abort")
	}
}

func TestInitOverwritesWithForce(t *testing.T) {
	init, git, store := newInit()
	git.Root = "/r"
	_ = store.Write(context.Background(), "/r/.sophia.yaml", &domain.ProjectConfig{Project: "old"})
	_, err := init.Run(context.Background(), application.InitInput{Project: "new", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(context.Background(), "/r/.sophia.yaml")
	if got.Project != "new" {
		t.Errorf("project = %q, want new", got.Project)
	}
}

func TestInitDefaultsBaseRefAndArtifactStore(t *testing.T) {
	init, _, store := newInit()
	res, err := init.Run(context.Background(), application.InitInput{Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(context.Background(), res.Path)
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
	if got.ArtifactStore != domain.ArtifactStoreEngram {
		t.Errorf("ArtifactStore = %q", got.ArtifactStore)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d", got.Version)
	}
}

func TestInitOverwriteWithForceWhenExistingIsInvalid(t *testing.T) {
	// The Store.Read returning ErrInvalidYAML must not abort init when --force is set.
	init, git, store := newInit()
	git.Root = "/r"
	store.ReadErr = domain.ErrInvalidYAML
	_, err := init.Run(context.Background(), application.InitInput{Project: "p", Force: true})
	if err != nil {
		t.Fatal(err)
	}
}
```

This test references `store.ReadErr` — extend FakeProjectConfigStore to support it (Step 2).

- [ ] **Step 2: Extend FakeProjectConfigStore with ReadErr**

Read `test/fakes/projectconfig.go` first. Add a `ReadErr error` field. Modify `Read` so it returns `s.ReadErr` first if non-nil.

```go
type FakeProjectConfigStore struct {
	mu      sync.Mutex
	store   map[string]*domain.ProjectConfig
	ReadErr error  // NEW: when non-nil, Read returns this
}

// In Read():
func (s *FakeProjectConfigStore) Read(_ context.Context, path string) (*domain.ProjectConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ReadErr != nil {
		return nil, s.ReadErr
	}
	cfg, ok := s.store[path]
	if !ok {
		return nil, domain.ErrConfigMissing
	}
	out := *cfg
	return &out, nil
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL (Initializer undefined).

- [ ] **Step 4: Implement**

`internal/application/initializer.go`:
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

// InitializerDeps groups the ports the Initializer needs.
type InitializerDeps struct {
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// InitInput controls Run.
type InitInput struct {
	Project       string
	BaseRef       string                   // empty ⇒ "main"
	ArtifactStore domain.ArtifactStoreMode // empty ⇒ ArtifactStoreEngram
	Force         bool
}

// InitResult reports what Run did.
type InitResult struct {
	Path     string // absolute path to the written .sophia.yaml
	RepoRoot string
}

// Initializer orchestrates `sophia init`.
type Initializer struct {
	deps InitializerDeps
}

// NewInitializer constructs an Initializer.
func NewInitializer(d InitializerDeps) *Initializer { return &Initializer{deps: d} }

// Run resolves the repo root and writes .sophia.yaml. Fails with
// domain.ErrNotARepo when run outside a git repository. Refuses to
// overwrite an existing config unless Force=true.
func (i *Initializer) Run(ctx context.Context, in InitInput) (InitResult, error) {
	if in.Project == "" {
		return InitResult{}, errors.New("init: --project is required")
	}
	root, err := i.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return InitResult{}, fmt.Errorf("%w: %v", domain.ErrNotARepo, err)
	}
	path := filepath.Join(root, ".sophia.yaml")

	if !in.Force {
		// Refuse to overwrite if a valid config already exists.
		// Treat ErrConfigMissing as "ok, write it".
		// Treat ErrInvalidYAML the same as "valid but corrupt": still requires --force.
		_, err := i.deps.ProjectStore.Read(ctx, path)
		if err == nil {
			return InitResult{}, fmt.Errorf("init: %s already exists; pass --force to overwrite", path)
		}
		if !errors.Is(err, domain.ErrConfigMissing) {
			return InitResult{}, fmt.Errorf("init: %s exists but is unreadable (%v); pass --force to repair", path, err)
		}
	}

	baseRef := in.BaseRef
	if baseRef == "" {
		baseRef = "main"
	}
	store := in.ArtifactStore
	if store == "" {
		store = domain.ArtifactStoreEngram
	}

	cfg := &domain.ProjectConfig{
		Version:       1,
		Project:       in.Project,
		BaseRef:       baseRef,
		ArtifactStore: store,
	}
	if err := i.deps.ProjectStore.Write(ctx, path, cfg); err != nil {
		return InitResult{}, fmt.Errorf("write: %w", err)
	}
	return InitResult{Path: path, RepoRoot: root}, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/application/... ./test/fakes/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/initializer.go internal/application/initializer_test.go test/fakes/projectconfig.go
git commit -m "feat(application): add Initializer (resolve repo root, write .sophia.yaml)"
```

---

### Task 7: application/status.go — StatusReader use case

**Files:**
- Create: `internal/application/status.go`
- Create: `internal/application/status_test.go`

- [ ] **Step 1: Write the failing test**

```go
package application_test

import (
	"context"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newStatus() (*application.StatusReader, *fakes.FakeStateStore, *fakes.FakeGitInspector, *fakes.FakeProjectConfigStore) {
	state := fakes.NewFakeStateStore()
	git := fakes.NewFakeGitInspector()
	store := fakes.NewFakeProjectConfigStore()
	r := application.NewStatusReader(application.StatusDeps{
		State:        state,
		Git:          git,
		ProjectStore: store,
	})
	return r, state, git, store
}

func TestStatusEmptyWhenNoProjectAndNoGlobal(t *testing.T) {
	r, _, _, _ := newStatus()
	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsEmpty {
		t.Errorf("expected empty, got %+v", out)
	}
}

func TestStatusPrefersProjectScoped(t *testing.T) {
	r, state, git, store := newStatus()
	git.Root = "/repo"
	cfg := &domain.ProjectConfig{Version: 1, Project: "p"}
	_ = store.Write(context.Background(), "/repo/.sophia.yaml", cfg)
	fp := domain.ComputeFingerprint("p", "/repo", git.Remote)
	_ = state.SetLast(context.Background(), fp, "01HX-project")
	_ = state.SetGlobalLast(context.Background(), "01HX-global")

	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.ChangeID != "01HX-project" {
		t.Errorf("ChangeID = %q (want project-scoped)", out.ChangeID)
	}
	if out.Source != application.StatusSourceProject {
		t.Errorf("Source = %q", out.Source)
	}
}

func TestStatusFallsBackToGlobal(t *testing.T) {
	r, state, _, _ := newStatus()
	_ = state.SetGlobalLast(context.Background(), "01HX-global")

	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEmpty {
		t.Fatal("expected populated")
	}
	if out.ChangeID != "01HX-global" {
		t.Errorf("ChangeID = %q", out.ChangeID)
	}
	if out.Source != application.StatusSourceGlobal {
		t.Errorf("Source = %q", out.Source)
	}
}

func TestStatusUsesNoConfigOutsideRepo(t *testing.T) {
	// Outside a repo, project-scoped lookup is impossible — fallback to global.
	r, state, git, _ := newStatus()
	git.NotARepo = true
	_ = state.SetGlobalLast(context.Background(), "01HX-global")
	out, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.ChangeID != "01HX-global" || out.Source != application.StatusSourceGlobal {
		t.Errorf("expected global fallback, got %+v", out)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/application/status.go`:
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

// StatusSource indicates where the resolved last_change_id came from.
type StatusSource string

// Status sources.
const (
	StatusSourceProject StatusSource = "project"
	StatusSourceGlobal  StatusSource = "global"
)

// StatusDeps groups the ports StatusReader needs.
type StatusDeps struct {
	State        outbound.StateStore
	Git          outbound.GitInspector
	ProjectStore outbound.ProjectConfigStore
}

// StatusOutput is the shape returned by Resolve.
type StatusOutput struct {
	IsEmpty  bool
	ChangeID domain.ChangeID
	Source   StatusSource
}

// StatusReader is the M3 placeholder `sophia status` use case. It resolves
// the local last_change_id only — no orchestrator call. Real status (with
// HTTP fetch) ships in M8.
type StatusReader struct {
	deps StatusDeps
}

// NewStatusReader constructs a StatusReader.
func NewStatusReader(d StatusDeps) *StatusReader { return &StatusReader{deps: d} }

// Resolve walks: project-scoped → global → empty.
func (r *StatusReader) Resolve(ctx context.Context) (StatusOutput, error) {
	if id, src, err := r.tryProject(ctx); err == nil && !id.IsZero() {
		return StatusOutput{ChangeID: id, Source: src}, nil
	}
	id, err := r.deps.State.GetGlobalLast(ctx)
	if err != nil {
		return StatusOutput{}, fmt.Errorf("global: %w", err)
	}
	if !id.IsZero() {
		return StatusOutput{ChangeID: id, Source: StatusSourceGlobal}, nil
	}
	return StatusOutput{IsEmpty: true}, nil
}

// tryProject returns the project-scoped change id, or zero/error.
func (r *StatusReader) tryProject(ctx context.Context) (domain.ChangeID, StatusSource, error) {
	root, err := r.deps.Git.RepoRoot(ctx, ".")
	if err != nil {
		return "", "", err
	}
	cfgPath := filepath.Join(root, ".sophia.yaml")
	cfg, err := r.deps.ProjectStore.Read(ctx, cfgPath)
	if err != nil {
		if errors.Is(err, domain.ErrConfigMissing) {
			return "", "", err
		}
		return "", "", err
	}
	remote, _ := r.deps.Git.RemoteURL(ctx, root)
	fp := domain.ComputeFingerprint(cfg.Project, root, remote)
	id, err := r.deps.State.GetLast(ctx, fp)
	if err != nil {
		return "", "", err
	}
	return id, StatusSourceProject, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/status.go internal/application/status_test.go
git commit -m "feat(application): add StatusReader (project → global → empty)"
```

---

## Phase 5 — Cobra commands `init` and `status`

### Task 8: cli/init.go

**Files:**
- Modify: `internal/adapters/inbound/cli/root.go` (add fields to Deps, replace stub)
- Create: `internal/adapters/inbound/cli/init.go`
- Create: `internal/adapters/inbound/cli/init_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newInitDeps() (cli.Deps, *fakes.FakeProjectConfigStore) {
	store := fakes.NewFakeProjectConfigStore()
	return cli.Deps{
		Initializer: application.NewInitializer(application.InitializerDeps{
			Git:          fakes.NewFakeGitInspector(),
			ProjectStore: store,
		}),
	}, store
}

func TestInitCommandRequiresProjectFlag(t *testing.T) {
	deps, _ := newInitDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"init"})
	if err := c.Execute(); err == nil {
		t.Error("expected error when --project missing")
	}
}

func TestInitCommandWritesConfig(t *testing.T) {
	deps, store := newInitDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"init", "--project", "ms-cotizacion"})
	if err := c.Execute(); err != nil {
		t.Fatalf("init err: %v", err)
	}
	if !strings.Contains(out.String(), "ms-cotizacion") {
		t.Errorf("output missing project name: %q", out.String())
	}
	// verify file written at fake repo root /repo
	got, err := store.Read(t.Context(), "/repo/.sophia.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Project != "ms-cotizacion" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.BaseRef != "main" {
		t.Errorf("BaseRef = %q", got.BaseRef)
	}
}

func TestInitForceOverwrite(t *testing.T) {
	deps, store := newInitDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"init", "--project", "old"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}

	c2 := cli.NewRoot(deps)
	c2.SetOut(&bytes.Buffer{})
	c2.SetArgs([]string{"init", "--project", "new"})
	if err := c2.Execute(); err == nil {
		t.Error("expected abort without --force")
	}

	c3 := cli.NewRoot(deps)
	c3.SetOut(&bytes.Buffer{})
	c3.SetArgs([]string{"init", "--project", "new", "--force"})
	if err := c3.Execute(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(t.Context(), "/repo/.sophia.yaml")
	if got.Project != "new" {
		t.Errorf("Project = %q after --force", got.Project)
	}
}
```

(Note: `t.Context()` requires Go 1.24+. This was used in M2 tests — confirmed available.)

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL (Deps.Initializer missing).

- [ ] **Step 3: Implement init.go**

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func newInitCmd(d Deps) *cobra.Command {
	var (
		project       string
		baseRef       string
		artifactStore string
		force         bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .sophia.yaml at the resolved repo root",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Initializer == nil {
				return fmt.Errorf("init: initializer not wired")
			}
			res, err := d.Initializer.Run(cmd.Context(), application.InitInput{
				Project:       project,
				BaseRef:       baseRef,
				ArtifactStore: domain.ArtifactStoreMode(artifactStore),
				Force:         force,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (project=%s, base_ref=%s)\n",
				res.Path, project, baseRefDefault(baseRef))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project slug (required)")
	cmd.Flags().StringVar(&baseRef, "base-ref", "main", "default git ref for new Changes")
	cmd.Flags().StringVar(&artifactStore, "artifact-store", "engram", "artifact store: engram | openspec | hybrid | none")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing or invalid .sophia.yaml")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func baseRefDefault(s string) string {
	if s == "" {
		return "main"
	}
	return s
}
```

- [ ] **Step 4: Update root.go**

Read `internal/adapters/inbound/cli/root.go` first.

Add to `Deps`:
```go
Initializer  *application.Initializer
StatusReader *application.StatusReader
```

Replace `root.AddCommand(newStubCmd("init", ...))` with `root.AddCommand(newInitCmd(d))`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/cli/init.go internal/adapters/inbound/cli/init_test.go internal/adapters/inbound/cli/root.go
git commit -m "feat(cli): add init command with --project/--base-ref/--artifact-store/--force"
```

---

### Task 9: cli/status.go — placeholder (local resolution only)

**Files:**
- Modify: `internal/adapters/inbound/cli/root.go` (replace stub)
- Create: `internal/adapters/inbound/cli/status.go`
- Create: `internal/adapters/inbound/cli/status_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func newStatusDeps() (cli.Deps, *fakes.FakeStateStore) {
	state := fakes.NewFakeStateStore()
	return cli.Deps{
		StatusReader: application.NewStatusReader(application.StatusDeps{
			State:        state,
			Git:          fakes.NewFakeGitInspector(),
			ProjectStore: fakes.NewFakeProjectConfigStore(),
		}),
	}, state
}

func TestStatusEmptyMessage(t *testing.T) {
	deps, _ := newStatusDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No local change found") {
		t.Errorf("output missing empty message: %q", out.String())
	}
}

func TestStatusPrintsChangeID(t *testing.T) {
	deps, state := newStatusDeps()
	_ = state.SetGlobalLast(t.Context(), "01HX-test")
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "01HX-test") {
		t.Errorf("output missing change id: %q", out.String())
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL (StatusReader not wired).

- [ ] **Step 3: Implement status.go**

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of a Change (M3: local resolution only; orchestrator call ships in M8)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.StatusReader == nil {
				return fmt.Errorf("status: reader not wired")
			}
			out, err := d.StatusReader.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if out.IsEmpty {
				fmt.Fprintln(w, "No local change found.")
				fmt.Fprintln(w, "Use sophia changes or pass <change-id> explicitly.")
				return nil
			}
			fmt.Fprintf(w, "last change: %s (source=%s)\n", out.ChangeID, out.Source)
			return nil
		},
	}
}
```

- [ ] **Step 4: Update root.go**

Replace `root.AddCommand(newStubCmd("status", ...))` with `root.AddCommand(newStatusCmd(d))`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/cli/status.go internal/adapters/inbound/cli/status_test.go internal/adapters/inbound/cli/root.go
git commit -m "feat(cli): add status command (M3 placeholder, local resolution only)"
```

---

### Task 10: Drop init/status from stubs_test

**Files:**
- Modify: `internal/adapters/inbound/cli/stubs_test.go`

- [ ] **Step 1: Read current stubs_test.go**

Read the file. Find the `cases` map.

- [ ] **Step 2: Update**

Edit the `cases` map in `TestStubsAnnounceMilestone`. Remove `"init": "M3"` and `"status": "M3"`. New map:
```go
cases := map[string]string{
    "run":     "M4",
    "attach":  "M8",
    "changes": "M8",
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/inbound/cli/stubs_test.go
git commit -m "test(cli): drop init/status from stub list (now wired in M3)"
```

---

## Phase 6 — Bootstrap rewiring

### Task 11: bootstrap/wire.go — wire Initializer + StatusReader + filestate + yamlconfig

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`

- [ ] **Step 1: Update wire_test.go**

Read existing wire_test.go. Then add a new test (keep existing ones):
```go
func TestNewWiresM3Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"init", "status"} {
		c, _, err := root.Find([]string{sub})
		if err != nil {
			t.Errorf("%s not found: %v", sub, err)
		}
		if c == nil {
			t.Errorf("%s cmd is nil", sub)
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/bootstrap/...`
Expected: PASS for existing tests, FAIL for new (init/status not wired yet).

- [ ] **Step 3: Update wire.go**

Read existing wire.go. Add to imports:
```go
"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/filestate"
"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
```

Inside `New`, after `paths` is constructed and BEFORE the `cli.Deps{}` literal, add:

```go
// Resolve XDG paths once for state-aware adapters. Errors here mean
// the binary still works for read-only commands; init/status will
// re-resolve and fail with a clearer message.
xdg, _ := paths.Resolve()

projectStore := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{})
state := filestate.New(filestate.Config{StateRoot: xdg.StateRoot})

initializer := application.NewInitializer(application.InitializerDeps{
    Git:          git,
    ProjectStore: projectStore,
})
statusReader := application.NewStatusReader(application.StatusDeps{
    State:        state,
    Git:          git,
    ProjectStore: projectStore,
})
```

And update the `cli.Deps{}` literal:
```go
deps := cli.Deps{
    Doctor:       doctor,
    Provisioner:  provisioner,
    Initializer:  initializer,
    StatusReader: statusReader,
    Version:      info.Version,
    Commit:       info.Commit,
    BuildDate:    info.BuildDate,
}
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/bootstrap/... -race && make build`
Expected: PASS, binary builds.

- [ ] **Step 5: Smoke**

Run:
```bash
./bin/sophia init --help
./bin/sophia status --help
```
Expected: both show help with their flags.

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/wire.go internal/bootstrap/wire_test.go
git commit -m "feat(bootstrap): wire M3 (Initializer, StatusReader, yamlconfig, filestate)"
```

---

## Phase 7 — Integration test

### Task 12: test/integration/init_test.go — opt-in real-FS test

**Files:**
- Create: `test/integration/init_test.go`

- [ ] **Step 1: Write the test (gated by build tag)**

```go
//go:build integration

package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/filestate"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/gitcli"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/yamlconfig"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func gitAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	return true
}

func TestInitInRealRepo(t *testing.T) {
	if !gitAvailable(t) {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	if out, err := exec.Command("git", "-C", tmp, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	git := gitcli.New(gitcli.Config{})
	store := yamlconfig.NewProjectStore(yamlconfig.ProjectConfig{})
	init := application.NewInitializer(application.InitializerDeps{
		Git:          git,
		ProjectStore: store,
	})
	res, err := init.Run(context.Background(), application.InitInput{
		Project:       "ms-cotizacion",
		ArtifactStore: domain.ArtifactStoreEngram,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if filepath.Dir(res.Path) != tmp {
		// EvalSymlinks may resolve macOS /tmp → /private/tmp; allow either.
		want, _ := filepath.EvalSymlinks(tmp)
		if filepath.Dir(res.Path) != want {
			t.Errorf("written outside repo: %s (want %s or %s)", res.Path, tmp, want)
		}
	}

	cfg, err := store.Read(context.Background(), res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "ms-cotizacion" {
		t.Errorf("Project = %q", cfg.Project)
	}
}

func TestStateStoreAtomicWriteSurvivesRestart(t *testing.T) {
	tmp := t.TempDir()
	s := filestate.New(filestate.Config{StateRoot: tmp})
	fp := domain.Fingerprint("fp-int")

	if err := s.SetLast(context.Background(), fp, "01HX"); err != nil {
		t.Fatal(err)
	}

	// Re-open in a fresh Store to simulate process restart.
	s2 := filestate.New(filestate.Config{StateRoot: tmp})
	got, err := s2.GetLast(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got != "01HX" {
		t.Errorf("got %q after restart", got)
	}
}
```

- [ ] **Step 2: Verify tag isolation**

Run: `go test ./...` — verify integration NOT run.
Run: `go test -tags=integration ./test/integration/...` — expect PASS or SKIP.

- [ ] **Step 3: Commit**

```bash
git add test/integration/init_test.go
git commit -m "test(integration): add init + filestate integration tests"
```

---

## Phase 8 — Final validation

### Task 13: Final validation pass

**Files:** none (verification only).

- [ ] **Step 1: vet + tests + race**

Run:
```bash
go vet ./...
go test -race ./...
```
Expected: exit 0.

- [ ] **Step 2: Lint**

Run: `golangci-lint run`
Expected: exit 0. Fix issues in place. Acceptable `//nolint` patterns: `gosec` on subprocess shellouts (precedent set in M2), `unparam` on fakes.

- [ ] **Step 3: Coverage**

Run:
```bash
go test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
go tool cover -func=cover.out | tail -n 1
```
Expected: total ≥ 70%.

- [ ] **Step 4: Binary smoke**

Build first: `make build`

Then in a fresh /tmp directory:
```bash
mkdir -p /tmp/m3-smoke && cd /tmp/m3-smoke && rm -rf .git .sophia.yaml
git init -q
/Users/russell/Documents/2026/sophia-cli/bin/sophia init --project ms-cotizacion
test -f .sophia.yaml && echo "init wrote config"
cat .sophia.yaml
/Users/russell/Documents/2026/sophia-cli/bin/sophia status
```

Expected:
- `init` writes `.sophia.yaml` containing `version: 1`, `project: ms-cotizacion`, `base_ref: main`, `artifact_store: engram`
- `status` prints `No local change found.\nUse sophia changes or pass <change-id> explicitly.`

Also test outside-repo behavior:
```bash
cd /tmp && rm -rf m3-no-repo && mkdir m3-no-repo && cd m3-no-repo
/Users/russell/Documents/2026/sophia-cli/bin/sophia init --project p
echo "exit code: $?"
```
Expected: exit non-zero, error mentions "not a git repository".

- [ ] **Step 5: E2E smoke**

Run: `go test -tags=e2e_smoke ./test/e2e/...`
Expected: PASS.

- [ ] **Step 6: Final commit and tag**

```bash
cd /Users/russell/Documents/2026/sophia-cli
git add -A
git status
git commit -m "chore(m3): final validation pass" || echo "nothing to commit"
git tag -a m3-project-state -m "M3 Project & State complete"
```

---

## Self-review checklist

- [ ] **Spec coverage:** Every M3 DoD item from spec §7.2 has at least one task.
  - `sophia init` with all flags + `--force` repairs invalid → Tasks 6, 8
  - Fingerprint with Abs/Clean/EvalSymlinks tolerant — already in M1, used unchanged here
  - filestate.Store with atomic write → Task 5
  - yamlconfig.ProjectStore Read/Write/Find → Task 3
  - sophia status (project → global → empty) → Tasks 7, 9
  - afero.MemMapFs in tests → Tasks 3, 4
- [ ] **No placeholders:** No "TBD"/"TODO"/"similar to" in steps.
- [ ] **Type consistency:** `application.InitializerDeps`, `application.StatusDeps`, `cli.Deps.{Initializer,StatusReader}` consistent across tasks.
- [ ] **Frequent commits:** every task ends with at least one commit.
- [ ] **TDD discipline:** every code-producing task has a failing test before implementation.
- [ ] **No premature M4+ scope:** no HTTP call from status, no `sophia run`, no SSE, no TUI.

---

## Pending decisions (carried into M3 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M3-01 | If repo has no remote, is project name in `.sophia.yaml` enough for fingerprint uniqueness? | Yes — fingerprint = sha256(project + repo_root + ""), where empty remote is normal. Two clones of the same repo with same project name do produce the same fingerprint by design. |
| D-M3-02 | Should `init` accept and persist `repo_root` in `.sophia.yaml`? | No — `repo_root` is recomputed every call from `git rev-parse`. Spec §3.2 schema doesn't include it. |
| D-M3-03 | Should `status` resolve `.sophia.yaml` from CWD ancestors (Find) or only at repo root? | Repo root (via `git rev-parse --show-toplevel`). Find-via-ancestors is reserved for V1.1+ where running outside the repo root might be useful. |

---

## Risks specific to M3

| ID | Risk | Mitigation |
|---|---|---|
| RM3-01 | `EvalSymlinks` fails on sandboxed FS or symlinked $TMPDIR (macOS) | Integration test allows EvalSymlinks-resolved alternative. Fingerprint module already uses tolerant fallback (per M1). |
| RM3-02 | yaml.v3 KnownFields(false) silently drops typo'd keys | Spec §3.2 explicitly accepts unknown fields (forward-compat). Acceptable. |
| RM3-03 | Atomic write left a `.tmp.*` file on a previous crash | Implementation cleans up via `defer os.Remove(tmp.Name())`. Even leftover files don't affect correctness — `os.Rename` overwrites the target. |
| RM3-04 | Fingerprint changes when user moves repo (repo_root changes) | Documented behavior — user re-runs `sophia init` if needed. State for the old path is orphan but harmless. |

---

## What this plan does NOT cover (intentional)

- HTTP fetch from `status` → M8.
- `sophia run` polling → M4.
- SSE consumer → M5.
- TUI → M6.
- Real browser opener → M7.
- Environment variable layering for flags → M4 minimum.
- `<configRoot>/config.yaml` reading from CLI commands (UserStore is built but not yet read by any service — M4 wires it for orchestrator URL override).

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-05-sophia-cli-m3-project-state.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Use `superpowers:subagent-driven-development`.

**2. Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints.
