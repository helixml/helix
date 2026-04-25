# Requirements: Fix thread detachment when opening currently-open thread via new threads UI

## Context

The two most recent upstream Zed merges into the Helix fork — PR #42 (`feature/001864-merge-latest-zed`) and PR #43 (`feature/001909-merge-latest-zed`) — landed the new multi-workspace agents sidebar (`crates/sidebar/`), the `ThreadSwitcher` overlay (`crates/sidebar/src/thread_switcher.rs`), and substantial reshuffling of `crates/agent_ui/src/agent_panel.rs` (active-view tracking via `BaseView`, `retained_threads`, `from_existing_thread`).

The Helix fork has its own thread bring-up path: `external_websocket_sync::thread_service::open_existing_thread_sync` loads a thread via `connection.load_session()` and pushes it into the panel via `notify_thread_display` → `ConversationView::from_existing_thread(entity)`. The Helix WebSocket sync subscribes to the resulting `Entity<AcpThread>` to forward `NewEntry` / `EntryUpdated` / `Stopped` events to the Helix server.

A previous fix (commit `d7be64fad1`, design doc `helix/design/2026-04-24-acp-thread-entity-routing-after-restart.md`) addressed a split-brain on **Zed restart**: panel restoration created entity X, Helix sent `open_thread`, `load_session_async` created entity Y, the panel rebound to Y, X's `ConversationView` was dropped, and its `cx.on_release` blindly called `unregister_thread(session_id)` — clobbering Y's registration. The fix introduced `unregister_thread_if_matches` to make `on_release` safe.

## Observed Bug

After the two merges, a new variant of the same split-brain manifests **interactively**:

> If the user uses the new Zed threads UI to open the currently-open thread, that thread becomes "detached" — the thread appears to stop running in Zed (no further entries appear in the panel), but updates are still flowing to the Helix session (Helix continues to receive `message_added` / `message_completed` events).

Concretely: after the click, the active `ConversationView` in the panel observes a different `Entity<AcpThread>` than the one the Helix WebSocket sync is subscribed to. The agent (Claude / NativeAgent) keeps streaming into the original entity (X), Helix sees those events, but the UI is bound to a fresh entity (Y) that receives nothing.

## Reproduction Sketch

1. Start a Helix-driven Zed session (Helix sends `open_thread` for thread T).
2. Send a prompt; observe streaming in the panel and arriving at the Helix server.
3. While the thread is still streaming (or even idle but loaded), click on T in the new sidebar threads list (or open the `ThreadSwitcher` overlay and select T).
4. **Bug**: Zed's panel shows T but stops updating. Helix server logs continue to receive events for the same session.

## User Stories

1. **As a Helix user**, when I click on the currently-open thread in the new agents sidebar, nothing observable should change — the panel keeps showing the live, streaming thread, and Helix continues to receive events from the same `AcpThread` entity. There must be exactly one live entity per session.

2. **As a Helix developer**, when an interactive UI action would re-resolve a session that already has a live entity (registered in `THREAD_REGISTRY` and/or being observed by an active `ConversationView`), the existing entity must be reused — not replaced by a fresh one from `connection.load_session()`.

## Acceptance Criteria

- [ ] Clicking the currently-active thread in the new sidebar list (`crates/sidebar/src/sidebar.rs` `activate_thread`) is a no-op for the panel: no new `ConversationView` is created, no second `connection.load_session()` is issued, the active `Entity<AcpThread>` in the panel is unchanged.
- [ ] Hovering / previewing the currently-active thread in the `ThreadSwitcher` (`ThreadSwitcherEvent::Preview`) is also a no-op for the active entity. Previewing a *different* thread and dismissing back to the original must restore the original entity (not create a new one).
- [ ] After the click flow, exactly one entity for the session is registered in `external_websocket_sync::THREAD_REGISTRY`, and the Helix WebSocket subscription (`PERSISTENT_SUBSCRIPTIONS`) points at the same entity.
- [ ] After the click flow, the panel's active `ConversationView` observes the same entity that Helix is subscribed to. Subsequent agent output appears in both the Zed panel and the Helix server.
- [ ] The existing E2E test `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` continues to pass for both `zed-agent` and `claude` agents.
- [ ] A new regression test (unit or E2E) covers the "click currently-open thread" scenario specifically, asserting that the entity ID held by the active CV does not change across the click.

## Out of Scope

- General refactor of the new sidebar / ThreadSwitcher.
- Changes to upstream Zed code that aren't strictly necessary to fix the routing — keep Helix-only fixes behind `#[cfg(feature = "external_websocket_sync")]` where possible (per `portingguide.md`).
- Fixing other suspected post-merge regressions not covered by this report.
