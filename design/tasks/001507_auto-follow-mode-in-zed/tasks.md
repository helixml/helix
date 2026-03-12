# Implementation Tasks

## Investigation & Confirmation

- [ ] Reproduce the bug: send a message from Helix web UI, confirm Zed editor does NOT follow the agent's file opens/cursor movements
- [ ] Confirm that sending the same message directly in Zed's agent panel DOES activate following

## Core Fix: Activate following in AgentPanel ThreadDisplayNotification handler

- [ ] In `zed/crates/agent_ui/src/agent_panel.rs`, in the `ThreadDisplayNotification` handler (~line 845), after `set_active_view` and before `focus_panel`: read `should_be_following` from the newly created `AcpThreadView` and call `workspace.follow(CollaboratorId::Agent, window, cx)` if true
- [ ] In the same handler, in the early-return path where the panel is already showing the same thread entity (~line 819): also check `should_be_following` on the existing `AcpThreadView` and call `workspace.follow(CollaboratorId::Agent, window, cx)` if true (re-engages following for follow-up messages)

## Testing

- [ ] Manual test: send message from Helix web UI → verify Zed editor follows agent (opens files, scrolls to cursor)
- [ ] Manual test: click follow toggle OFF in Zed → send Helix message → verify editor does NOT follow
- [ ] Manual test: click follow toggle ON → send another Helix message → verify following resumes
- [ ] Manual test: send a follow-up message to an existing thread → verify following re-engages
- [ ] Manual test: type a message directly in Zed agent panel → verify normal follow behavior is unchanged
- [ ] Build check: `cargo build --features external_websocket_sync -p zed` compiles cleanly
- [ ] Run unit tests: `cargo test -p external_websocket_sync`
