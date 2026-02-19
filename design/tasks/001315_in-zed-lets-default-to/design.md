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

**Approach: Flip the default to `true`**

This is a one-line change. No need for a configurable setting in our Helix fork - if configurability is needed later, the settings-sync-daemon can set it via the existing Zed settings mechanism.

## Change Required

In `crates/agent_ui/src/acp/thread_view/active_thread.rs`, update `AcpThreadView::new`:

```rust
// Before
should_be_following: false,

// After
should_be_following: true,
```

## File Changes Summary

| File | Change |
|------|--------|
| `crates/agent_ui/src/acp/thread_view/active_thread.rs` | Change `should_be_following: false` to `true` |

## Testing

- Manual: Start new thread, verify editor follows agent by default
- Manual: Toggle button still works to disable follow mode during generation