# Requirements: Merge Latest Zed Upstream into Fork

## Context

The Helix fork of Zed (`/home/retro/work/zed/`) adds a WebSocket-based sync layer between Zed and the Helix API server. The fork applies all changes behind `#[cfg(feature = "external_websocket_sync")]` feature gates to minimize merge conflicts.

The last major upstream merge was documented in `/home/retro/work/helix/design/2025-12-09-zed-upstream-merge.md` (550 commits, December 9, 2025). Since then, additional Helix-specific commits have been landed on `main`.

## User Stories

**As a developer**, I want the Zed fork to be current with upstream Zed so that bug fixes, performance improvements, and new features from the Zed community are available in the Helix product.

**As a developer**, I want merge conflicts resolved correctly so that none of the four critical fork-specific fixes are accidentally dropped.

**As a developer maintaining the fork long-term**, I want the porting guide updated after this merge so that the next merge is as straightforward as this one.

## Acceptance Criteria

1. The fork's `main` branch incorporates all commits from the Zed upstream `main` (or a designated release tag) with no outstanding merge conflicts.
2. All four critical fixes are verified present after the merge (see `portingguide.md` — Critical Fixes section).
3. `cargo check --package zed --features external_websocket_sync` passes with no errors.
4. `cargo test -p external_websocket_sync` passes — all unit and protocol integration tests.
5. The Docker-based E2E test (`crates/external_websocket_sync/e2e-test/`) passes all phases, including the newer Phase 5 (Stopped/mid-stream interrupt) added in commit `8b033a4`.
6. The porting guide (`portingguide.md`) is updated with:
   - Any new upstream files that now conflict or require attention in future merges.
   - Any new critical fixes discovered during this merge.
   - An updated commit history table.
   - Updated E2E test phase descriptions if phases changed.
7. No regressions in fork-specific functionality: WebSocket sync, session persistence, title bar status indicator, enterprise TLS bypass.

## Out of Scope

- New feature development beyond conflict resolution.
- Removing or deprecating any fork-specific functionality.
- Updating the Helix platform Go backend (`/home/retro/work/helix/`).
