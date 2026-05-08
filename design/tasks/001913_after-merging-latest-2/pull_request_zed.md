# Fix thread detachment when re-opening live session via new sidebar

## Summary

After the two upstream-merge PRs (#42 / #43) landed the new agents sidebar, clicking the currently-open thread in the sidebar caused the panel to "detach" from the live `Entity<AcpThread>`: Zed's panel stopped updating but Helix kept receiving `message_added` / `message_completed` events for the same session. Single-thread, single-click was enough to trigger it — no `ThreadSwitcher`, no restart.

Root cause: two parallel dedup paths in `agent_panel.rs` use different identity checks. `notify_thread_display` (Helix bring-up) compares `Entity<AcpThread>` references; `panel.load_agent_thread` (UI clicks) compares `acp::SessionId`. When the session-id check missed, `external_thread` issued a duplicate `connection.load_session()`, producing a fresh entity Y. Helix's WebSocket subscription stayed on the original X (events kept flowing) but the panel rebound to Y (silent) — the same split-brain shape as `d7be64fad1` (restart variant), different trigger.

## Fix

Adds an `#[cfg(feature = "external_websocket_sync")]`-gated guard at the top of `pub fn load_agent_thread` in `crates/agent_ui/src/agent_panel.rs`, before the existing `has_session` block:

1. Look up `external_websocket_sync::get_thread(session_id)`.
2. If a live entity exists → entity-id check against the active CV → no-op.
3. Else entity-id check against retained CVs → promote the match.
4. Else wrap the live entity via `ConversationView::from_existing_thread(…)` (same path `notify_thread_display` uses).
5. Else fall through to the unchanged upstream logic.

The new code is purely additive in front of the existing function body. Outside the Helix feature, behaviour is unchanged.

## Changes

- `crates/agent_ui/src/agent_panel.rs` — entity-identity guard at the top of `load_agent_thread`.
- `portingguide.md` — Critical Fix #11 documenting the guard, its rebase requirements, and its relationship to the previous `unregister_thread_if_matches` fix.

## Notes

- **No live repro / E2E in this environment.** Host has no Rust toolchain and no Docker access for the `run_docker_e2e.sh` setup. Build was attempted via `./stack build-zed dev` (zed-builder docker image); status noted in the spec task. Reviewer to run the manual repro and the E2E script before merging.
- **Hypothesis-agnostic.** The design considered three possible reasons the session-id dedup could miss (`root_session_id` `None`/stale, session-id mismatch between paths, `BaseView::Uninitialized` at click time). The entity-id guard short-circuits all three without needing to confirm which is hit. If a follow-up investigation finds a specific root cause, a smaller upstream fix is also possible — but this guard is the safe Helix-side floor.

Release Notes:

- Fixed thread becoming detached when clicking the currently-open thread in the new agents sidebar (Helix mode).
