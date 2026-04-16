# Merge upstream Zed (692 commits, April 2026)

## Summary

Merges 692 upstream commits from `zed-industries/zed` into the Helix fork, bringing the fork up to date with upstream `main`.

## Changes

**Notable upstream changes absorbed:**
- Sidebar rework: `ActiveView`→`BaseView`, history moved to `OverlayView`, draft threads, retained threads
- `selected_agent_type`→`selected_agent` rename
- `set_active_view`→`set_base_view` rename
- `ContextServerStatus::AuthRequired` variant added
- `ConversationView` now has `thread_id` and `root_session_id` fields
- `should_render_onboarding`→`should_render_new_user_onboarding` + `dismiss_ai_onboarding` added
- Agent panel worktree/branch pickers
- Cross-channel thread import support
- Draft thread management
- Parallel agents onboarding
- Removed notification panel, Claude upsell, `assistant_text_thread` crate, `AgentV2FeatureFlag`
- Bumped to Zed v0.234

**Post-merge fixes:**
1. `ActiveView`→`BaseView` rename in all cfg-gated WebSocket blocks
2. `active_view`→`base_view` field access
3. `set_active_view`→`set_base_view` method call
4. `selected_agent_type`→`selected_agent` field assignment
5. History check moved from `BaseView` match to `overlay_view` check
6. `AuthRequired` added to `ContextServerStatus` match in UI state query
7. `AuthRequired` added to `pending_server_starts` tracking in context_server_registry
8. `thread_id` and `root_session_id` fields added to `from_existing_thread()`
9. `load_session_id`→`resume_session_id` in cfg-gated blocks (upstream renamed the variable)

**Verification:**
- All 9 post-merge fixes verified present
- E2E test infrastructure verified intact
- Portingguide updated with 6 new checklist items (35-40)

## Test plan

- [ ] `cargo check --package zed --features external_websocket_sync` compiles
- [ ] `cargo test -p external_websocket_sync` passes
- [ ] `cargo test -p acp_thread test_second_send` passes
- [ ] E2E test: all phases pass for `zed-agent`
- [ ] E2E test: all phases pass for `claude` agent

Release Notes:

- N/A
