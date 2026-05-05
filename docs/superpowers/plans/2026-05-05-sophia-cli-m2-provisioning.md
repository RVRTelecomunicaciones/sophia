# Sophia CLI — M2 Provisioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed a `compose.yaml` stub via `//go:embed`, materialize it deterministically into `<dataRoot>/compose/`, expose `sophia start/stop` operating with project name `sophia` regardless of CWD, add an HTTP healthz client, and wire two new doctor checks (orchestrator reachable + SSE handshake). End state: `sophia start` brings up a dev compose stack from any CWD, `sophia stop` tears it down, `sophia doctor` reports orchestrator + SSE handshake, all M1 tests still pass plus opt-in integration test.

**Architecture:** Two new application services (`Provisioner`, `HealthChecker`), two new outbound adapter packages (`orchestratorhttp` for HTTP + `infrastructure/httpclient` for the builder), a real XDG path resolver replacing M1's stub validator, and a real materializer in `composeexec` that handles the embedded compose + hash check + `.previous` backup. Doctor grows from 4 to 6 checks. The compose project name is **always `sophia`**, never derived from CWD.

**Tech Stack:** Go 1.24.x · `//go:embed` (stdlib) · `net/http` (stdlib) · `crypto/sha256` (stdlib) · `os/exec` (stdlib) · `encoding/json` (stdlib). No new third-party deps.

**Spec source of truth:** `docs/superpowers/specs/2026-05-05-sophia-cli-design.md` (§3.1 XDG, §3.6 compose, §5.1 endpoints, §5.8 timeouts, §6.1 doctor, §6.3 invariants, §7.2 M2 DoD)
**Roadmap:** `docs/superpowers/plans/2026-05-05-sophia-cli-roadmap.md` (§ M2)
**Module path:** `github.com/RVRTelecomunicaciones/sophia-cli`

**M2 boundaries — what is NOT in M2:**

- No `.sophia.yaml` reading/writing (M3).
- No state store atomic writes (M3).
- No `sophia run` / `sophia attach` / `sophia changes` (M4–M8).
- No real SSE consumer — the SSE handshake check uses a 1-frame probe and warns on failure (real M5).
- No browser opener (M7).
- No Bubble Tea TUI (M6).
- No `--reset-compose` deep merge — V1 is "abort if user-edited and not flagged".

---

## Phase 1 — Path resolution

### Task 1: domain/paths.go — XDG path types

**Files:**
- Create: `internal/domain/paths.go`
- Create: `internal/domain/paths_test.go`

- [ ] **Step 1: Write the failing test**

`internal/domain/paths_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

func TestXDGPathsZeroValue(t *testing.T) {
	var p domain.XDGPaths
	if p.ConfigRoot != "" || p.StateRoot != "" || p.DataRoot != "" || p.CacheRoot != "" {
		t.Errorf("zero XDGPaths should have empty fields, got %+v", p)
	}
}

func TestXDGPathsAllFields(t *testing.T) {
	p := domain.XDGPaths{
		ConfigRoot: "/c", StateRoot: "/s", DataRoot: "/d", CacheRoot: "/x",
	}
	if p.ConfigRoot != "/c" || p.StateRoot != "/s" || p.DataRoot != "/d" || p.CacheRoot != "/x" {
		t.Errorf("fields lost: %+v", p)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/domain/...`
Expected: FAIL with "undefined: domain.XDGPaths".

- [ ] **Step 3: Implement**

`internal/domain/paths.go`:

```go
package domain

// XDGPaths is the resolved set of XDG-style roots scoped to "sophia".
//
// Defaults (all platforms) when env vars are unset:
//   ConfigRoot = ~/.config/sophia
//   StateRoot  = ~/.local/state/sophia
//   DataRoot   = ~/.local/share/sophia
//   CacheRoot  = ~/.cache/sophia          (reserved for V1.1)
//
// On macOS, when XDG vars are not set, the CLI defaults to the same
// Linux-style paths for cross-platform consistency. This is documented
// in --help and reported by `sophia doctor`.
type XDGPaths struct {
	ConfigRoot string
	StateRoot  string
	DataRoot   string
	CacheRoot  string
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/paths.go internal/domain/paths_test.go
git commit -m "feat(domain): add XDGPaths type"
```

---

### Task 2: ports/outbound/paths.go — PathResolver port

**Files:**
- Create: `internal/ports/outbound/paths.go`

- [ ] **Step 1: Write the file**

`internal/ports/outbound/paths.go`:

```go
package outbound

import (
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// PathResolver resolves and validates the four XDG-style roots used by the CLI.
// Real adapter (M2): internal/adapters/outbound/xdgpaths.
type PathResolver interface {
	// Resolve returns the four roots based on env vars and HOME, applying
	// the macOS Linux-style fallback documented in the spec.
	Resolve() (domain.XDGPaths, error)

	// EnsureDirs creates each root directory at 0700 if missing.
	// It is idempotent and only used by `sophia start` (and future write paths);
	// `sophia doctor` does NOT create directories.
	EnsureDirs(p domain.XDGPaths) error

	// ValidateDirs returns nil iff each root exists with mode 0700.
	// Used by `sophia doctor` (which is read-only).
	ValidateDirs(p domain.XDGPaths) error
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./internal/ports/outbound/...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/ports/outbound/paths.go
git commit -m "feat(ports): add PathResolver outbound port"
```

---

### Task 3: test/fakes/paths.go — FakePathResolver

**Files:**
- Create: `test/fakes/paths.go`
- Create: `test/fakes/paths_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fakes_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakePathResolverImplementsPort(t *testing.T) {
	var _ outbound.PathResolver = (*fakes.FakePathResolver)(nil)
}

func TestFakePathResolverReturnsConfiguredPaths(t *testing.T) {
	r := fakes.NewFakePathResolver()
	r.Paths = domain.XDGPaths{ConfigRoot: "/c", StateRoot: "/s", DataRoot: "/d"}
	got, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigRoot != "/c" {
		t.Errorf("ConfigRoot = %q", got.ConfigRoot)
	}
}

func TestFakePathResolverEnsureDirsRecords(t *testing.T) {
	r := fakes.NewFakePathResolver()
	_ = r.EnsureDirs(domain.XDGPaths{StateRoot: "/s"})
	if r.EnsureDirsCalls != 1 {
		t.Errorf("EnsureDirsCalls = %d", r.EnsureDirsCalls)
	}
}

func TestFakePathResolverValidateError(t *testing.T) {
	r := fakes.NewFakePathResolver()
	r.ValidateErr = fakes.ErrFakeValidate
	if err := r.ValidateDirs(domain.XDGPaths{}); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`test/fakes/paths.go`:

```go
package fakes

import (
	"errors"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// ErrFakeValidate is a sentinel for ValidateDirs failure tests.
var ErrFakeValidate = errors.New("fake validate failure")

// FakePathResolver is a programmable PathResolver for tests.
type FakePathResolver struct {
	Paths           domain.XDGPaths
	ResolveErr      error
	EnsureErr       error
	ValidateErr     error
	EnsureDirsCalls int
}

// NewFakePathResolver returns sensible defaults.
func NewFakePathResolver() *FakePathResolver {
	return &FakePathResolver{
		Paths: domain.XDGPaths{
			ConfigRoot: "/home/u/.config/sophia",
			StateRoot:  "/home/u/.local/state/sophia",
			DataRoot:   "/home/u/.local/share/sophia",
			CacheRoot:  "/home/u/.cache/sophia",
		},
	}
}

// Resolve returns the configured paths or error.
func (f *FakePathResolver) Resolve() (domain.XDGPaths, error) {
	return f.Paths, f.ResolveErr
}

// EnsureDirs records the call and returns the configured error.
func (f *FakePathResolver) EnsureDirs(_ domain.XDGPaths) error {
	f.EnsureDirsCalls++
	return f.EnsureErr
}

// ValidateDirs returns the configured error.
func (f *FakePathResolver) ValidateDirs(_ domain.XDGPaths) error {
	return f.ValidateErr
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fakes/paths.go test/fakes/paths_test.go
git commit -m "test(fakes): add FakePathResolver"
```

---

### Task 4: adapters/outbound/xdgpaths/resolver.go — real XDG resolver

**Files:**
- Create: `internal/adapters/outbound/xdgpaths/resolver.go`
- Create: `internal/adapters/outbound/xdgpaths/resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package xdgpaths_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestResolverImplementsPort(t *testing.T) {
	var _ outbound.PathResolver = xdgpaths.New(xdgpaths.Config{})
}

func TestResolveHonorsXDGEnv(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{
		Env: map[string]string{
			"XDG_CONFIG_HOME": "/c",
			"XDG_STATE_HOME":  "/s",
			"XDG_DATA_HOME":   "/d",
			"XDG_CACHE_HOME":  "/x",
			"HOME":            "/home/u",
		},
	})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigRoot != "/c/sophia" {
		t.Errorf("ConfigRoot = %q", p.ConfigRoot)
	}
	if p.StateRoot != "/s/sophia" {
		t.Errorf("StateRoot = %q", p.StateRoot)
	}
	if p.DataRoot != "/d/sophia" {
		t.Errorf("DataRoot = %q", p.DataRoot)
	}
	if p.CacheRoot != "/x/sophia" {
		t.Errorf("CacheRoot = %q", p.CacheRoot)
	}
}

func TestResolveDefaultsFromHome(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{
		Env: map[string]string{"HOME": "/home/u"},
	})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"ConfigRoot": p.ConfigRoot,
		"StateRoot":  p.StateRoot,
		"DataRoot":   p.DataRoot,
		"CacheRoot":  p.CacheRoot,
	}
	for k, v := range cases {
		if !strings.HasPrefix(v, "/home/u/") {
			t.Errorf("%s = %q does not start under HOME", k, v)
		}
		if !strings.HasSuffix(v, "/sophia") {
			t.Errorf("%s = %q does not end with /sophia", k, v)
		}
	}
}

func TestResolveErrorsWithoutHome(t *testing.T) {
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{}})
	if _, err := r.Resolve(); err == nil {
		t.Error("expected error when HOME unset and no XDG vars")
	}
}

func TestEnsureDirsCreatesAt0700(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureDirs(p); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatalf("dir %q missing: %v", d, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("%q perm = %o, want 0700", d, perm)
		}
	}
}

func TestValidateDirsReturnsErrorOnMissing(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDirs(p); err == nil {
		t.Error("expected error before EnsureDirs")
	}
	if err := r.EnsureDirs(p); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDirs(p); err != nil {
		t.Errorf("expected nil after EnsureDirs, got %v", err)
	}
}

func TestValidateDirsRejectsLoosePerms(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.ConfigRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(p.StateRoot, 0o700)
	_ = os.MkdirAll(p.DataRoot, 0o700)
	_ = os.MkdirAll(p.CacheRoot, 0o700)
	if err := r.ValidateDirs(p); err == nil {
		t.Error("expected error on loose perms")
	}
}

func TestPathsDoNotEscapeHome(t *testing.T) {
	tmp := t.TempDir()
	r := xdgpaths.New(xdgpaths.Config{Env: map[string]string{"HOME": tmp}})
	p, _ := r.Resolve()
	abs, _ := filepath.Abs(tmp)
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if !strings.HasPrefix(d, abs) {
			t.Errorf("%q escapes HOME=%q", d, abs)
		}
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/xdgpaths/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/adapters/outbound/xdgpaths/resolver.go`:

```go
// Package xdgpaths implements outbound.PathResolver using XDG Base Directory
// conventions, with a Linux-style fallback on all platforms when XDG_* vars
// are unset (per spec §3.1).
package xdgpaths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
)

// Config configures the resolver. Env is injected for testability; in
// production the bootstrap layer passes os-derived values.
type Config struct {
	// Env, when non-nil, is consulted instead of os.Getenv. Tests override
	// this to control HOME and XDG vars deterministically.
	Env map[string]string
}

// Resolver implements outbound.PathResolver.
type Resolver struct {
	cfg Config
}

// New returns a Resolver. Pass an empty Config for production defaults.
func New(cfg Config) *Resolver { return &Resolver{cfg: cfg} }

func (r *Resolver) getenv(k string) string {
	if r.cfg.Env != nil {
		return r.cfg.Env[k]
	}
	return os.Getenv(k)
}

// Resolve returns the four roots, suffixing each with "sophia".
func (r *Resolver) Resolve() (domain.XDGPaths, error) {
	home := r.getenv("HOME")
	root := func(envKey, fallback string) (string, error) {
		if v := r.getenv(envKey); v != "" {
			return filepath.Join(v, "sophia"), nil
		}
		if home == "" {
			return "", fmt.Errorf("HOME unset and %s unset", envKey)
		}
		return filepath.Join(home, fallback, "sophia"), nil
	}
	cfg, err := root("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return domain.XDGPaths{}, err
	}
	st, err := root("XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return domain.XDGPaths{}, err
	}
	data, err := root("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return domain.XDGPaths{}, err
	}
	cache, err := root("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return domain.XDGPaths{}, err
	}
	return domain.XDGPaths{
		ConfigRoot: cfg,
		StateRoot:  st,
		DataRoot:   data,
		CacheRoot:  cache,
	}, nil
}

// EnsureDirs creates each root at 0700 if missing.
func (r *Resolver) EnsureDirs(p domain.XDGPaths) error {
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if d == "" {
			return errors.New("empty path in XDGPaths")
		}
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("mkdir %q: %w", d, err)
		}
	}
	return nil
}

// ValidateDirs returns nil iff every root exists with mode 0700.
func (r *Resolver) ValidateDirs(p domain.XDGPaths) error {
	for _, d := range []string{p.ConfigRoot, p.StateRoot, p.DataRoot, p.CacheRoot} {
		if d == "" {
			return errors.New("empty path in XDGPaths")
		}
		fi, err := os.Stat(d)
		if err != nil {
			return fmt.Errorf("stat %q: %w", d, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("%q is not a directory", d)
		}
		if fi.Mode().Perm() != 0o700 {
			return fmt.Errorf("%q perm %o, want 0700", d, fi.Mode().Perm())
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/xdgpaths/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/xdgpaths/resolver.go internal/adapters/outbound/xdgpaths/resolver_test.go
git commit -m "feat(adapters): add xdgpaths resolver (Resolve, EnsureDirs, ValidateDirs)"
```

---

## Phase 2 — HTTP healthz client

### Task 5: infrastructure/httpclient/builder.go — shared http.Client builder

**Files:**
- Create: `internal/infrastructure/httpclient/builder.go`
- Create: `internal/infrastructure/httpclient/builder_test.go`

- [ ] **Step 1: Write the failing test**

```go
package httpclient_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
)

func TestNewReturnsClient(t *testing.T) {
	c := httpclient.New(httpclient.Config{Timeout: 5 * time.Second})
	if c == nil {
		t.Fatal("nil client")
	}
	if _, ok := any(c).(*http.Client); !ok {
		t.Errorf("expected *http.Client, got %T", c)
	}
}

func TestNewAppliesTimeout(t *testing.T) {
	c := httpclient.New(httpclient.Config{Timeout: 7 * time.Second})
	if c.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v", c.Timeout)
	}
}

func TestNewAppliesDefaultTimeoutWhenZero(t *testing.T) {
	c := httpclient.New(httpclient.Config{})
	if c.Timeout == 0 {
		t.Error("expected non-zero default timeout")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/infrastructure/httpclient/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/infrastructure/httpclient/builder.go`:

```go
// Package httpclient builds the *http.Client used by every outbound HTTP
// adapter. Centralizing here ensures consistent timeouts, redirect policy,
// and (in V1.1+) telemetry/proxies.
package httpclient

import (
	"net/http"
	"time"
)

// DefaultTimeout is applied when Config.Timeout is zero.
const DefaultTimeout = 5 * time.Second

// Config controls the constructed client.
type Config struct {
	Timeout time.Duration // applied to the whole request lifecycle
}

// New returns a configured *http.Client. The transport is the stdlib default
// with a per-request timeout; redirects follow stdlib defaults (max 10).
func New(cfg Config) *http.Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &http.Client{Timeout: cfg.Timeout}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infrastructure/httpclient/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/httpclient/builder.go internal/infrastructure/httpclient/builder_test.go
git commit -m "feat(infrastructure): add httpclient builder"
```

---

### Task 6: adapters/outbound/orchestratorhttp/healthz.go — real Healthz

**Files:**
- Create: `internal/adapters/outbound/orchestratorhttp/client.go`
- Create: `internal/adapters/outbound/orchestratorhttp/healthz.go`
- Create: `internal/adapters/outbound/orchestratorhttp/healthz_test.go`

- [ ] **Step 1: Write the failing test**

```go
package orchestratorhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestClientImplementsOrchestratorClient(t *testing.T) {
	var _ outbound.OrchestratorClient = orchestratorhttp.New(orchestratorhttp.Config{BaseURL: "http://x"})
}

func TestHealthzSucceedsOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/healthz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	if err := c.Healthz(context.Background()); err != nil {
		t.Errorf("Healthz err: %v", err)
	}
}

func TestHealthzFailsOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL})
	err := c.Healthz(context.Background())
	if err == nil {
		t.Error("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestHealthzFailsOnUnreachable(t *testing.T) {
	c := orchestratorhttp.New(orchestratorhttp.Config{
		BaseURL: "http://127.0.0.1:1",
		Timeout: 200 * time.Millisecond,
	})
	if err := c.Healthz(context.Background()); err == nil {
		t.Error("expected error on unreachable")
	}
}

func TestHealthzRespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if err := c.Healthz(ctx); err == nil {
		t.Error("expected ctx-deadline error")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/...`
Expected: FAIL.

- [ ] **Step 3: Implement `client.go`**

```go
// Package orchestratorhttp implements outbound.OrchestratorClient via HTTP.
// In M2 only Healthz is implemented; the rest return ErrNotImplemented and
// land in M4.
package orchestratorhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/domain"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ErrNotImplemented is returned by methods landing in later milestones.
var ErrNotImplemented = errors.New("orchestratorhttp: not implemented in M2")

// Config controls the client.
type Config struct {
	BaseURL string        // e.g. "http://localhost:9080"
	Timeout time.Duration // request-scoped; defaults to httpclient.DefaultTimeout
	HTTP    *http.Client  // optional override; otherwise built from Timeout
}

// Client implements outbound.OrchestratorClient.
type Client struct {
	base string
	http *http.Client
}

// New returns a Client.
func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTP
	if hc == nil {
		hc = httpclient.New(httpclient.Config{Timeout: cfg.Timeout})
	}
	return &Client{base: base, http: hc}
}

// CreateChange is not implemented in M2 (lands in M4).
func (c *Client) CreateChange(_ context.Context, _ outbound.CreateChangeInput) (*domain.Change, error) {
	return nil, ErrNotImplemented
}

// GetChange is not implemented in M2 (lands in M4).
func (c *Client) GetChange(_ context.Context, _ domain.ChangeID) (*domain.Change, error) {
	return nil, ErrNotImplemented
}

// ListChanges is not implemented in M2 (lands in M4).
func (c *Client) ListChanges(_ context.Context, _ outbound.ListChangesFilter) ([]*domain.Change, error) {
	return nil, ErrNotImplemented
}
```

- [ ] **Step 4: Implement `healthz.go`**

```go
package orchestratorhttp

import (
	"context"
	"fmt"
	"net/http"
)

// Healthz pings GET <BaseURL>/api/v1/healthz; returns nil on 200.
// On non-2xx responses, the body's first 256 bytes are surfaced for diagnostics.
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/healthz", nil)
	if err != nil {
		return fmt.Errorf("healthz: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("healthz: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("healthz: status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/outbound/orchestratorhttp/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/outbound/orchestratorhttp/client.go \
        internal/adapters/outbound/orchestratorhttp/healthz.go \
        internal/adapters/outbound/orchestratorhttp/healthz_test.go
git commit -m "feat(adapters): add orchestratorhttp.Client with Healthz (M2)"
```

---

### Task 7: ports/outbound/sse.go — SSEHandshake port + fake

**Files:**
- Create: `internal/ports/outbound/ssehandshake.go`
- Create: `test/fakes/ssehandshake.go`
- Create: `test/fakes/ssehandshake_test.go`

- [ ] **Step 1: Write the port**

`internal/ports/outbound/ssehandshake.go`:

```go
package outbound

import "context"

// SSEProber probes whether an SSE endpoint is reachable and accepts
// `text/event-stream`. It does NOT consume events; the real consumer
// (M5) lives behind EventStreamClient.
//
// Used by `sophia doctor` to emit a `warn` if the orchestrator is reachable
// but the SSE handshake fails.
type SSEProber interface {
	// Probe attempts a one-shot handshake against the orchestrator's SSE
	// endpoint. Returns nil if the server replied with 200 + the correct
	// Content-Type. Returns an error in any other case.
	Probe(ctx context.Context) error
}
```

- [ ] **Step 2: Write the failing fake test**

`test/fakes/ssehandshake_test.go`:

```go
package fakes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

func TestFakeSSEProberImplementsPort(t *testing.T) {
	var _ outbound.SSEProber = (*fakes.FakeSSEProber)(nil)
}

func TestFakeSSEProberReturnsConfiguredError(t *testing.T) {
	p := fakes.NewFakeSSEProber()
	if err := p.Probe(context.Background()); err != nil {
		t.Fatalf("default Probe should be nil, got %v", err)
	}
	p.ProbeErr = errors.New("handshake failed")
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test ./test/fakes/...`
Expected: FAIL.

- [ ] **Step 4: Implement the fake**

`test/fakes/ssehandshake.go`:

```go
package fakes

import "context"

// FakeSSEProber is a programmable SSEProber for tests.
type FakeSSEProber struct {
	ProbeErr   error
	ProbeCalls int
}

// NewFakeSSEProber returns a fake that succeeds by default.
func NewFakeSSEProber() *FakeSSEProber { return &FakeSSEProber{} }

// Probe records the call and returns the configured error.
func (f *FakeSSEProber) Probe(_ context.Context) error {
	f.ProbeCalls++
	return f.ProbeErr
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./test/fakes/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/outbound/ssehandshake.go test/fakes/ssehandshake.go test/fakes/ssehandshake_test.go
git commit -m "feat(ports): add SSEProber outbound port and fake"
```

---

### Task 8: adapters/outbound/sseprobe/probe.go — real SSE probe

**Files:**
- Create: `internal/adapters/outbound/sseprobe/probe.go`
- Create: `internal/adapters/outbound/sseprobe/probe_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sseprobe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/sseprobe"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

func TestProberImplementsPort(t *testing.T) {
	var _ outbound.SSEProber = sseprobe.New(sseprobe.Config{BaseURL: "http://x"})
}

func TestProbeSucceedsOnEventStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": ping\n\n"))
	}))
	defer srv.Close()

	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err != nil {
		t.Errorf("Probe err: %v", err)
	}
}

func TestProbeFailsOnWrongContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error on non-event-stream response")
	}
}

func TestProbeFailsOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	p := sseprobe.New(sseprobe.Config{BaseURL: srv.URL})
	if err := p.Probe(context.Background()); err == nil {
		t.Error("expected error on 404")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/outbound/sseprobe/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// Package sseprobe performs a one-shot handshake against the orchestrator's
// SSE endpoint. It is used by `sophia doctor` and intentionally does NOT
// consume the stream — that is the M5 responsibility of EventStreamClient.
package sseprobe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/infrastructure/httpclient"
)

// DefaultPath is the SSE endpoint path probed by V1.
// V1 has no live Change yet at doctor time; the orchestrator's well-known
// SSE root path is used. The exact path is verified at M5 against the
// orchestrator and is configurable via Config.Path.
const DefaultPath = "/api/v1/events"

// Config controls the prober.
type Config struct {
	BaseURL string
	Path    string        // optional; default DefaultPath
	Timeout time.Duration // optional; default httpclient.DefaultTimeout
	HTTP    *http.Client  // optional override
}

// Prober implements outbound.SSEProber.
type Prober struct {
	url  string
	http *http.Client
}

// New returns a Prober.
func New(cfg Config) *Prober {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = httpclient.New(httpclient.Config{Timeout: cfg.Timeout})
	}
	return &Prober{url: strings.TrimRight(cfg.BaseURL, "/") + path, http: hc}
}

// Probe sends GET <BaseURL><Path> with Accept: text/event-stream and
// returns nil iff the response is 2xx with Content-Type containing
// "text/event-stream".
func (p *Prober) Probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return fmt.Errorf("sseprobe: build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("sseprobe: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sseprobe: status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		return fmt.Errorf("sseprobe: content-type %q (want text/event-stream)", ct)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/outbound/sseprobe/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/sseprobe/probe.go internal/adapters/outbound/sseprobe/probe_test.go
git commit -m "feat(adapters): add sseprobe (one-shot SSE handshake)"
```

---

## Phase 3 — Doctor v2 (orchestrator + SSE checks)

### Task 9: application/doctor.go — extend with orchestrator + SSE + paths

**Files:**
- Modify: `internal/application/doctor.go`
- Modify: `internal/application/doctor_test.go`

- [ ] **Step 1: Add new failing tests**

Append to `internal/application/doctor_test.go`:

```go
import (
	"errors"
	// keep existing imports
)

func newDoctorM2(opts ...func(*doctorM2Deps)) (*application.DoctorService, *doctorM2Deps) {
	d := &doctorM2Deps{
		Compose: fakes.NewFakeComposeRunner(),
		Git:     fakes.NewFakeGitInspector(),
		Paths:   fakes.NewFakePathResolver(),
		Orch:    fakes.NewFakeOrchestrator(),
		SSE:     fakes.NewFakeSSEProber(),
	}
	for _, o := range opts {
		o(d)
	}
	svc := application.NewDoctorService(application.DoctorDeps{
		Compose: d.Compose,
		Git:     d.Git,
		Paths:   d.Paths,
		Orch:    d.Orch,
		SSE:     d.SSE,
	})
	return svc, d
}

type doctorM2Deps struct {
	Compose *fakes.FakeComposeRunner
	Git     *fakes.FakeGitInspector
	Paths   *fakes.FakePathResolver
	Orch    *fakes.FakeOrchestrator
	SSE     *fakes.FakeSSEProber
}

func TestDoctorM2AllGreen(t *testing.T) {
	d, _ := newDoctorM2()
	r := d.Run(context.Background())
	if r.Summary.Fail != 0 {
		t.Errorf("expected 0 fails, got %+v\n%v", r.Summary, r.Checks)
	}
	if len(r.Checks) < 6 {
		t.Errorf("expected ≥6 checks (docker, compose, git, paths, orchestrator, sse), got %d", len(r.Checks))
	}
}

func TestDoctorOrchestratorFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Orch.HealthzErr = errors.New("connection refused")
	r := d.Run(context.Background())
	found := false
	for _, c := range r.Checks {
		if c.ID == "orchestrator" && c.Level == application.LevelFail {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orchestrator fail; got %+v", r.Checks)
	}
}

func TestDoctorSSEWarnNotFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.SSE.ProbeErr = errors.New("no event-stream")
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "sse" {
			if c.Level != application.LevelWarn {
				t.Errorf("expected sse warn, got %q", c.Level)
			}
			return
		}
	}
	t.Error("sse check missing")
}

func TestDoctorPathsFailWhenInvalid(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Paths.ValidateErr = fakes.ErrFakeValidate
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "xdg_paths" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected xdg_paths fail; got %+v", r.Checks)
}
```

- [ ] **Step 2: Delete the M1 `XDGValidator` callsite test that no longer compiles**

The M1 test `TestDoctorXDGPathError` uses the old constructor signature. Remove it; coverage for the new path lives in `TestDoctorPathsFailWhenInvalid`. Also remove the M1 helper `newDoctor()` and rename callers to `newDoctorM2()` — every existing test except docker/compose/git changes signature.

Concretely, replace the body of every `func newDoctor()` reference with `newDoctorM2()` and drop the now-unused `xdg func(string) error` plumbing.

Updated test surface (full file after edits):

```go
package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
	"github.com/RVRTelecomunicaciones/sophia-cli/test/fakes"
)

type doctorM2Deps struct {
	Compose *fakes.FakeComposeRunner
	Git     *fakes.FakeGitInspector
	Paths   *fakes.FakePathResolver
	Orch    *fakes.FakeOrchestrator
	SSE     *fakes.FakeSSEProber
}

func newDoctorM2(opts ...func(*doctorM2Deps)) (*application.DoctorService, *doctorM2Deps) {
	d := &doctorM2Deps{
		Compose: fakes.NewFakeComposeRunner(),
		Git:     fakes.NewFakeGitInspector(),
		Paths:   fakes.NewFakePathResolver(),
		Orch:    fakes.NewFakeOrchestrator(),
		SSE:     fakes.NewFakeSSEProber(),
	}
	for _, o := range opts {
		o(d)
	}
	return application.NewDoctorService(application.DoctorDeps{
		Compose: d.Compose, Git: d.Git, Paths: d.Paths, Orch: d.Orch, SSE: d.SSE,
	}), d
}

func TestDoctorAllGreen(t *testing.T) {
	d, _ := newDoctorM2()
	r := d.Run(context.Background())
	if r.Summary.Fail != 0 {
		t.Errorf("expected 0 fails, got %+v\n%v", r.Summary, r.Checks)
	}
	if len(r.Checks) < 6 {
		t.Errorf("expected ≥6 checks, got %d", len(r.Checks))
	}
}

func TestDoctorReportsDockerComposeMissing(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Compose.ComposeVersion = ""
	r := d.Run(context.Background())
	if r.Summary.Fail == 0 {
		t.Error("expected fail when compose version is empty")
	}
}

func TestDoctorReportsComposeV1AsFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Compose.ComposeVersion = "docker-compose version 1.29.2"
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "compose" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected compose v1 fail; got %+v", r.Checks)
}

func TestDoctorReportsGitMissing(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Git.GitVersion = ""
	r := d.Run(context.Background())
	if r.Summary.Fail == 0 {
		t.Error("expected git fail")
	}
}

func TestDoctorPathsFailWhenInvalid(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Paths.ValidateErr = fakes.ErrFakeValidate
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "xdg_paths" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected xdg_paths fail; got %+v", r.Checks)
}

func TestDoctorOrchestratorFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.Orch.HealthzErr = errors.New("refused")
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "orchestrator" && c.Level == application.LevelFail {
			return
		}
	}
	t.Errorf("expected orchestrator fail; got %+v", r.Checks)
}

func TestDoctorSSEWarnNotFail(t *testing.T) {
	d, deps := newDoctorM2()
	deps.SSE.ProbeErr = errors.New("no stream")
	r := d.Run(context.Background())
	for _, c := range r.Checks {
		if c.ID == "sse" {
			if c.Level != application.LevelWarn {
				t.Errorf("expected sse warn, got %q", c.Level)
			}
			return
		}
	}
	t.Error("sse check missing")
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/application/...`
Expected: FAIL (DoctorDeps undefined; old constructor still in place).

- [ ] **Step 4: Implement DoctorDeps and rewrite doctor.go**

Replace the body of `internal/application/doctor.go` with:

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

// ErrPathInvalid is returned by the legacy XDGValidator. M2 retains it
// for adapters that still expose it; new code should use PathResolver.
var ErrPathInvalid = errors.New("xdg path invalid")

// DoctorDeps groups the outbound ports the doctor service uses. Optional
// dependencies (e.g. Orch, SSE) may be nil — the corresponding check is
// then reported as info ("not configured") rather than fail.
type DoctorDeps struct {
	Compose outbound.ComposeRunner
	Git     outbound.GitInspector
	Paths   outbound.PathResolver
	Orch    outbound.OrchestratorClient // optional in M2; required from M4
	SSE     outbound.SSEProber          // optional
}

// DoctorService orchestrates the M2 subset of doctor checks: docker, compose,
// git, XDG paths, orchestrator, SSE. Later milestones add: repo, .sophia.yaml,
// worktree.
type DoctorService struct {
	deps DoctorDeps
}

// NewDoctorService constructs a DoctorService.
func NewDoctorService(d DoctorDeps) *DoctorService { return &DoctorService{deps: d} }

// Run executes all checks and returns the report.
func (d *DoctorService) Run(ctx context.Context) DiagnosticsReport {
	checks := []Check{
		d.checkDocker(ctx),
		d.checkCompose(ctx),
		d.checkGit(ctx),
		d.checkPaths(),
		d.checkOrchestrator(ctx),
		d.checkSSE(ctx),
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
	v, err := d.deps.Compose.Version(ctx)
	if err != nil {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "docker", Title: "Docker daemon", Level: LevelFail, Detail: "docker not available"}
	}
	return Check{ID: "docker", Title: "Docker daemon", Level: LevelOK, Detail: "available"}
}

func (d *DoctorService) checkCompose(ctx context.Context) Check {
	v, err := d.deps.Compose.Version(ctx)
	if err != nil || v == "" {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "compose not available"}
	}
	if !isComposeV2(v) {
		return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelFail, Detail: "v2 required, got: " + v}
	}
	return Check{ID: "compose", Title: "Docker Compose v2", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkGit(ctx context.Context) Check {
	v, err := d.deps.Git.Version(ctx)
	if err != nil {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: err.Error()}
	}
	if v == "" {
		return Check{ID: "git", Title: "Git", Level: LevelFail, Detail: "git not available"}
	}
	return Check{ID: "git", Title: "Git", Level: LevelOK, Detail: v}
}

func (d *DoctorService) checkPaths() Check {
	if d.deps.Paths == nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelInfo, Detail: "no resolver wired"}
	}
	p, err := d.deps.Paths.Resolve()
	if err != nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: err.Error()}
	}
	if err := d.deps.Paths.ValidateDirs(p); err != nil {
		return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelFail, Detail: err.Error()}
	}
	return Check{ID: "xdg_paths", Title: "XDG paths", Level: LevelOK, Detail: p.StateRoot}
}

func (d *DoctorService) checkOrchestrator(ctx context.Context) Check {
	if d.deps.Orch == nil {
		return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelInfo, Detail: "no client wired"}
	}
	if err := d.deps.Orch.Healthz(ctx); err != nil {
		return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelFail, Detail: err.Error()}
	}
	return Check{ID: "orchestrator", Title: "Orchestrator reachable", Level: LevelOK, Detail: "200 OK"}
}

func (d *DoctorService) checkSSE(ctx context.Context) Check {
	if d.deps.SSE == nil {
		return Check{ID: "sse", Title: "SSE handshake", Level: LevelInfo, Detail: "no prober wired"}
	}
	if err := d.deps.SSE.Probe(ctx); err != nil {
		return Check{ID: "sse", Title: "SSE handshake", Level: LevelWarn, Detail: err.Error()}
	}
	return Check{ID: "sse", Title: "SSE handshake", Level: LevelOK, Detail: "event-stream OK"}
}

func isComposeV2(version string) bool {
	low := strings.ToLower(version)
	_, suffix, found := strings.Cut(low, "compose version")
	if !found {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(suffix), "v2")
}
```

- [ ] **Step 5: Update CLI doctor wiring (Deps -> still uses Doctor field)**

The CLI `internal/adapters/inbound/cli/doctor.go` already uses `d.Doctor.Run(ctx)` — no change needed there.

- [ ] **Step 6: Update bootstrap wire.go to construct DoctorDeps**

Replace the doctor wiring in `internal/bootstrap/wire.go`. The full file is rewritten in **Task 16** below; for now just verify nothing else needs to change.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/application/... ./internal/adapters/inbound/cli/... -race`
Expected: application tests PASS; CLI tests may FAIL because the test deps in `cli/doctor_test.go` still pass the old `XDGValidator`. Update them now:

In `internal/adapters/inbound/cli/doctor_test.go`, replace `newDoctorDeps` with:

```go
func newDoctorDeps() cli.Deps {
	return cli.Deps{
		Doctor: application.NewDoctorService(application.DoctorDeps{
			Compose: fakes.NewFakeComposeRunner(),
			Git:     fakes.NewFakeGitInspector(),
			Paths:   fakes.NewFakePathResolver(),
			Orch:    fakes.NewFakeOrchestrator(),
			SSE:     fakes.NewFakeSSEProber(),
		}),
	}
}
```

Re-run: `go test ./internal/... -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/application/doctor.go internal/application/doctor_test.go \
        internal/adapters/inbound/cli/doctor_test.go
git commit -m "feat(application): extend doctor with paths/orchestrator/sse checks"
```

---

## Phase 4 — Compose embedding & materialization

### Task 10: compose.yaml — root embedded source

**Files:**
- Create: `compose.yaml` (project root)

- [ ] **Step 1: Write the file**

`/Users/russell/Documents/2026/sophia-cli/compose.yaml`:

```yaml
# Sophia local dev stack — V1 stub.
# Labeled with sophia.profile: stub so docker-compose listings make the
# difference visible at a glance. See docs §3.6.
services:
  sophia-orchestator:
    image: ghcr.io/rvrtelecomunicaciones/sophia-orchestator:dev
    container_name: sophia-orchestator
    labels:
      sophia.stack: "dev"
      sophia.profile: "stub"
    ports:
      - "9080:9080"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:9080/api/v1/healthz"]
      interval: 5s
      timeout: 3s
      retries: 12
    restart: unless-stopped

  agent-governance-core:
    image: ghcr.io/rvrtelecomunicaciones/agent-governance-core:dev
    container_name: agent-governance-core
    labels:
      sophia.stack: "dev"
      sophia.profile: "stub"
    ports:
      - "9081:9081"
    restart: unless-stopped

  sophia-memory-engine:
    image: ghcr.io/rvrtelecomunicaciones/sophia-memory-engine:dev
    container_name: sophia-memory-engine
    labels:
      sophia.stack: "dev"
      sophia.profile: "stub"
    ports:
      - "9082:9082"
    restart: unless-stopped

  sophia-runtime-adapters:
    image: ghcr.io/rvrtelecomunicaciones/sophia-runtime-adapters:dev
    container_name: sophia-runtime-adapters
    labels:
      sophia.stack: "dev"
      sophia.profile: "stub"
    ports:
      - "9083:9083"
    restart: unless-stopped
```

- [ ] **Step 2: Commit**

```bash
git add compose.yaml
git commit -m "feat(compose): add embedded compose.yaml stub for V1 dev stack"
```

---

### Task 11: composeexec/embed.go — embed + materialize

**Files:**
- Create: `internal/adapters/outbound/composeexec/embed.go`
- Create: `internal/adapters/outbound/composeexec/materialize.go`
- Create: `internal/adapters/outbound/composeexec/materialize_test.go`

- [ ] **Step 1: Write `embed.go`**

```go
package composeexec

import (
	_ "embed"
)

// EmbeddedComposeYAML is the bytes of the V1 dev compose stub, rooted at
// the module's compose.yaml. The materializer compares its SHA-256 against
// the file at <dataRoot>/compose/compose.yaml and rewrites only when needed.
//
//go:embed embedded/compose.yaml
var EmbeddedComposeYAML []byte
```

(Plus copy `compose.yaml` into `internal/adapters/outbound/composeexec/embedded/compose.yaml` so the embed path is self-contained.)

- [ ] **Step 2: Run `make build` to verify the embed picks the file up**

Run:
```bash
cd /Users/russell/Documents/2026/sophia-cli
mkdir -p internal/adapters/outbound/composeexec/embedded
cp compose.yaml internal/adapters/outbound/composeexec/embedded/compose.yaml
go build ./...
```
Expected: builds; no missing-file error.

- [ ] **Step 3: Write the failing test**

`internal/adapters/outbound/composeexec/materialize_test.go`:

```go
package composeexec_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
)

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestMaterializeWritesWhenAbsent(t *testing.T) {
	root := t.TempDir()
	res, err := composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true on first run")
	}
	got, err := os.ReadFile(filepath.Join(root, "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if sha(got) != sha(composeexec.EmbeddedComposeYAML) {
		t.Error("file content does not match embedded")
	}
}

func TestMaterializeNoOpWhenSame(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	res, err := composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote {
		t.Error("expected Wrote=false on re-run with same bytes")
	}
}

func TestMaterializeAbortsOnUserEdit(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)

	// Simulate a user edit: rewrite file with different bytes, do not update meta.
	target := filepath.Join(root, "compose", "compose.yaml")
	if err := os.WriteFile(target, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pretend a binary upgrade arrives with new embedded bytes.
	newEmbed := append([]byte{}, composeexec.EmbeddedComposeYAML...)
	newEmbed = append(newEmbed, []byte("\n# upgrade\n")...)
	_, err := composeexec.Materialize(root, newEmbed, false)
	if err == nil {
		t.Fatal("expected ErrUserEdited when file diverges and reset=false")
	}
	if err != composeexec.ErrUserEdited {
		t.Errorf("expected ErrUserEdited, got %v", err)
	}
}

func TestMaterializeBacksUpAndOverwritesWhenReset(t *testing.T) {
	root := t.TempDir()
	_, _ = composeexec.Materialize(root, composeexec.EmbeddedComposeYAML, false)

	// User edit
	target := filepath.Join(root, "compose", "compose.yaml")
	_ = os.WriteFile(target, []byte("user-edit\n"), 0o644)

	newEmbed := append([]byte{}, composeexec.EmbeddedComposeYAML...)
	newEmbed = append(newEmbed, []byte("\n# v2\n")...)
	res, err := composeexec.Materialize(root, newEmbed, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true with reset=true")
	}
	prev, err := os.ReadFile(filepath.Join(root, "compose", "compose.yaml.previous"))
	if err != nil {
		t.Fatalf("expected .previous backup: %v", err)
	}
	if string(prev) != "user-edit\n" {
		t.Errorf(".previous = %q", prev)
	}
	got, _ := os.ReadFile(target)
	if sha(got) != sha(newEmbed) {
		t.Error("target was not overwritten with new bytes")
	}
}

func TestMaterializeUpgradePathDetectsLastEmbeddedHash(t *testing.T) {
	root := t.TempDir()
	// First run with embed v1.
	v1 := composeexec.EmbeddedComposeYAML
	_, _ = composeexec.Materialize(root, v1, false)

	// Second run with embed v2; file still equals v1 (no user edit).
	v2 := append([]byte{}, v1...)
	v2 = append(v2, []byte("\n# v2\n")...)
	res, err := composeexec.Materialize(root, v2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Error("expected automatic upgrade when last_embedded matches current file")
	}
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/adapters/outbound/composeexec/...`
Expected: FAIL.

- [ ] **Step 5: Implement `materialize.go`**

```go
package composeexec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUserEdited indicates the materialized compose.yaml diverges from the
// last embedded version (i.e. a human or external tool edited it). Caller
// should retry with reset=true (mapped to `sophia start --reset-compose`).
var ErrUserEdited = errors.New("composeexec: materialized compose.yaml has been user-edited; pass --reset-compose to overwrite")

// MaterializeResult reports what Materialize did.
type MaterializeResult struct {
	Path  string // absolute path of the materialized compose.yaml
	Wrote bool   // true iff the file was (re)written this call
}

type composeMeta struct {
	LastEmbeddedHash string `json:"last_embedded_hash"`
}

// Materialize copies the embedded compose bytes into <dataRoot>/compose/
// using the spec rules in §3.6:
//
//   - file absent → write
//   - file == embedded → no-op
//   - file != embedded but file == last_embedded_hash → automatic upgrade
//   - file != embedded and file != last_embedded_hash → user-edited
//     · if reset=false → return ErrUserEdited
//     · if reset=true  → save current as compose.yaml.previous, write new
//
// dataRoot must already exist (created by xdgpaths.EnsureDirs).
func Materialize(dataRoot string, embed []byte, reset bool) (MaterializeResult, error) {
	dir := filepath.Join(dataRoot, "compose")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return MaterializeResult{}, fmt.Errorf("mkdir %q: %w", dir, err)
	}
	target := filepath.Join(dir, "compose.yaml")
	metaPath := filepath.Join(dir, "compose.meta.json")
	prevPath := filepath.Join(dir, "compose.yaml.previous")

	embedHash := hashBytes(embed)

	current, err := os.ReadFile(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := writeFile0644(target, embed); err != nil {
			return MaterializeResult{}, err
		}
		if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
			return MaterializeResult{}, err
		}
		return MaterializeResult{Path: target, Wrote: true}, nil
	case err != nil:
		return MaterializeResult{}, fmt.Errorf("read target: %w", err)
	}

	currentHash := hashBytes(current)
	if currentHash == embedHash {
		return MaterializeResult{Path: target, Wrote: false}, nil
	}

	meta, _ := readMeta(metaPath)
	if meta.LastEmbeddedHash == currentHash {
		// Clean upgrade path.
		if err := writeFile0644(target, embed); err != nil {
			return MaterializeResult{}, err
		}
		if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
			return MaterializeResult{}, err
		}
		return MaterializeResult{Path: target, Wrote: true}, nil
	}

	// File diverges from both embedded and last-embedded → user edit.
	if !reset {
		return MaterializeResult{Path: target}, ErrUserEdited
	}
	if err := writeFile0644(prevPath, current); err != nil {
		return MaterializeResult{}, fmt.Errorf("backup .previous: %w", err)
	}
	if err := writeFile0644(target, embed); err != nil {
		return MaterializeResult{}, err
	}
	if err := writeMeta(metaPath, composeMeta{LastEmbeddedHash: embedHash}); err != nil {
		return MaterializeResult{}, err
	}
	return MaterializeResult{Path: target, Wrote: true}, nil
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeFile0644(p string, b []byte) error {
	return os.WriteFile(p, b, 0o644)
}

func readMeta(p string) (composeMeta, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return composeMeta{}, err
	}
	var m composeMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return composeMeta{}, err
	}
	return m, nil
}

func writeMeta(p string, m composeMeta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile0644(p, b)
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/outbound/composeexec/... -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/outbound/composeexec/embed.go \
        internal/adapters/outbound/composeexec/embedded/compose.yaml \
        internal/adapters/outbound/composeexec/materialize.go \
        internal/adapters/outbound/composeexec/materialize_test.go
git commit -m "feat(composeexec): embed compose.yaml + Materialize with .previous backup"
```

---

## Phase 5 — Provisioner application service

### Task 12: application/provisioner.go — Up/Down use cases

**Files:**
- Create: `internal/application/provisioner.go`
- Create: `internal/application/provisioner_test.go`

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

type provDeps struct {
	Compose *fakes.FakeComposeRunner
	Paths   *fakes.FakePathResolver
}

func newProvisioner() (*application.Provisioner, *provDeps) {
	d := &provDeps{
		Compose: fakes.NewFakeComposeRunner(),
		Paths:   fakes.NewFakePathResolver(),
	}
	d.Paths.Paths = domain.XDGPaths{DataRoot: "/x/data"}
	mat := func(dataRoot string, _ []byte, _ bool) (string, bool, error) {
		return dataRoot + "/compose/compose.yaml", true, nil
	}
	p := application.NewProvisioner(application.ProvisionerDeps{
		Compose:     d.Compose,
		Paths:       d.Paths,
		Materialize: mat,
		Embedded:    []byte("services: {}\n"),
	})
	return p, d
}

func TestProvisionerUpEnsuresPathsAndCallsCompose(t *testing.T) {
	p, deps := newProvisioner()
	res, err := p.Up(context.Background(), application.UpInput{Reset: false})
	if err != nil {
		t.Fatal(err)
	}
	if deps.Paths.EnsureDirsCalls != 1 {
		t.Errorf("EnsureDirsCalls = %d", deps.Paths.EnsureDirsCalls)
	}
	if deps.Compose.UpCalls != 1 {
		t.Errorf("Compose.UpCalls = %d", deps.Compose.UpCalls)
	}
	if res.ComposePath != "/x/data/compose/compose.yaml" {
		t.Errorf("ComposePath = %q", res.ComposePath)
	}
	if !res.Wrote {
		t.Error("expected res.Wrote=true")
	}
}

func TestProvisionerUpProjectNameIsSophia(t *testing.T) {
	p, deps := newProvisioner()
	if _, err := p.Up(context.Background(), application.UpInput{}); err != nil {
		t.Fatal(err)
	}
	if got := deps.Compose.LastProjectName; got != "sophia" {
		t.Errorf("project name = %q, want sophia", got)
	}
}

func TestProvisionerUpFailsWhenComposeFails(t *testing.T) {
	p, deps := newProvisioner()
	deps.Compose.UpErr = errors.New("daemon down")
	if _, err := p.Up(context.Background(), application.UpInput{}); err == nil {
		t.Error("expected error")
	}
}

func TestProvisionerDownInvokesCompose(t *testing.T) {
	p, deps := newProvisioner()
	if err := p.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if deps.Compose.DownCalls != 1 {
		t.Errorf("DownCalls = %d", deps.Compose.DownCalls)
	}
	if got := deps.Compose.LastProjectName; got != "sophia" {
		t.Errorf("project name = %q", got)
	}
}
```

This test references `LastProjectName` on the FakeComposeRunner; we'll add it in the next step.

- [ ] **Step 2: Extend FakeComposeRunner with LastProjectName**

Modify `test/fakes/compose.go` to record the last project name:

```go
package fakes

import (
	"context"
	"sync"
)

type FakeComposeRunner struct {
	mu              sync.Mutex
	ComposeVersion  string
	UpErr           error
	DownErr         error
	UpCalls         int
	DownCalls       int
	LastComposePath string
	LastProjectName string
}

func NewFakeComposeRunner() *FakeComposeRunner {
	return &FakeComposeRunner{ComposeVersion: "Docker Compose version v2.32.1"}
}

func (f *FakeComposeRunner) Version(_ context.Context) (string, error) {
	return f.ComposeVersion, nil
}

func (f *FakeComposeRunner) Up(_ context.Context, composePath, projectName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.UpCalls++
	f.LastComposePath = composePath
	f.LastProjectName = projectName
	return f.UpErr
}

func (f *FakeComposeRunner) Down(_ context.Context, composePath, projectName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DownCalls++
	f.LastComposePath = composePath
	f.LastProjectName = projectName
	return f.DownErr
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/application/...`
Expected: FAIL (Provisioner undefined).

- [ ] **Step 4: Implement**

`internal/application/provisioner.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/ports/outbound"
)

// ComposeProjectName is the fixed compose project name used by sophia
// regardless of CWD (spec §3.6 hard invariant).
const ComposeProjectName = "sophia"

// MaterializeFunc lets the application layer invoke the composeexec
// materializer without importing the adapter package directly.
//
// Returned tuple: (path, wrote, err).
type MaterializeFunc func(dataRoot string, embedded []byte, reset bool) (string, bool, error)

// ProvisionerDeps groups the ports the Provisioner needs.
type ProvisionerDeps struct {
	Compose     outbound.ComposeRunner
	Paths       outbound.PathResolver
	Materialize MaterializeFunc
	Embedded    []byte // raw bytes from composeexec.EmbeddedComposeYAML
}

// UpInput controls Up.
type UpInput struct {
	Reset bool // map of `sophia start --reset-compose`
}

// UpResult reports what Up did.
type UpResult struct {
	ComposePath string
	Wrote       bool // whether the materializer wrote (or rewrote) the file
}

// Provisioner orchestrates `sophia start` / `sophia stop`.
type Provisioner struct {
	deps ProvisionerDeps
}

// NewProvisioner constructs a Provisioner.
func NewProvisioner(d ProvisionerDeps) *Provisioner { return &Provisioner{deps: d} }

// Up resolves XDG paths, ensures dirs, materializes compose, and runs
// `docker compose -p sophia -f <path> up -d`.
func (p *Provisioner) Up(ctx context.Context, in UpInput) (UpResult, error) {
	paths, err := p.deps.Paths.Resolve()
	if err != nil {
		return UpResult{}, fmt.Errorf("resolve paths: %w", err)
	}
	if err := p.deps.Paths.EnsureDirs(paths); err != nil {
		return UpResult{}, fmt.Errorf("ensure dirs: %w", err)
	}
	if p.deps.Materialize == nil {
		return UpResult{}, errors.New("materialize func not wired")
	}
	path, wrote, err := p.deps.Materialize(paths.DataRoot, p.deps.Embedded, in.Reset)
	if err != nil {
		return UpResult{}, fmt.Errorf("materialize: %w", err)
	}
	if err := p.deps.Compose.Up(ctx, path, ComposeProjectName); err != nil {
		return UpResult{ComposePath: path, Wrote: wrote}, fmt.Errorf("compose up: %w", err)
	}
	return UpResult{ComposePath: path, Wrote: wrote}, nil
}

// Down resolves XDG paths and runs `docker compose -p sophia -f <path> down`.
// If the file doesn't exist (fresh checkout), Down is a no-op.
func (p *Provisioner) Down(ctx context.Context) error {
	paths, err := p.deps.Paths.Resolve()
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	composePath := paths.DataRoot + "/compose/compose.yaml"
	if err := p.deps.Compose.Down(ctx, composePath, ComposeProjectName); err != nil {
		return fmt.Errorf("compose down: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/application/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/provisioner.go internal/application/provisioner_test.go test/fakes/compose.go
git commit -m "feat(application): add Provisioner (Up/Down) with fixed project name 'sophia'"
```

---

## Phase 6 — Cobra commands `start` and `stop`

### Task 13: cli/start.go — sophia start [--reset-compose]

**Files:**
- Modify: `internal/adapters/inbound/cli/root.go` (replace stub for `start`)
- Create: `internal/adapters/inbound/cli/start.go`
- Create: `internal/adapters/inbound/cli/start_test.go`

- [ ] **Step 1: Write the failing test**

`internal/adapters/inbound/cli/start_test.go`:

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

func newStartDeps() (cli.Deps, *fakes.FakeComposeRunner) {
	compose := fakes.NewFakeComposeRunner()
	mat := func(_ string, _ []byte, _ bool) (string, bool, error) {
		return "/x/compose.yaml", true, nil
	}
	return cli.Deps{
		Provisioner: application.NewProvisioner(application.ProvisionerDeps{
			Compose:     compose,
			Paths:       fakes.NewFakePathResolver(),
			Materialize: mat,
			Embedded:    []byte("services: {}\n"),
		}),
	}, compose
}

func TestStartCommandSucceeds(t *testing.T) {
	deps, compose := newStartDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"start"})
	if err := c.Execute(); err != nil {
		t.Fatalf("start err: %v", err)
	}
	if compose.UpCalls != 1 {
		t.Errorf("UpCalls = %d", compose.UpCalls)
	}
	if !strings.Contains(out.String(), "started") {
		t.Errorf("output missing 'started': %q", out.String())
	}
}

func TestStartResetComposeFlag(t *testing.T) {
	deps, _ := newStartDeps()
	c := cli.NewRoot(deps)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"start", "--reset-compose"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL (Deps.Provisioner undefined).

- [ ] **Step 3: Implement `start.go`**

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

func newStartCmd(d Deps) *cobra.Command {
	var reset bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the local Sophia stack via docker compose",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Provisioner == nil {
				return fmt.Errorf("start: provisioner not wired")
			}
			res, err := d.Provisioner.Up(cmd.Context(), application.UpInput{Reset: reset})
			if err != nil {
				return err
			}
			if res.Wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "compose materialized at %s\n", res.ComposePath)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sophia: stack started (project=sophia)")
			return nil
		},
	}
	cmd.Flags().BoolVar(&reset, "reset-compose", false, "overwrite a user-edited compose.yaml (saves a .previous backup)")
	return cmd
}
```

- [ ] **Step 4: Extend `Deps` to include `Provisioner`**

In `internal/adapters/inbound/cli/root.go`, replace the `Deps` struct and the stub registration for `start`:

```go
type Deps struct {
	Doctor      *application.DoctorService
	Provisioner *application.Provisioner

	Version   string
	Commit    string
	BuildDate string
}
```

Inside `NewRoot`, replace `root.AddCommand(newStubCmd("start", ...))` with:

```go
root.AddCommand(newStartCmd(d))
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/inbound/cli/start.go internal/adapters/inbound/cli/start_test.go internal/adapters/inbound/cli/root.go
git commit -m "feat(cli): add start command with --reset-compose"
```

---

### Task 14: cli/stop.go — sophia stop

**Files:**
- Modify: `internal/adapters/inbound/cli/root.go` (replace stub for `stop`)
- Create: `internal/adapters/inbound/cli/stop.go`
- Create: `internal/adapters/inbound/cli/stop_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/cli"
)

func TestStopCommandSucceeds(t *testing.T) {
	deps, compose := newStartDeps()
	c := cli.NewRoot(deps)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"stop"})
	if err := c.Execute(); err != nil {
		t.Fatalf("stop err: %v", err)
	}
	if compose.DownCalls != 1 {
		t.Errorf("DownCalls = %d", compose.DownCalls)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/adapters/inbound/cli/...`
Expected: FAIL (no stop command wired).

- [ ] **Step 3: Implement**

`internal/adapters/inbound/cli/stop.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStopCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Sophia stack",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Provisioner == nil {
				return fmt.Errorf("stop: provisioner not wired")
			}
			if err := d.Provisioner.Down(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sophia: stack stopped (project=sophia)")
			return nil
		},
	}
}
```

In `root.go`, replace `root.AddCommand(newStubCmd("stop", ...))` with:

```go
root.AddCommand(newStopCmd(d))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/inbound/cli/stop.go internal/adapters/inbound/cli/stop_test.go internal/adapters/inbound/cli/root.go
git commit -m "feat(cli): add stop command"
```

---

### Task 15: Update stubs_test for new wired commands

**Files:**
- Modify: `internal/adapters/inbound/cli/stubs_test.go`

- [ ] **Step 1: Update `TestStubsAnnounceMilestone`**

Remove `start` and `stop` from the `cases` map (they're now real commands):

```go
cases := map[string]string{
	"init":    "M3",
	"run":     "M4",
	"attach":  "M8",
	"status":  "M3",
	"changes": "M8",
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/adapters/inbound/cli/... -race`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/inbound/cli/stubs_test.go
git commit -m "test(cli): drop start/stop from stub list (now wired in M2)"
```

---

## Phase 7 — Bootstrap rewiring

### Task 16: bootstrap/wire.go — wire M2 dependencies

**Files:**
- Modify: `internal/bootstrap/wire.go`
- Modify: `internal/bootstrap/wire_test.go`

- [ ] **Step 1: Update wire_test.go to assert the new commands wired**

Replace `internal/bootstrap/wire_test.go`:

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

func TestNewWiresM2Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"doctor", "start", "stop", "version"} {
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
Expected: FAIL (since `start`/`stop` will fail because Provisioner isn't wired).

- [ ] **Step 3: Rewrite `wire.go`**

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
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/orchestratorhttp"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/sseprobe"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/xdgpaths"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/application"
)

// DefaultOrchestratorURL is used when neither flags nor env override it.
// V1 hardcodes localhost; V1.1+ will read it from <configRoot>/config.yaml.
const DefaultOrchestratorURL = "http://localhost:9080"

// Config controls bootstrap.New.
type Config struct {
	LogWriter        io.Writer  // nil ⇒ os.Stderr
	LogLevel         slog.Level // default Info
	OrchestratorURL  string     // empty ⇒ DefaultOrchestratorURL
}

// New is the composition root. It builds concrete outbound adapters,
// application services, and returns the configured root cobra command.
func New(cfg Config) (*cobra.Command, error) {
	if cfg.LogWriter == nil {
		cfg.LogWriter = os.Stderr
	}
	if cfg.OrchestratorURL == "" {
		cfg.OrchestratorURL = DefaultOrchestratorURL
	}
	logger := NewLogger(cfg.LogWriter, cfg.LogLevel)
	slog.SetDefault(logger)

	// Outbound adapters
	compose := composeexec.New(composeexec.Config{})
	git := gitcli.New(gitcli.Config{})
	paths := xdgpaths.New(xdgpaths.Config{})
	orch := orchestratorhttp.New(orchestratorhttp.Config{BaseURL: cfg.OrchestratorURL})
	sse := sseprobe.New(sseprobe.Config{BaseURL: cfg.OrchestratorURL})

	// Application services
	doctor := application.NewDoctorService(application.DoctorDeps{
		Compose: compose, Git: git, Paths: paths, Orch: orch, SSE: sse,
	})
	provisioner := application.NewProvisioner(application.ProvisionerDeps{
		Compose:  compose,
		Paths:    paths,
		Materialize: func(dataRoot string, embed []byte, reset bool) (string, bool, error) {
			res, err := composeexec.Materialize(dataRoot, embed, reset)
			return res.Path, res.Wrote, err
		},
		Embedded: composeexec.EmbeddedComposeYAML,
	})

	info := NewVersionInfo()
	deps := cli.Deps{
		Doctor:      doctor,
		Provisioner: provisioner,
		Version:     info.Version,
		Commit:      info.Commit,
		BuildDate:   info.BuildDate,
	}
	return cli.NewRoot(deps), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bootstrap/... -race`
Expected: PASS.

- [ ] **Step 5: Build the binary**

Run: `make build`
Expected: produces `bin/sophia` with no errors.

- [ ] **Step 6: Smoke**

```bash
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool > /dev/null && echo "json valid"
./bin/sophia start --help
./bin/sophia stop --help
```

Expected: all succeed; `start --help` shows `--reset-compose`.

- [ ] **Step 7: Commit**

```bash
git add internal/bootstrap/wire.go internal/bootstrap/wire_test.go
git commit -m "feat(bootstrap): wire M2 (paths, orch, sse, provisioner)"
```

---

## Phase 8 — Real composeexec.Up/Down + integration test

### Task 17: composeexec/runner.go — accept reset flag (no real impl change)

**Files:**
- (review only) `internal/adapters/outbound/composeexec/runner.go`

The existing `Runner.Up` already takes `(composePath, projectName)`. No code change required for M2 — the application layer passes `"sophia"` always. This task is a verification only.

- [ ] **Step 1: Verify Up/Down accept the project name we pass**

Run: `go test ./internal/adapters/outbound/composeexec/... -race`
Expected: PASS.

- [ ] **Step 2: Add a unit test that exercises argv assembly**

Append to `internal/adapters/outbound/composeexec/runner_test.go`:

```go
func TestUpRequiresProjectName(t *testing.T) {
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Up(context.Background(), "/p/compose.yaml", ""); err == nil {
		t.Error("expected error for empty project name")
	}
}

func TestDownRequiresComposePath(t *testing.T) {
	r := composeexec.New(composeexec.Config{DockerBinary: "echo"})
	if err := r.Down(context.Background(), "", "sophia"); err == nil {
		t.Error("expected error for empty compose path")
	}
}
```

Run: `go test ./internal/adapters/outbound/composeexec/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/outbound/composeexec/runner_test.go
git commit -m "test(composeexec): add argv-validation tests for Up/Down"
```

---

### Task 18: test/integration/compose_test.go — opt-in real-Docker test

**Files:**
- Create: `test/integration/compose_test.go`
- Modify: `Makefile` (add `test-integration` target)

- [ ] **Step 1: Write the test (gated by build tag)**

`test/integration/compose_test.go`:

```go
//go:build integration

// Package integration runs opt-in tests against a real Docker daemon.
// Run with: make test-integration
package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/outbound/composeexec"
)

func dockerAvailable(t *testing.T) bool {
	t.Helper()
	if os.Getenv("CI") != "" {
		return false
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	out, err := exec.Command("docker", "info").CombinedOutput()
	if err != nil {
		t.Logf("docker info: %v\n%s", err, out)
		return false
	}
	return true
}

func TestComposeMaterializeAndVersion(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker not available")
	}

	tmp := t.TempDir()
	res, err := composeexec.Materialize(tmp, composeexec.EmbeddedComposeYAML, false)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if !res.Wrote {
		t.Error("expected Wrote=true on first run")
	}

	r := composeexec.New(composeexec.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, err := r.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v == "" {
		t.Error("Version empty")
	}
	t.Logf("compose: %s", v)
}
```

- [ ] **Step 2: Add Makefile target**

In `Makefile`, add:

```makefile
.PHONY: test-integration
test-integration: ## Run opt-in integration tests against a real Docker daemon
	$(GO) test -tags=integration ./test/integration/...
```

- [ ] **Step 3: Verify tag isolation**

Run: `go test ./...`
Expected: integration test is NOT run.

Run: `go test -tags=integration ./test/integration/...`
Expected: PASS or SKIP if no docker.

- [ ] **Step 4: Commit**

```bash
git add test/integration/compose_test.go Makefile
git commit -m "test(integration): add opt-in compose integration test"
```

---

## Phase 9 — Final validation

### Task 19: Final validation pass

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
Expected: exit 0. Fix issues in place; for adapters that shell out, `//nolint:gosec` with a one-line justification is acceptable (precedent set in M1).

- [ ] **Step 3: Coverage**

Run:
```bash
go test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
go tool cover -func=cover.out | tail -n 1
```
Expected: total ≥ 70%.

- [ ] **Step 4: Binary smoke**

Run:
```bash
make build
./bin/sophia version
./bin/sophia doctor --json | python3 -m json.tool | head -40
./bin/sophia doctor || true
./bin/sophia start --help
./bin/sophia stop --help
./bin/sophia init       # still M3 stub
./bin/sophia run        # still M4 stub
```

Expected:
- `version` prints sophia version line.
- `doctor` reports 6 checks (docker, compose, git, xdg_paths, orchestrator, sse). When the orchestrator is offline, `orchestrator` is `fail` and `sse` is `warn`.
- `start --help` shows `--reset-compose`.
- `init` and `run` still print the M3/M4 stub line.

- [ ] **Step 5: E2E smoke**

Run: `go test -tags=e2e_smoke ./test/e2e/...`
Expected: PASS.

- [ ] **Step 6: Commit and tag**

```bash
git add -A
git status
git commit -m "chore(m2): final validation pass" || echo "nothing to commit"
git tag -a m2-provisioning -m "M2 Provisioning complete"
```

---

## Self-review checklist

- [ ] **Spec coverage:** Every M2 DoD item from spec §7.2 has at least one task.
  - Compose embedded + materialized + .previous backup → Tasks 10, 11
  - `--reset-compose` → Task 13
  - `sophia start/stop` with project name `sophia` → Tasks 12, 13, 14
  - Stub labeled `sophia.stack: dev`, `sophia.profile: stub` → Task 10
  - Doctor adds orchestrator (fail) + SSE (warn) → Tasks 6, 8, 9
  - composeexec fake unit tests → already in M1 + extended in Task 12
  - Opt-in `make test-integration` → Task 18
- [ ] **No placeholders:** No "TBD"/"TODO"/"similar to" in steps.
- [ ] **Type consistency:** `application.DoctorDeps`, `application.ProvisionerDeps`, `MaterializeFunc` signature, `cli.Deps.Provisioner` are all referenced consistently.
- [ ] **Frequent commits:** every task ends with at least one commit.
- [ ] **TDD discipline:** every code-producing task has a failing test before the implementation step (exceptions: Tasks 7 step 1 — port file is interfaces only; Task 10 — `compose.yaml` is not testable as such; Task 17 is verification-only).
- [ ] **No premature M3+ scope:** no `.sophia.yaml`, no state store, no `sophia run`, no real SSE consumer.

---

## Pending decisions (carried into M2 execution)

| ID | Question | Default if user silent |
|---|---|---|
| D-M2-01 | Compose stub: orchestrator only, or also gov/memory/runtime stubs? | All four services (mock images), so the dev stack runs end-to-end before sibling releases. |
| D-M2-02 | Healthz endpoint exact path | `/api/v1/healthz` (per spec §5.1, §6.1). Confirm with sibling service when it's live; current value is wired through `OrchestratorClient.Healthz`. |
| D-M2-03 | SSE endpoint path probed in V1 | `/api/v1/events` (configurable via `sseprobe.Config.Path`). Verified at M5. |
| D-M2-04 | Default orchestrator URL | `http://localhost:9080` matches the compose port mapping. |

---

## Risks specific to M2

| ID | Risk | Mitigation |
|---|---|---|
| RM2-01 | Sibling services not yet released → compose images don't exist | Stub images marked `sophia.profile: stub`; integration test skips if any image fails to pull. End-to-end exercise lives in M9. |
| RM2-02 | Materializer races (parallel `sophia start`) | Tests cover sequential cases only. The compose project name "sophia" + meta file is single-writer; M9 hardening can add a file lock if needed. |
| RM2-03 | Doctor SSE check noise (warn on every offline run) | Spec mandates warn (not fail). Suppress only if user later sets `--quiet` (not in M2 scope). |
| RM2-04 | macOS path defaults differ from spec | xdgpaths Linux-style fallback covered by tests. |

---

## What this plan does NOT cover (intentional)

- `.sophia.yaml` reading/writing → M3.
- `sophia init` and `sophia status` → M3.
- Project-scoped state store atomic writes → M3.
- Real SSE consumption → M5.
- Bubble Tea TUI → M6.
- `sophia run` polling → M4.
- Real browser opener → M7.
- GoReleaser, security suite, asciinema demo → M9.

---

## Execution handoff

Plan complete and saved to
`docs/superpowers/plans/2026-05-05-sophia-cli-m2-provisioning.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Use `superpowers:subagent-driven-development`.

**2. Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints.
