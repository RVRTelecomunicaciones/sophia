# Sophia CLI

> Agentic change orchestrator companion — drives Sophia, observes Changes, gates approvals.

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/RVRTelecomunicaciones/sophia-cli.svg)](https://pkg.go.dev/github.com/RVRTelecomunicaciones/sophia-cli)
[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](https://go.dev/dl/)

`sophia` is the human entry point to the Sophia ecosystem. It creates and observes
Changes executed by `sophia-orchestrator`, streams events over SSE, and renders a
Bubble Tea TUI with a timeline view, an apply-board, and an approval banner. The
CLI itself does not coordinate phases, evaluate policy, or store canonical state —
the orchestrator owns that. `sophia` is the cockpit, not the engine.

---

## Status

This is the **v0.1.0** release line — the first publicly tagged version of the CLI.
Every command listed below is real and exercised by `-race` tests plus an end-to-end
suite gated by the `e2e_smoke` build tag. The wire shape (orchestrator JSON, SSE
events, exit codes) is locked at v0.1.0 per the [spec](./docs/) and will only change
under a major version bump.

---

## Install

### From a release archive

Download a tarball from the [latest release](https://github.com/RVRTelecomunicaciones/sophia-cli/releases/latest)
and extract it to a directory on your `PATH`:

```bash
# macOS arm64 example — adjust OS/ARCH for your platform
curl -sL -o sophia.tar.gz \
  https://github.com/RVRTelecomunicaciones/sophia-cli/releases/latest/download/sophia-cli_$(uname -s | tr A-Z a-z)_arm64.tar.gz
tar xzf sophia.tar.gz
sudo install -m 755 sophia /usr/local/bin/sophia
sophia version
```

Verify the SHA256 against `checksums.txt` from the same release page.

### From source

Requires Go 1.26+ (the `toolchain` directive in `go.mod` pins `go1.26.2`).

```bash
git clone https://github.com/RVRTelecomunicaciones/sophia-cli.git
cd sophia-cli
make build       # → ./bin/sophia
./bin/sophia version
```

`go install` works too once the module is published:

```bash
go install github.com/RVRTelecomunicaciones/sophia-cli/cmd/sophia@latest
```

---

## Quickstart

```bash
# 1. Verify your environment
sophia doctor

# 2. Bring up the local Sophia stack (Docker compose)
sophia start

# 3. In a project directory, initialize .sophia.yaml
cd ~/code/my-service
sophia init

# 4. Start a Change. The TUI opens; press Q to detach.
sophia run "implement /healthz endpoint"

# 5. Reattach later, list past Changes, query status
sophia attach <change-id>
sophia changes --limit 5
sophia status
```

For machine-readable output, every observation command supports `--no-tui --json`
(streams JSONL to stdout) or `--json` (one-shot JSON).

---

## Commands

| Command | What it does |
|---------|--------------|
| `sophia doctor` | Environment diagnostics — Docker, git, orchestrator reachability, SSE endpoint, XDG paths. |
| `sophia start` | Start the bundled Sophia stack via Docker compose. |
| `sophia stop` | Stop the Docker compose stack. |
| `sophia init` | Write `.sophia.yaml` at the resolved git repo root. |
| `sophia run "<message>"` | Create a Change and observe it through to a terminal status. |
| `sophia attach <change-id>` | Re-observe an existing Change (TUI by default; `--no-tui --json` for streams). |
| `sophia changes [flags]` | List recent Changes. Flags: `--limit`, `--status`, `--project`, `--json`. |
| `sophia status [<change-id>]` | Show a Change snapshot. Resolves arg → project-scoped `last_change_id` → global → empty. |
| `sophia version` | Print version, commit, build date. |

Run `sophia <command> --help` for full flag listings.

### Exit codes (spec §2.3)

| Code | Meaning |
|------|---------|
| 0 | Success (terminal status `done`, or empty `status`). |
| 1 | Change ended `blocked` or `failed`. |
| 3 | Config error, orchestrator unreachable, change not found, malformed `.sophia.yaml`. |
| 4 | Transient — context canceled or fetch timeout. |
| 5 | `--approval-timeout` exceeded in `--no-tui --json` mode. |

---

## Configuration

### `.sophia.yaml` (per-project)

Lives at the repo root. Created by `sophia init`.

```yaml
version: 1
project: my-service
base_ref: main
artifact_store: engram
```

### XDG paths

`sophia` follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

- State (`last_change_id`, project fingerprints): `$XDG_STATE_HOME/sophia/` (default `~/.local/state/sophia/`)
- Data (compose materialization): `$XDG_DATA_HOME/sophia/` (default `~/.local/share/sophia/`)
- User config (overrides for project defaults): `$XDG_CONFIG_HOME/sophia/config.yaml` (default `~/.config/sophia/config.yaml`)

### Environment variables

| Variable | Purpose |
|----------|---------|
| `SOPHIA_ORCHESTRATOR_URL` | Orchestrator base URL (default `http://localhost:9080`). |
| `SOPHIA_PROJECT` | Override `.sophia.yaml`'s `project` for this invocation. |
| `SOPHIA_BASE_REF` | Override `.sophia.yaml`'s `base_ref` for this invocation. |
| `XDG_STATE_HOME`, `XDG_DATA_HOME`, `XDG_CONFIG_HOME` | Standard XDG overrides; honored by every command. |

---

## Demo

An asciinema cast of the quickstart flow is planned for the v0.1.1 release —
it requires a live orchestrator + `asciinema` recorded against a real terminal,
neither of which were available at v0.1.0 cut. Track the asset in
[issue #1](https://github.com/RVRTelecomunicaciones/sophia-cli/issues/1) (to
be filed alongside v0.1.0) or generate one locally:

```bash
brew install asciinema   # or: pipx install asciinema
mkdir -p assets/demo
asciinema rec assets/demo/sophia-quickstart.cast --idle-time-limit 1.5
# … run: sophia doctor; sophia run "demo"; sophia changes; sophia status …
asciinema play assets/demo/sophia-quickstart.cast
```

Anti-secret scrub before sharing the cast (D-M9-16):

```bash
rg -i -e 'token' -e 'secret' -e 'bearer' -e 'ghp_' -e 'AKIA' \
       -e 'password' -e 'api[_-]?key' \
       assets/demo/*.cast && echo "SECRET — delete and re-record"
```

---

## Architecture

`sophia` is built on hexagonal architecture (Ports & Adapters):

```
domain  →  ports  →  application  →  adapters
                                     ↑
                              CLI / TUI / SSE / HTTP / git / fs
```

The application layer owns the use cases (`Runner`, `Attacher`, `Lister`,
`StatusReader`, `ConfigResolver`, etc.) and never imports adapters directly.
Adapters fulfill outbound ports (orchestrator HTTP, SSE stream, state store, git
inspector, project config store) and inbound ports (Cobra CLI, Bubble Tea TUI,
JSONL sink).

Two ADRs document the foundational decisions:

- [ADR-0001 — Hexagonal architecture](docs/adr/0001-hexagonal-architecture.md)
- [ADR-0002 — Bubble Tea v2 on charm.land](docs/adr/0002-bubbletea-v2-charm-land.md)

---

## Development

Common Make targets:

```bash
make build          # ./bin/sophia
make test           # go test ./...
make vet
make lint           # golangci-lint
make coverage       # domain + application coverage report
make e2e            # build-tag-gated end-to-end smoke tests (require ./bin/sophia)
make vuln           # govulncheck — blocks on reachable HIGH/CRITICAL CVEs
make security       # gosec — blocks on HIGH severity findings
make licenses       # regenerate THIRD_PARTY_LICENSES.md
make release-snapshot  # local goreleaser snapshot (no publish)
```

The full M1–M8 milestone history lives in [CHANGELOG.md](CHANGELOG.md). Each
milestone tag (`m1-foundation` … `m8-attach-changes-status`) is preserved on `main`.

---

## License

Sophia CLI is distributed under the [Apache License 2.0](LICENSE). See
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for the dependency inventory
and [docs/release/security-notes.md](docs/release/security-notes.md) for the
security review log.
