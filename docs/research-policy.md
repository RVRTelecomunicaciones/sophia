# Research Policy — Sophia ecosystem

> **Effective:** 2026-05-07. Applies to all external research conducted for
> any Sophia repo (`sophia-cli`, `sophia-orchestator`, `sophia-runtime-adapters`,
> `agent-governance-core`, `sophia-memory-engine`) and any new repo that
> joins the ecosystem. Violations of this policy block the decision they
> were used to justify.

## Scope

This policy governs the use of external information sources to support
decisions that affect architecture, CI, release process, security
posture, or HTTP/RPC APIs. Internal research (reading the project's own
code, commit history, ADRs, plans, specs) is exempt.

## Why

The Sophia ecosystem moved fast through 2026 H1; libraries it depends
on (`bubbletea v2`, `lipgloss v2`, `go-sse`, `goreleaser v2`,
`golangci-lint v2`, etc.) had public release lines that diverged
significantly within the first half of 2026. Pre-2026 guidance for any
of these is at substantial risk of being silently outdated, and
pre-2026 guidance for the Sophia repos themselves does not exist
(they were authored in 2026). Defaulting to fresh sources eliminates
an entire class of "stale snippet" bugs.

## Freshness rules

### Priority 1 — primary sources

Use sources published between **2026-03-07 and 2026-05-07** (the most
recent 60 days at policy effective date) FIRST. The window slides
forward; a future decision in 2026-09 should target 2026-07-09 →
2026-09-08 as primary unless a narrower window is documented in the
research log.

### Priority 2 — broadened window

If primary-window sources are insufficient, broaden to **2026-01-01 →
the present**. The research log MUST record that the broadening was
necessary and why.

### Excluded — pre-2026

Sources from 2025 or earlier MUST NOT be used to justify NEW decisions
in 2026. They MAY be cited for historical context (e.g. "the v1 line
was released in 2025-Q4 and is now deprecated"), but cannot stand
alone as the basis for an architecture, CI, release, security, or
API decision.

### Exception — undated official docs

Official documentation pages without a publication date (vendor docs,
RFCs, project READMEs that don't carry "Last updated" metadata) may
serve as a **secondary reference only**. They MUST be tagged in the
research log with the literal string `official current docs, undated`,
and MUST NOT be the SOLE basis for a decision that changes
architecture, CI, release, security, or APIs. Pair them with at least
one dated 2026 source from Priority 1 or 2.

### Social-media / blog sources

Medium articles, LinkedIn posts, X/Twitter threads, Reddit threads,
forum posts, personal blogs:
- 2026 publication date → admissible as secondary signal.
- pre-2026 → discard, regardless of perceived authority of author.

## Source confidence order

When multiple sources disagree, prefer in this order:

1. Official documentation of the upstream library/standard.
2. Official release notes (e.g. GitHub Releases page).
3. GitHub releases / issues / discussions on the project's own repo.
4. Project CHANGELOGs.
5. Maintainer-authored technical discussions (PR descriptions, design
   docs, RFC comments).
6. Forums / Reddit / LinkedIn / X — secondary signal only, never
   sole basis.

## Research log requirement

Every decision that consults external sources MUST record an entry in
the research log of the affected plan/ADR/repo. Template:

```markdown
### YYYY-MM-DD — <decision short title>

- **Problem:** <one sentence describing the question>
- **Source(s) consulted:**
  - <URL or citation> — <publication date YYYY-MM-DD>
  - <URL or citation> — `official current docs, undated`
  - <URL or citation> — <publication date YYYY-MM-DD>
- **Decision:** <what was decided>
- **Impact:** <which plan / spec / code surface changed>
- **Researcher:** <user or agent name>
```

Logs live alongside the plan or ADR they inform. M10's research log is
at `docs/superpowers/plans/2026-05-07-sophia-m10-wire-alignment-v0.2.0.md`
under the "Research log" section (added by this policy).

## Enforcement

- Any PR or commit message that cites a source for a decision MUST
  include the publication date.
- Reviewers MUST reject decisions justified by pre-2026 sources unless
  the cited source is the only available one AND a `official current
  docs, undated` tag accompanies it.
- The research log is part of the deliverable; an empty log on a
  research-driven phase is a release blocker.

## Exemptions

The following research activities are NOT subject to this policy:

- Reading the project's own code, tests, and documentation.
- Reading the project's own ADRs, plans, CHANGELOGs.
- Querying the project's own Git history.
- Standard library Go documentation (`pkg.go.dev` for `stdlib`).
- The Go language spec.
- The HTTP/SSE/JSON RFCs.

These are evergreen by design and don't carry the staleness risk this
policy targets.

## Revision

This policy itself follows the freshness rules: revisions need a 2026
external rationale, recorded in the research log of the revising
commit.
