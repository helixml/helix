# PR: Merge upstream Zed (509 commits, April 2026)

**Branch:** `feature/001723-merge-latest-zed` -> `main`

## Summary

Merges 509 upstream commits from `zed-industries/zed` into the Helix fork, bringing the fork up to date with upstream `main`.

**Notable upstream changes absorbed:**
- Removed `assistant_text_thread` and slash command crates (#52757)
- Removed `AgentV2FeatureFlag` (#52792)
- Removed Claude upsell (#52831)
- Removed notification panel (#50204)
- `selected_agent_type` -> `selected_agent` rename
- `ContextServerStatus::AuthRequired` variant added
- `should_render_onboarding` -> `should_render_new_user_onboarding`
- New `agent_layout_onboarding` block in agent panel
- `ProjectGroup` -> `Project` rename in sidebar
- Removed `AgentSessionInfo` usage in sidebar
- New workspace re-exports (NewThread, NextProject, SidebarEvent, etc.)
- `track-project-leak` feature flag added
- `audio` feature added to `agent_ui`

**Post-merge fixes (3 commits):**
1. `HeadlessConnection`: added missing `agent_id()` method, updated `new_session()` signature to `PathList`
2. Removed dead imports from deleted `assistant_text_thread` crate
3. Removed stale `AgentV2FeatureFlag` usage
4. Fixed `selected_agent_type` -> `selected_agent` rename in cfg-gated block
5. Restored `login`/`history` fields in `AcpServerView::from_existing_thread` (still needed, unlike `ConversationView`)
6. Removed dangling conflict marker

**Verification:**
- All 31 code-level rebase checklist items verified
- Portingguide updated with 5 new checklist items (35-39) for patterns discovered during this merge
- Build/test verification pending CI

## Test plan

- [ ] `cargo check --package zed --features external_websocket_sync` compiles
- [ ] `cargo test -p external_websocket_sync` passes
- [ ] `cargo test -p acp_thread test_second_send` passes (Stopped invariant)
- [ ] E2E test: all 10 phases pass for `zed-agent`
- [ ] E2E test: all 10 phases pass for `claude` agent

Release Notes:

- N/A
