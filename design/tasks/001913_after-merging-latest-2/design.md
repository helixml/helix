# Design: Fix thread detachment when opening currently-open thread

## Architecture Background

There are now **two independent paths** that resolve a session ID into a displayed `ConversationView` in the panel, and they use **different identity checks**:

### Path A — Helix-driven (entity-based)

`thread_service::open_existing_thread_sync` → `connection.load_session(session_id)` → `register_thread(acp_thread_id, entity)` → `notify_thread_display(entity)` → callback in `agent_panel.rs:1030+` → `ConversationView::from_existing_thread(entity, …)`.

The dedup check at `agent_panel.rs:1052-1077` compares **`Entity` references**:

```rust
if active_thread.read(cx).thread == notification.thread_entity {
    // skip — already showing this entity
    return;
}
```

If the entity differs, the panel rebinds to the new entity and retains the old `ConversationView`.

### Path B — UI-driven (session-id-based)

User click in the new sidebar → `sidebar::activate_thread_locally` → `load_agent_thread_in_workspace` → `panel.load_agent_thread(agent, session_id, …)`. The dedup check at `agent_panel.rs:2493-2507` compares **`acp::SessionId`**:

```rust
let has_session = |cv| {
    cv.read(cx).root_session_id.as_ref().is_some_and(|id| id == &session_id)
};
if let BaseView::AgentThread { conversation_view } = &self.base_view {
    if has_session(conversation_view) { return; }   // active match
}
// else check retained_threads, else external_thread() — which calls load_session() AGAIN
```

When `has_session` matches, this is a clean no-op. When it doesn't, `external_thread` → `create_agent_thread` → `ConversationView::new(resume_session_id=Some(session_id))` → `connection.load_session(session_id)`. For the same session, this **may produce a different `Entity<AcpThread>`** depending on the agent's `load_session` implementation. The new entity is registered (overwriting the registry), the new CV becomes active, the old CV moves to `retained_threads`. The old entity is still alive (held by the retained CV) and **still emits events** — which Helix's existing subscription on it forwards to the server. The visible panel is now bound to the new (silent) entity.

Net effect = the bug described in `requirements.md`.

## Root-Cause Hypothesis

> **Confirmed by reviewer**: bug exhibits with **a single thread in the sidebar**. This rules out anything that depends on multiple entries (preview-while-different-thread-active, retained eviction by other threads, etc.). The trigger is the bare click on the currently-open, only thread.

The `has_session()` check in path B should match for the currently-open thread but doesn't. Remaining candidates:

1. **`ConversationView::root_session_id` is `None` (or stale) on the active CV when the click arrives.** Most likely cause. `from_existing_thread` (the path Helix uses to bring up a thread) does set `root_session_id` to `Some(thread.session_id())` at construction (`conversation_view.rs:1015`) — but if anything **resets** the field later (e.g. `set_server_state`, `reset()`, a reconnect rebinding `server_state` without re-asserting `root_session_id`), the field could go stale or `None` even though the CV is still visibly displaying the thread. Worth grepping every `root_session_id =` assignment and every code path that mutates `server_state` / `ServerState::Connected` independently of `root_session_id`. Specifically check `conversation_view.rs:1322` (`this.root_session_id = Some(root_session_id.clone())` — fires from `set_server_state`-adjacent code) and verify it always runs in the helix-bring-up path.

2. **`metadata.session_id` ≠ entity's `session_id()`.** The sidebar passes `metadata.session_id` to `panel.load_agent_thread`; the active CV stores `root_session_id` derived from `thread.session_id()`. For Helix-loaded threads, the metadata is populated by `handle_conversation_event` in `thread_metadata_store.rs:1163` from `view.root_thread(cx).read(cx).session_id().clone()`. If the metadata was instead loaded from disk with a slightly different id (e.g. trailing whitespace, case, or a re-issued session id from a `load_session` round-trip), the `==` comparison fails. Worth dumping `(metadata.session_id, cv.root_session_id)` from a single-thread repro to confirm whether they're literally equal.

3. **The active `BaseView` is `Uninitialized` or a draft when the click arrives**, despite the user perceiving the thread as "open". With a single Helix-driven thread, this could happen if the panel restored to `Uninitialized` (or an empty draft CV) and the helix `notify_thread_display` callback hasn't yet rebound to the running thread — yet the running thread is still visible somehow (e.g. via a different UI surface, or the user only thinks it's the active panel view). In that case `has_session(active CV)` is `false` because the active CV isn't the thread CV at all; the thread's CV (if any) is in `retained_threads` or only reachable via `THREAD_REGISTRY`. Path B then falls through to `external_thread` and the entity-clobber happens. **Investigation must capture which `BaseView` variant is active at the moment of the click.**

The fix below addresses the symptom regardless of which hypothesis is confirmed — it short-circuits on entity identity (which `THREAD_REGISTRY` always knows authoritatively for Helix threads) before any of these `root_session_id`-based checks run. The investigation step still matters: we want a precise regression test, and if hypothesis #1 turns out to be the real bug we should also fix the underlying assignment so non-Helix builds (which don't have the new guard) aren't silently broken by the same root cause.

## Fix Strategy

The minimum-change, defensive fix is a single new guard at the top of `panel.load_agent_thread` (path B), gated on `#[cfg(feature = "external_websocket_sync")]`:

> **If `external_websocket_sync::get_thread(&session_id.to_string())` returns a live entity, and the active or any retained `ConversationView`'s `active_thread` already observes that exact entity (compared by `EntityId`, not `SessionId`), reuse that view instead of falling through to `external_thread`.**

This bridges the two identity worlds: path B picks up the entity-based dedup that path A already enforces. It avoids issuing a duplicate `connection.load_session()` and, more importantly, prevents the registry from being clobbered by a fresh entity for a session that already has a live one.

### Concrete change shape

In `crates/agent_ui/src/agent_panel.rs::load_agent_thread` (currently lines 2470-2538), before the existing `has_session` checks, add:

```rust
#[cfg(feature = "external_websocket_sync")]
{
    let session_id_str = session_id.to_string();
    if let Some(live) = external_websocket_sync::get_thread(&session_id_str)
        .and_then(|w| w.upgrade())
    {
        let live_entity_id = live.entity_id();
        let observes_live = |cv: &Entity<ConversationView>| -> bool {
            cv.read(cx)
                .active_thread()
                .is_some_and(|t| t.read(cx).thread.entity_id() == live_entity_id)
        };

        // Active CV already showing the live entity → no-op.
        if let BaseView::AgentThread { conversation_view } = &self.base_view {
            if observes_live(conversation_view) {
                self.clear_overlay_state();
                cx.emit(AgentPanelEvent::ActiveViewChanged);
                return;
            }
        }

        // Retained CV holds the live entity → promote it (set_base_view will
        // retain the current view), no new load_session.
        let retained_key = self
            .retained_threads
            .iter()
            .find(|(_, cv)| observes_live(cv))
            .map(|(id, _)| *id);
        if let Some(thread_id) = retained_key {
            if let Some(conversation_view) = self.retained_threads.remove(&thread_id) {
                self.set_base_view(
                    BaseView::AgentThread { conversation_view },
                    focus,
                    window,
                    cx,
                );
                return;
            }
        }

        // Live entity exists but no view observes it (e.g. user dismissed
        // every CV after a workspace transition). Wrap it via the entity-based
        // path used by Helix bring-up so we don't issue a second load_session.
        let server = agent.server(self.fs.clone(), self.thread_store.clone());
        let conversation_view = cx.new(|cx| {
            ConversationView::from_existing_thread(
                live.clone(),
                server,
                self.connection_store.clone(),
                agent.clone(),
                self.workspace.clone(),
                self.project.clone(),
                Some(self.thread_store.clone()),
                self.prompt_store.clone(),
                window,
                cx,
            )
        });
        self.set_base_view(
            BaseView::AgentThread { conversation_view },
            focus,
            window,
            cx,
        );
        return;
    }
}

// Existing fallback: session_id-based has_session checks, then external_thread.
```

The fix is **purely additive** in front of the existing logic and is fully gated. Outside the Helix feature, behavior is unchanged.

### Why guard at `load_agent_thread` instead of upstream of it?

The single-thread, single-click repro proves the trigger is the bare click in the sidebar, not anything in `ThreadSwitcher` or its `Preview` handler. So fixing `Preview` would not address the bug. Guarding `load_agent_thread` itself catches every interactive entry point (sidebar click, switcher confirm, action handlers) and matches the invariant we actually want: *one live `Entity<AcpThread>` per session, observed by exactly one active CV.*

### Defense-in-depth (apply if investigation confirms the matching hypothesis)

- If hypothesis #1 (`root_session_id` becoming `None`/stale) is confirmed: repair the assignment in `conversation_view.rs` so the field stays in sync with the entity across `set_server_state` / `reset` / reconnect. Without this, non-Helix builds (which don't get the new `THREAD_REGISTRY` guard) are silently broken by the same root cause.
- If hypothesis #2 (session_id mismatch) is confirmed: in `ConversationView::from_existing_thread` and `ConversationView::new`, store `root_session_id = thread.read(cx).session_id()` from the actual entity (not from the resume parameter) so both paths agree on the canonical id.
- If hypothesis #3 (`BaseView::Uninitialized` at click time) is confirmed: ensure `notify_thread_display` runs (or the panel restoration produces a `BaseView::AgentThread` for the loaded thread) before the user can click — likely a sequencing fix in the helix bring-up, not the panel.

## Trade-offs

- **Slight coupling of the upstream-merged `agent_panel.rs` to `external_websocket_sync`.** Already present in this file (the `notify_thread_display` callback at line 1027+). The new guard is similarly gated, so rebase risk is limited to this single function and the existing upstream-merge sweeps in `portingguide.md` already cover `agent_panel.rs`.
- **Falls back to `external_thread` only when no live entity exists for the session.** That's the same behavior pure upstream Zed has, so non-Helix builds are unaffected.
- **Does not delete or change `unregister_thread_if_matches`.** The earlier `d7be64fad1` fix is still required for the restart scenario; this fix complements it for the interactive scenario.

## Verification

1. **Manual repro** (per `requirements.md` §"Reproduction Sketch"). Confirm: pre-fix, the active CV's `thread.entity_id()` changes after the click; post-fix, it does not.
2. **Existing E2E**: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` for both `E2E_AGENTS=zed-agent` and `E2E_AGENTS=claude`. Must pass unchanged.
3. **New regression test**: a unit test in `crates/agent_ui/src/agent_panel.rs` (or `crates/external_websocket_sync/src/protocol_test.rs`) that simulates: register an entity in `THREAD_REGISTRY`, set the panel to display it via `from_existing_thread`, then call `panel.load_agent_thread(same_session_id, …)` and assert `panel.active_conversation_view().active_thread().thread.entity_id()` is unchanged.
4. **Log inspection**: with `eprintln!` traces in `THREAD_SERVICE` already present, confirm pre-fix shows a duplicate `register_thread: overwriting thread '…' with different entity` warning around the click; post-fix shows none.

## Documentation

- Add a short note to `/home/retro/work/zed/portingguide.md` under the rebase checklist: "When `agent_panel.rs::load_agent_thread` is touched upstream, re-check the `external_websocket_sync` guard at the top — it must run **before** the upstream `has_session` checks." Reference this task and the previous one (`001868`/`d7be64fad1`).
- Cross-link to `helix/design/2026-04-24-acp-thread-entity-routing-after-restart.md` from a new short note (or extend that doc) describing the interactive variant.

## Notes Discovered While Investigating

These are observations future agents working on similar tasks should know:

- **Two parallel dedup mechanisms exist** for "is this session already loaded": entity-based (Path A, used by Helix's `notify_thread_display` handler) and session-id-based (Path B, used by upstream `panel.load_agent_thread`). They live in the same file (`agent_panel.rs`) but never reference each other. Any new path that opens a thread by session_id needs to consult **both** to be safe in Helix mode.
- **`ThreadSwitcher` preview is destructive**, not visual-only — `Preview` calls `load_agent_thread_in_workspace` which mutates `BaseView`. This is an upstream Zed design choice; we work around it rather than fight it. Note: this was initially a hypothesis for *this* bug but ruled out by the single-thread repro. It is still worth knowing about for future bugs in this area.
- **`AgentConnectionCache`** (added in `ba7e97aea6`, partially reverted in `350de991de`) shares ACP connections per `(Project, AgentId)` between `thread_service` and the UI. So calling `connection.load_session()` from both paths uses the same underlying agent process — but each call may still produce a distinct `Entity<AcpThread>` wrapper, which is the entity that subscriptions attach to.
- **`from_existing_thread` exists in two places**: `crates/agent_ui/src/conversation_view.rs:771` (the live one used by Helix bring-up) and `crates/agent_ui/src/acp/thread_view.rs:555` (declared but not wired in — see 001909 design notes "`HeadlessConnection` is dead code"). Don't get confused by the second one.
- **`retained_threads` is the upstream mechanism that makes the bug invisible-but-not-fatal**: the old CV (with the live subscription) survives in the map, so Helix events keep flowing. Without `retained_threads`, the old CV would be dropped, `on_release` would fire `unregister_thread_if_matches`, and we'd see a different (worse) failure mode. Don't "clean up" `retained_threads` as part of this fix.

## Update 2026-05-08: Re-checked after merge of upstream PR #49 (`001980-merge-latest-zed`)

The latest Zed merge brought 184 commits but **did not** touch the bug's premise:

- `agent_panel.rs::load_agent_thread` — unchanged. My patch merged cleanly.
- `conversation_view.rs` — 308 lines changed (NewThreadLocation removal, error stderr, queued-message paste). Zero changes to `from_existing_thread`, `on_release`, `root_session_id`, or `register_thread`/`unregister_thread`.
- `sidebar.rs` — adds an early-return when clicking the already-active *workspace* (same shape as my fix, but workspace-level not thread-level; doesn't dedup the thread bring-up).

Build green against new main (`./stack build-zed dev`). Patch and `portingguide.md` Critical Fix #11 still in place. Pushed merged zed branch as `684d46cbf8`.

The bug should still reproduce against current main without my patch. The fix is still warranted.
