# Design: Prevent Keyboard Focus Stealing When Following Agent

## Overview

The "follow agent" feature in Zed tracks the agent's file navigation and displays the files the agent is working on. Currently, this feature also transfers keyboard focus to the editor when the agent opens/navigates to a file. This design separates visual tracking from keyboard focus.

## Root Cause Analysis

The focus stealing occurs in `workspace/src/workspace.rs` in `leader_updated()`:

```rust
pane.update(cx, |pane, cx| {
    let focus_active_item = pane.has_focus(window, cx) || transfer_focus;
    // ... add/activate item ...
    if focus_active_item {
        pane.focus_active_item(window, cx)  // <-- This steals focus
    }
});
```

When `follow()` is called (for `CollaboratorId::Agent`), it focuses the follower state's pane, which then causes `has_focus()` to return true, leading to `focus_active_item()` being called on every agent location change.

The issue is that `follow()` at line 5050 does:
```rust
window.focus(&follower_state.pane().focus_handle(cx), cx);
```

This makes sense for peer collaboration (you want to see what your teammate is doing), but for agent following, users want **visual** tracking without **input** focus transfer.

## Design Decision

**Option A: Add a flag to control focus behavior in `follow()`**  
Add `steal_focus: bool` parameter to `follow()` and `leader_updated()`.

**Option B: Never steal focus for Agent collaborator type**  
Special-case `CollaboratorId::Agent` to skip focus transfer.

**Option C: Separate "visual follow" from "focus follow"**  
Create distinct behaviors for tracking location vs. transferring input focus.

**Chosen: Option B** - It's minimal, targeted, and matches user expectations. Agent following should be non-intrusive by default. Users can click to explicitly take focus.

## Implementation Approach

### Change 1: Don't focus pane when starting to follow agent

In `workspace.rs` `follow()` method, skip the focus call for agents:

```rust
// if you're already following, find the right pane and focus it.
if let Some(follower_state) = self.follower_states.get(&leader_id) {
    // Only focus pane for peer collaborators, not agents
    if !matches!(leader_id, CollaboratorId::Agent) {
        window.focus(&follower_state.pane().focus_handle(cx), cx);
    }
    return;
}
```

### Change 2: Don't transfer focus in `leader_updated()` for agent

In `leader_updated()`, skip focus transfer for agent:

```rust
pane.update(cx, |pane, cx| {
    // For agent following, never steal focus - only update visual state
    let focus_active_item = !matches!(leader_id, CollaboratorId::Agent)
        && (pane.has_focus(window, cx) || transfer_focus);
    
    if let Some(index) = pane.index_for_item(item.as_ref()) {
        pane.activate_item(index, false, false, window, cx);
    } else {
        pane.add_item(item.boxed_clone(), false, false, None, window, cx)
    }

    if focus_active_item {
        pane.focus_active_item(window, cx)
    }
});
```

## Files to Modify

| File | Change |
|------|--------|
| `zed/crates/workspace/src/workspace.rs` | Modify `follow()` to not focus pane for Agent |
| `zed/crates/workspace/src/workspace.rs` | Modify `leader_updated()` to not transfer focus for Agent |

## Testing Strategy

1. **Manual testing:**
   - Start an agent task
   - Type in the prompt while agent opens files
   - Verify keystrokes stay in prompt, not in editor

2. **Existing tests:**
   - Run `cargo test -p workspace` to ensure no regressions
   - Run agent_ui tests to ensure follow toggle still works

3. **Edge cases:**
   - User explicitly clicks editor while following → should focus
   - User toggles follow off/on → should not steal focus
   - Agent opens multiple files rapidly → should not steal focus

## Risks

- **Low risk:** Changes are localized to two methods
- **Peer following unaffected:** Changes only apply to `CollaboratorId::Agent`
- **Visual tracking preserved:** Only focus behavior changes, not which file is displayed