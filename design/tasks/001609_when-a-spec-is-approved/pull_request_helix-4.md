# Auto-navigate to workspace after spec approval

## Summary
When a spec is approved from the review view, the app now automatically navigates back to the workspace/chat view so the user can monitor implementation progress without extra clicks.

## Changes
- `SpecTaskReviewPage.tsx`: after approval (`onImplementationStarted`), navigate to `project-specs` workspace with `openTask` param instead of staying on `project-task-detail`
- `TabsView.tsx`: after approval in the workspace review tab, close the review tab and open/switch to the parent task's chat tab so the user sees the agent's activity
