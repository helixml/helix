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

The `has_session()` check in path B should match for the currently-open thread but apparently does not in some interactive flow. Likely candidates (to be confirmed by logs during step 1 below):

1. **`ThreadSwitcher` preview side-effects.** `ThreadSwitcher::new` always emits `Preview { metadata }` for the entry at `selected_index = 1.min(len-1)` — i.e. the second entry, **not** the currently-active one. The `Preview` handler in `sidebar.rs:3776-3794` calls `Self::load_agent_thread_in_workspace(workspace, metadata, false, …)` unconditionally. This swaps the panel's active CV to the previewed (different) thread, retaining the original. If the user then selects / clicks the originally-open thread to come back, path B's `has_session` check now compares the *new* active CV (different session) — the dedup falls through to `external_thread`, which calls `load_session` for the original session and produces a fresh entity Y. The user perceives this as "I clicked on the open thread and it broke."

2. **`from_existing_thread` vs `external_thread` `root_session_id` mismatch.** `from_existing_thread` (path A) sets `root_session_id` from `thread.read(cx).session_id()` — the entity's actual id. `create_agent_thread` (path B) sets it from `resume_session_id` (the metadata's id). If the agent's `load_session` ever returns an entity whose `session_id()` differs from the requested id, the two views can disagree on which `acp::SessionId` is the canonical one — `has_session` then returns false even though both views "are" the same session.

3. **Stale `retained_threads` shadowing.** After preview-induced swaps, the same session may exist in `retained_threads` AND as the active view. The retain-promote path at `agent_panel.rs:2509-2525` handles "active doesn't match, retained does" correctly, but if the comparison is happening on different `acp::SessionId` representations (per #2), it may miss the retained match too and create a fresh CV.

We do **not** need to confirm all three before fixing — the fix below addresses the symptom for any of them. The investigation step exists to confirm which path is hit and to write a precise regression test.

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

### Why not "fix the `Preview` handler" instead?

That would also work for hypothesis #1, but doesn't help if a user genuinely clicks the active thread without going through the ThreadSwitcher (a legitimate action in the new sidebar). Guarding `load_agent_thread` covers both entry points and is closer to the real invariant we want: *one live entity per session, one observing CV.*

### Defense-in-depth (optional, only if investigation finds it warranted)

If hypothesis #2 (session_id mismatch) is confirmed, also: in `ConversationView::from_existing_thread` and `ConversationView::new`, store `root_session_id = thread.read(cx).session_id()` from the actual entity (not from the resume parameter) so both paths agree on the canonical id. This is a one-line change but only justified if logs show the divergence.

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
- **`ThreadSwitcher` preview is destructive**, not visual-only — `Preview` calls `load_agent_thread_in_workspace` which mutates `BaseView`. This is an upstream Zed design choice; we work around it rather than fight it.
- **`AgentConnectionCache`** (added in `ba7e97aea6`, partially reverted in `350de991de`) shares ACP connections per `(Project, AgentId)` between `thread_service` and the UI. So calling `connection.load_session()` from both paths uses the same underlying agent process — but each call may still produce a distinct `Entity<AcpThread>` wrapper, which is the entity that subscriptions attach to.
- **`from_existing_thread` exists in two places**: `crates/agent_ui/src/conversation_view.rs:771` (the live one used by Helix bring-up) and `crates/agent_ui/src/acp/thread_view.rs:555` (declared but not wired in — see 001909 design notes "`HeadlessConnection` is dead code"). Don't get confused by the second one.
- **`retained_threads` is the upstream mechanism that makes the bug invisible-but-not-fatal**: the old CV (with the live subscription) survives in the map, so Helix events keep flowing. Without `retained_threads`, the old CV would be dropped, `on_release` would fire `unregister_thread_if_matches`, and we'd see a different (worse) failure mode. Don't "clean up" `retained_threads` as part of this fix.
