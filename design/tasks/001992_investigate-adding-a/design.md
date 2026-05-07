# Headless Zed: Design

## TL;DR — what already exists vs. what is missing

| Area | Status |
| --- | --- |
| Linux GPUI platform that boots without a display | ✅ Done. `HeadlessClient` (`crates/gpui_linux/src/linux/headless/client.rs`) + `gpui::guess_compositor()` falls back to `"Headless"` when `ZED_HEADLESS=1` or no display vars are set. |
| `Application` constructor that uses the headless platform | ✅ Done. `gpui_platform::headless()` returns one. |
| Zed binary actually opting into the headless platform | ❌ Missing. `crates/zed/src/main.rs` always passes `false` to `current_platform(...)`. |
| Skipping window creation when headless | ❌ Missing. `restore_or_create_workspace` runs unconditionally and tries to open a window. The `HeadlessClient::open_window` then fails with the *"neither DISPLAY nor WAYLAND_DISPLAY is set"* message. |
| `external_websocket_sync` wiring | ❌ Tightly coupled to the agent panel. `setup_thread_handler` and `init_websocket_service` are called from `initialize_agent_panel` (`crates/zed/src/zed.rs:826-855`), which only runs once a workspace window exists. |
| `Project` / `ThreadStore` / `Fs` available without a workspace | ⚠️ Need to construct manually. All of these are independently constructible (`Project::local`, `ThreadStore::new`, `RealFs::new`); none of them require a window. |
| MCP `ContextServerStore` and `AgentServerStore` | ✅ Already work in `HeadlessProject` (`crates/remote_server/src/headless_project.rs:233-252`). They start whenever a `Project` exists; UI is irrelevant. |

So the answer to "does Zed already support headless?" is **partially**: the GPUI/platform plumbing is there, but the *Helix-specific* sync wiring is hard-bound to the agent-panel UI and there is no top-level switch in `main()` to skip workspace creation.

## High-level architecture

We reuse the existing `ZED_HEADLESS` env var. When set, `main()` takes a different fork *after* all global initialization (settings, telemetry, language registry, agent registry, etc.) but *before* `restore_or_create_workspace`:

```
main()
  ├── let headless = std::env::var_os("ZED_HEADLESS").is_some();
  ├── Application::with_platform(gpui_platform::current_platform(headless))
  ├── …all current global init… (settings, client, language registry, agents, etc.)
  └── if headless:
        run_headless(app_state, cx)        ← NEW
      else:
        restore_or_create_workspace(...)   ← existing
```

`run_headless` constructs the things that the agent panel would normally hand to `setup_thread_handler`, then calls `setup_thread_handler` and `init_websocket_service` with no workspace involved.

### `run_headless` — pseudocode

```rust
fn run_headless(app_state: Arc<AppState>, cx: &mut App) -> anyhow::Result<()> {
    use external_websocket_sync::{ExternalSyncSettings, WebSocketSyncConfig};

    // 1. A bare local project rooted at $PWD (or a path provided via --headless-cwd).
    let cwd = std::env::current_dir().context("getting cwd")?;
    let project = Project::local(
        app_state.client.clone(),
        app_state.node_runtime.clone(),
        app_state.user_store.clone(),
        app_state.languages.clone(),
        app_state.fs.clone(),
        None, // env
        cx,
    );

    // 2. ThreadStore — the same store the agent panel constructs.
    let thread_store = cx.new(|cx| ThreadStore::new(
        project.clone(),
        app_state.fs.clone(),
        prompt_builder.clone(),
        cx,
    ));

    // 3. Wire WebSocket → callbacks → ThreadStore. Same call as agent_panel.
    external_websocket_sync::setup_thread_handler(
        project,
        thread_store,
        app_state.fs.clone(),
        cx,
    );

    // 4. Start the WebSocket client.
    let settings = ExternalSyncSettings::get_global(cx);
    if settings.enabled && settings.websocket_sync.enabled {
        external_websocket_sync::init_websocket_service(WebSocketSyncConfig {
            enabled: true,
            url: settings.websocket_sync.external_url.clone(),
            auth_token: settings.websocket_sync.auth_token.clone().unwrap_or_default(),
            use_tls: settings.websocket_sync.use_tls,
            skip_tls_verify: settings.websocket_sync.skip_tls_verify,
        });
    }

    // 5. Headless `query_ui_state` responder — see decision below.
    install_headless_ui_state_responder(cx);

    // 6. Headless `thread_display` swallower (no panel to switch to).
    install_headless_thread_display_responder(cx);

    Ok(())
    // app.run()'s on_finish_launching has nothing else to do; the calloop event loop
    // owned by HeadlessClient keeps the process alive until quit().
}
```

After `on_finish_launching` returns, the `HeadlessClient::run()` calloop loop (`headless/client.rs:118`) blocks the main thread. The Tokio runtime started by `init_websocket_service` lives on background threads. Both stay up until SIGTERM.

## Key decisions

### D-1 — Reuse the existing `ZED_HEADLESS` env var

GPUI already gates platform selection on `ZED_HEADLESS` (`crates/gpui/src/platform.rs:88`). Adding a parallel `--headless` CLI flag would require operators to remember which one to use; reusing the env var keeps a single switch end-to-end. Resolution:

```rust
let headless = std::env::var_os("ZED_HEADLESS").is_some();
```

This is read once near the top of `main()` and then used to (1) pick the platform via `current_platform(headless)`, (2) skip the single-instance check, (3) branch into `run_headless` instead of `restore_or_create_workspace`, and (4) skip window/menu setup. Operators discover it via the headless mode docs added under `crates/external_websocket_sync/README.md`.

### D-2 — `Project::local` over `HeadlessProject`

`HeadlessProject` (`crates/remote_server/src/headless_project.rs`) is the SSH-server flavour: it expects an `AnyProtoClient` and proxies file/buffer ops over RPC. The Helix WebSocket sync isn't an RPC client of that kind — it's a thread driver. A regular `Project::local` rooted at `$PWD` gives us:

- A `ContextServerStore` automatically (`Project` constructs one in `Project::local`).
- An `AgentServerStore` automatically.
- A `WorktreeStore`, `BufferStore`, `LspStore` — all available to MCP tools / the agent's grep/read tools.
- The same `Project` shape that the agent panel expects, which means `setup_thread_handler` does not need a separate code path.

If we ever want to drop the worktree/LSP overhead, that's a future optimization; correctness first.

### D-3 — `query_ui_state` returns a fixed headless response

The `query_ui_state` callback in the panel reads `ActiveView::AgentThread { conversation_view }` (`portingguide.md` line 100) — there is no equivalent in headless mode. Decision: register a headless responder that always replies with:

```json
{
  "active_view": "headless",
  "thread_id": <most-recently-touched thread id, or null>,
  "entry_count": <entries in that thread, or 0>,
  "mcp_servers": <ContextServerStore::all_servers() snapshot>,
  "active_model": <LanguageModelRegistry default>
}
```

Keep the field shape stable so Helix doesn't need a code path per mode. The "most recently touched thread" comes from `THREAD_REGISTRY`.

### D-4 — `notify_thread_display` becomes a no-op (with a log line)

The thread-display callback exists to switch the agent panel to the right thread. With no panel, it is logged and dropped. The thread is still in `THREAD_REGISTRY` so follow-ups still target it correctly. This means the existing "split-brain detection" code path in the panel is simply not reachable headlessly.

### D-5 — Skip everything that needs a window

In `run_headless`, do **not** call:

- `restore_or_create_workspace` / `restore_multiworkspace`
- `cx.activate(true)` (no-op on `HeadlessClient` but also misleading)
- `cx.set_menus(...)` (no-op headlessly but pointless)
- `component_preview::init` (UI only)
- The `cx.observe_global::<SettingsStore>` block that walks `cx.windows()` to update appearances (windows is empty)

Telemetry events (`"App Opened"`) should still fire so we can count headless workers.

### D-6 — Single-instance check stays off in headless

`failed_single_instance_check` already short-circuits when `args.allow_multiple_instances` is set. Headless implies multi-instance allowed (operators want to scale workers); set the same effect: when `headless`, treat as `allow_multiple_instances = true`. Document this in the headless mode README.

### D-7 — Feature gating

`run_headless` lives behind `#[cfg(feature = "external_websocket_sync")]` because without the Helix sync the binary has nothing useful to do headlessly. If the feature is off and `ZED_HEADLESS` is set, the binary exits with a clear error: *"`ZED_HEADLESS` requires the `external_websocket_sync` feature; rebuild with `cargo build --features external_websocket_sync -p zed`"*.

### D-8 — File location

Put the new code in `crates/zed/src/headless.rs` (module declared in `main.rs`), not in `external_websocket_sync`. Reasons:

- It needs `AppState`, `Project::local`, `ThreadStore::new`, `LanguageRegistry`, all of which `external_websocket_sync` does not currently depend on. Pulling them in would invert the dependency graph.
- Keeps the rebase blast radius in one Helix-specific file we own, instead of touching upstream code.
- The branch in `main()` (`if headless { headless::run(...) } else { restore_or_create_workspace(...) }`) is a single conditional — easy to keep through rebases.

## Risks / things that will probably bite

- **`Project::local` may try to spawn LSP servers** for files in `$PWD`. If `$PWD` is `/` or contains a giant repo, this is a startup-time problem. Mitigation: read an optional `ZED_HEADLESS_CWD` env var to override; default to `$PWD`. Log the chosen cwd at startup.
- **Tokio runtime startup race with WebSocket auto-start** — already documented in `external_websocket_sync.rs:480-487`. The headless path constructs the runtime via `init_websocket_service` exactly the same way the windowed path does, so this is no worse.
- **Callback channels assume an `App`** for `cx.spawn`. `setup_thread_handler` already runs from `cx.spawn`, so it works as long as we call it from inside `app.run`'s `on_finish_launching` closure.
- **`crashes::init`** on Linux uses ashpd / desktop-notification proxies for the failure path. In headless containers the desktop portal isn't reachable. Reliability init should not regress headless boot — verify and, if necessary, gate the desktop-notification fallback behind `if !headless`.
- **`zlog::init_output_file`** uses `paths::log_file()`. In a stateless container, `$XDG_DATA_HOME` may not exist — already handled by `init_paths()` so this should be fine, but verify in a clean container.

## Out of scope (explicitly)

- Headless on macOS / Windows. The `MacPlatform::new(true)` and `WindowsPlatform::new(true)` paths exist but we're not validating them in this iteration.
- Off-screen rendering (`current_headless_renderer` in `gpui_platform`). Different feature, different goal.
- A dedicated `zed-headless` binary. Single binary, env-var-gated.
- A `--headless` CLI flag. The env var is sufficient.
- Migrating `setup_thread_handler` to a workspace-less location *upstream*. We keep its existing call site in `agent_panel`; in headless mode we additionally call it from `run_headless`. If both ever fire (they shouldn't, because there is no panel in headless mode), `init_*_callback` overwrites the previous registration — verify this is safe before shipping.

## How we test

- **Manual smoke test:** in a Linux container with no display, `unset DISPLAY WAYLAND_DISPLAY && ZED_HEADLESS=1 zed` runs and waits.
- **WebSocket E2E:** add a headless variant of the Docker E2E test (`crates/external_websocket_sync/e2e-test`). Re-run all 10 phases against the headless binary. The E2E test currently launches Zed under Xvfb in `Dockerfile.runtime`; for the headless variant we drop Xvfb entirely and launch Zed with `ZED_HEADLESS=1` in the environment.
- **Unit:** a small test in `external_websocket_sync` that calls `setup_thread_handler` against a `Project::test`-style entity, sends a `chat_message` through the mock Helix server, and asserts the response shape — exercises the wiring without booting the binary.
