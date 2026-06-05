# Implementation Tasks: Merge Latest Zed Upstream (002059)

## Preflight

- [ ] Re-check `helixml/zed#58` and `helixml/helix#2480` status; pick posture (a), (b), or (c) per design.md. Document the choice at the top of the porting-guide merge section.
- [ ] Read `/home/retro/work/zed/portingguide.md` end-to-end, with focus on Critical Fixes, Rebase Checklist (including suffix items 12a/31a/39a/41a), and the Merge 001980 + Merge 001996 narratives.
- [ ] Read `/home/retro/work/helix-specs/design/tasks/002029_merge-latest-zed/` design.md + tasks.md + pull_request_*.md (the 002029 narrative is *not* on main yet; this is the closest precedent).
- [ ] In `/home/retro/work/zed`, add upstream remote: `git remote add upstream https://github.com/zed-industries/zed.git && git fetch upstream main`.
- [ ] Record upstream HEAD SHA and commit count in the merge section of `portingguide.md`.

## Branch setup

- [ ] Create `feature/002059-merge-latest-zed` on `helixml/zed` (off the chosen baseline per posture decision).
- [ ] Create the same-named branch on `helixml/helix` from current `main`.

## Merge & resolve

- [ ] `git merge upstream/main` on the Zed feature branch.
- [ ] Resolve each conflict; for **each** conflict, immediately append an entry under `## Merge 002059 (YYYY-MM-DD)` → `### Conflicts and Resolutions` in `portingguide.md` with the Upstream change / HEAD change / Resolution / Risk / Reasoning subfields. Commit per conflict or per small batch.
- [ ] Apply default rules: `Cargo.lock` → `--theirs`; `.github/workflows/*` → upstream.
- [ ] For each Helix patch potentially affected, decide: keep / adapt to new API / retire (with documented justification). Critical Fix #10 stays retired.

## Build & repair

- [ ] `./stack build-zed dev` — fix any compile errors. Add `### Pre-existing Breakage Repaired` entries to the porting-guide section for each repair.
- [ ] Verify Fix 1b is the **first** statement of `BaseView::Uninitialized` branch in `ensure_thread_initialized`.
- [ ] Re-grep `ConnectedServerState` field set; update `ConversationView::from_existing_thread()` to construct any new fields.
- [ ] Add arms for any new `BaseView` / `ActiveView` / `ContextServerStatus` variants in Helix UI state queries.

## Silent-drift greps (must all be 0 in `agent_ui/src/` and `agent/src/agent.rs`)

- [ ] `ActiveView`, `set_active_view`, `draft_threads`, `background_threads`, `selected_agent_type`, `smol::Timer`.
- [ ] `Stopped` not followed by `(` (pattern-match tuple variant — including test code).

## Test

- [ ] `cargo test` — zero failures.
- [ ] `cd crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy`.
- [ ] Run `crates/external_websocket_sync/e2e-test/run_e2e.sh` with `--agent zed-agent`; confirm all 17 phases pass.
- [ ] Run `crates/external_websocket_sync/e2e-test/run_e2e.sh` with `--agent claude`; confirm all 17 phases pass. (Re-run on Phase 1 npm-install flake.)
- [ ] If Phase 17 fails, restore Fix 1b's cfg-gated early return at the top of `BaseView::Uninitialized` and re-run. Do not proceed without Phase 17 green.

## Finalize porting guide

- [ ] Verify the `## Merge 002059 (YYYY-MM-DD)` section is complete: every conflict resolved → an entry; every pre-existing breakage repaired → an entry.
- [ ] If a Helix patch was retired, add a note to its Critical Fix section in `portingguide.md` (mirror the Fix #10 RETIRED treatment from 002029).
- [ ] Update the Commit History table at the bottom of `portingguide.md` with new SHAs and one-line descriptions.

## Out-of-band fork-push reconciliation

- [ ] Before opening PRs: `git fetch origin && git merge origin/main` on the Zed feature branch; re-run cargo test + E2E if anything came in.
- [ ] Same on the Helix feature branch.

## PRs

- [ ] Open `helixml/zed` PR `feature/002059-merge-latest-zed` → `main` with a summary of upstream range, conflict count, and any retirements.
- [ ] Bump `ZED_COMMIT` in `sandbox-versions.txt` on the Helix feature branch to the head SHA of the Zed feature branch.
- [ ] Open `helixml/helix` PR `feature/002059-merge-latest-zed` → `main` **before** the Zed PR is merged.
- [ ] Cross-link the two PRs in their descriptions.

## Done definition

- [ ] All five acceptance criteria from requirements.md met.
- [ ] Porting guide entry committed and pushed.
- [ ] Both PRs open and linked.
- [ ] E2E green for both `zed-agent` and `claude` personalities, all 17 phases.
