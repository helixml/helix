# Design: Hide "Open PR" Button Until Agent Has Pushed

## Current Behavior

In `SpecTaskActionButtons.tsx`, the implementation phase block (line 447: `if (task.status === "implementation")`) renders the "Open PR" / "Accept" button with no check on whether the agent has pushed commits. The only disable conditions are `isArchived` and mutation pending state.

The backend already tracks push state via `LastPushAt` (set in `git_http_server.go:handleFeatureBranchPush`), and the frontend API type already includes `last_push_at?: string`. The field just isn't used in the button component.

## Approach

Frontend-only change in `SpecTaskActionButtons.tsx`:

1. Add `last_push_at` to the `SpecTaskForActions` interface (it's already in the generated API types but not in this component's interface)
2. Derive a `hasPushed` boolean: `const hasPushed = !!task.last_push_at`
3. When `!hasPushed`, disable both "Open PR" / "Accept" and "Reject" buttons and show a tooltip "Waiting for agent to push code..."

### Why disable instead of hide?

Disabling with a tooltip is better than hiding because:
- The user knows the button *will* appear — they just need to wait
- Hiding it might cause confusion ("where did the button go?")
- A tooltip explains what's happening

### Why not block on the backend?

The `approveImplementation` handler already sends a push instruction to the agent, so it somewhat handles the no-push case. But preventing the user from clicking prematurely gives a better UX and avoids race conditions where the PR is created before code is pushed.

## Files Modified

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskActionButtons.tsx` | Added `last_push_at` to interface, added `hasPushed` boolean, disabled both Reject and Open PR/Accept buttons when `!hasPushed` in both inline and stacked variants |
| `frontend/src/components/tasks/TaskCard.tsx` | Pass `last_push_at` from task object at the implementation phase call site |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Pass `last_push_at` from task object at both call sites (header + split view) |

## Implementation Notes

- `last_push_at` was already present in the generated API types (`api.ts` lines 4996, 5298) and in `SpecTaskKanbanBoard.tsx` (line 199) — just never wired into the action buttons component.
- The tooltip condition uses a ternary chain: `isArchived ? "Task is archived" : !hasPushed ? "Waiting for agent to push code..." : ""` — `isArchived` takes precedence since it's the more permanent state.
- `last_push_at` is updated via WebSocket/polling when the git HTTP server detects a push, so the button will enable in real-time without page refresh.
- There are exactly 3 call sites that needed updating: 1 in TaskCard.tsx (stacked/kanban), 2 in SpecTaskDetailContent.tsx (inline/header and inline/split-view).
- WARNING: Could not test in browser — the inner Helix API stack is not running (only sandbox + registry containers are up). Build passes successfully.
