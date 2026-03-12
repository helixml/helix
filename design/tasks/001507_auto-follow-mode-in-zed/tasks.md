# Implementation Tasks

## Bug 1: Auto-Follow Not Activating for External WebSocket Messages

### Investigation
- [x] Reproduce: send a message from Helix web UI, confirm Zed editor does NOT follow the agent's file opens/cursor movements
- [x] Confirm that sending the same message directly in Zed's agent panel DOES activate following

### Fix: Activate following in AgentPanel ThreadDisplayNotification handler
- [x] In `zed/crates/agent_ui/src/agent_panel.rs`, in the `ThreadDisplayNotification` handler (~line 845), after `set_active_view` and before `focus_panel`: read `should_be_following` from the newly created `AcpThreadView` and call `workspace.follow(CollaboratorId::Agent, window, cx)` if true
- [x] In the same handler, in the early-return path where the panel is already showing the same thread entity (~line 819): also check `should_be_following` on the existing `AcpThreadView` and call `workspace.follow(CollaboratorId::Agent, window, cx)` if true (re-engages following for follow-up messages)

### Testing
- [ ] Manual: send message from Helix web UI → verify Zed editor follows agent (opens files, scrolls to cursor)
- [ ] Manual: click follow toggle OFF in Zed → send Helix message → verify editor does NOT follow
- [ ] Manual: click follow toggle ON → send another Helix message → verify following resumes
- [ ] Manual: send a follow-up message to an existing thread → verify following re-engages
- [ ] Manual: type a message directly in Zed agent panel → verify normal follow behavior is unchanged

## Bug 2: Split-Brain — Duplicate Agent Instances After View Switching

### Investigation
- [x] Reproduce: start a long-running agent task from Helix → while generating, navigate to thread list → click the same or different thread → observe two agents running in parallel (one still writing files, one responding to stop)
- [x] Confirm the old `Entity<AcpThread>` stays alive via `previous_view` by adding logging in `AcpServerView::on_release`

### Fix Part A: Cancel running threads when switching away from AgentThread view
- [x] In `zed/crates/agent_ui/src/agent_panel.rs`, in `set_active_view` (~line 1780), before the view replacement logic: if the outgoing `active_view` is an `AgentThread`, read its `active_thread()` and call `cancel_generation(cx)` on it
- [x] Also cancel generation on `previous_view` if it holds an `AgentThread` being replaced — specifically in the `!current_is_special && new_is_special` branch where `previous_view` gets overwritten, cancel the outgoing `previous_view` first

### Fix Part B: Clear stale `previous_view` on special → non-special transitions
- [x] In `zed/crates/agent_ui/src/agent_panel.rs`, in the `set_active_view` first branch (`current_is_special && !new_is_special`): cancel any running generation in `previous_view` if it holds an AgentThread, then set `self.previous_view = None` before assigning the new active view
- [x] Verify the "back" button flow still works: History → back should restore `previous_view` (this path uses `go_back` which calls `self.previous_view.take()` before `set_active_view`, so it's unaffected)

### Fix Part C: Clean up THREAD_REGISTRY on entity replacement
- [x] In `zed/crates/external_websocket_sync/src/thread_service.rs`: add `pub fn unregister_thread(acp_thread_id: &str)` that removes the entry from `THREAD_REGISTRY`
- [x] In `register_thread`, log a warning when overwriting an existing entry with a different entity (helps diagnose future split-brain issues)
- [x] In `zed/crates/agent_ui/src/acp/thread_view.rs`, in `from_existing_thread`'s `on_release` callback: call `unregister_thread` with the session ID to clean up the registry when a headless view is dropped

### Testing
- [ ] Manual: start long-running task from Helix → navigate to thread list → click same thread → verify only one agent is running (no ghost file writes)
- [ ] Manual: start task → navigate to thread list → click different thread → verify first task's generation was cancelled
- [ ] Manual: start task → navigate to thread list → press "back" → verify original thread is restored and functional
- [ ] Manual: start task → navigate to thread list → click same thread → send stop → verify it actually stops (no continued file writes from orphaned agent)

## Build Verification
- [x] `./stack build-zed release` compiles cleanly (had to fix borrow checker errors — clone Entity before update to avoid conflicting borrows)
- [ ] `cargo test -p external_websocket_sync` passes — needs VM with cargo
- [ ] `cargo test -p agent_ui` passes (if applicable) — needs VM with cargo