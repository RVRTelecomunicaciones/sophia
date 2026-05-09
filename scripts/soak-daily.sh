#!/usr/bin/env bash
# soak-daily.sh — durable daily soak smoke for the M10 v0.2.0 release.
#
# Runs the read-only D-M10-15 / D-M10-16 release-blocker gates and
# writes a per-day log to ~/.local/state/sophia/soak/YYYY-MM-DD.log
# plus a `latest-summary.txt`. Does NOT commit, push, or modify the
# soak matrix — that's reserved for the interactive Claude flow.
#
# Designed to be invoked by launchd (~/Library/LaunchAgents/com.sophia.soak.plist)
# at 09:13 local daily during the v0.2.0-rc.1 → v0.2.0 soak window
# (2026-05-08 → 2026-05-15). Idempotent: re-running on the same day
# overwrites that day's log.
#
# Exit code: always 0. The log captures pass/fail; we never want
# launchd to retry-spam a transient miss.

set -u

# --- paths + env ---
cli_repo="$(cd "$(dirname "$0")/.." && pwd)"
orch_repo="$cli_repo/../sophia-orchestator"
state_dir="${SOPHIA_SOAK_STATE:-$HOME/.local/state/sophia/soak}"
mkdir -p "$state_dir"

today=$(date +%Y-%m-%d)
log="$state_dir/$today.log"
summary="$state_dir/latest-summary.txt"

# launchd starts with a near-empty PATH. Reach into the canonical
# install paths so go/git/gh/shasum resolve.
export PATH="/usr/local/go/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH"

red=0
section() { printf '\n=== %s ===\n' "$*"; }
note()    { printf '  - %s\n' "$*"; }
green()   { printf '  ✓ %s\n' "$*"; }
fail()    { red=$((red+1)); printf '  ✗ %s\n' "$*"; }

# --- run as a script, capture all stdout/stderr to the log ---
{
  printf 'soak-daily %s — host=%s repo=%s orch=%s\n' \
    "$today" "$(hostname -s)" "$cli_repo" "$orch_repo"

  # gate 1: SHA256 cross-repo invariant
  section "gate 1: SHA256 cross-repo"
  cli_sha=$(shasum -a 256 "$cli_repo/docs/specs/sophia-wire-v1.md" 2>/dev/null | awk '{print $1}')
  orch_sha=$(shasum -a 256 "$orch_repo/docs/specs/sophia-wire-v1.md" 2>/dev/null | awk '{print $1}')
  if [ -n "$cli_sha" ] && [ "$cli_sha" = "$orch_sha" ]; then
    green "match: $cli_sha"
  else
    fail "drift or missing: cli=$cli_sha orch=$orch_sha"
  fi

  # gate 2: cli go test
  section "gate 2: cli go test ./..."
  if ( cd "$cli_repo" && go test ./... -timeout 180s ) >/tmp/soak-cli-test.$$ 2>&1; then
    green "passed ($(grep -c '^ok ' /tmp/soak-cli-test.$$) packages)"
  else
    fail "go test failed (see log below)"
    grep -E "FAIL|^---" /tmp/soak-cli-test.$$ | head -20
  fi
  rm -f /tmp/soak-cli-test.$$

  # gate 3: cli GOWORK=off
  section "gate 3: cli GOWORK=off go test ./..."
  if ( cd "$cli_repo" && GOWORK=off go test ./... -timeout 180s ) >/tmp/soak-cli-gw.$$ 2>&1; then
    green "passed ($(grep -c '^ok ' /tmp/soak-cli-gw.$$) packages)"
  else
    fail "GOWORK=off go test failed"
    grep -E "FAIL|^---" /tmp/soak-cli-gw.$$ | head -20
  fi
  rm -f /tmp/soak-cli-gw.$$

  # gate 4: cli contract suite
  section "gate 4: cli go test -tags=contract"
  if ( cd "$cli_repo" && go test -tags=contract ./test/contract/... -timeout 120s ) >/tmp/soak-cli-c.$$ 2>&1; then
    green "passed"
  else
    fail "contract suite failed"
    grep -E "FAIL|^---" /tmp/soak-cli-c.$$ | head -20
  fi
  rm -f /tmp/soak-cli-c.$$

  # gate 5: orch go test
  section "gate 5: orch go test ./..."
  if ( cd "$orch_repo" && go test ./... -timeout 180s ) >/tmp/soak-orch-test.$$ 2>&1; then
    green "passed ($(grep -c '^ok ' /tmp/soak-orch-test.$$) packages)"
  else
    fail "orch go test failed"
    grep -E "FAIL|^---" /tmp/soak-orch-test.$$ | head -20
  fi
  rm -f /tmp/soak-orch-test.$$

  # gate 6: orch GOWORK=off
  section "gate 6: orch GOWORK=off go test ./..."
  if ( cd "$orch_repo" && GOWORK=off go test ./... -timeout 180s ) >/tmp/soak-orch-gw.$$ 2>&1; then
    green "passed ($(grep -c '^ok ' /tmp/soak-orch-gw.$$) packages)"
  else
    fail "orch GOWORK=off go test failed"
    grep -E "FAIL|^---" /tmp/soak-orch-gw.$$ | head -20
  fi
  rm -f /tmp/soak-orch-gw.$$

  # GitHub CI status (informational, doesn't gate)
  section "GitHub CI status (informational)"
  if command -v gh >/dev/null 2>&1; then
    cli_ci=$(gh -R RVRTelecomunicaciones/sophia         run list --branch main --limit 1 --json status,conclusion 2>/dev/null \
              | jq -r '.[0] | "\(.status)/\(.conclusion // "n/a")"' 2>/dev/null)
    orch_ci=$(gh -R RVRTelecomunicaciones/sophia-orchestrator run list --branch main --limit 1 --json status,conclusion 2>/dev/null \
              | jq -r '.[0] | "\(.status)/\(.conclusion // "n/a")"' 2>/dev/null)
    note "cli main:  ${cli_ci:-unavailable}"
    note "orch main: ${orch_ci:-unavailable}"
  else
    note "gh CLI not found; skipping CI status check"
  fi

  # final summary
  section "summary"
  if [ "$red" -gt 0 ]; then
    printf '  RESULT: %d gate(s) RED — soak Day finding\n' "$red"
    printf '  Action: when next interactive Claude session opens, triage and update matrix.\n'
  else
    printf '  RESULT: all 6 gates GREEN\n'
  fi
  printf '\nlog file: %s\n' "$log"
} >"$log" 2>&1

# Always update the latest-summary so vimdiff `latest-summary.txt` shows
# yesterday vs today at a glance.
{
  printf 'last-run: %s\n' "$(date +%Y-%m-%dT%H:%M:%S%z)"
  printf 'red-gates: %d\n' "$red"
  printf 'log: %s\n' "$log"
  printf '\n--- tail ---\n'
  tail -20 "$log"
} >"$summary"

# Always exit 0; the log carries the pass/fail signal.
exit 0
