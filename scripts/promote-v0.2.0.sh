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

# --- gate 6: tags don't already exist on remote ---
hdr "gate 6: v0.2.0 tag absence (no double-tag)"
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
