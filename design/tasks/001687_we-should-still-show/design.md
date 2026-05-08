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

## Implementation Notes

- Made the one-line change at `frontend/src/components/tasks/SpecTaskActionButtons.tsx:616`.
- Verified with `npx tsc --noEmit` — passes cleanly.
- `yarn build` failed on a write-permission issue under `frontend/dist/external-libs` (pre-existing env constraint, not a code issue). 21,406 modules transformed cleanly before the dist write step. Per CLAUDE.md the dev container at port 8081 hot-reloads `frontend/src/`, so no rebuild is needed for live testing.
- The dev environment was not running during implementation (no docker containers up), so live UI testing was not performed in this session. The change is a one-line condition extension with no behavior change for the existing `pull_request` status path; risk is minimal.
- The error-alert branch at line 606 (`task.status === "pull_request" && !hasAnyPR && task.metadata?.error`) is intentionally NOT extended — that error message is only meaningful while PR creation is being attempted, not after the task is done.
