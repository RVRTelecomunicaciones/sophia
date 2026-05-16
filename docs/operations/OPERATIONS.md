# sophia CLI — Operations Guide

Target audience: developer installing the CLI locally to drive SDD cycles against a deployed sophia-orchestator.

---

## Install

### From source (recommended until v1.0)

```bash
go install github.com/RVRTelecomunicaciones/sophia/cmd/sophia@latest
```

Requires Go 1.25+ (`toolchain go1.26.3` is used in CI). The binary is named `sophia`.

### Local build

```bash
make build          # outputs ./bin/sophia
./bin/sophia version
```

### Release binaries (GoReleaser)

GoReleaser is configured (`.goreleaser.yaml`) and produces `tar.gz` archives for:

| OS      | Arch          |
|---------|---------------|
| Linux   | amd64, arm64  |
| Darwin  | amd64, arm64  |

Check the GitHub Releases page (`RVRTelecomunicaciones/sophia`) for published artifacts. Archives contain `LICENSE`, `README.md`, and `CHANGELOG.md`. Verify with `checksums.txt` (SHA-256).

---

## Configuration

Configuration is layered. Later layers win.

| Priority | Source                                            | Notes                                          |
|----------|---------------------------------------------------|------------------------------------------------|
| 1 (low)  | Built-in defaults                                 | `http://localhost:9080`, timeout 30s           |
| 2        | User config (`$XDG_CONFIG_HOME/sophia/config.yaml`) | Falls back to `~/.config/sophia/config.yaml`  |
| 3        | Project config (`.sophia.yaml` at repo root)      | Written by `sophia init`                       |
| 4        | Environment variables                             | `SOPHIA_ORCHESTRATOR_URL`, `SOPHIA_PROJECT`, `SOPHIA_BASE_REF` |
| 5 (high) | CLI flags                                         | `--api-key`, `--project`, `--base-ref`, etc.  |

### User config (`~/.config/sophia/config.yaml`)

```yaml
version: 1
orchestrator:
  url: https://sophia.example.com
  timeout_seconds: 60
```

Written with mode `0600`. The directory is created with `0700` if absent.

### Project config (`.sophia.yaml`)

```yaml
version: 1
project: my-service
base_ref: main
artifact_store: engram   # engram | openspec | hybrid | none
```

Created by `sophia init --project <slug>`. Committed to the repo.

### Auth

Remote orchestrators (non-loopback URL) require an API key. Loopback URLs (`localhost`, `127.x`, `::1`) allow anonymous access.

| Method          | Example                                            |
|-----------------|----------------------------------------------------|
| Environment var | `export SOPHIA_API_KEY=<key>`                     |
| Flag            | `sophia run "..." --api-key <key>`                |

The key is never logged. Only "set" / "missing" is surfaced in diagnostics.

---

## Common Workflows

### 1. Initialize a repo for the first time

```bash
sophia init --project my-service
```

Writes `.sophia.yaml` at the git repo root. Use `--artifact-store openspec` to write SDD artifacts as files instead of the default engram backend.

### 2. Start the local stack (optional — loopback only)

```bash
sophia start              # docker compose up; materializes compose.yaml if absent
sophia stop               # docker compose down
```

Only needed when running a local orchestrator. Remote targets skip this.

### 3. Create and observe a Change (TUI)

```bash
sophia run "implement user login with JWT"
```

Opens the interactive TUI. Use arrow keys to scroll the phase timeline. Press `[O]` to open an approval gate in the browser.

### 4. Create a Change without TUI (CI / scripting)

```bash
sophia run "implement user login with JWT" --no-tui --json
```

Streams JSONL events to stdout. Exit codes: `0` success, `3` general error, `4` context cancelled, `5` approval timeout.

```bash
# Override project and orchestrator URL inline:
sophia run "..." --no-tui --json \
  --project my-service \
  --approval-timeout 15m \
  SOPHIA_ORCHESTRATOR_URL=https://sophia.example.com sophia run ...
```

### 5. Attach to an in-flight Change

```bash
sophia attach <change-id>            # TUI
sophia attach <change-id> --no-tui --json   # JSONL stream
```

Resumes SSE from the last event (via `Last-Event-ID`). Safe to run after a disconnect.

### 6. Approve / reject a phase gate

```bash
sophia status                        # prints change-id and current phase-id
sophia approve <phase-id> -r "LGTM"
sophia reject  <phase-id> -r "needs more context"
```

`--approver` defaults to `$USER`. Idempotent: if the gate was already decided, exits `0` with an informational message.

### 7. Abort a Change

```bash
sophia abort <change-id> -r "pivoting direction"
```

Signals the orchestrator to terminate. Idempotent against already-terminal changes.

### 8. Diagnostics

```bash
sophia doctor           # table output: checks orchestrator reachability, git, docker
sophia doctor --json    # machine-readable JSON report
```

Fails (non-zero exit) when any check is `FAIL`-level. Use before filing a bug.

---

## Logs and Diagnostics

### Log output

The CLI writes structured logs (`log/slog`, JSONL format) to **stderr**. Default level is `INFO`.

There is no `--verbose` flag in the current codebase. To increase log verbosity, the log level must be changed at the bootstrap layer (development builds only). For production debugging, use `--json` with `sophia run` or `sophia attach` and capture the JSONL event stream:

```bash
sophia run "..." --no-tui --json 2>sophia-stderr.log | tee sophia-events.jsonl
```

### Tracing

Every CLI invocation mints a W3C `Traceparent` header (stable `trace_id` per run, rotating `span_id` per HTTP request). The orchestrator logs this trace context. To correlate a failing cycle:

1. Run with `--no-tui --json` and capture stderr.
2. Look for `"trace_id"` in the stderr JSONL.
3. Provide `trace_id` when filing a bug — the orchestrator can filter all spans for that invocation.

### State files

Local change state is stored under `$XDG_STATE_HOME/sophia/` (`~/.local/state/sophia/` by default). This is the source of truth for `sophia status` when the orchestrator is unreachable.

---

## Troubleshooting

### 1. `auth: remote orchestrator requires an API key`

**Symptom:** Any command that talks to the orchestrator exits with this message.
**Cause:** The orchestrator URL resolves to a non-loopback host and `SOPHIA_API_KEY` is unset (and `--api-key` was not passed).
**Fix:**
```bash
export SOPHIA_API_KEY=<your-key>
# or pass inline:
sophia run "..." --api-key <your-key>
```

### 2. `dial tcp: connection refused` / orchestrator unreachable

**Symptom:** `sophia doctor` shows a FAIL on the orchestrator check. `sophia run` exits immediately with a connection error.
**Cause:** `SOPHIA_ORCHESTRATOR_URL` points to an unreachable host, or the local stack is not running.
**Fix:**
```bash
sophia doctor                   # confirms which URL is being targeted
sophia start                    # starts local stack if targeting localhost:9080
# For remote: verify URL and network access
echo $SOPHIA_ORCHESTRATOR_URL
```

### 3. SSE stream disconnects mid-phase (heartbeat timeout)

**Symptom:** `sophia run` or `sophia attach` terminates early with a stream error during a long phase.
**Cause:** The SSE watchdog fires after 60 s of silence (no events, no heartbeats from the orchestrator). The reconnect backoff is 1 s → 2 s → 4 s → 8 s → 16 s → 30 s, capped at 5 retries per phase. If all retries are exhausted, the CLI exits.
**Fix:** Use `sophia attach <change-id>` to reconnect. The stream resumes via `Last-Event-ID`. The Change continues running on the orchestrator side regardless of CLI disconnects.

### 4. Exit code 5 — approval timeout

**Symptom:** `sophia run --no-tui --json` exits with code `5` and message `approval gate timed out`.
**Cause:** A phase raised an approval gate and no `approve` / `reject` was received within `--approval-timeout` (default 30 m).
**Fix:**
```bash
sophia status                        # find the phase-id
sophia approve <phase-id>
sophia attach <change-id> --no-tui --json   # re-attach to observe remaining phases
```
In CI, increase `--approval-timeout` or automate approval via `sophia approve`.

### 5. `.sophia.yaml` not found — `project not set`

**Symptom:** `sophia run` fails with `project not set (need .sophia.yaml or --project / SOPHIA_PROJECT)`.
**Cause:** The command was run outside a git repo, or `sophia init` was never called in this repo.
**Fix:**
```bash
sophia init --project <slug>    # creates .sophia.yaml at repo root
# or pass project inline:
sophia run "..." --project <slug>
```

---

## References

- `.sophia.yaml` — project config (repo root)
- `~/.config/sophia/config.yaml` — user config
- `~/.local/state/sophia/` — local change state
- `docs/adr/0003-cross-repo-wire-alignment.md` — sophia-wire-v1 contract
- `docs/release/manual-smoke-checklist.md` — release smoke procedure
- `CHANGELOG.md` — version history
