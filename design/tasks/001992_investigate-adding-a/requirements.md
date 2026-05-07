# Headless Zed: Requirements

## Goal

Make Zed run without any X11 / Wayland display when `ZED_HEADLESS=1` is set, while still serving:

1. The **external WebSocket sync** (`external_websocket_sync` crate) that Helix uses to drive Zed's agent threads.
2. **MCP context servers** and **agent servers** (Claude Code, Qwen, Gemini, NativeAgent).

The process must not call `cx.open_window(...)` for any user-facing window, must not require `DISPLAY` or `WAYLAND_DISPLAY`, and must not draw frames anywhere.

We reuse the existing `ZED_HEADLESS` env var rather than introducing a new CLI flag — the GPUI platform layer already gates on it (`crates/gpui/src/platform.rs:88`), so a single switch keeps the behavior consistent end-to-end.

## Background — what Zed already supports

The investigation phase of this task established the following:

- **Linux platform layer is already headless-capable.** `crates/gpui_linux/src/linux.rs` returns a `LinuxPlatform` wrapping `HeadlessClient` either when the `headless` argument is `true` *or* when `gpui::guess_compositor()` returns `"Headless"`. `guess_compositor()` returns `"Headless"` whenever `ZED_HEADLESS` is set in the environment, *or* when neither `WAYLAND_DISPLAY` nor `DISPLAY` is non-empty (`crates/gpui/src/platform.rs:88`).
- **A headless `Application` constructor exists.** `gpui_platform::headless()` returns `Application::with_platform(current_platform(true))` (`crates/gpui_platform/src/gpui_platform.rs:17`). The Zed binary, however, currently always calls `current_platform(false)` (`crates/zed/src/main.rs:104, 328`).
- **`HeadlessClient::open_window` returns an error** with the message *"neither DISPLAY nor WAYLAND_DISPLAY is set. You can run in headless mode"* (`crates/gpui_linux/src/linux/headless/client.rs:93`). So even today, running Zed under `ZED_HEADLESS=1` boots the platform but blows up the moment any code path tries to open a window.
- **`remote_server` / `HeadlessProject` already runs without UI**, but it is the SSH remote-host daemon (it serves an editor running elsewhere). It is *not* hooked up to `external_websocket_sync` and does not host the agent panel callbacks — so it isn't a drop-in solution.
- **The Helix integration is currently coupled to the agent panel UI.** `external_websocket_sync::setup_thread_handler` and `init_websocket_service` are both invoked from `initialize_agent_panel` in `crates/zed/src/zed.rs:826-855`, which only runs once a workspace window exists. The thread display, thread open, and UI state query callbacks (`init_thread_display_callback`, `init_ui_state_query_callback`, etc.) are registered from `agent_ui::AgentPanel`. Without a window, none of this fires.
- **`ConversationView::HeadlessConnection`** already exists (`crates/agent_ui/src/conversation_view.rs`) but it is a no-op `AgentConnection` for *WebSocket-created threads inside the panel* — not a way to run the panel-less.

## User stories

### US-1 — Operator runs Zed in a container with no display

> As an operator, I want to launch `ZED_HEADLESS=1 zed` on a Linux box that has no Wayland compositor and no X server, so Zed can serve as a long-lived agent worker for Helix without me having to install Xvfb or anything similar.

**Acceptance:**
- Running `ZED_HEADLESS=1 zed` on a host where `unset DISPLAY WAYLAND_DISPLAY` exits cleanly only after explicit termination (Ctrl-C or SIGTERM); it does not crash, does not print "neither DISPLAY nor WAYLAND_DISPLAY is set", and does not attempt to open a window.
- `pgrep -af zed` shows a single Zed process; it does not require Xvfb / Xwayland.
- No GPU device is opened (`/dev/dri/*` is not touched).

### US-2 — WebSocket sync continues to work headlessly

> As Helix, I want my existing WebSocket protocol with Zed to keep working byte-for-byte when Zed runs headlessly, so I do not have to fork the protocol or run a different client.

**Acceptance:**
- With `external_websocket_sync.enabled = true` and a reachable Helix endpoint, `ZED_HEADLESS=1 zed` connects to the WebSocket, replies to `chat_message`, emits `MessageAdded` / `MessageCompleted` / `Stopped`, and supports interrupts and follow-up messages (the same surface exercised by phases 1–10 of `crates/external_websocket_sync/e2e-test`).
- The E2E test passes against the headless binary in addition to the windowed binary.

### US-3 — MCP and agent servers continue to work headlessly

> As an operator, I want MCP context servers and ACP agents (Claude Code, Qwen, Gemini, NativeAgent) to work in headless mode the same as they do in windowed mode.

**Acceptance:**
- `mcp.enabled = true` with at least one MCP server (e.g. filesystem) loads tools, and `wait_for_tools_ready` resolves in the same way it does in windowed mode.
- A `chat_message` that exercises `claude` and `zed-agent` agents both produce streaming responses end-to-end.

### US-4 — `query_ui_state` either works or is replaced by a defined no-op

> As Helix, I want either a real answer to `query_ui_state` or a documented, deterministic response when Zed has no UI.

**Acceptance:** investigated and decided in `design.md`. The chosen behavior is documented and tested.

## Non-goals

- Building a separate `zed-headless` binary. We branch inside the existing `zed` binary based on `ZED_HEADLESS`; sharing the binary keeps the Helix fork's diff against upstream small.
- Adding a `--headless` CLI flag. The env var is sufficient and avoids two parallel switches.
- Headless support on macOS or Windows in this iteration (Linux + FreeBSD only — that's where the Helix containers run).
- Removing or weakening any of the rebase-fragile fixes in `portingguide.md`. The headless mode must reuse the *same* `thread_service.rs` + `websocket_sync.rs` paths, not a parallel implementation.
- Headless rendering / off-screen screenshotting (the `current_headless_renderer` story in `gpui_platform`). Out of scope.
- Multi-window / multi-workspace support in headless mode. One process, no windows.

## Open questions for design

1. **Workspace-less wiring of the agent.** `setup_thread_handler` requires `Entity<Project>`, `Entity<ThreadStore>`, and `Arc<dyn Fs>`. In windowed mode all three come out of the workspace. In headless mode we need to construct them without ever creating a `Workspace`. Where should that live — in `crates/zed/src/main.rs`, in a new `crates/zed/src/headless.rs` module, or in `external_websocket_sync` itself?
2. **Whether to bind a bare `Project`** (`Project::local(...)` with the cwd) or a `HeadlessProject` (the SSH-server flavour). The latter is meant for RPC, the former is what every workspace already uses.
3. **`query_ui_state` semantics in headless mode.** Return `null` for `active_view` / `thread_id`, an empty `mcp_servers` map, etc.? Or always report the most-recently-active thread, since headless mode has no notion of "active view"? Decided in design.
