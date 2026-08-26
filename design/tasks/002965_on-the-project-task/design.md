# Design: Show Only Active Branches When Creating Project Tasks

## Current State

`NewSpecTaskForm.tsx` calls `GET /api/v1/git/repositories/{id}/branches`, receives only branch-name strings, removes the default branch, and sorts the remainder. The generic endpoint cannot describe merge state. The backend already provides `GitRepositoryService.IsBranchMerged`, which checks whether a branch tip is an ancestor of the target branch with Git merge-base semantics. External reads sync branches first, but `SyncAllBranches` intentionally disables pruning, so an upstream-deleted branch may remain locally.

## Design

Add an opt-in active-only mode to the repository branches API rather than changing the existing endpoint's default behavior. The task form requests active branches for the project's primary repository; existing callers continue to request all branches.

Under the repository's existing read lock and external-sync flow, the backend will:

1. Resolve the repository's configured default branch and its current tip.
2. List candidate branch refs, excluding the default branch and `helix-specs`.
3. For external repositories, compare candidates with the authoritative upstream branch names so stale local refs deleted upstream are excluded without globally pruning local-only Helix refs.
4. Mark a candidate active when its tip is not an ancestor of the default-branch tip.
5. Return active branch names in the existing string-array response shape.

This Git-level check deliberately uses the current tip rather than the task's stored `MergedToMain` flag. Stored merge state belongs to one task and becomes stale if a branch receives later commits; tip reachability correctly makes such a branch active again. Keep alphabetical ordering in the frontend to preserve current presentation.

## Key Decisions and Constraints

- The filter is opt-in to avoid changing Git repository detail screens or other consumers of the all-branches endpoint.
- No time threshold is used. “Active” describes unmerged work now, not how recently a merge occurred.
- Do not enable blanket fetch pruning: `helix-specs` and other local-only work can legitimately be absent upstream.
- The existing ancestry helper detects merge-commit and fast-forward merges. Squash/rebase merge handling remains dependent on the answer in `requirements.md`.
- Preserve authorization, locking, and sync behavior of the current branches handler.

## Testing

Service/handler tests should construct histories for unmerged, fully merged, and merged-then-advanced branches, plus simulate an external branch missing upstream but present locally. Frontend tests should verify the active-only request, rendered options, ordering, and empty/error behavior.
