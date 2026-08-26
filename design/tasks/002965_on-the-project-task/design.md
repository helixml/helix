# Design: Show Only Active Branches When Creating Project Tasks

## Current State

`NewSpecTaskForm.tsx` calls `GET /api/v1/git/repositories/{id}/branches`, receives only branch-name strings, removes the default branch, and sorts the remainder. The generic endpoint cannot describe merge state. Spec tasks record `MergedToMain`, `MergedAt`, and `LastPushCommitHash`. The existing `IsBranchMerged` ancestry check is not sufficient here: a fresh branch created at the default tip looks identical to a fully merged branch in the commit graph. External reads sync branches first, but `SyncAllBranches` intentionally disables pruning, so an upstream-deleted branch may remain locally.

## Design

Update the existing repository branches API to return active branches. The task form continues using the same endpoint and response shape, without a new query mode or frontend API contract.

Under the repository's existing read lock and external-sync flow, the backend will:

1. Resolve the repository's configured default branch.
2. List candidate branch refs, excluding the default branch and `helix-specs`.
3. For external repositories, compare candidates with the authoritative upstream branch names so stale local refs deleted upstream are excluded without globally pruning local-only Helix refs.
4. Collect known merged head commits by source branch from Helix task merge records.
5. Hide a candidate only when its current tip equals a known merged head commit for that same branch. Treat candidates without matching merge evidence as active.
6. Return active branch names in the existing string-array response shape.

Matching the current tip to the recorded merged head handles both sides of the requirement: a fresh branch at the default tip has no merge record and stays visible, while an unchanged merged branch is hidden. If new commits are pushed after merging, the tip no longer matches and the branch becomes active again. Keep alphabetical ordering in the frontend to preserve current presentation.

## Key Decisions and Constraints

- No time threshold is used. A matching merged head remains inactive until the branch advances.
- Do not enable blanket fetch pruning: `helix-specs` and other local-only work can legitimately be absent upstream.
- Prefer false positives in the selector over false negatives: without affirmative merge evidence, keep the branch visible.
- Preserve authorization, locking, and sync behavior of the current branches handler.

## Testing

Service/handler tests should cover a fresh branch equal to the default tip, an unchanged branch with a matching merge record, a merged-then-advanced branch, a branch with no merge metadata, and an external branch missing upstream but present locally. Frontend tests should verify the existing request, rendered options, ordering, and empty/error behavior.

## Implementation Notes

- Review correction: repository activity must not depend on Helix project/task records because branches can be created remotely. The implementation will use merged pull-request source branch/head metadata from the repository provider instead. Direct merges without provider evidence remain visible conservatively.
- External repositories are enumerated with authoritative upstream head refs because the local bare mirror intentionally retains unpruned Helix-only refs. Internal repositories continue to use their local refs.
- Missing merge evidence is handled conservatively: the branch remains visible.
- Targeted backend tests for active-branch filtering and upstream-ref parsing pass, as does the existing two-test `NewSpecTaskForm` suite. The full services package passes. The full server package still has two unrelated pre-existing failures in `TestInProcClient_DeleteLinkedAgentPreservesConfiguredProjectAndUnsetsAgentID` and `TestInProcClient_DeleteLinkedAgentContinuesWhenSessionStopFails`.
