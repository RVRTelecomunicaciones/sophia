# Sophia CLI — M9 Hardening & Release Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `sophia-cli v0.1.0` by hardening release infrastructure and documentation. M1–M8 delivered the feature surface; M9 wraps that surface in the artifacts a public release demands — license, README, changelog, ADRs, reproducible builds, signed checksums, automated release pipeline, security scans, and a canonical demo. The first taggable release lives at the end of M9 as `v0.1.0`.

**Out of scope (explicit anti-list — DO NOT add):**

- NO new features.
- NO new commands.
- NO Context7/MCP integration.
- NO profiles.
- NO additional persistent memory beyond what M5–M8 already ship.
- NO authentication flows.
- NO remote endpoints.
- NO new application-layer logic.
- NO breaking changes to the wire format, CLI flags, or domain types shipped in M1–M8.

If a task tempts you toward any of the above, STOP and re-scope. M9 is a packaging milestone, not a feature milestone.

**Architecture:** The runtime architecture is frozen at the M8 tag (`m8-attach-changes-status`). M9 only touches:

- Repository root: `LICENSE`, `README.md`, `CHANGELOG.md`, `.goreleaser.yaml`, `Makefile` additions.
- `docs/adr/` (new directory): `0001-hexagonal-architecture.md`, `0002-bubbletea-v2-charm-land.md`.
- `.github/workflows/`: a new `release.yml`; updates to `ci.yml` to bump Go to 1.26.x and add security/vuln scans.
- `test/e2e/`: a final pass to ensure tag-gated tests run cleanly and document the manual smoke checklist.
- `assets/demo/`: asciinema cast file plus README inclusion.
- (Optional) `test/tui/`: a teatest for `attachTUI` to recover the M8 1% coverage gap.

**M9 dependency graph** (top-to-bottom; horizontal items are independent):

```
Phase 1 — Documentation foundations
    Task 1 (LICENSE)
    Task 2 (README quickstart)        ← depends on Task 1 for license badge
    Task 3 (CHANGELOG)
    Task 4 (ADR-0001 architecture)
    Task 5 (ADR-0002 bubbletea v2)

Phase 2 — Release infrastructure
    Task 6 (.goreleaser.yaml + Makefile release targets)
    Task 7 (GitHub Actions release workflow + CI hardening)
            ← depends on Task 6

Phase 3 — Quality gates
    Task 8 (Security suite: gosec + govulncheck + license check)
    Task 9 (E2E suite final pass + manual smoke checklist)

Phase 4 — Demo + optional coverage
    Task 10 (Asciinema demo)         ← depends on Task 2 (README links the cast)
    Task 11 (Optional: attachTUI teatest, recover the 1% coverage gap)

Phase 5 — Release
    Task 12 (v0.1.0 final validation + tag + GitHub release)
            ← depends on every prior task
```

Tasks within a phase can run in any order unless an explicit dependency is noted. Task 12 is the only true sequential gate.

---

## Phase 1 — Documentation foundations

### Task 1: `LICENSE` — Apache-2.0 (D-M9-01)

**Files:**
- Create: `LICENSE`

**Decision (D-M9-01):** License is Apache-2.0. Rationale: enterprise Go ecosystem default (Kubernetes, gRPC, Prometheus, OpenTelemetry, etcd, Terraform); patent grant covers contributors; permissive enough for downstream commercial use; explicit attribution clause. If you want MIT or BSD-3-Clause instead, flip this BEFORE starting Task 2 (README references the license).

- [ ] **Step 1: Write the LICENSE file**

Use the canonical Apache-2.0 text verbatim. Curl-able from `https://www.apache.org/licenses/LICENSE-2.0.txt` or paste the exact 202-line standard form. Replace the bracketed placeholders at the end with:

- `[yyyy]` → `2026`
- `[name of copyright owner]` → `RVR Telecomunicaciones`

Do NOT modify any other line of the license text.

- [ ] **Step 2: Verify**

```bash
head -1 LICENSE   # → "                                 Apache License"
wc -l LICENSE     # → 202
```

- [ ] **Step 3: Commit**

```bash
git add LICENSE
git commit -m "docs: add Apache-2.0 LICENSE (D-M9-01)"
```

---

### Task 2: `README.md` — quickstart, install, usage, links

**Files:**
- Create: `README.md`

The README is the public face of the binary. Required sections:

1. **Title + tagline** (one line: "Sophia CLI — agentic change orchestrator companion").
2. **Status badges**: Go version, license (Apache-2.0), CI status, latest release.
3. **What is sophia?** (3–5 sentences. Describes the spec §2.2 vision: drives an agentic orchestrator over local Docker compose, observes Changes via SSE, supports approval gates).
4. **Quick install** (Homebrew tap placeholder, `go install`, manual binary download from releases).
5. **Quickstart** (one of: `sophia doctor` → `sophia start` → `sophia init` → `sophia run "msg"` → expected output snippet).
6. **Commands** (one-line summary per command: `doctor`, `start`, `stop`, `init`, `version`, `run`, `attach`, `changes`, `status`).
7. **Configuration** (`.sophia.yaml` minimal example; XDG state paths; `SOPHIA_*` env vars).
8. **Demo** (link to `assets/demo/sophia-quickstart.cast` — Task 10 produces this).
9. **Architecture** (one paragraph + link to `docs/adr/0001-hexagonal-architecture.md`).
10. **Contributing** (link to `CONTRIBUTING.md` if it exists; otherwise placeholder pointing at issues).
11. **License** (Apache-2.0, link to LICENSE).

- [ ] **Step 1: Draft `README.md`**

Aim for ~150–250 lines. Use Markdown headers (`##`), fenced code blocks, no HTML.

> **Note on badges:** the CI badge URL depends on the GitHub repo path — derive from `git remote get-url origin`. The latest-release badge can use `https://img.shields.io/github/v/release/<owner>/<repo>` or be added in Task 12 once `v0.1.0` exists. License badge: `https://img.shields.io/badge/license-Apache--2.0-blue.svg`.

- [ ] **Step 2: Verify command surface matches reality**

```bash
./bin/sophia --help | rg -e doctor -e start -e stop -e init -e version -e run -e attach -e changes -e status
```

Every command listed in the README must be present in the binary's help output. NO commands listed in the README that don't exist.

- [ ] **Step 3: Verify quickstart commands actually work**

Walk through every command in the README's quickstart section against a fresh checkout. Each command must either succeed or fail with the documented exit code. If a command produces different output than what the README claims, fix the README — NOT the command (M9 is feature-frozen).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add README with quickstart, install, and command reference"
```

---

### Task 3: `CHANGELOG.md` — Keep a Changelog format, populate M1–M8

**Files:**
- Create: `CHANGELOG.md`

Use the [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) format with [Semantic Versioning 2.0.0](https://semver.org/). Every milestone tag M1–M8 maps to a section; a fresh `[Unreleased]` section sits at the top.

Section template per milestone:

```markdown
## [m8-attach-changes-status] — 2026-05-06
### Added
- `sophia attach <change-id>` …
- `sophia changes [--limit N] [--status S] [--project P] [--json]` …
- `sophia status [<change-id>] [--json]` real fetch (was M3 placeholder).

### Changed
- `application.StatusReader.Resolve` signature now takes `ResolveInput` and returns `StatusReport` with the live snapshot.

### Fixed
- `cli.approvalTimeoutSink.startTimer` no longer resets the timer on re-emit (D-M8-13).

### Internal
- Extracted `Runner.Observe(ctx, RunResult, sink)` as the shared observation engine for `Run` and `Attach`.
```

> **Verification gate:** before populating, walk `git log --oneline | rg "^[a-f0-9]+ feat\|fix\|refactor\|test\|docs"` to enumerate every commit since the repo's first commit. Map commits to milestones via the tag list (`git tag -l 'm*'`). A commit that doesn't fit any tagged milestone goes under `[Unreleased]`.

- [ ] **Step 1: Survey commit history**

```bash
git log --oneline --reverse | head -200
git tag -l 'm*' --sort=v:refname
```

- [ ] **Step 2: Draft `CHANGELOG.md`**

Sections (top-to-bottom):

```
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
(empty unless something landed since the M8 tag and the v0.1.0 cut)

## [m8-attach-changes-status] — 2026-05-06
…

## [m7-applyboard-approval] — 2026-05-06
…

## [m6-tui-timeline] — 2026-05-06
…

## [m5-sse-upgrade] — 2026-05-05
…

## [m4-run-polling] — 2026-05-05
…

## [m3-project-state] — 2026-05-05
…

## [m2-provisioning] — 2026-05-05
…

## [m1-foundation] — 2026-05-05
…
```

Each milestone gets `### Added`, `### Changed`, `### Fixed`, `### Internal` (omit empty sections). Keep entries terse — one line each, link to commits/PRs only when the change is non-obvious. Use the engram session summaries from `mem_search` as a starting point if the commit messages are sparse.

- [ ] **Step 3: Verify**

```bash
# All listed tags exist:
git tag -l m1-foundation m2-provisioning m3-project-state m4-run-polling \
              m5-sse-upgrade m6-tui-timeline m7-applyboard-approval \
              m8-attach-changes-status

# CHANGELOG references no tag that doesn't exist:
rg '^## \[m\d' CHANGELOG.md | sed 's/.*\[//;s/\].*//' | while read tag; do
    git rev-parse "$tag" > /dev/null || echo "MISSING TAG: $tag"
done
```

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add CHANGELOG covering M1-M8 (Keep a Changelog format)"
```

---

### Task 4: `docs/adr/0001-hexagonal-architecture.md`

**Files:**
- Create: `docs/adr/0001-hexagonal-architecture.md`

ADR (Architecture Decision Record) format per [Michael Nygard's template](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions). Sections:

1. **Title**: `0001 — Hexagonal architecture (Ports & Adapters)`
2. **Status**: `Accepted`
3. **Date**: `2026-05-05` (M1 inception)
4. **Context**: Why we chose hexagonal over a layered MVC or flat package structure. Mention the M5/M6/M7/M8 evolution as evidence the model held up: SSE swapped polling without touching domain; TUI added without touching application; `Runner.Observe` extracted in M8 reused by `Attach` because the port boundary was already clean.
5. **Decision**: The four-tier rule — `domain` → `ports/{inbound,outbound}` → `application` → `adapters/{inbound,outbound}` — plus the "no domain imports adapters; no application imports adapters; no adapter imports another adapter" lattice.
6. **Consequences**:
   - Pros: testability via `test/fakes`, clean DI in `bootstrap/wire.go`, swappable SSE/HTTP clients, no leakage from `bubbletea` into application logic.
   - Cons: more boilerplate (every outbound call has port + adapter + fake); newcomers need orientation; small tasks split across 3+ files.
7. **References**: link to `internal/ports/outbound/orchestrator.go` and `internal/application/runner.go` as canonical examples.

- [ ] **Step 1: Draft the ADR**

Aim for ~80–150 lines. Include a small ASCII diagram showing the four tiers and import direction.

- [ ] **Step 2: Cross-check the import lattice claim**

```bash
# domain imports nothing project-internal except other domain
rg '^import' internal/domain/ -A 20 | rg 'sophia-cli/internal/(application|adapters|ports)' && echo "VIOLATION"

# application imports ports + domain only (NOT adapters)
rg '^\s*"github\.com/RVRTelecomunicaciones/sophia-cli/internal/adapters' internal/application/ && echo "VIOLATION"
```

If either grep prints `VIOLATION`, the ADR is lying. Either fix the offending import (preferred) or update the ADR's "Consequences" section to acknowledge the exception.

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0001-hexagonal-architecture.md
git commit -m "docs(adr): record hexagonal architecture decision (ADR-0001)"
```

---

### Task 5: `docs/adr/0002-bubbletea-v2-charm-land.md`

**Files:**
- Create: `docs/adr/0002-bubbletea-v2-charm-land.md`

Documents the M6 decision to use `charm.land/bubbletea/v2` (and `lipgloss/v2`) instead of the v1 `github.com/charmbracelet/bubbletea`. Sections:

1. **Title**: `0002 — Bubble Tea v2 on charm.land`
2. **Status**: `Accepted`
3. **Date**: `2026-05-06` (M6 commit)
4. **Context**: M6 needed a TUI framework. Bubble Tea v1 was the obvious incumbent; v2 was on the cusp of release. We picked v2 to avoid a forced migration mid-product.
5. **Decision**: Use `charm.land/bubbletea/v2 v2.0.6`, `charm.land/bubbles/v2 v2.1.0`, `charm.land/lipgloss/v2 v2.0.3`. Pin to those minor versions until a v3 forces a migration.
6. **Consequences**:
   - Pros: `tea.View` model (replaces `string`-returning `View()`), proper window-size as `tea.WindowSizeMsg`, `lipgloss.Color()` as a function (composable), teatest/v2 for golden-file integration. Aligned with charm's long-term release line.
   - Cons: smaller community than v1 at the time of adoption; minor API changes still landing; `charm.land` is a redirect to `github.com/charmbracelet` packages but Go module resolution caches that as a different module path — care needed when reading other people's snippets that use the v1 import path.
7. **References**: `internal/adapters/inbound/tui/program.go` for `tea.NewProgram` usage; `test/tui/timeline_test.go` for teatest/v2 patterns.

- [ ] **Step 1: Draft the ADR**

Same format as ADR-0001. ~80–120 lines.

- [ ] **Step 2: Verify pinned versions match `go.mod`**

```bash
rg "charm.land/(bubbletea|bubbles|lipgloss)" go.mod
```

If the versions in the ADR drift from `go.mod`, fix the ADR (or commit a `go mod tidy` if `go.mod` drifted unintentionally).

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0002-bubbletea-v2-charm-land.md
git commit -m "docs(adr): record Bubble Tea v2 on charm.land decision (ADR-0002)"
```

---

## Phase 2 — Release infrastructure

### Task 6: `.goreleaser.yaml` + `Makefile` release targets

**Files:**
- Create: `.goreleaser.yaml`
- Modify: `Makefile` (add `release-snapshot`, `release-check` targets)

`goreleaser` is the canonical Go release tool. Configure it for cross-platform builds (linux/darwin × amd64/arm64), SHA256 checksums, archive naming, and GitHub release upload. v1.x of goreleaser is feature-frozen but stable; v2.x is current. **Use v2.x.**

- [ ] **Step 1: Verify `goreleaser` is installable**

```bash
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser --version    # ≥ 2.0
```

If `go install` fails (private module, network, etc.), document the install path you actually used and adjust the CI workflow (Task 7) accordingly.

- [ ] **Step 2: Write `.goreleaser.yaml`**

Minimal config (no Docker, no Homebrew tap yet — those come in M10+):

```yaml
version: 2

project_name: sophia-cli

before:
  hooks:
    - go mod tidy
    - go vet ./...

builds:
  - id: sophia
    main: ./cmd/sophia
    binary: sophia
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap.Version={{.Version}}
      - -X github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap.Commit={{.Commit}}
      - -X github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap.BuildDate={{.Date}}

archives:
  - id: default
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - README.md
      - CHANGELOG.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot-{{ .ShortCommit }}"

release:
  github:
    owner: RVRTelecomunicaciones
    name: sophia-cli
  draft: false
  prerelease: auto
  name_template: "{{.ProjectName}} v{{.Version}}"

changelog:
  use: github
  sort: asc
  groups:
    - title: Features
      regexp: "^.*feat[(\\w)]*:+.*$"
      order: 0
    - title: Bug fixes
      regexp: "^.*fix[(\\w)]*:+.*$"
      order: 1
    - title: Other
      order: 999
```

> **Verification gate:** confirm the GitHub owner + repo name. If the repo lives under a different org, update `release.github.owner` and `release.github.name`. The publish step will fail loudly if the names are wrong.

- [ ] **Step 3: Add Makefile targets**

```makefile
release-snapshot: ## Build a local snapshot release with goreleaser (no publish)
	goreleaser release --snapshot --clean --skip=publish,sign

release-check: ## Validate .goreleaser.yaml without building anything
	goreleaser check
```

- [ ] **Step 4: Run a snapshot release locally**

```bash
make release-check
make release-snapshot
ls dist/
```

Expected: `dist/` contains `sophia-cli_*-snapshot-*_linux_amd64.tar.gz`, `..._linux_arm64.tar.gz`, `..._darwin_amd64.tar.gz`, `..._darwin_arm64.tar.gz`, plus `checksums.txt`. Each archive contains `sophia`, `LICENSE`, `README.md`, `CHANGELOG.md`.

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml Makefile
git commit -m "build(release): add goreleaser config + snapshot/check Makefile targets"
```

---

### Task 7: GitHub Actions release workflow + CI hardening

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml` (bump Go to 1.26.x; add govulncheck + gosec; bump golangci-lint to v2.x).

Two changes:

**Release workflow** (`release.yml`): triggered by tag push matching `v*.*.*`. Runs `goreleaser release` with the GitHub token, publishes a Release with archives + checksums + auto-generated changelog from commit history.

**CI hardening** (`ci.yml`): bump Go to `1.26.x` (the actual toolchain in `go.mod`), bump golangci-lint to `v2.x` (current major), add `govulncheck` and `gosec` steps (Task 8 introduces them as Makefile targets — CI just calls them).

> **Verification gate:** confirm what Go version `go.mod`'s `toolchain` directive declares. The CI matrix MUST match. If `go.mod` says `toolchain go1.26.2`, the CI's `setup-go` should use `1.26.x`.

- [ ] **Step 1: Write `release.yml`**

```yaml
name: release

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26.x'
          cache: true

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Update `ci.yml`**

Diff vs current:

- `go-version: '1.24.x'` → `go-version: '1.26.x'` (matches `go.mod` toolchain).
- `golangci/golangci-lint-action@v6` with `version: v1.64` → keep the action at `v6`, bump `version: v2.0` (or whatever major v2 ships current at runtime — pin to a specific minor to avoid surprise).
- ADD a `govulncheck` step:
  ```yaml
  - name: govulncheck
    run: |
      go install golang.org/x/vuln/cmd/govulncheck@latest
      govulncheck ./...
  ```
- ADD a `gosec` step:
  ```yaml
  - name: gosec
    uses: securego/gosec@master
    with:
      args: '-quiet ./...'
  ```

> **Note on `golangci-lint v2.x`:** the v2 line uses a different config syntax (`linters:` is no longer a top-level key but lives under `version: 2`). If `.golangci.yml` exists in the repo and it's v1-shaped, Task 7 also adopts the v2 config — DO NOT silently downgrade the lint version just to keep an old config. Run `golangci-lint migrate` once locally and commit the new config alongside the workflow change.

- [ ] **Step 3: Local dry-run**

```bash
# Validate the release workflow syntax (uses act if installed, otherwise eyeball)
act --container-architecture linux/amd64 release --secret GITHUB_TOKEN=fake -n

# Validate CI YAML
yq eval '.jobs.build-test-lint.steps' .github/workflows/ci.yml | head -30
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml .github/workflows/ci.yml .golangci.yml
git commit -m "ci: add release workflow + bump Go to 1.26.x + add gosec/govulncheck"
```

---

## Phase 3 — Quality gates

### Task 8: Security suite — `gosec`, `govulncheck`, license check

**Files:**
- Modify: `Makefile` (add `security`, `vuln`, `licenses` targets)
- Create: `docs/release/security-notes.md` (documents accepted MEDIUM/LOW findings + license fallbacks)

Three local Make targets the developer can run before pushing, mirroring the CI checks. **Severity thresholds (D-M9-11):**

- `make vuln` (govulncheck): blocks on **reachable HIGH/CRITICAL** vulnerabilities in code paths the binary actually calls. govulncheck's reachability analysis is what makes it suitable as a gate — non-reachable advisories surface as warnings only.
- `make security` (gosec): blocks on **HIGH severity** findings. The `-severity high` flag enforces this.
- MEDIUM and LOW findings are accepted ONLY with a per-finding justification recorded in `docs/release/security-notes.md`. No blanket waivers. Each entry: rule ID, file:line, why it's a false positive or accepted risk, reviewer name + date.
- `make licenses`: emits `THIRD_PARTY_LICENSES.md`. Catches GPL/AGPL contaminations early.

> **Verification gate:** before adding `make licenses`, scan current dependencies with `go-licenses csv ./...` and confirm no copyleft licenses are present. If any are, escalate to the user — that's a feature decision, not an M9 task.

- [ ] **Step 1: Install the tools locally**

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/google/go-licenses@latest
```

- [ ] **Step 2: Add Makefile targets**

```makefile
.PHONY: vuln security licenses

vuln: ## Run govulncheck — blocks on reachable HIGH/CRITICAL CVEs
	govulncheck ./...

security: ## Run gosec — blocks on HIGH severity findings
	gosec -severity high -quiet ./...

licenses: ## Generate THIRD_PARTY_LICENSES.md (best-effort if go-licenses fails)
	@if go-licenses report ./... --template scripts/licenses.tmpl > THIRD_PARTY_LICENSES.md 2>/dev/null; then \
	    echo "go-licenses report generated"; \
	else \
	    echo "go-licenses failed (likely Go 1.26 incompatibility); falling back to go list inventory"; \
	    scripts/licenses-fallback.sh > THIRD_PARTY_LICENSES.md; \
	fi
```

If `scripts/licenses.tmpl` doesn't exist, create a minimal one:

```
# Third-Party Licenses

The sophia-cli binary statically links the following Go modules:

{{range .}}
## {{.Name}} ({{.LicenseName}})
- Module: `{{.Name}}`
- Version: `{{.Version}}`
- License: [{{.LicenseName}}]({{.LicenseURL}})
{{end}}
```

**Fallback script** `scripts/licenses-fallback.sh` (used when `go-licenses` is incompatible with the toolchain):

```bash
#!/usr/bin/env bash
set -euo pipefail
cat <<'HEADER'
# Third-Party Licenses (best-effort inventory)

> Generated via `go list -m -json all` because `go-licenses` did not run on
> this toolchain. License field is the module's published license metadata
> if available; absence does NOT mean the module is unlicensed — verify
> manually before redistribution.

| Module | Version | License (if known) |
|--------|---------|--------------------|
HEADER
go list -m -json all | jq -r 'select(.Main != true) | "| \(.Path) | \(.Version) | (verify manually) |"'
```

Make it executable: `chmod +x scripts/licenses-fallback.sh`. The fallback marks the inventory as `(verify manually)` because `go list` doesn't expose a license field; this is acceptable for v0.1.0 with the gap documented in `docs/release/security-notes.md`.

- [ ] **Step 3: Run the suite locally and capture findings**

```bash
make vuln       # → 0 reachable HIGH/CRITICAL (warnings on non-reachable are OK)
make security   # → 0 HIGH-severity findings
make licenses
head -30 THIRD_PARTY_LICENSES.md
```

**Triage policy (D-M9-11):**

- **gosec HIGH** → fix in-place. Hardcoded creds, weak crypto, command injection: real bugs, fix them.
- **gosec MEDIUM/LOW** → either fix OR add to `docs/release/security-notes.md` with rule ID, file:line, justification, reviewer name + date. NO blanket exclusions.
- **gosec false positive** → annotate at the call site with `// #nosec G123 - <one-line reason>`. The annotation IS the justification.
- **govulncheck reachable HIGH/CRITICAL** → bump the dep (`go get -u <module>`), test, commit. If no fix exists, escalate.
- **govulncheck non-reachable** → log to `docs/release/security-notes.md`. govulncheck distinguishes these automatically; the warning vs error in the output IS the source of truth.

- [ ] **Step 4: Write `docs/release/security-notes.md`**

Initial template (populate with whatever findings the local run produced):

```markdown
# Security Notes

This document tracks accepted MEDIUM/LOW gosec findings and non-reachable
govulncheck advisories for the sophia-cli release line. Every entry is
human-reviewed; renew the review on every minor release.

## gosec accepted findings (MEDIUM / LOW)

(none for v0.1.0 — leave empty if `make security` produced no medium/low output)

## govulncheck non-reachable advisories

(none for v0.1.0 — leave empty if every advisory was either reachable+fixed or absent)

## License inventory caveats

- THIRD_PARTY_LICENSES.md is generated by `go-licenses report` when the toolchain
  supports it. On Go 1.26.x, the fallback at `scripts/licenses-fallback.sh` produces
  a best-effort inventory from `go list -m -json all` without license-field resolution.
  This is acknowledged for v0.1.0; a follow-up will reconcile against the upstream
  `go-licenses` releases once 1.26.x is supported.

## Sign-off

- Reviewer: __________
- Date: ____-__-__
- Tag at review: ____________
```

- [ ] **Step 5: Commit**

```bash
git add Makefile scripts/licenses.tmpl scripts/licenses-fallback.sh \
        THIRD_PARTY_LICENSES.md docs/release/security-notes.md
git commit -m "build: add security suite (gosec HIGH gate, govulncheck reachable gate, license inventory)"
```

---

### Task 9: E2E suite final pass + manual smoke checklist

**Files:**
- Modify: `Makefile` (add `e2e` target if missing)
- Create: `docs/release/manual-smoke-checklist.md`

The e2e suite already exists (`test/e2e/run_polling_test.go` from M5, `attach_workflow_test.go` from M8, build-tag-gated by `e2e_smoke`). M9 ensures it runs cleanly under `make e2e`, documents the tag, and writes the manual smoke checklist that gates v0.1.0.

- [ ] **Step 1: Add `e2e` Makefile target**

```makefile
.PHONY: e2e

e2e: build ## Run the build-tag-gated e2e smoke tests against the freshly built binary
	go test -tags=e2e_smoke -timeout 60s ./test/e2e/...
```

`build` is already defined; depending on it ensures the binary exists at `./bin/sophia` before the e2e tests exec it.

- [ ] **Step 2: Run the suite locally**

```bash
make e2e
```

Expected: PASS for `TestSmokeRunAgainstStub` (M5) and `TestSmokeAttachWorkflow` (M8). If either flakes under `-race`, raise the timing constants — DO NOT silence `-race`.

- [ ] **Step 3: Write `docs/release/manual-smoke-checklist.md`**

This is the human-driven validation that Task 12 references before tagging `v0.1.0`. Sections:

1. **Pre-requisites**
   - A live orchestrator at `http://localhost:9080` (or `SOPHIA_ORCHESTRATOR_URL`).
   - A repo with `.sophia.yaml` configured.
   - At least one in-progress and one completed Change available.

2. **Smoke matrix** (one bullet per scenario; the human ticks each):
   - [ ] `sophia doctor` → green across every check.
   - [ ] `sophia run "smoke v0.1.0"` → TUI renders, terminal status reached.
   - [ ] `sophia attach <id>` (running Change) → reattaches, SSE resumes.
   - [ ] `sophia attach <id>` (terminal Change) → opens, immediately closes.
   - [ ] `sophia attach <id> --no-tui --json --approval-timeout 60s` against a Change pending approval → eager-arm fires, exit 5 if not approved in time.
   - [ ] `sophia changes` → table aligns.
   - [ ] `sophia changes --json | python3 -m json.tool` → valid JSON array.
   - [ ] `sophia changes --status running` → filter passes.
   - [ ] `sophia changes --project ""` → all projects.
   - [ ] `sophia status` → resolution per spec §2.5.
   - [ ] `sophia status <id>` → flag wins.
   - [ ] `sophia status --json` → valid JSON object or `null`.
   - [ ] Stale `last_change_id` → `sophia status` exit 3 with "change not found".
   - [ ] Outside repo → `sophia status` falls back to global.
   - [ ] No global state → `sophia status` exit 0 with "No local change found".

3. **Sign-off**: reviewer name + date at the bottom. Commit the file with the sign-off when done.

- [ ] **Step 4: Commit**

```bash
git add Makefile docs/release/manual-smoke-checklist.md
git commit -m "test(e2e): add make e2e target + manual smoke checklist for v0.1.0"
```

---

## Phase 4 — Demo + optional coverage

### Task 10: Asciinema demo

**Files:**
- Create: `assets/demo/sophia-quickstart.cast`
- Modify: `README.md` (link the cast)

A 60–90 second asciinema recording showing the quickstart: `sophia doctor` → `sophia start` → `sophia init` → `sophia run "demo"` → terminal status. The cast file lives in the repo (~50–100KB); `asciinema.org` upload is optional.

> **Pre-req:** `asciinema` CLI installed (`brew install asciinema` or `pipx install asciinema`).

- [ ] **Step 1: Record**

```bash
mkdir -p assets/demo
asciinema rec assets/demo/sophia-quickstart.cast \
    --idle-time-limit 1.5 \
    --title "sophia-cli quickstart"
# … run the demo …
# Ctrl-D or `exit` when done.
```

Walk-through script (under 90 seconds):

1. `clear`
2. `sophia version` (shows the v0.1.0-rc tag the demo is recorded against; no real v0.1.0 yet — that's Task 12)
3. `sophia doctor` (shows green checks against a local orchestrator)
4. `cat .sophia.yaml` (shows the project config)
5. `sophia run "implement /healthz endpoint" --no-tui --json | head -20`
6. `sophia changes --limit 3`
7. `sophia status`

If a step takes > 5 seconds of dead time (orchestrator latency), pause the recording (`asciinema rec --pause` workflow) or trim the cast post-hoc with `asciinema-trim` or by hand-editing the JSON.

- [ ] **Step 2: Validate the cast plays back**

```bash
asciinema play assets/demo/sophia-quickstart.cast --speed 2
```

Look for: legible text, no overflow, no error noise, total duration ≤ 90s @ 1x.

- [ ] **Step 2.A: Anti-secret scrub (mandatory before any commit)**

The `.cast` file is plain JSON-Lines containing every character that hit the terminal — including any token, password, or API key the demo command line or output happened to expose. Run this grep:

```bash
rg -i -e 'token' -e 'secret' -e 'bearer' -e 'ghp_' -e 'AKIA' -e 'password' \
       -e 'api[_-]?key' -e 'authorization' -e 'sk_live' -e 'sk_test' \
       assets/demo/sophia-quickstart.cast && {
    echo "SECRET DETECTED — DO NOT COMMIT. Delete the cast and re-record."
    rm assets/demo/sophia-quickstart.cast
    exit 1
}
echo "scrub clean"
```

If the grep matches anything, treat the cast as compromised: delete it, fix the demo environment (rotate the leaked credential, scrub the shell history, set `unset HISTFILE` for the recording session), and re-record from scratch. Do NOT try to manually edit the JSON to hide a secret — keystroke timing leaks it back.

- [ ] **Step 3: Link from README**

Add to the README's "Demo" section:

```markdown
## Demo

[![asciicast](https://asciinema.org/a/PLACEHOLDER.svg)](https://asciinema.org/a/PLACEHOLDER)

Local playback:

\`\`\`bash
asciinema play assets/demo/sophia-quickstart.cast
\`\`\`
```

If you uploaded to asciinema.org, replace `PLACEHOLDER` with the cast ID. If not, drop the badge and keep just the local-playback block.

- [ ] **Step 4: Commit**

```bash
git add assets/demo/sophia-quickstart.cast README.md
git commit -m "docs: add asciinema quickstart demo"
```

---

### Task 11 (Optional): `attachTUI` teatest — recover the 1% coverage gap

**Files:**
- Create: `test/tui/attach_test.go`

M8 closed at 79.0% total coverage with `attachTUI` at 0% (no pseudo-tty test). This task recovers the 1% gap if and only if the implementation fits in ~150 lines and one teatest case. If it grows, drop it — the M8 sign-off explicitly accepted the gap.

> **Bail-out criteria:** if writing the test requires plumbing a new field into `cli.Deps` (e.g., a TUI program override hook), STOP. That is feature creep; the gap stays.

- [ ] **Step 1: Sketch the test**

Use the existing `test/tui/timeline_test.go` and `test/tui/applyboard_banner_test.go` as templates. Driver: teatest with `WithInput(nil)` + `WithOutput(buf)`. Drive a fake `Attacher` (or a fake `RunResult`) via the bridge.

```go
//go:build !windows

package tui_integration_test

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/RVRTelecomunicaciones/sophia-cli/internal/adapters/inbound/tui"
)

func TestAttachTUIShowsSnapshotAndQuitsOnQ(t *testing.T) {
	// Setup: build a tui.Program manually (the same way attachTUI does), feed it
	// a snapshot via Bridge().OnSnapshot, then send 'q' and assert hint output.
	// The point is exercising the program lifecycle in attachTUI's path —
	// snapshot rendering + Q-detach hint + Close().
	//
	// Skip if this requires changes to cli/attach.go beyond exporting a hook —
	// see bail-out criteria.
}
```

- [ ] **Step 2: Run the test under -race + coverage**

```bash
go test -race -tags=tui_e2e ./test/tui/...
go test -coverprofile=cover.cli.out ./internal/adapters/inbound/cli/...
go tool cover -func=cover.cli.out | rg attachTUI
```

Expected: `attachTUI` rises from 0% to ≥ 60%. Total cli coverage rises by ~1%.

- [ ] **Step 3: Commit (only if step 2 passes the bail-out check)**

```bash
git add test/tui/attach_test.go
git commit -m "test(tui): add teatest for attachTUI to close the 1% coverage gap"
```

If you bailed out (step 1 required cli.Deps changes), DO NOT commit a half-test. Just record the bail-out in the M9 deviation notes (Task 12 step 5) so the gap is acknowledged in the v0.1.0 release notes.

---

## Phase 5 — Release

### Task 12: `v0.1.0` final validation + tag + GitHub release

**Files:** none (verification only).

The final gate. Runs every checked-in quality bar, performs the manual smoke, then tags `v0.1.0`. Pushing the tag triggers the GitHub Actions release workflow (Task 7) which builds + uploads artifacts.

- [ ] **Step 1: Tree hygiene**

```bash
git status               # → clean
git log --oneline -1     # → the last M9 commit (Task 10 or Task 11)
```

- [ ] **Step 1.A: Repo owner/name verification (D-M9-12)**

`.goreleaser.yaml` and `release.yml` both hard-code `RVRTelecomunicaciones/sophia-cli`. Confirm against the actual git remote AND the GitHub repo metadata. If either disagrees, ABORT and update `.goreleaser.yaml` (`release.github.owner` + `release.github.name`) before proceeding.

```bash
# Source of truth #1: git remote
git remote -v | rg -m1 '^origin' | rg -o 'github.com[:/]([^/]+)/([^.]+)' -r '$1/$2'

# Source of truth #2: gh CLI (requires `gh auth login`)
gh repo view --json nameWithOwner -q .nameWithOwner

# Both MUST equal the values in .goreleaser.yaml. If not, abort, fix, recommit.
rg -e '^\s*owner:\s*' -e '^\s*name:\s*' .goreleaser.yaml
```

If `gh` isn't installed or you're offline, accept the `git remote` output as the only source — but document the gap in the M9 deviation notes.

- [ ] **Step 1.B: Tag idempotence (D-M9-13)**

`v0.1.0` MUST be a fresh tag. If it already exists locally or remotely, ABORT — do NOT force-tag, do NOT delete and recreate. Force-tags rewrite history that consumers may have already pulled; recreated releases break checksum trust.

```bash
# Local check:
git rev-parse --verify v0.1.0 2>/dev/null && {
    echo "ABORT: v0.1.0 already exists locally"
    exit 1
}

# Remote check:
git ls-remote --tags origin 'refs/tags/v0.1.0' | grep -q v0.1.0 && {
    echo "ABORT: v0.1.0 already exists on origin"
    exit 1
}
```

If either check fires, escalate to the user. Re-tagging `v0.1.0` requires explicit human approval AND a new patch (`v0.1.1`) is the safer path.

- [ ] **Step 2: Full validation pass**

```bash
make vet
make test                # → all green
make lint                # → clean
make vuln                # → 0 unfixed CVEs
make security            # → 0 high-severity findings
make e2e                 # → PASS
make release-check       # → goreleaser config valid
make release-snapshot    # → dist/ populates with all 4 archives + checksums
```

If any of these fails, FIX THE FAILURE — do NOT skip the gate. M9's whole point is shipping a release that passes every guardrail.

- [ ] **Step 3: Manual smoke (per `docs/release/manual-smoke-checklist.md`)**

**Hard gate (D-M9-14):** the `v0.1.0` tag MUST NOT be pushed without a signed manual smoke checklist. Tasks 1–11 can land freely without this; Task 12 step 5 (tag push) requires it. If you don't have:
- a real terminal (TTY-attached for the TUI flows),
- a running orchestrator at `SOPHIA_ORCHESTRATOR_URL`,
- at least one in-progress + one terminal Change available,

then STOP at the end of step 4 (CHANGELOG promotion). Park the release on a `release/v0.1.0` branch, document what's blocked, and resume when the human reviewer with the live setup can sign off. Do NOT push the tag.

Execute every bullet in the checklist against the live orchestrator. Tick each. Add the reviewer's name + date at the bottom. Commit the signed checklist:

```bash
git add docs/release/manual-smoke-checklist.md
git commit -m "release: sign off manual smoke for v0.1.0"
```

- [ ] **Step 4: Update `CHANGELOG.md`**

Promote `[Unreleased]` to `[v0.1.0] — 2026-MM-DD`. Add a fresh empty `[Unreleased]` at the top. Commit:

```bash
git add CHANGELOG.md
git commit -m "release: prepare CHANGELOG for v0.1.0"
```

- [ ] **Step 5: Tag and push**

```bash
git tag -a v0.1.0 -m "v0.1.0 — first public release"
git push origin main
git push origin v0.1.0
```

The tag push triggers `release.yml`. Watch the Actions tab; goreleaser builds + uploads + creates the GitHub Release. Verify:

- The Release page shows 4 archives (linux amd64/arm64, darwin amd64/arm64) + `checksums.txt`.
- The Release notes are auto-generated from commit history.
- `https://github.com/RVRTelecomunicaciones/sophia-cli/releases/latest` redirects to `v0.1.0`.

- [ ] **Step 6: Post-release smoke**

Download the release artifact for your platform, verify the checksum, run the binary:

```bash
curl -sL https://github.com/RVRTelecomunicaciones/sophia-cli/releases/download/v0.1.0/checksums.txt | sha256sum --check --ignore-missing
tar xzf sophia-cli_0.1.0_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/').tar.gz
./sophia version
./sophia --help
```

`sophia version` MUST report `v0.1.0` (not `0.1.0-dev`). If it reports anything else, the ldflags injection failed — diagnose before announcing the release.

---

## Decision Register (D-M9-NN)

| ID | Question | Decision |
|----|----------|----------|
| D-M9-01 | License? | Apache-2.0 — enterprise Go default, patent grant, attribution clause. |
| D-M9-02 | goreleaser version? | v2.x. v1 is feature-frozen; v2 is current and supports the `version: 2` config syntax. |
| D-M9-03 | Cross-platform matrix? | linux + darwin × amd64 + arm64. Windows deferred to M10+ unless a user files a request. |
| D-M9-04 | Archive format? | `tar.gz` for linux/darwin; `zip` reserved for windows (when added). |
| D-M9-05 | Should v0.1.0 be a draft or final release? | Final (`prerelease: auto` lets goreleaser flag pre-1.0 versions automatically; v0.1.0 is intentionally pre-stable but not `-rc`). |
| D-M9-06 | Should `v0.1.0` be signed? | NO — sigstore/cosign is M10+. Plain SHA256 in `checksums.txt` is enough for the first release. |
| D-M9-07 | gosec false positives policy? | Annotate at the call site with `// #nosec G<rule> - <reason>`; do NOT add blanket exclusions in `.gosec.yml`. |
| D-M9-08 | govulncheck unfixed CVE in untouched code path? | Document in `THIRD_PARTY_LICENSES.md` (section "Known unaddressed advisories") with the `vulncheck` output verbatim. |
| D-M9-09 | Asciinema upload to asciinema.org? | Optional. The `.cast` file lives in-repo regardless. If uploaded, replace the README badge placeholder with the real cast ID. |
| D-M9-10 | Optional Task 11 bail-out criteria? | Bail if the test needs ANY change to `cli/attach.go` beyond exporting a single hook (e.g., a `tui.Program` override field). The 1% gap is accepted at M8; M9 only takes a stab if the cost is trivial. |
| D-M9-11 | Security severity gates? | govulncheck blocks on **reachable HIGH/CRITICAL** (govulncheck's reachability classification IS the source of truth — non-reachable is a warning). gosec blocks on **HIGH severity** (`-severity high`). MEDIUM/LOW from either tool require per-finding justification in `docs/release/security-notes.md` (rule ID + file:line + reason + reviewer + date). NO blanket exclusions. Annotate gosec false positives at the call site with `// #nosec G123 - reason`. |
| D-M9-12 | Repo owner/name source of truth? | Two-source verification at Task 12 step 1.A: `git remote -v` AND `gh repo view --json nameWithOwner`. BOTH must agree with `.goreleaser.yaml`'s `release.github.owner`/`release.github.name`. If `gh` is unavailable, `git remote` is sole source — but the gap is documented. If either source disagrees with the YAML, abort and fix BEFORE Task 12 step 5 (tag push). |
| D-M9-13 | Tag idempotence? | `v0.1.0` MUST be a fresh tag. Task 12 step 1.B greps `git rev-parse --verify v0.1.0` AND `git ls-remote --tags origin`. If either reports the tag exists, ABORT. NO force-tag. NO delete-and-recreate of releases. Re-tagging requires explicit human approval and a new patch (`v0.1.1`) is the safer path. |
| D-M9-14 | Manual smoke as a release hard gate? | YES. Tasks 1–11 land freely without manual smoke. Task 12 step 5 (tag push) requires a signed `docs/release/manual-smoke-checklist.md`. If the human reviewer doesn't have a real terminal + live orchestrator + Changes in distinct states, the milestone STOPS at step 4 (CHANGELOG promotion). The release branch parks on `release/v0.1.0` until smoke is signed. Tag NEVER pushes without smoke. |
| D-M9-15 | go-licenses on Go 1.26.x? | Use `go-licenses report` if it works; fall back to `scripts/licenses-fallback.sh` (which uses `go list -m -json all`) if it doesn't. The fallback marks the inventory as best-effort and the gap is recorded in `docs/release/security-notes.md`. v0.1.0 ships with the best-effort inventory if go-licenses is incompatible. |
| D-M9-16 | Asciinema secret scrubbing? | Mandatory grep before commit. Pattern set: `token`, `secret`, `bearer`, `ghp_`, `AKIA`, `password`, `api[_-]?key`, `authorization`, `sk_live`, `sk_test`. Any match → delete cast, fix env (rotate creds, `unset HISTFILE`), re-record. NO manual JSON editing — keystroke timing leaks the secret back. |

---

## Risk Register

| ID | Risk | Mitigation |
|----|------|------------|
| RM9-01 | goreleaser breaks on the cross-compile (e.g., a CGO dep sneaks in) | `CGO_ENABLED=0` enforced in `.goreleaser.yaml`. `make release-snapshot` runs locally as a canary in Task 6. |
| RM9-02 | The release workflow needs `GITHUB_TOKEN` permissions broader than `contents: write` | Verified: goreleaser only needs `contents: write` for releases. If goreleaser ever asks for more, document it in the workflow. |
| RM9-03 | The asciinema cast contains a leaked secret | Reviewer manually scrubs the `.cast` file before commit. The cast is plain text; `rg -i 'token\|password\|secret' assets/demo/*.cast` runs before commit. |
| RM9-04 | `gosec` finds genuine issues in M5/M6/M7/M8 code | Fix them in M9 — that IS in scope (security hardening). Do NOT carry findings into v0.1.0. |
| RM9-05 | Manual smoke uncovers a regression that requires code changes | STOP, re-tag the milestone (`m8-fix-attach-something`, etc.), update CHANGELOG with a `### Fixed` entry, re-run the entire validation pipeline. v0.1.0 ships clean or it doesn't ship. |
| RM9-06 | Go 1.26.x bump in CI breaks an indirect dep | Tested locally in Task 7 step 3 before pushing. If broken, pin the offending dep with `go get <module>@<version>` and document. |
| RM9-07 | golangci-lint v1→v2 config migration produces unexpected lint failures | The migration is mechanical (`golangci-lint migrate`). New v2 lints that fire are real findings — fix them in-place. Don't downgrade to v1 to silence them. |
| RM9-08 | Repo doesn't have a `CONTRIBUTING.md`; the README links to one | Either create a 30-line `CONTRIBUTING.md` (issue triage + PR style) OR drop the link from the README. M9 picks the lighter path: drop the link if M10 hasn't surfaced an actual contributor flow yet. |
| RM9-09 | `v0.1.0` accidentally re-tagged | D-M9-13 + Task 12 step 1.B abort guards. Force-tag and tag deletion are forbidden in M9 — no exceptions, full stop. |
| RM9-10 | `go-licenses` regression on Go 1.26.x toolchain | D-M9-15 fallback path. Best-effort inventory is acceptable for v0.1.0 with the gap documented; M10 reconciles when upstream catches up. |
| RM9-11 | Asciinema cast leaks a secret post-commit | D-M9-16 grep gate before commit. Once leaked to a public repo, treat as a compromised credential: rotate immediately, scrub via `git filter-repo` if the cast is the only file affected, force-push only after coordinating with every consumer. M9 does NOT script this remediation — the grep IS the prevention. |
| RM9-12 | Repo owner/name in `.goreleaser.yaml` doesn't match the actual GitHub repo | D-M9-12 two-source verification at Task 12. The release workflow fails noisily on a wrong owner — but the failure happens AFTER the tag is pushed, which is too late under D-M9-13. The pre-flight check in step 1.A is the only safe ordering. |
| RM9-13 | Manual smoke unavailable at Task 12 time (no terminal / no orchestrator / no live Changes) | D-M9-14 hard stop. Tag is NEVER pushed without smoke. The release parks on a `release/v0.1.0` branch and resumes when the reviewer has the live setup. This is acceptable; v0.1.0 doesn't have a hard ship date. |

---

## Verification Matrix

| Gate | Tool | Pass criteria | Where it runs |
|------|------|---------------|---------------|
| Compile | `go build ./...` | exit 0 | local + CI |
| Unit + race | `go test -race ./...` | exit 0 | local + CI |
| Vet | `go vet ./...` | exit 0 | local + CI |
| Lint | `golangci-lint run` | exit 0 | local + CI |
| Vuln | `govulncheck ./...` | 0 unfixed CVEs in called code paths | local + CI (Task 7/8) |
| Security | `gosec ./...` | 0 high-severity | local + CI (Task 7/8) |
| Coverage | `go test -coverprofile…` | total ≥ 80% (≥ 79% acceptable if Task 11 bailed out) | local + CI |
| E2E | `go test -tags=e2e_smoke ./test/e2e/...` | exit 0 | local (`make e2e`) |
| Release config | `goreleaser check` | exit 0 | local + CI |
| Release dry-run | `goreleaser release --snapshot --clean` | dist/ populates | local |
| Manual smoke | `docs/release/manual-smoke-checklist.md` | every box ticked + reviewer signature | manual (Task 12 step 3) |

---

## Implementation Notes — Deviations from Plan

<!-- Append observations during execution. Empty until Task 1 starts. -->
