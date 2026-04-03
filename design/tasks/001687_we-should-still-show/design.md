# Design: Show Pull Requests on Finished Tasks

## Root Cause

In `frontend/src/components/tasks/SpecTaskActionButtons.tsx`, pull request buttons are rendered inside a guard:

```typescript
if (task.status === "pull_request" && hasAnyPR) {
  // render PR button(s)
}
```

When `status` transitions to `"done"`, this condition fails and the PR section renders nothing (falls through to `return null`).

## Fix

Extend the status check to also include `"done"`:

```typescript
if ((task.status === "pull_request" || task.status === "done") && hasAnyPR) {
  // render PR button(s) — same JSX, no other changes
}
```

No other changes are needed. The PR data (`repo_pull_requests`) is already persisted on the task and passed to this component. The existing single-PR and multi-PR rendering paths handle both statuses correctly.

## What Does NOT Change

- PR data structure — already stored and passed down
- Button appearance — same "View Pull Request" / dropdown UI
- Error alert for failed PR creation — only shown during `pull_request` status (no change needed)

## Pattern Note

`SpecTaskActionButtons` is a shared component used in both the full task detail page (`SpecTaskDetailContent.tsx`) and inline card views. The single status check fix propagates to both automatically.
