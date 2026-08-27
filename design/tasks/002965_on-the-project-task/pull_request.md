# Show only active branches for project tasks

## Summary

Filter the repository branch list used by project task creation using only the synced local Git mirror. Deleted upstream branches, the default branch, `helix-specs`, and unchanged branch tips recorded as non-first parents of default-branch merge commits are omitted. Fresh branches, branches advanced after merging, and uncertain squash/rebase/fast-forward cases remain visible. The implementation does not depend on Helix project records or provider APIs.

## Testing

Added targeted backend tests for upstream-ref parsing and active-branch filtering, including a real temporary Git repository covering a normal merge, a fresh branch at the default tip, and a branch advanced after merging. All targeted backend tests passed, and `NewSpecTaskForm.test.tsx` passed with 2 tests.
