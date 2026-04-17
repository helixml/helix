# Design: Hide "Open PR" Button Until Agent Has Pushed

## Current Behavior

In `SpecTaskActionButtons.tsx`, the implementation phase block (line 447: `if (task.status === "implementation")`) renders the "Open PR" / "Accept" button with no check on whether the agent has pushed commits. The only disable conditions are `isArchived` and mutation pending state.

The backend already tracks push state via `LastPushAt` (set in `git_http_server.go:handleFeatureBranchPush`), and the frontend API type already includes `last_push_at?: string`. The field just isn't used in the button component.

## Approach

Frontend-only change in `SpecTaskActionButtons.tsx`:

1. Add `last_push_at` to the `SpecTaskForActions` interface (it's already in the generated API types but not in this component's interface)
2. Derive a `hasPushed` boolean: `const hasPushed = !!task.last_push_at`
3. When `!hasPushed`, disable both the "Open PR" / "Accept" button and the "Reject" button, showing a tooltip like "Waiting for agent to push code..."

### Why disable instead of hide?

Disabling with a tooltip is better than hiding because:
- The user knows the button *will* appear — they just need to wait
- Hiding it might cause confusion ("where did the button go?")
- A tooltip explains what's happening

### Why not block on the backend?

The `approveImplementation` handler already sends a push instruction to the agent, so it somewhat handles the no-push case. But preventing the user from clicking prematurely gives a better UX and avoids race conditions where the PR is created before code is pushed.

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskActionButtons.tsx` | Add `last_push_at` to interface, disable button when not pushed |

## Codebase Notes

- The task type is `SpecTaskForActions` (lines 41-52 of `SpecTaskActionButtons.tsx`) — a subset of the full task type. `last_push_at` needs to be added to it.
- Both inline (line 493) and full-size (line 569+) button variants need the disable condition.
- The `isDirectPush` flag (line 237) determines button label ("Accept" vs "Open PR") — the disable logic applies to both variants.
- `last_push_at` is updated via WebSocket/polling when the git HTTP server detects a push, so the button will enable in real-time without page refresh.
