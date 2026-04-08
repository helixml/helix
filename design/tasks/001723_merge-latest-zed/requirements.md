# Requirements: Merge Latest Zed Upstream into Helix Fork

## Context

The Helix fork of Zed (`helixml/zed`) is currently **506 commits behind** upstream `zed-industries/zed` (upstream HEAD: `7597666c08`, fork HEAD: `42e50d49ba`). The fork has **165 Helix-specific commits** ahead of upstream. The last merge was task 001617 (March 2026).

## User Stories

**As a Helix platform engineer**, I want the Zed fork updated to the latest upstream so that Helix benefits from bug fixes, performance improvements (e.g., reduced GPU spinner usage, scroll snapping fixes), and new capabilities (parallel agents, worktree archival, subagent permissions).

**As a Helix user**, I want zero regressions in the WebSocket sync between Zed and Helix API, so my coding sessions continue to work reliably.

**As a future merge engineer**, I want the porting guide updated with all new conflicts and divergence points discovered during this merge, so the next merge is faster and less error-prone.

## Acceptance Criteria

1. **Fork fully up to date**: All 506 upstream commits merged into the Helix fork's `main` branch
2. **All 8 critical fixes preserved**: Each fix from `portingguide.md` verified present after merge (entity lifetime, no duplicate sends, content_only, notify_thread_display, stale entry flush, Stopped invariant, entity unregistration, cancel drop)
3. **All 30+ rebase checklist items verified**: Every item in the porting guide's Rebase Checklist confirmed
4. **Compilation**: `cargo check --package zed --features external_websocket_sync` passes
5. **Unit tests pass**: `cargo test -p external_websocket_sync` — all green
6. **AcpThread invariant test passes**: `cargo test -p acp_thread test_second_send`
7. **E2E tests pass**: All 10 phases of the Docker E2E test pass for both `zed-agent` and `claude` agents
8. **Porting guide updated**: `portingguide.md` reflects all new conflict resolutions, file renames, API signature changes, and new upstream patterns encountered during this merge
9. **Sandbox version pinned**: `ZED_COMMIT` in helix repo's `sandbox-versions.txt` updated to the new fork HEAD SHA
10. **PR summary**: Clear description of notable upstream changes (breaking changes, new features, deprecations)

## Scope — Excluded

- No new Helix features (maintenance sync only)
- No cherry-picking of unrelated upstream experimental work
- No changes to the Helix Go API server (`/home/retro/work/helix/`)
