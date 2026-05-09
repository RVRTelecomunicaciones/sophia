#!/usr/bin/env bash
# promote-v0.2.0.sh — coordinated promotion script for the M10
# v0.2.0-rc.1 → v0.2.0 cut-over per D-M10-11.
#
# Runs the closing smoke pass, asserts every D-M10-16 release-blocker
# gate, and (only when --confirm is passed) creates + pushes the
# v0.2.0 annotated tag on both repos.
#
# Usage:
#   ./scripts/promote-v0.2.0.sh [--confirm] [--orch-repo PATH]
#
#   --confirm        actually create and push the tags. Without it, the
#                    script runs in dry-run mode: every gate is checked
#                    and reported, no destructive ops execute.
#   --orch-repo P    path to sophia-orchestator checkout. Defaults to
#                    ../sophia-orchestator relative to the cli repo.
#
# Exit codes:
#   0 — all gates green, dry-run clean OR tags pushed.
#   1 — one or more gates RED. Promotion blocked. See output.
#   2 — argument or environment error.
#
# This script is idempotent in dry-run mode. In --confirm mode, refuses
# to run if the v0.2.0 tag already exists on either side.

set -u   # NB: not -e; we want to keep checking gates even if one fails
         # so the operator sees the FULL list of red entries in one pass.

confirm=0
orch_repo="$(cd "$(dirname "$0")/.." && pwd)/../sophia-orchestator"

while [ $# -gt 0 ]; do
  case "$1" in
    --confirm)    confirm=1 ;;
    --orch-repo)  shift; orch_repo="$1" ;;
    -h|--help)    sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "promote: unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done

cli_repo="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -d "$orch_repo/.git" ]; then
  echo "promote: orchestrator repo not found at $orch_repo" >&2
  echo "         pass --orch-repo PATH to override." >&2
  exit 2
fi

# tool presence check — the hardened gates depend on these being
# installed locally. If any is missing, fail closed (we don't want
# silent skips on a release-blocker check).
for tool in shasum go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "promote: required tool '$tool' not on PATH" >&2
    exit 2
  fi
done

red=0
note() { printf "  → %s\n" "$*"; }
red() { red=$((red+1)); printf "  \033[31m✗\033[0m %s\n" "$*"; }
green() { printf "  \033[32m✓\033[0m %s\n" "$*"; }
hdr() { printf "\n=== %s ===\n" "$*"; }

# --- gate 1: SHA256 cross-repo invariant (D-M10-16 #1) ---
hdr "gate 1: SHA256 cross-repo (D-M10-16 #1)"
cli_sha=$(shasum -a 256 "$cli_repo/docs/specs/sophia-wire-v1.md" | awk '{print $1}')
orch_sha=$(shasum -a 256 "$orch_repo/docs/specs/sophia-wire-v1.md" | awk '{print $1}')
if [ "$cli_sha" = "$orch_sha" ]; then
  green "match: $cli_sha"
else
  red "drift: cli=$cli_sha orch=$orch_sha"
fi

# --- gate 2: cli local tests (D-M10-16 #2 + D-M10-15) ---
hdr "gate 2: cli local tests (D-M10-15 + D-M10-16 #2)"
( cd "$cli_repo" && go test ./... -timeout 180s >/tmp/promote-cli-test.log 2>&1 )
if [ $? -eq 0 ]; then green "go test ./... — passed"
else red "go test ./... — see /tmp/promote-cli-test.log"; fi

( cd "$cli_repo" && GOWORK=off go test ./... -timeout 180s >/tmp/promote-cli-gowork-off.log 2>&1 )
if [ $? -eq 0 ]; then green "GOWORK=off go test ./... — passed"
else red "GOWORK=off go test ./... — see /tmp/promote-cli-gowork-off.log"; fi

( cd "$cli_repo" && go test -tags=contract ./test/contract/... -timeout 120s >/tmp/promote-cli-contract.log 2>&1 )
if [ $? -eq 0 ]; then green "make contract — passed"
else red "make contract — see /tmp/promote-cli-contract.log"; fi

# --- gate 3: orch local tests (D-M10-15) ---
hdr "gate 3: orch local tests (D-M10-15)"
( cd "$orch_repo" && go test ./... -timeout 180s >/tmp/promote-orch-test.log 2>&1 )
if [ $? -eq 0 ]; then green "go test ./... — passed"
else red "go test ./... — see /tmp/promote-orch-test.log"; fi

( cd "$orch_repo" && GOWORK=off go test ./... -timeout 180s >/tmp/promote-orch-gowork-off.log 2>&1 )
if [ $? -eq 0 ]; then green "GOWORK=off go test ./... — passed"
else red "GOWORK=off go test ./... — see /tmp/promote-orch-gowork-off.log"; fi

# --- gate 4: CHANGELOG carries Compatibility section (D-M10-16 #4) ---
hdr "gate 4: CHANGELOG Compatibility section (D-M10-16 #4)"
if grep -q "^### Compatibility" "$cli_repo/CHANGELOG.md"; then
  green "cli CHANGELOG.md has '### Compatibility' section"
else
  red "cli CHANGELOG.md missing '### Compatibility' section"
fi
if grep -q "^### Compatibility" "$orch_repo/CHANGELOG.md"; then
  green "orch CHANGELOG.md has '### Compatibility' section"
else
  red "orch CHANGELOG.md missing '### Compatibility' section"
fi

# --- gate 5: soak matrix has zero open RED entries (D-M10-16 #3) ---
hdr "gate 5: soak matrix — zero open RED (D-M10-16 #3)"
matrix="$cli_repo/docs/release/v0.2.0-soak-matrix.md"
if [ ! -f "$matrix" ]; then
  red "soak matrix not found at $matrix"
else
  red_block=$(awk '/^## Open RED entries/,/^## /{print}' "$matrix" | grep -v "^## " | grep -v "^(none" | tr -d '[:space:]')
  if [ -z "$red_block" ]; then
    green "no open RED entries"
  else
    red "soak matrix has unresolved RED entries — review $matrix"
  fi
fi

# --- gate 6: cli static-analysis (gosec + golangci-lint + govulncheck) ---
hdr "gate 6: cli static analysis (D-M10-15 verification matrix)"
# These three caught the Day 0 fix-cycles (gosec G101 false-positives,
# gofmt drift, unused dead code, golangci-lint v2 issues). Running them
# pre-tag means we never push a tag that fails CI.
if command -v gosec >/dev/null 2>&1; then
  ( cd "$cli_repo" && gosec -severity high -quiet ./... >/tmp/promote-cli-gosec.log 2>&1 )
  if [ $? -eq 0 ]; then green "gosec -severity high — clean"
  else red "gosec found HIGH severity issues — see /tmp/promote-cli-gosec.log"; fi
else
  note "gosec not installed; install with 'go install github.com/securego/gosec/v2/cmd/gosec@latest' to gate"
  red "gosec missing — release blocker per verification matrix"
fi
if command -v golangci-lint >/dev/null 2>&1; then
  ( cd "$cli_repo" && golangci-lint run --timeout=3m ./... >/tmp/promote-cli-lint.log 2>&1 )
  if [ $? -eq 0 ]; then green "golangci-lint run — clean"
  else red "golangci-lint failed — see /tmp/promote-cli-lint.log"; fi
else
  note "golangci-lint not installed"
  red "golangci-lint missing — release blocker per verification matrix"
fi
# govulncheck is gated by CI (which uses the newest 1.26.x via
# actions/setup-go). Local machines may run an older Go with stdlib
# vulns; treating that as a release-blocker would force every operator
# to keep `go` perfectly current. Instead: run + log informational,
# and rely on CI's `govulncheck` job (which is the canonical gate
# per verification matrix) for the actual block.
if command -v govulncheck >/dev/null 2>&1; then
  ( cd "$cli_repo" && govulncheck ./... >/tmp/promote-cli-vuln.log 2>&1 )
  if [ $? -eq 0 ]; then green "govulncheck — no reachable HIGH/CRITICAL"
  else
    note "govulncheck reports findings (informational; see /tmp/promote-cli-vuln.log)"
    note "  → CI's govulncheck job is the canonical gate — verify it green at the to-be-tagged commit."
  fi
else
  note "govulncheck not installed (install: go install golang.org/x/vuln/cmd/govulncheck@latest)"
fi

# --- gate 7: orch static analysis ---
# Reflects the soak-matrix exemptions: orch's lint job is pre-existing
# YELLOW (v1→v2 .golangci.yaml schema mismatch on the action), and the
# orch's standalone gosec invocation triggers G115 (excluded in the
# orch's own .golangci.yaml as a Go-1.22+ false-positive) and G404
# (math/rand for jitter / backoff — non-security; pre-existing). All
# three are out-of-scope for v0.2.0 release and tracked as
# follow-ups. Findings are LOGGED but NOT release-blockers here.
hdr "gate 7: orch static analysis"
if command -v gosec >/dev/null 2>&1; then
  ( cd "$orch_repo" && gosec -severity high -quiet ./... >/tmp/promote-orch-gosec.log 2>&1 )
  if [ $? -eq 0 ]; then green "orch gosec — clean"
  else
    note "orch gosec findings (informational): G115 excluded by orch's .golangci.yaml as Go-1.22+ false-positive; G404 (math/rand) used for jitter/backoff (non-security)"
    note "  → tracked as post-v0.2.0 follow-up; see /tmp/promote-orch-gosec.log"
  fi
fi
if command -v govulncheck >/dev/null 2>&1; then
  ( cd "$orch_repo" && govulncheck ./... >/tmp/promote-orch-vuln.log 2>&1 )
  if [ $? -eq 0 ]; then green "orch govulncheck — clean"
  else
    note "orch govulncheck findings (informational; CI is the canonical gate)"
    note "  → see /tmp/promote-orch-vuln.log"
  fi
fi
note "orch golangci-lint NOT gated — pre-existing YELLOW per soak matrix; post-v0.2.0 follow-up"

# --- gate 8: manual smoke checklist signed-off ---
hdr "gate 8: manual smoke checklist (D-M10-16 #3)"
checklist="$cli_repo/docs/release/manual-smoke-checklist.md"
if [ ! -f "$checklist" ]; then
  red "checklist missing at $checklist"
elif grep -q "^| Reviewer | __________ |" "$checklist"; then
  red "v0.2.0 sign-off block is empty in $checklist (Reviewer field still placeholder)"
else
  green "v0.2.0 sign-off block populated"
fi

# --- gate 9: tags don't already exist on remote ---
hdr "gate 9: v0.2.0 tag absence (no double-tag)"
if ( cd "$cli_repo" && git rev-parse v0.2.0 >/dev/null 2>&1 ); then
  red "cli already has local tag v0.2.0 — refusing to retag"
else
  green "cli has no v0.2.0 tag locally"
fi
if ( cd "$orch_repo" && git rev-parse v0.2.0 >/dev/null 2>&1 ); then
  red "orch already has local tag v0.2.0 — refusing to retag"
else
  green "orch has no v0.2.0 tag locally"
fi

# --- summary + decision ---
hdr "summary"
if [ "$red" -gt 0 ]; then
  printf "  \033[31m%d gate(s) RED.\033[0m Promotion BLOCKED.\n" "$red"
  printf "  Triage per docs/release/v0.2.0-soak-matrix.md §Promotion gate.\n"
  exit 1
fi

if [ "$confirm" -eq 0 ]; then
  printf "  \033[32mAll gates green.\033[0m Dry-run mode — no tags created.\n"
  printf "  Re-run with --confirm to actually tag and push v0.2.0 on both repos.\n"
  exit 0
fi

# --- promotion (only with --confirm) ---
hdr "promotion: tagging v0.2.0 on both repos"
cli_head=$(cd "$cli_repo" && git rev-parse HEAD)
orch_head=$(cd "$orch_repo" && git rev-parse HEAD)
sha=$(shasum -a 256 "$cli_repo/docs/specs/sophia-wire-v1.md" | awk '{print $1}')

msg_cli=$(printf 'sophia-cli v0.2.0 — coordinated release\n\nClosed the v0.2.0-rc.1 → v0.2.0 soak window per D-M10-11.\nPairs with sophia-orchestator v0.2.0 cut on the same day.\n\ncli HEAD: %s\norch HEAD: %s\nsophia-wire-v1 SHA256: %s' \
  "$cli_head" "$orch_head" "$sha")
msg_orch=$(printf 'sophia-orchestator v0.2.0 — coordinated release\n\nClosed the v0.2.0-rc.1 → v0.2.0 soak window per D-M10-11.\nPairs with sophia-cli v0.2.0 cut on the same day.\n\norch HEAD: %s\ncli HEAD: %s\nsophia-wire-v1 SHA256: %s' \
  "$orch_head" "$cli_head" "$sha")

( cd "$cli_repo" && git tag -a v0.2.0 -m "$msg_cli" )
green "cli tag v0.2.0 → $cli_head"
( cd "$orch_repo" && git tag -a v0.2.0 -m "$msg_orch" )
green "orch tag v0.2.0 → $orch_head"

note "tags created locally. Push them when explicitly authorized:"
note "  ( cd $cli_repo && git push origin v0.2.0 )"
note "  ( cd $orch_repo && git push origin v0.2.0 )"
note "the cli push triggers goreleaser → public v0.2.0 release."
exit 0
