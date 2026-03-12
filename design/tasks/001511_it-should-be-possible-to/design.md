# Design: Allow Move to Backlog from Pull Request and Done States

## Summary

Remove the frontend guards that prevent moving tasks to backlog from `pull_request` and `done` states. This is a frontend-only change — the backend already accepts arbitrary status updates.

## Current State

Two files independently compute `canMoveToBacklog` and both explicitly exclude `pull_request` and `done`:

**`SpecTaskDetailContent.tsx` (detail view, ~line 464):**
```ts
const canMoveToBacklog =
    task &&
    !isQueued &&
    task.status !== "backlog" &&
    task.status !== "done" &&          // ← blocks done
    task.status !== "pull_request" &&  // ← blocks pull_request
    !isTaskArchived;
```

**`TaskCard.tsx` (Kanban card menu, ~line 610):**
```ts
const canMoveToBacklog =
    !isQueued &&
    task.phase !== "backlog" &&
    task.phase !== "completed" &&      // ← blocks done (phase mapping)
    task.phase !== "pull_request" &&   // ← blocks pull_request
    task.status !== "done" &&          // ← blocks done
    task.status !== "pull_request";    // ← blocks pull_request
```

## Proposed Change

Remove the `done` and `pull_request` exclusions from both `canMoveToBacklog` computations.

**`SpecTaskDetailContent.tsx` after change:**
```ts
const canMoveToBacklog =
    task &&
    !isQueued &&
    task.status !== "backlog" &&
    !isTaskArchived;
```

**`TaskCard.tsx` after change:**
```ts
const canMoveToBacklog =
    !isQueued &&
    task.phase !== "backlog" &&
    task.status !== "backlog";
```

## Key Decisions

1. **No backend changes.** The `updateSpecTask` handler (`spec_driven_task_handlers.go` ~line 702) sets whatever status is sent. No transition validation exists server-side, so none needs adding.

2. **Keep the queued exclusion.** Tasks in `queued_implementation`, `queued_spec_generation`, or `spec_approved` are mid-transition (an agent is about to pick them up). Moving these to backlog mid-flight could cause race conditions. The existing "Remove from queue" action handles this case separately.

3. **Keep the archived exclusion.** Archived tasks are deliberately hidden. Users should unarchive first.

4. **The existing `useMoveToBacklog` hook already handles agent cleanup.** It calls `stopAgent` before setting status to backlog, so no new cleanup logic is needed for `pull_request` or `done` states.

5. **Merged tasks going back to backlog is intentional.** The user accepts that re-planning a merged task may produce a second PR from the same branch. This is fine — Git handles it naturally.

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Remove `done` and `pull_request` from `canMoveToBacklog` guard |
| `frontend/src/components/tasks/TaskCard.tsx` | Remove `done`, `pull_request`, and `completed` phase from `canMoveToBacklog` guard |

## Risks

- **Low risk.** This only removes UI restrictions. The backend path is already exercised by other statuses using the same `useMoveToBacklog` hook.
- A user could accidentally move a done task to backlog. This is acceptable — they can always re-mark it done or the workflow handles it naturally.