# Requirements: Merge Latest Zed Upstream (002059)

## Background

Routine upstream-merge cadence for the Helix fork of Zed (`helixml/zed`).
Goal: pull the latest `zed-industries/zed` commits into the fork, preserve every
Helix-specific patch, update `portingguide.md` live, and verify with `cargo
test` plus the full external WebSocket sync e2e suite before closing.

**Baseline at task start (2026-06-01):**
- Fork main HEAD: `e60a1b2789` (2026-05-21) — `revert(context_server): DEFAULT_REQUEST_TIMEOUT back to upstream 60s`
- Last upstream merge on main: `bf544922aa` (2026-05-11), fence `8bdd78e023`
- 16 Helix-only non-merge commits since that fence
- **Prior task 002029 PRs are STILL OPEN** as of 2026-06-01:
  - `helixml/zed#58` (head `fb97e2cf95` on `feature/002029-merge-latest-zed`) — merges upstream to `13e7c11768`
  - `helixml/helix#2480` (bumps `ZED_COMMIT` to `fb97e2cf95`)
  - No activity since 2026-05-25
- `upstream` git remote is **not configured** locally in `/home/retro/work/zed`

## User Stories

**As a Helix maintainer**, I want the fork to track the latest `zed-industries/zed`
HEAD so that upstream bug fixes, performance work, and protocol improvements
land continuously rather than as risky catch-up merges.

**As a Helix user**, I want every existing Helix-specific behaviour to keep
working after the merge — WebSocket sync, agent panel routing, headless mode,
ACP cancel protocol, draft-thread suppression, multiple-instance support, all
eleven Critical Fixes (minus retired #10).

**As a future merger**, I want every conflict resolution recorded in
`portingguide.md` as it happens so the next merge starts with full context
rather than archaeology.

## Acceptance Criteria

1. The Helix Zed fork carries the latest upstream `zed-industries/zed` HEAD
   reachable as of the merge attempt (or an explicitly documented safe-stop SHA
   if a later commit is too risky).
2. `portingguide.md` has a new `## Merge 002059 (YYYY-MM-DD)` section with
   `### Conflicts and Resolutions` (per-conflict: Upstream change / HEAD change
   / Resolution / Risk / Reasoning) and `### Pre-existing Breakage Repaired`
   subsections, matching the format used by Merge 001980 and Merge 001996.
   Entries are added live as each conflict is resolved, not retrospectively.
3. `cargo test` passes — zero failures — on the merged branch.
4. The external WebSocket sync e2e suite (`crates/external_websocket_sync/e2e-test/`)
   passes **all 17 phases for both `zed-agent` and `claude` personalities**.
   Phase 17 is the explicit hard gate for PR #56 Fix 1b survival; Phase 13 is
   the hard gate for the `turn_cancelled`-before-`cancel` ordering; Phase 15
   for PR #55's `EntryUpdated` emit; Phase 16 for PR #56 Fix 1a + PR #57.
5. A PR is open against `helixml/zed` main on a new branch
   `feature/002059-merge-latest-zed` (the `002029` branch name already exists
   on origin and must not be reused). A paired `helixml/helix` PR on the same
   branch name bumps `ZED_COMMIT` in `sandbox-versions.txt`.
6. Every retired Helix patch (if any) has an explicit justification in the
   porting guide. Critical Fix #10 stays retired.

## Out of Scope

- Net-new Helix feature development.
- Modifying e2e tests themselves unless a legitimate upstream API change
  strictly requires it.
- Upstreaming Helix patches back to `zed-industries`.

## Open Question for the Implementation Agent

How does this merge relate to the still-open prior PRs (`zed#58`, `helix#2480`)?
The implementation must pick one of: (a) wait for 002029 to land first, then
merge from main with upstream at `13e7c11768` as the new baseline; (b) start
from `feature/002029-merge-latest-zed` (stack on top of it); (c) take over and
extend the 002029 PRs with additional commits per the Round 2 precedent. The
design doc recommends option (a) as the simplest, with (b) as fallback if 002029
remains stalled. See design.md for the decision tree.
