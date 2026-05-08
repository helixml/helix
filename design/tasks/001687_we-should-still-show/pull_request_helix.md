# Show pull request links on finished spec tasks

## Summary

When a spec task is marked `done` (merged), its pull request links currently disappear from the task detail page. This makes it impossible to find which PR was associated with a finished task. This PR keeps the existing PR button(s) visible after the task transitions to `done` so users can navigate back to the PR for review history, comments, or code.

## Changes

- `frontend/src/components/tasks/SpecTaskActionButtons.tsx`: extend the PR-button render condition from `task.status === "pull_request"` to also include `task.status === "done"`. No other UI changes — single-PR and multi-PR rendering paths handle the new status automatically.

## Notes

- The error-alert branch (shown when PR creation fails) is intentionally left bound to the `pull_request` status — that message is only meaningful during the PR creation phase.
- `SpecTaskActionButtons` is shared between the full task detail page and inline task cards, so the fix propagates to both.
