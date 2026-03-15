# Implementation Tasks

## Setup

- [ ] Add `upstream` remote pointing to `https://github.com/zed-industries/zed` if not already present
- [ ] Fetch upstream: `git fetch upstream`
- [ ] Check how many upstream commits to merge: `git log HEAD..upstream/main --oneline | wc -l`
- [ ] Create branch: `git checkout -b upstream-merge-$(date +%Y-%m-%d)`

## Merge & Conflict Resolution

- [ ] Run `git merge upstream/main` and capture conflict list
- [ ] Resolve `Cargo.toml` (workspace) — preserve `external_websocket_sync` member and dependency
- [ ] Resolve `crates/zed/src/zed.rs` — preserve cfg-gated WebSocket sync init
- [ ] Resolve `crates/agent/src/agent.rs` — verify Critical Fix #1 (`load_session` entity lifetime) is present
- [ ] Resolve `crates/agent_ui/src/agent_panel.rs` — preserve all cfg-gated callback blocks
- [ ] Resolve `crates/agent_ui/src/acp/thread_view.rs` — preserve `HeadlessConnection`, `from_existing_thread()`, THREAD_REGISTRY, no duplicate WebSocket sends (Critical Fix #2)
- [ ] Resolve `crates/acp_thread/src/acp_thread.rs` — preserve `content_only()` method (Critical Fix #3)
- [ ] Resolve `crates/feature_flags/src/flags.rs` — `AcpBetaFeatureFlag::enabled_for_all()` returns `true`
- [ ] Resolve `crates/extensions_ui/src/extensions_ui.rs` — preserve agent keyword/upsell removal
- [ ] Resolve `crates/recent_projects/src/dev_container_suggest.rs` — preserve `suggest_dev_container` early return
- [ ] Resolve `crates/http_client_tls/src/http_client_tls.rs` — preserve `NoCertVerifier`
- [ ] Resolve `crates/reqwest_client/src/reqwest_client.rs` — preserve insecure TLS support
- [ ] Resolve `crates/agent_settings/src/agent_settings.rs` — preserve `show_onboarding`, `auto_open_panel`
- [ ] Resolve `crates/title_bar/` — preserve connection status indicator
- [ ] Verify `from_existing_thread()` still matches `ConnectedServerState` struct fields after upstream changes
- [ ] Verify no duplicate WebSocket event sends were re-introduced in `thread_view.rs`

## Build Verification

- [ ] `cargo check --package zed --features external_websocket_sync` — must compile clean
- [ ] `cargo test -p external_websocket_sync` — unit tests pass

## Documentation

- [ ] Update `portingguide.md` — add any new upstream files that conflicted with Helix changes
- [ ] Update `portingguide.md` — document any new struct/trait/API changes encountered
- [ ] Update `portingguide.md` — add new entries to the Rebase Checklist if new files needed patching
- [ ] Update `portingguide.md` — add merge commit to Commit History table

## Push & CI

- [ ] Push branch and open PR against `main`
- [ ] Confirm Drone CI `zed-e2e-test` step passes (all 7 E2E phases)
- [ ] Merge PR to `main`

## Post-Merge

- [ ] Update `sandbox-versions.txt` in the helix repo: set `ZED_COMMIT` to the new HEAD commit hash
- [ ] Push helix repo change so CI picks up the new Zed build
