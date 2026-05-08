# Merge Resolution Log

Every conflict resolution decision during the upstream merge, for review.

**Merge:** `upstream/main` (commit `62bd61a679`) into `feature/001864-merge-latest-zed` (based on `helixml/zed` main at `1e07aea47a`)

**Total conflicts:** 35 files

---

## Category 1: Accept Upstream (no Helix changes in these files)

These files have no Helix-specific modifications. Resolved by accepting upstream version (`git checkout --theirs`).

| # | File | Rationale |
|---|------|-----------|
| 1 | `.github/workflows/assign-reviewers.yml` | No Helix changes, CI not used |
| 2 | `.github/workflows/autofix_pr.yml` | No Helix changes, CI not used |
| 3 | `.github/workflows/bump_patch_version.yml` | No Helix changes, CI not used |
| 4 | `.github/workflows/cherry_pick.yml` | No Helix changes, CI not used |
| 5 | `.github/workflows/extension_auto_bump.yml` | No Helix changes, CI not used |
| 6 | `.github/workflows/extension_bump.yml` | No Helix changes, CI not used |
| 7 | `.github/workflows/extension_tests.yml` | No Helix changes, CI not used |
| 8 | `.github/workflows/extension_workflow_rollout.yml` | No Helix changes, CI not used |
| 9 | `.github/workflows/pr_labeler.yml` | No Helix changes, CI not used |
| 10 | `.github/workflows/release.yml` | No Helix changes, CI not used |
| 11 | `.github/workflows/run_tests.yml` | No Helix changes, CI not used |
| 12 | `.github/workflows/run_agent_evals.yml` | Deleted upstream, accepted deletion (`git rm`) |
| 13 | `assets/keymaps/default-linux.json` | No Helix changes |
| 14 | `assets/keymaps/default-macos.json` | No Helix changes |
| 15 | `assets/keymaps/default-windows.json` | No Helix changes |
| 16 | `assets/keymaps/vim.json` | No Helix changes |
| 17 | `assets/settings/default.json` | No Helix changes |
| 18 | `crates/editor/src/edit_prediction_tests.rs` | No Helix changes |
| 19 | `crates/editor/src/editor.rs` | No Helix changes |
| 20 | `crates/editor/src/tasks.rs` | No Helix changes |
| 21 | `crates/agent/src/tools/context_server_registry.rs` | No Helix changes |
| 22 | `crates/agent_servers/src/acp.rs` | No Helix changes |
| 23 | `crates/workspace/src/multi_workspace.rs` | No Helix changes |
| 24 | `crates/zed/src/main.rs` | No Helix changes |

## Category 2: Manual merge (both sides have meaningful changes)

### 25. `crates/feature_flags/src/flags.rs`

**Upstream change:** PR #54206 revamped feature flag system — added `type Value = PresenceFlag;` associated type to every `FeatureFlag` impl.

**Helix change:** `AcpBetaFeatureFlag::enabled_for_all()` returns `true` (enables ACP beta features unconditionally in Helix builds).

**Resolution:** Accepted upstream's new `type Value` lines everywhere, preserved Helix's `enabled_for_all() -> true` override on `AcpBetaFeatureFlag`.

**Risk:** Low — changes are orthogonal (new associated type vs. method override).

### 26. `crates/recent_projects/src/dev_container_suggest.rs`

**Upstream change:** Added `let cli_auto_open = workspace.open_in_dev_container();` line.

**Helix change:** Early return at top of `suggest_on_worktree_updated` checking `RemoteSettings::get_global(cx).suggest_dev_container`.

**Resolution:** Kept Helix early return first, then upstream's `cli_auto_open` line after it.

**Risk:** Low — Helix's early return exits before the new upstream code runs (correct behavior: if setting disabled, skip entirely).

### 27. `Cargo.lock`

**Upstream change:** Extensive dependency updates across hundreds of crates.

**Helix change:** Added `external_websocket_sync` and related dependencies.

**Resolution:** Accepted upstream's Cargo.lock entirely (`git checkout --theirs`). The Helix-specific dependencies will be re-added by Cargo when it regenerates the lockfile during the next `cargo build --features external_websocket_sync`.

**Risk:** Low — Cargo.lock is always regenerated from Cargo.toml dependency declarations.

### 28. `crates/acp_thread/src/acp_thread.rs`

**Upstream change:** Added token usage tracking: `token_usage` field, `TokenUsageUpdated` event, `update_token_usage()` method, and token tracking in the response handler wrapped with `cx.has_flag::<AcpBetaFeatureFlag>()`.

**Helix change:** Critical Fix #9 — `stopped_emitted_for_task` guard to prevent duplicate `AcpThreadEvent::Stopped` emission. Uses `AtomicBool` checked before emitting Stopped on both cancellation and normal completion paths.

**Resolution:** Combined both changes. Upstream's token usage block (`if cx.has_flag::<AcpBetaFeatureFlag>() && let Some(response_usage) = &r.usage { ... }`) is placed first, followed by Helix's stopped-emission guard (`if !stopped_emitted_for_task.load(Ordering::Acquire) { cx.emit(AcpThreadEvent::Stopped(r.stop_reason)); }`).

**Risk:** Medium — Both changes touch the response handler. The ordering matters: token usage must be recorded before the Stopped event fires, since Stopped triggers message_completed on the server. Verified that the guard correctly wraps only the Stopped emission, not the token tracking.

### 29. `crates/agent_ui/Cargo.toml`

**Upstream change:** Added `audio = ["dep:audio"]` feature and `audio` dependency.

**Helix change:** `external_websocket_sync = ["external_websocket_sync_dep"]` feature and `time_format.workspace = true`, `tokio.workspace = true` dependencies.

**Resolution:** Kept both feature definitions and both dependency sets. Helix features and upstream features are orthogonal.

**Risk:** Low — additive changes on both sides.

### 30. `crates/agent_ui/src/agent_panel.rs`

**Upstream change:** Major refactor — renamed `ActiveView` → `BaseView`, `active_view` → `base_view`, `background_threads` → `retained_threads`, `should_render_onboarding` → `should_render_new_user_onboarding`, removed `ActiveView::History` variant, added `OverlayView` enum, `AgentThread` struct, `thread_metadata_to_debug_json()` function.

**Helix change:** Cfg-gated WebSocket integration code in `new()`: thread display callback (~100 lines handling incoming thread notifications, entity comparison to avoid brain-split, auto-follow activation) and UI state query callback (~70 lines responding to E2E test queries about active view state, MCP server status, model info). Also: `should_render_new_user_onboarding` early return for Helix builds, cfg-gated imports for `external_websocket_sync` and `tokio::sync::mpsc`.

**Resolution (4 conflict regions):**
1. **Imports:** Kept Helix's cfg-gated `external_websocket_sync` and `mpsc` imports. Removed dead imports (`ThreadHistory`, `ThreadHistoryView`, etc.) that upstream deleted.
2. **Enum/struct definitions:** Accepted upstream's `BaseView` enum (replacing `ActiveView`), `AgentThread` struct, and `thread_metadata_to_debug_json()`.
3. **Struct fields:** Accepted upstream's `base_view`, `overlay_view`, `draft_thread`, `retained_threads` field names.
4. **Onboarding:** Combined upstream's renamed `should_render_new_user_onboarding` method with Helix's `if cfg!(feature = "external_websocket_sync") { return false; }` early return.

**Post-merge fixup:** Updated ~8 references in Helix's cfg-gated code blocks to use new names: `ActiveView` → `BaseView`, `this.active_view` → `this.base_view`, `set_active_view` → `set_base_view`, `selected_agent_type` → `selected_agent`. Removed `ActiveView::History` match arm (variant no longer exists; `BaseView` only has `Uninitialized` and `AgentThread`).

**Risk:** High — most complex file. The Helix WebSocket code references struct fields and enum variants extensively. All references have been updated to match upstream's new names. Needs compilation verification.

### 31. `crates/agent_ui/src/conversation_view.rs`

**Upstream change:** Renamed `id` → `root_session_id` in thread creation flow. Changed `ConnectedServerState` to use `root_session_id` field. Restructured thread initialization.

**Helix change:** `from_existing_thread()` constructor for wrapping existing `Entity<AcpThread>` without creating a new ACP connection. THREAD_REGISTRY registration block in thread creation. `UserCreatedThread` handling with `.clone()` on `current` for HashMap insertion.

**Resolution (2 conflict regions):**
1. **THREAD_REGISTRY block:** Kept Helix's cfg-gated THREAD_REGISTRY registration but updated `session_id` extraction from `current.read(cx).thread.read(cx).session_id()` to `root_session_id.to_string()` to match upstream's variable rename. Added upstream's `this.root_session_id = Some(root_session_id.clone());` line after the Helix block.
2. **ConnectedServerState:** Used upstream's `root_session_id` in the state struct, kept `.clone()` on `current` for Helix's `UserCreatedThread` code that needs it in the HashMap.

**Risk:** Medium — the variable rename (`id` → `root_session_id`) required updating Helix code that referenced the old variable name. Verified that `from_existing_thread()` signature and THREAD_REGISTRY registration are intact.

### 32. `crates/title_bar/Cargo.toml`

**Upstream change:** Removed `stories` feature. Updated dependency list.

**Helix change:** `external_websocket_sync = ["dep:external_websocket_sync"]` feature and corresponding optional dependency.

**Resolution:** Kept Helix's `external_websocket_sync` feature. Accepted upstream's removal of `stories` feature. Both sets of dependency changes kept.

**Risk:** Low — orthogonal changes.

### 33. `crates/title_bar/src/title_bar.rs`

**Upstream change:** Removed `OnboardingBanner` entirely (component and related code). Updated various rendering methods.

**Helix change:** `render_helix_connection_status` methods (two cfg-gated variants: one for `external_websocket_sync` feature showing WebSocket status, one stub returning `Empty` when feature is off). `render_organization_menu_button` method. Previously had `.visible_when(|_cx| false)` on `OnboardingBanner` to hide it in Helix builds.

**Resolution:** Accepted upstream's `OnboardingBanner` removal — Helix's `.visible_when(false)` override is no longer needed since the component no longer exists. Kept Helix's `render_helix_connection_status` and `render_organization_menu_button` methods (these are additive, not conflicting).

**Risk:** Low — the OnboardingBanner removal actually simplifies Helix's code (one less override needed).

### 34. `crates/workspace/src/workspace.rs`

**Upstream change:** Expanded re-export list from `multi_workspace` module. Various refactors.

**Helix change:** Agent focus guard (`if !matches!(leader_id, CollaboratorId::Agent)`) in the follow logic to prevent recursive focus stealing.

**Resolution:** Accepted upstream's expanded re-export list. The agent focus guard survived auto-merge (no conflict on that specific code region).

**Risk:** Low — the guard is in an area upstream didn't modify. Verified present at the expected location.

### 35. `crates/zed/Cargo.toml`

**Upstream change:** Added `track-project-leak` feature and `input_latency_ui` dependency.

**Helix change:** `external_websocket_sync` feature definition (propagates to sub-crates) and `external_websocket_sync` optional dependency.

**Resolution:** Kept both Helix's feature/dependency definitions AND upstream's new feature/dependency. Changes are in different sections of the file.

**Risk:** Low — additive changes on both sides, no overlap.

---

## Verification Summary

| Check | Status |
|-------|--------|
| All 9 critical fixes present | PASS |
| No conflict markers remaining | PASS |
| Feature gate propagation (zed → agent_ui → title_bar) | PASS |
| `from_existing_thread()` intact | PASS |
| THREAD_REGISTRY registration intact | PASS |
| `ConnectedServerState` uses `root_session_id` | PASS |
| `stopped_emitted_for_task` guard intact | PASS |
| `AcpBetaFeatureFlag::enabled_for_all() -> true` | PASS |
| Agent focus guard in workspace.rs | PASS |
| Onboarding bypass in agent_panel.rs | PASS |
| Dev container suggest early return | PASS |
| Cargo check | BLOCKED (no Rust toolchain in environment) |
