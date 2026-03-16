# Implementation Tasks

## Setup

- [ ] Add `upstream` remote: `git remote add upstream https://github.com/zed-industries/zed`
- [ ] Fetch upstream: `git fetch upstream`
- [ ] Note the upstream HEAD commit hash (document in portingguide.md update later)

## Merge

- [ ] On `main` branch, run: `git merge upstream/main`
- [ ] If conflicts arise, resolve each conflicted file — upstream wins except for `#[cfg(feature = "external_websocket_sync")]` blocks which must be preserved

## Conflict Verification Checklist (run after resolving each file)

- [ ] `crates/agent/src/agent.rs`: Confirm `let agent = self.0.clone()` is present in `load_session()` (Critical Fix #1)
- [ ] `crates/agent_ui/src/acp/thread_view.rs`: Confirm no `MessageAdded`/`MessageCompleted` WebSocket sends in event handlers (Critical Fix #2); confirm `HeadlessConnection`, `from_existing_thread()`, `UserCreatedThread`, `ThreadTitleChanged`, THREAD_REGISTRY registration, history refresh on `Stopped`/`TitleUpdated` are all present
- [ ] `crates/acp_thread/src/acp_thread.rs`: Confirm `content_only()` method on `AssistantMessage` exists (Critical Fix #3)
- [ ] `crates/external_websocket_sync/src/thread_service.rs`: Confirm `notify_thread_display()` is called before follow-up to non-visible thread (Critical Fix #4)
- [ ] `crates/agent_ui/src/agent_panel.rs`: Confirm all four cfg-gated callback blocks are present (thread creation, thread display, thread open, UI state query), onboarding dismissal, and `acp_history_store()`
- [ ] `crates/agent_ui/src/acp/thread_view.rs`: Confirm `from_existing_thread()` struct fields match current `ConnectedServerState` definition
- [ ] `crates/feature_flags/src/flags.rs`: Confirm `AcpBetaFeatureFlag::enabled_for_all()` returns `true`
- [ ] `crates/extensions_ui/src/extensions_ui.rs`: Confirm Claude/Codex/Gemini keyword and upsell removal preserved
- [ ] `crates/recent_projects/src/dev_container_suggest.rs`: Confirm `suggest_dev_container` early return preserved
- [ ] `crates/http_client_tls/src/http_client_tls.rs`: Confirm `NoCertVerifier` and `ZED_HTTP_INSECURE_TLS` support preserved
- [ ] `crates/reqwest_client/src/reqwest_client.rs`: Confirm insecure TLS support preserved
- [ ] `crates/title_bar/`: Confirm connection status indicator and `external_websocket_sync` dependency preserved
- [ ] `crates/agent_settings/src/agent_settings.rs`: Confirm `show_onboarding` and `auto_open_panel` fields preserved
- [ ] `Cargo.toml` (workspace): Confirm `crates/external_websocket_sync` member and `external_websocket_sync` workspace dep preserved
- [ ] `.dockerignore`: Confirm simplified fork version preserved (not upstream's version)

## Build & Test

- [ ] Run `cargo check --package zed --features external_websocket_sync` — must compile with no errors
- [ ] Run `cargo test -p external_websocket_sync` — all unit and protocol tests must pass
- [ ] Build Docker E2E image: `docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile .`
- [ ] Run Docker E2E test: `docker run --rm -e ANTHROPIC_API_KEY=... -e TEST_TIMEOUT=240 zed-ws-e2e` — all 8 phases must pass

## Update Porting Guide

- [ ] Add the upstream HEAD commit hash merged and date to `portingguide.md`
- [ ] Add the merge commit to the "Commit History" table in `portingguide.md`
- [ ] Add any newly conflicted upstream files to the "Modified Upstream Files" section
- [ ] Add any new critical fixes discovered during this merge
- [ ] Update the "Rebase Checklist" with any new items
- [ ] Verify the E2E test phase descriptions in `portingguide.md` match `run_e2e.sh` (currently 8 phases)

## Push

- [ ] Push merged `main` to origin: `git push origin main`
