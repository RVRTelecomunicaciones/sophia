#!/usr/bin/env bash
#
# Fallback inventory used by `make licenses` when `go-licenses` cannot run
# against the current toolchain. Produces a best-effort module list from
# `go list -m -json all`. License field is NOT resolved — manual review
# required before redistribution.
#
# This is acknowledged for v0.1.0 in docs/release/security-notes.md. M10
# revisits when go-licenses upstream catches up to Go 1.26.x.

set -euo pipefail

cat <<'HEADER'
# Third-Party Licenses (best-effort inventory)

> Generated via `go list -m -json all` because `go-licenses` did not run on
> this toolchain. License names below are NOT resolved; cross-check against
> each module's upstream `LICENSE` file before redistribution.

| Module | Version | License |
|--------|---------|---------|
HEADER

go list -m -json all | jq -r '
  select(.Main != true)
  | "| `\(.Path)` | `\(.Version // "(replaced)")` | (verify manually) |"
'
