# Implementation Tasks

## Code Changes (Complete)

### Bug 1: Auto-Follow Not Activating for External WebSocket Messages
- [x] Add `workspace.follow(CollaboratorId::Agent)` in `AgentPanel` `ThreadDisplayNotification` handler for new external threads (after `set_active_view`)
- [x] Add `workspace.follow(CollaboratorId::Agent)` in early-return same-entity path for follow-up messages (re-engages following when it has lapsed)
- [x] Both paths check `should_be_following` to respect user's toggle state

### Bug 2: Split-Brain — Duplicate Agent Instances After View Switching
- [x] In `set_active_view`: cancel running generation in `previous_view` when transitioning `special → non-special` (History → new thread), then clear `previous_view`
- [x] In `set_active_view`: cancel running generation in `previous_view` in the `else` branch (direct thread-to-thread replacement)
- [x] Gate both cancellation blocks behind `#[cfg(feature = "external_websocket_sync")]`
- [x] Add `unregister_thread()` to `thread_service.rs` for registry cleanup on entity drop
- [x] Add diagnostic logging in `register_thread()` when overwriting existing entries with different entities
- [x] Call `unregister_thread` in `from_existing_thread`'s `on_release` callback

### Build & Merge
- [x] `./stack build-zed release` compiles cleanly
- [x] `./stack build-ubuntu` passes — new Zed binary packaged into desktop image
- [x] Zed PR #20 merged to `helixml/zed` main (2026-03-13)
- [x] Helix PR #1899 merged to `helixml/helix` main (2026-03-13) — pins new Zed commit in `sandbox-versions.txt`

## Testing (Not Started)

Testing requires a deployed desktop image containing the merged Zed binary. The tester needs to start sessions from the Helix web UI and observe Zed behavior via the desktop stream.

### Prerequisites
- [x] Verify `sandbox-versions.txt` points to a Zed commit that includes the PR #20 merge (commit `6c5ac28` or later on `helixml/zed` main) — confirmed via `strings /zed-build/zed | grep auto-follow` showing all diagnostic strings from PR #20
- [x] Build a desktop image containing the new Zed binary (`./stack build-zed release && ./stack build-ubuntu`) — current session's Zed binary already contains the fix
- [x] Start a fresh session so it picks up the new image — testing in session `ses_01kkkjbmjxxgwpmn28hsktcq2d`

### Bug 1: Auto-Follow Verification
- [x] Send a message from the Helix web UI → verify Zed editor follows the agent as it opens and edits files — **PASSED**: edited `auto-follow-test.md`, editor switched to show it with cursor on the edited line (screenshot `05-after-edit.png`)
- [x] Send a follow-up message to the same thread → verify following re-engages — **PASSED**: opened `CONTRIBUTING.md` after editing `auto-follow-test.md`, editor followed to the new file (screenshot `06-after-switch.png`)
- [ ] Click the follow toggle OFF in Zed's agent panel → send a Helix message → verify the editor does NOT follow (respects user's choice) — needs manual UI interaction with the toggle button
- [ ] Click the follow toggle back ON → send another Helix message → verify following resumes — needs manual UI interaction
- [ ] Type a message directly in Zed's agent panel → verify normal follow behavior is unchanged (regression check) — needs manual UI interaction

### Bug 2: Split-Brain Verification
- [ ] Start a long-running agent task from Helix → while generating, click "View All" in the agent panel → click the same thread from the history list → verify only one agent is running (no ghost file writes from the old agent — check container logs for duplicate tool call activity) — needs manual UI interaction with agent panel navigation
- [ ] Start a task → navigate to thread list → click a different thread → verify the first task's generation was cancelled (no continued file writes from the old task) — needs manual UI interaction
- [ ] Start a task → navigate to thread list → press "back" → verify the original thread is restored and functional (the `previous_view` restore path still works) — needs manual UI interaction
- [ ] Start a task → navigate to thread list → click same thread → send stop → verify it actually stops (no continued file writes from an orphaned agent) — needs manual UI interaction

**Note:** Bug 2 (split-brain) tests require clicking UI elements in Zed's agent panel (View All, thread list, back button) which can't be automated via the ACP agent — they need a human tester or a UI automation tool that can interact with Zed's native GPUI elements.

### Unit Tests (If Rust Toolchain Available)
- [ ] `cargo test -p external_websocket_sync` passes
- [ ] `cargo test -p agent_ui` passes (if applicable)