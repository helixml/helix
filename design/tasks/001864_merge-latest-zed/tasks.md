# Implementation Tasks

## Pre-Merge: Update Porting Guide with New Fork Patches

Since the last upstream merge (task 001723, Apr 16), the fork has gained new Helix-specific commits (PRs #37-#41). These must be documented in `portingguide.md` before merging upstream.

- [ ] Review fork commits since last merge
- [ ] Document new patches (PRs #37-#41) in portingguide.md — especially PR #40 (request_id desync fix, Critical Fix #9) and PR #41 (ACP auto-approve)
- [ ] Verify rebase checklist item count is accurate

## Setup

- [ ] Create feature branch `feature/001864-merge-latest-zed` from `main`
- [ ] Merge `origin/feature/001723-merge-latest-zed` into it (698 commits: prior upstream merge + post-merge fixes, never merged to main)
- [ ] Add upstream remote: `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] Fetch upstream and check divergence since `d066ff0ae5` (where 001723 left off)

## Merge Latest Upstream

- [ ] Merge `upstream/main` to pick up commits after Apr 15 (feature flag overrides, sidebar fixes, tsgo LSP fix)
- [ ] Resolve any merge conflicts, preserving Helix-specific patches

## Verify Critical Fixes

- [ ] Grep-verify all 9 critical fixes from portingguide.md are present
- [ ] Check `from_existing_thread()` constructor signature consistency
- [ ] Check ConnectedServerState fields in both AcpServerView and ConversationView

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync`
- [ ] `cargo test -p external_websocket_sync`
- [ ] Run E2E Docker tests (if ANTHROPIC_API_KEY available)

## Update Documentation

- [ ] Update `portingguide.md` with any new conflict patterns or checklist items from this merge

## Finalize

- [ ] Push feature branch to `helixml/zed`
- [ ] Create PR description files
- [ ] After PR merges: update `sandbox-versions.txt` in helix repo with new `ZED_COMMIT` SHA
