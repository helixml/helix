# Requirements: Merge Latest Zed Upstream into Helix Fork

## Context

The Helix fork (`helixml/zed`) was last merged with upstream (`zed-industries/zed`) on April 16, 2026 (task 001723). Since then, upstream has accumulated ~5 days of new commits (Apr 16-19). The fork has also gained 5 new Helix-specific commits (PRs #37-#41, including ACP auto-approve and request_id desync fixes).

Previous merge specs: 001723, 001617, 001560, 001554.

## User Stories

1. **As a platform engineer**, I need the fork updated to upstream `main` so Helix benefits from the latest Zed bug fixes (tsgo LSP fix, remote project sidebar fix) and the feature flag overrides system.

2. **As a Helix user**, I need zero regressions in WebSocket sync — all 9 critical fixes documented in `portingguide.md` must be preserved, and E2E tests must pass.

3. **As a future merge engineer**, I need the porting guide updated with any new conflict patterns, renames, or rebase checklist items discovered during this merge.

## Acceptance Criteria

1. Fork `main` branch includes all upstream commits through latest `zed-industries/zed` main
2. All 9 critical fixes from `portingguide.md` verified present (grep-checked)
3. All 40 rebase checklist items in `portingguide.md` pass
4. `cargo check --package zed --features external_websocket_sync` succeeds
5. `cargo test -p external_websocket_sync` passes
6. E2E Docker tests pass for both `zed-agent` and `claude` agent rounds
7. `portingguide.md` updated with any new findings from this merge
8. `sandbox-versions.txt` in helix repo updated with new `ZED_COMMIT` SHA
9. PR opened against `helixml/zed` with clear description of upstream changes and conflict resolutions

## Scope - Excluded

- No new Helix features on top of the merge
- No cherry-picking individual upstream commits
- No changes to Helix Go API server or controlplane
