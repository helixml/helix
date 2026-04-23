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

- [x] Merge `upstream/main` directly into feature branch (973 commits, fresh merge since task 001554)
- [x] Resolve all 35 merge conflicts, preserving Helix-specific patches
  - 24 files: accept upstream (no Helix changes)
  - 11 files: manual merge (both sides have meaningful changes)
  - All resolutions documented in `merge_resolution_log.md`

## Verify Critical Fixes

- [x] Grep-verify all 9 critical fixes from portingguide.md are present
- [x] Check `from_existing_thread()` constructor signature consistency
- [x] Check ConnectedServerState fields (uses `root_session_id` per upstream rename)

## Post-Merge Fixups

- [x] Update agent_panel.rs old-name references: ActiveView→BaseView, active_view→base_view, set_active_view→set_base_view, selected_agent_type→selected_agent
- [x] Remove `ActiveView::History` match arm (variant removed in upstream's BaseView enum)

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync` — BLOCKED: no Rust toolchain in environment
- [ ] `cargo test -p external_websocket_sync` — BLOCKED: no Rust toolchain in environment
- [ ] Run E2E Docker tests (if ANTHROPIC_API_KEY available)

## Finalize

- [ ] Push feature branch to `helixml/zed`
- [ ] Create PR with resolution details
- [ ] After PR merges: update `sandbox-versions.txt` in helix repo with new `ZED_COMMIT` SHA
