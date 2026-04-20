# Implementation Tasks

## Setup

- [ ] Read previous merge spec (001723) and `portingguide.md` for current rebase checklist
- [ ] Create feature branch from `helixml/zed` main: `feature/001864-merge-latest-zed`
- [ ] Add upstream remote if not present: `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] Fetch upstream: `git fetch upstream`
- [ ] Check divergence: `git log --oneline main..upstream/main | wc -l`

## Merge

- [ ] Merge upstream/main into feature branch: `git merge upstream/main`
- [ ] Resolve all merge conflicts, preserving Helix-specific patches
- [ ] Pay special attention to feature flag changes (#54206) — verify no impact on compile-time `cfg(feature)` gates
- [ ] Check sidebar/workspace changes (#54198, #54207) for conflicts with agent_panel.rs

## Verify Critical Fixes

- [ ] Grep-verify all 9 critical fixes from portingguide.md are present
- [ ] Walk through all 40 rebase checklist items in portingguide.md
- [ ] Check `from_existing_thread()` constructor signature consistency
- [ ] Check ConnectedServerState fields in both AcpServerView and ConversationView

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync`
- [ ] `cargo test -p external_websocket_sync`
- [ ] Run E2E Docker tests for zed-agent round
- [ ] Run E2E Docker tests for claude round

## Update Documentation

- [ ] Update `portingguide.md` with any new conflict patterns, renames, or checklist items
- [ ] Add new rebase checklist items if needed (currently items 1-40)

## Finalize

- [ ] Push feature branch to `helixml/zed`
- [ ] Open PR against `helixml/zed` main with upstream change summary and conflict resolution notes
- [ ] After PR merges: update `sandbox-versions.txt` in helix repo with new `ZED_COMMIT` SHA
- [ ] After PR merges: rebuild Zed binary (`./stack build-zed release`) and desktop image (`./stack build-ubuntu`)
