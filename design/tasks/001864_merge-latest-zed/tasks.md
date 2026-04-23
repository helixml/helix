# Implementation Tasks

## Pre-Merge: Update Porting Guide with New Fork Patches

- [x] Review fork commits since last merge
- [x] Document new patches (PRs #32-#41) in portingguide.md — 20 new commit entries + 5 new rebase checklist items (#34-#38)
- [x] Verify rebase checklist item count is accurate (now items 1-41)

## Setup

- [x] Create feature branch `feature/001864-merge-latest-zed` from `main`
- [x] Add upstream remote and fetch (973 commits behind, 178 ahead)
- [x] Portingguide updates committed to feature branch

## Merge

- [~] Merge `upstream/main` directly into feature branch (973 commits, fresh merge since task 001554)
- [ ] Resolve all merge conflicts, preserving Helix-specific patches (reference 001723 branch resolutions as guide)

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
