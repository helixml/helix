# Implementation Tasks

## Code Change (Zed repo)

- [x] In `crates/agent_ui/src/acp/thread_view/active_thread.rs`, change `should_be_following: false` to `should_be_following: true` in `AcpThreadView::new` (around line 375)

## Helix repo update

- [x] Update `sandbox-versions.txt` to point to new ZED_COMMIT (23e7e46be779589793040143852ab6031d93c4e8)

## Testing

- [ ] Manual test: Start new agent thread, verify editor auto-follows agent activity
- [ ] Manual test: Toggle button still works to disable follow mode during generation
- [ ] Manual test: Turning off follow mid-generation stays off until next message

Note: Manual testing requires building Zed and running with the Helix stack. Testing will be done after deployment.