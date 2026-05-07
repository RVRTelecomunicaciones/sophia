#!/usr/bin/env bash
#
# Headless demo for asciinema recording. Shows the binary surface (version,
# command listing, flag listing, exit-code mapping) WITHOUT requiring a live
# orchestrator. The flow demo (run -> attach -> done) is deferred to v0.1.1
# when a recording against a live orchestrator can be staged.
#
# Run inside asciinema:
#   asciinema rec assets/demo/sophia-quickstart.cast \
#       -c "bash scripts/demo.sh" \
#       --idle-time-limit 1.0

set -euo pipefail

SOPHIA="${SOPHIA:-./bin/sophia}"
PAUSE="${PAUSE:-0.6}"

say() {
    printf "\n\033[1;36m$ %s\033[0m\n" "$1"
    sleep "$PAUSE"
}

run() {
    say "$*"
    eval "$@" 2>&1 || true
    sleep "$PAUSE"
}

clear

run "$SOPHIA version"
run "$SOPHIA --help"
run "$SOPHIA changes --help"
run "$SOPHIA attach --help"

# Empty status — no XDG state, no orchestrator needed.
say "XDG_STATE_HOME=\$(mktemp -d) $SOPHIA status"
XDG_STATE_HOME=$(mktemp -d) "$SOPHIA" status 2>&1 || true
sleep "$PAUSE"

# Exit-code demo: orchestrator-down maps to exit 3.
say "$SOPHIA status MISSING-ID --json"
"$SOPHIA" status MISSING-ID --json 2>&1 || true
echo "(exit code: $?)"
sleep "$PAUSE"

printf "\n\033[1;32mFor the full run -> attach -> done flow, see docs/superpowers/plans\033[0m\n"
sleep "$PAUSE"
