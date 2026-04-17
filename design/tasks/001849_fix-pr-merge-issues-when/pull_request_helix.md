# Fix PR merge issues: hide stale error on completed tasks, prevent duplicate PR creation

## Summary
Two bugs in the spec task PR workflow:
1. When a PR gets merged, the task card shows a stale error banner ("Pull request could not be created...") alongside the green "Task finished" message
2. When a PR is closed/deleted on GitHub, Helix creates a duplicate PR for the same branch

## Changes
- `frontend/src/components/tasks/TaskCard.tsx`: Hide error banner when task phase is "completed" (error preserved in metadata for debugging)
- `api/pkg/server/spec_task_workflow_handlers.go`: Add early return in `ensurePullRequestForRepo()` when the task already tracks a PR for the repo, preventing duplicate creation when the GitHub API (which only returns open PRs) can't see closed ones
