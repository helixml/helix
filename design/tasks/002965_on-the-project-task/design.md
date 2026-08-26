# Design: Show Only Active Branches When Creating Project Tasks

## Current State

`NewSpecTaskForm.tsx` calls `GET /api/v1/git/repositories/{id}/branches`, receives only branch-name strings, removes the default branch, and sorts the remainder. The generic endpoint cannot describe merge state. Spec tasks record `MergedToMain`, `MergedAt`, and `LastPushCommitHash`. The existing `IsBranchMerged` ancestry check is not sufficient here: a fresh branch created at the default tip looks identical to a fully merged branch in the commit graph. External reads sync branches first, but `SyncAllBranches` intentionally disables pruning, so an upstream-deleted branch may remain locally.

## Design

Update the existing repository branches API to return active branches. The task form continues using the same endpoint and response shape, without a new query mode or frontend API contract.

Under the repository's existing read lock and external-sync flow, the backend will:

1. Resolve the repository's configured default branch.
2. List candidate branch refs, excluding the default branch and `helix-specs`.
3. For external repositories, compare candidates with the authoritative upstream branch names so stale local refs deleted upstream are excluded without globally pruning local-only Helix refs.
4. Read merge commits reachable from the default branch and collect their non-first-parent SHAs.
5. Hide a candidate only when its current tip exactly matches one of those merge-parent SHAs. Treat candidates without matching merge evidence as active.
6. Return active branch names in the existing string-array response shape.

Matching the current tip to a merge parent handles both sides of the requirement: a fresh branch at the default tip stays visible, while an unchanged branch merged with a merge commit is hidden. If new commits are pushed after merging, the tip no longer matches and the branch becomes active again. Keep alphabetical ordering in the frontend to preserve current presentation.

## Key Decisions and Constraints

- No time threshold is used. A matching merge parent remains inactive until the branch advances.
- Do not enable blanket fetch pruning: `helix-specs` and other local-only work can legitimately be absent upstream.
- Prefer false positives in the selector over false negatives: without affirmative merge evidence, keep the branch visible.
- Preserve authorization, locking, and sync behavior of the current branches handler.

## Testing

Service/handler tests cover a fresh branch equal to the default tip, a branch merged with a merge commit, a merged-then-advanced branch, uncertain history, and an external branch missing upstream but present locally. Frontend tests verify the existing selector behavior.

## Implementation Notes

- Final user decision: repository activity must depend only on the synced local Git mirror, not Helix projects or provider APIs. Hide a branch only when its current tip exactly matches a non-first parent of a merge commit reachable from the default branch. Squash, rebase, fast-forward, and otherwise uncertain merges remain visible.
- External repositories are enumerated with authoritative upstream head refs because the local bare mirror intentionally retains unpruned Helix-only refs. Internal repositories continue to use their local refs.
- Missing merge evidence is handled conservatively: the branch remains visible.
- Targeted backend tests, including a real temporary Git repository merge, pass. The existing two-test `NewSpecTaskForm` suite also passes. The full server package previously showed two unrelated failures in `TestInProcClient_DeleteLinkedAgentPreservesConfiguredProjectAndUnsetsAgentID` and `TestInProcClient_DeleteLinkedAgentContinuesWhenSessionStopFails`.
