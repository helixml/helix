# Show only active task branches

## Summary

Filter the repository branch list used by project task creation so completed and deleted branches are not offered, while fresh and reactivated branches remain available.

## Changes

- Hide branch tips that match merged pull-request evidence from GitHub, GitLab, or Azure DevOps.
- Keep fresh, unknown, and post-merge advanced branches visible.
- Read external branch names from authoritative upstream refs so deleted branches are excluded.
- Exclude the default and `helix-specs` branches.

## Testing

- Added and passed targeted backend tests for merge-state filtering and upstream-ref parsing.
- Passed `NewSpecTaskForm.test.tsx` (2 tests).
- Passed the full services package; the full server package has two unrelated existing in-process client test failures documented in the design notes.
