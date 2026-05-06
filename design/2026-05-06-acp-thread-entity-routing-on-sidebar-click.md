# ACP Thread Entity Routing on Sidebar Click — Interactive Detachment Bug

**Date:** 2026-05-06
**Status:** Fix landed (Zed PR for `feature/001913-after-merging-latest-2`)
**Spec task:** `001913_after-merging-latest-2`
**Companion doc:** `2026-04-24-acp-thread-entity-routing-after-restart.md` (the restart variant of the same split-brain)

## Symptom

After the merges that landed the new agents sidebar (Zed PR #42 / PR #43), users reported:

> If you click on the currently-open thread in the new Zed threads UI, the thread becomes "detached" — the panel stops updating in Zed, but Helix keeps receiving `message_added` / `message_completed` events for the same session.

Single thread in the sidebar is enough to trigger it; no `ThreadSwitcher`, no second thread, no restart — just a bare click.

## Root cause

Two parallel dedup paths in `crates/agent_ui/src/agent_panel.rs` use **different identity checks** for "is this session already loaded":

- **Helix bring-up path** (`notify_thread_display` → `ConversationView::from_existing_thread`): compares `Entity<AcpThread>` references.
- **UI click path** (`panel.load_agent_thread`): compares `acp::SessionId` (`cv.root_session_id == session_id`).

When the second check misses for any reason — `root_session_id` is `None`, the canonical `acp::SessionId` differs between paths, the active view is `Uninitialized` — `load_agent_thread` falls through to `external_thread`, which calls `connection.load_session()` a second time. The agent returns a fresh `Entity<AcpThread>` Y for the same session. Y becomes the panel's active view; the original X gets retained. Helix's WebSocket subscription is on X — events keep flowing — but the panel is bound to Y, which receives nothing.

Same shape as the restart bug fixed by `d7be64fad1`/`unregister_thread_if_matches`, different trigger.

## Fix

`crates/agent_ui/src/agent_panel.rs::load_agent_thread`, gated on `#[cfg(feature = "external_websocket_sync")]`, before the existing `has_session` block:

1. Look up `external_websocket_sync::get_thread(session_id)`.
2. If a live entity exists, do an **entity-id** identity check against the active CV's `active_thread().thread.entity_id()`. Match → no-op.
3. Else check retained CVs by entity-id. Match → promote.
4. Else (no view observes the live entity) → wrap it via `ConversationView::from_existing_thread(live_entity, …)`. Same code path `notify_thread_display` uses.
5. Else (no live entity at all) → fall through to upstream `has_session` / `external_thread`.

Steps 1–4 use entity-id, mirroring what `notify_thread_display` does. The Helix-mode invariant is now enforced at the UI entry point too: **one live `Entity<AcpThread>` per session, observed by exactly one active CV.**

## Why both fixes matter

| Fix | Trigger | Mechanism |
|-----|---------|-----------|
| `d7be64fad1` (restart) | Zed restart + Helix `open_thread` | `cx.on_release` was clobbering a fresh `register_thread`. Replaced blind `unregister_thread` with `unregister_thread_if_matches`. |
| This task (interactive) | User clicks active thread in new sidebar | UI dedup couldn't find the live entity by `session_id`. Added entity-id guard at the top of `load_agent_thread`. |

Both target the same invariant. Removing either while the other exists silently re-opens one of the windows.

## Rebase note

Both fixes live in files that move with upstream. Re-check after merges:

- `agent_panel.rs::load_agent_thread` — the entity-id guard must stay above the upstream `has_session` block.
- `conversation_view.rs::from_existing_thread` and `acp/thread_view.rs::from_existing_thread` — `cx.on_release` must use `unregister_thread_if_matches`, not `unregister_thread`.

See `zed/portingguide.md` Critical Fix #11 for the canonical entry on the new guard.
