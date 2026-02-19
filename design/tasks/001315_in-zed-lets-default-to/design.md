# Design: Default Follow Mode for Agent

## Context

The Zed editor has a "follow mode" feature where the editor can track and display whatever the agent is currently working on. Currently this is disabled by default, requiring users to manually click the follow button. Users have requested this be enabled by default because "it's nice to watch."

## Current Implementation

In `zed/crates/agent_ui/src/acp/thread_view/active_thread.rs`:

```rust
// AcpThreadView::new (line ~375)
should_be_following: false,  // Currently defaults to false
```

The `should_be_following` field controls whether the workspace follows the agent's activity. When `true`, the editor scrolls to show files being edited and tracks cursor position.

## Design Decision

**Approach: Add a configurable setting with `true` as default**

Rather than just flipping the hardcoded boolean, we'll add a new setting `auto_follow_agent` to `AgentSettings`. This allows users who dislike the behavior to disable it.

### Why a setting instead of just changing the default?
- Some users may find auto-follow distracting
- Consistent with other agent behaviors (e.g., `always_allow_tool_actions`, `expand_edit_card`)
- Minimal additional complexity

## Changes Required

### 1. Settings Content (`settings_content/src/agent.rs`)
Add field to `AgentSettingsContent`:
```rust
/// Whether to automatically follow the agent's activity when sending messages.
/// Default: true
pub auto_follow_agent: Option<bool>,
```

### 2. Agent Settings (`agent_settings/src/agent_settings.rs`)
Add field to `AgentSettings` struct and `from_settings` implementation:
```rust
pub auto_follow_agent: bool,
// In from_settings:
auto_follow_agent: agent.auto_follow_agent.unwrap_or(true),
```

### 3. Thread View (`agent_ui/src/acp/thread_view/active_thread.rs`)
Read setting in `AcpThreadView::new`:
```rust
should_be_following: AgentSettings::get_global(cx).auto_follow_agent,
```

## File Changes Summary

| File | Change |
|------|--------|
| `crates/settings_content/src/agent.rs` | Add `auto_follow_agent` field |
| `crates/agent_settings/src/agent_settings.rs` | Add field + mapping |
| `crates/agent_ui/src/acp/thread_view/active_thread.rs` | Read setting for default |

## Testing

- Manual: Start new thread, verify editor follows agent by default
- Manual: Set `"agent": { "auto_follow_agent": false }`, verify follow mode is off by default
- Existing follow/unfollow toggle should continue to work