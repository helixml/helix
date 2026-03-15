# Requirements: Merge Latest Zed Upstream

## Context

The Helix Zed fork was last synced with upstream Zed on 2026-02-26 (merge commit `ddc071d503`). The fork adds a WebSocket-based bidirectional sync layer (`external_websocket_sync` crate) behind `#[cfg(feature = "external_websocket_sync")]` feature gates. Upstream Zed has continued development since then, and this task merges those changes in.

## User Stories

**As a developer**, I want the Zed fork to incorporate the latest upstream Zed improvements (bug fixes, performance, new features) while keeping all Helix-specific changes intact.

**As a Helix operator**, I want the E2E WebSocket sync tests to pass after the merge so I'm confident the Helix↔Zed integration still works correctly.

**As a future maintainer**, I want the porting guide (`portingguide.md`) updated with anything new discovered during this merge — new files modified, new API changes, new pitfalls — so the next merge is easier.

## Acceptance Criteria

1. `git merge upstream/main` (or equivalent) applied to the fork with all conflicts resolved
2. All Helix-specific changes preserved (per rebase checklist in `portingguide.md`)
3. `cargo check --package zed --features external_websocket_sync` passes with no errors
4. `cargo test -p external_websocket_sync` unit tests pass
5. E2E Docker test (`Dockerfile.ci`) passes all 7 phases (or current phase count)
6. `portingguide.md` updated to reflect any new files modified, API changes, or new critical fixes discovered
7. `sandbox-versions.txt` in the helix repo updated to the new `ZED_COMMIT` hash
8. PR opened against the zed repo `main` branch and CI (Drone `zed-e2e-test` step) passes
