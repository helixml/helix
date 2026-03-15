# Implementation Tasks

## Setup

- [x] Add `upstream` remote pointing to `https://github.com/zed-industries/zed` if not already present
- [x] Fetch upstream: `git fetch upstream`
- [x] Check how many upstream commits to merge: `git log HEAD..upstream/main --oneline | wc -l`
- [x] Create branch: `git checkout -b upstream-merge-$(date +%Y-%m-%d)`

## Merge & Conflict Resolution

- [x] Run `git merge upstream/main` and capture conflict list (488 upstream commits, 8 conflict files)
- [x] Resolve `Cargo.toml` (workspace) — preserve `external_websocket_sync` member and dependency
- [x] Resolve `crates/zed/src/zed.rs` — auto-merged clean
- [x] Resolve `crates/agent/src/agent.rs` — Critical Fix #1 preserved (`let agent = self.0.clone()`)
- [x] Resolve `crates/agent_ui/src/agent_panel.rs` — all 13 cfg-gated blocks preserved
- [x] Resolve `crates/agent_ui/src/connection_view.rs` — NOTE: upstream renamed `acp/thread_view.rs` → `connection_view.rs`; HeadlessConnection, from_existing_thread, unregister_thread all preserved
- [x] Resolve `crates/acp_thread/src/acp_thread.rs` — auto-merged, content_only() intact (Critical Fix #3)
- [x] Resolve `crates/feature_flags/src/flags.rs` — auto-merged, enabled_for_all() = true intact
- [x] Resolve `crates/extensions_ui/src/extensions_ui.rs` — auto-merged
- [x] Resolve `crates/recent_projects/src/dev_container_suggest.rs` — auto-merged
- [x] Resolve `crates/http_client_tls/src/http_client_tls.rs` — auto-merged, NoCertVerifier intact
- [x] Resolve `crates/reqwest_client/src/reqwest_client.rs` — auto-merged
- [x] Resolve `crates/agent_settings/src/agent_settings.rs` — show_onboarding, auto_open_panel preserved
- [x] Resolve `crates/title_bar/` — connection status indicator and optional dep preserved
- [~] Verify `from_existing_thread()` still matches `ConnectedServerState` struct fields after upstream changes
- [~] Verify no duplicate WebSocket event sends were re-introduced in `connection_view.rs`

## Build Verification

- [~] `cargo check --package zed --features external_websocket_sync` — must compile clean
- [ ] `cargo test -p external_websocket_sync` — unit tests pass

## Documentation

- [ ] Push the porting guide update commit already on local `main` (commit `059342a545`) — create a PR or include in the upstream merge branch. The spec agent has already drafted all updates.
- [ ] After merge, update `portingguide.md` — add any new upstream files that conflict with Helix changes during the actual merge
- [ ] After merge, add the upstream merge commit to the Commit History table

## Push & CI

- [ ] Push branch and open PR against `main`
- [ ] Confirm Drone CI `zed-e2e-test` step passes (all 7 E2E phases)
- [ ] Merge PR to `main`

## Post-Merge

- [ ] Update `sandbox-versions.txt` in the helix repo: set `ZED_COMMIT` to the new HEAD commit hash
- [ ] Push helix repo change so CI picks up the new Zed build
