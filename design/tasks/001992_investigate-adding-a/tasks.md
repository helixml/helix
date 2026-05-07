# Implementation Tasks

## Investigation summary (already done)

- [x] Confirm Linux platform layer already supports headless via `HeadlessClient` + `ZED_HEADLESS` env var (`crates/gpui_linux/src/linux/headless/client.rs`, `crates/gpui/src/platform.rs:88`)
- [x] Confirm `gpui_platform::headless()` constructor exists but is unused by the Zed binary
- [x] Confirm `external_websocket_sync` is wired through `initialize_agent_panel` and therefore needs a workspace today (`crates/zed/src/zed.rs:826-855`)
- [x] Confirm MCP / agent-server stores work without UI (used by `HeadlessProject` already)

## Implementation

- [ ] Add `--headless` flag to `Args` in `crates/zed/src/main.rs` (visible in `--help`, documents that it implies multi-instance)
- [ ] Compute `let headless = args.headless || std::env::var_os("ZED_HEADLESS").is_some();` once, before `Application::with_platform(...)`
- [ ] Pass `headless` into `gpui_platform::current_platform(headless)` (replaces both hard-coded `false` calls in `crates/zed/src/main.rs`)
- [ ] Force `failed_single_instance_check = false` when `headless` (skip the per-OS single-instance gate)
- [ ] Create new module `crates/zed/src/headless.rs`, declared as `mod headless;` in `main.rs`, gated `#[cfg(feature = "external_websocket_sync")]`
- [ ] In `headless::run`, build `Project::local` rooted at `$PWD` using fields from `AppState`
- [ ] In `headless::run`, build `ThreadStore` using the same `prompt_builder` the agent panel uses
- [ ] In `headless::run`, call `external_websocket_sync::setup_thread_handler(project, thread_store, fs, cx)`
- [ ] In `headless::run`, read `ExternalSyncSettings` and call `external_websocket_sync::init_websocket_service(...)` if enabled (mirrors `crates/zed/src/zed.rs:843-853`)
- [ ] Install a headless `query_ui_state` responder (registers via `external_websocket_sync::init_ui_state_query_callback`) returning the fixed shape from design D-3
- [ ] Install a headless `notify_thread_display` responder via `init_thread_display_callback` that just logs
- [ ] In `main()`, branch after global init: `if headless { headless::run(app_state, cx)?; } else { /* existing restore_or_create_workspace path */ }`
- [ ] Skip the `cx.set_menus`, `cx.activate(true)`, `component_preview::init`, and `cx.observe_global::<SettingsStore>` window-walking block when `headless`
- [ ] If `--headless` is set but the `external_websocket_sync` feature is off, exit with the documented error message and non-zero status

## Verification

- [ ] `cargo build --features external_websocket_sync -p zed` succeeds
- [ ] `cargo clippy --features external_websocket_sync -p zed` is clean (use `./script/clippy`, per `CLAUDE.md`)
- [ ] On a Linux box with `unset DISPLAY WAYLAND_DISPLAY`, `zed --headless` boots without printing the *"neither DISPLAY nor WAYLAND_DISPLAY is set"* error and stays running
- [ ] `pgrep -af zed` shows a single process; no Xvfb / Xwayland needed
- [ ] `ls /proc/$(pgrep zed)/fd | xargs -I{} readlink /proc/$(pgrep zed)/fd/{} | grep -E '/dev/dri|/tmp/.X11'` returns nothing (no display devices touched)

## E2E + tests

- [ ] Add a `--headless` variant of the Docker E2E test (`crates/external_websocket_sync/e2e-test`); drop Xvfb from the Dockerfile in that variant
- [ ] All 10 existing E2E phases pass in the headless variant (zed-agent and claude agents)
- [ ] Add a unit test in `external_websocket_sync` that calls `setup_thread_handler` against a test `Project` and drives a single `chat_message` through `mock_helix_server` — proves the wiring works without a workspace

## Documentation

- [ ] Update `crates/external_websocket_sync/README.md` with a "Headless mode" section pointing at `--headless`
- [ ] Add a `Modified upstream files` entry in `portingguide.md` for `crates/zed/src/main.rs` (the new flag + branch) and `crates/zed/src/headless.rs` (new file)
