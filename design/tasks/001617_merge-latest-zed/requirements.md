# Requirements: Merge Latest Zed Upstream into Fork

## Context

`zed-4` is a Helix fork of the [Zed editor](https://github.com/zed-industries/zed). The fork adds a WebSocket-based sync layer (`crates/external_websocket_sync/`) that connects Zed's agent panel to the Helix API server. All fork-specific changes are documented in `portingguide.md` and guarded by `#[cfg(feature = "external_websocket_sync")]` feature gates.

The fork needs to be brought up to date with the latest upstream Zed commits while preserving all customizations and passing all tests.

## User Stories

**As a developer**, I want the fork to track upstream Zed so that we benefit from upstream bug fixes, performance improvements, and new features without accumulating merge debt.

**As a developer**, I want the porting guide updated during the merge so that future merges start with accurate, current documentation rather than stale notes.

**As a QA engineer**, I want all tests — including the 10-phase external WebSocket sync E2E suite — to pass after the merge so that no regressions are introduced.

## Acceptance Criteria

1. The fork's `main` branch contains all commits from the latest upstream Zed `main` (or stable tag), with no unresolved merge conflicts.
2. All four critical fixes documented in `portingguide.md` are preserved post-merge:
   - `load_session()` entity lifetime fix in `crates/agent/src/agent.rs`
   - No duplicate WebSocket event sends from `thread_view.rs`
   - `content_only()` on `AssistantMessage` in `crates/acp_thread/src/acp_thread.rs`
   - Follow-up to non-visible thread calls `notify_thread_display()`
3. `cargo check --package zed --features external_websocket_sync` passes with no errors.
4. `cargo test -p external_websocket_sync` passes (all unit and protocol-level integration tests).
5. All 10 phases of the Docker-based E2E test pass (`crates/external_websocket_sync/e2e-test/run_e2e.sh`).
6. `portingguide.md` is updated to reflect:
   - Any new upstream files that required conflict resolution
   - Any upstream structural changes to monitored files (e.g., `ConnectedServerState` field changes)
   - The actual phase count (currently 10, portingguide.md still says 4)
   - Any new patterns or decisions made during this merge
7. No new features or behaviors are introduced beyond what is required for compatibility.
