# Implementation Tasks

## Code Change

- [~] In `crates/agent_ui/src/acp/thread_view/active_thread.rs`, change `should_be_following: false` to `should_be_following: true` in `AcpThreadView::new` (around line 375)

## Testing

- [ ] Manual test: Start new agent thread, verify editor auto-follows agent activity
- [ ] Manual test: Toggle button still works to disable follow mode during generation
- [ ] Manual test: Turning off follow mid-generation stays off until next message