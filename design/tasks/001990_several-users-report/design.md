# Design: Server-side reconcile on Approve (no agent round-trip)

## Approach (root cause, not workaround)

The current code refuses to do a real merge. We make it do one.

- **Implementation approve**: when fast-forward fails because `main` advanced, the
  server merges `main` into the feature branch in a worktree of the bare repo, then
  fast-forwards `main` to the new feature-branch tip, then pushes upstream (external
  repos). This is the same git operation the agent is currently asked to perform via
  chat — done in-process, synchronously, without a UX round-trip.
- **Spec approve**: when `SyncBaseBranch` finds the local mirror is behind upstream,
  fast-forward it. When it's diverged, treat upstream as authoritative for the base
  branch and reset the local mirror to upstream (with a warning log).

The agent is only involved when the server-side merge produces **real textual
conflicts**. That branch keeps the existing prompt-the-agent-and-wait-for-push flow,
but the user no longer has to click Accept twice — the next push triggers a
push-driven retry.

This removes the second click in the *common* case (no conflicts), and makes it
unnecessary in the *rare* case (real conflicts), without papering over the symptom.

## Why not the original "intent flag + retry" plan

My first draft added an `AwaitingMergeRetry` field on `SpecTask`, a new endpoint, new
frontend state. That treats the symptom (the two-click UX) without fixing the cause
(fast-forward-only merge). The retry-on-push hook is still useful for the conflict
path, so it stays — but it stops being the *primary* mechanism.

## Affected Code

### Backend — implementation approve

- `api/pkg/services/git_helpers.go`
  - Add `MergeBranchWithMergeCommit(ctx, repoPath, sourceBranch, targetBranch, signature)`.
    Implementation: open a working clone of the bare repo (existing pattern at
    `git_repository_service.go:1173-1242`), `git checkout sourceBranch`, `git merge
    targetBranch -m "..."`. If exit code is non-zero with conflict markers, return a
    typed `MergeConflictError{ConflictedFiles []string}`. If clean, push the result back
    to the bare repo's `sourceBranch` ref. Then in the bare repo, `MergeBranchFastForward`
    target → source as today.
  - Keep `MergeBranchFastForward` — used as the fast path when ancestor check passes.
- `api/pkg/server/spec_task_workflow_handlers.go::approveImplementation`
  - After `MergeBranchFastForward` fails, **do not** revert status / send agent prompt /
    return early. Instead call `MergeBranchWithMergeCommit` (new helper) using the
    approving user's signature.
  - On clean merge: continue down the existing finalize path (push to upstream for
    external, set `MergedToMain`/`MergedAt`/`Status=done`, trigger golden build).
  - On `MergeConflictError`: revert status to `implementation_review`, send the existing
    agent rebase prompt **with the list of conflicted files prepended**, return
    `200` with the conflicted-files list in the response body. (This is the only
    path that still requires later push-driven retry — see next bullet.)
- `api/pkg/services/git_http_server.go::handleFeatureBranchPush`
  - When the pushed branch belongs to a task that previously hit a real conflict on
    approve (`task.Status == implementation_review` *and* the task carries a recorded
    approval intent — see field below), re-run the same approve-implementation merge
    path inline. This path is reused, not a parallel implementation.
- `api/pkg/types/simple_spec_task.go`
  - Add **one** field: `PendingMergeConflict bool` (or equivalent). Set when the merge
    above hits a real conflict; cleared when the retry succeeds or when the user clicks
    Cancel approval / explicitly stops. This is required only for the conflict-retry
    path; it's not used for the common no-conflict case.

### Backend — spec approve

- `api/pkg/services/git_repository_service_pull.go::SyncBaseBranch`
  - Replace the current "diverged → return `BranchDivergenceError`" with:
    - Behind only: fast-forward (existing).
    - Strictly ahead (local has commits upstream doesn't): refuse with the existing
      error — local should never be ahead of upstream for a base branch in normal
      operation; surfacing it is correct.
    - Diverged: log a warning, force-update the local ref to upstream's commit. Upstream
      is authoritative for the project's base branch.
  - `BranchDivergenceError` and `FormatDivergenceErrorForUser` stay for the
    "strictly ahead" case and for the explicit-sync UI button (story 2's Force Sync
    escape hatch).
- `api/pkg/services/spec_driven_task_service.go::ApproveSpecs`
  - No code change required if `SyncBaseBranch` no longer returns
    `BranchDivergenceError` for the diverged case. The existing error-formatting branch
    becomes effectively dead for the spec-approve flow and can be removed.

### Frontend

- `frontend/src/services/specTaskWorkflowService.ts::useApproveImplementation`
  - Drop the `"Branch has diverged - agent is rebasing. Click Accept again..."` warning
    branch. Replace with: when the response indicates a real conflict (new flag in
    response), show *"Merge conflict detected — agent is resolving. The merge will
    complete automatically once the agent pushes a fix."* — informational, no CTA.
  - The successful no-conflict-but-needed-merge case returns `status === "done"` /
    `pull_request` exactly like today; no UI change needed.
- `frontend/src/components/tasks/SpecTaskActionButtons.tsx` /
  `frontend/src/components/tasks/TaskCard.tsx`
  - When `task.PendingMergeConflict` is true, hide the **Accept** button and show
    "Resolving conflict…" inline.
- No new endpoints required. Cancel-approval is *not* needed in this design — there
  is nothing to cancel in the no-conflict path, and the conflict-retry path is short
  and self-correcting (push fails or succeeds; user can stop the agent via existing
  controls).

## Server-side flow

```
approveImplementation:
  ... existing auth/OAuth/status checks (unchanged) ...

  for external repo:
    acquire repo lock
    SyncAllBranches (best effort)
    capture oldDefaultBranchRef for rollback

  attempt MergeBranchFastForward(feature → main)
  if fast-forward succeeded:
    finalize: push upstream (external), set Status=done, MergedAt, MergedToMain, golden build
    return done

  attempt MergeBranchWithMergeCommit(feature ← main)   // NEW: merge main into feature
  if clean:
    fast-forward main → feature                        // now an ancestor
    finalize: push upstream (external), set Status=done, MergedAt, MergedToMain, golden build
    return done

  // MergeConflictError
  set Status=implementation_review
  set PendingMergeConflict=true
  send agent rebase prompt with conflicted files listed
  return task with conflict info  → frontend renders informational toast

handleFeatureBranchPush (only matters for the conflict path):
  if task.Status == implementation_review && task.PendingMergeConflict:
    re-run approveImplementation finalize logic
    on success: clear PendingMergeConflict, Status=done
    on failure: leave PendingMergeConflict, write metadata.error, leave Status=implementation_review
```

## Key decisions

1. **Real merge, server-side, in process.** Existing pattern (`git_repository_service.go`
   tempClone block) shows how to safely use a working clone over a bare repo. We reuse
   it. No `os/exec("git rebase")` shell-out from the handler.

2. **Merge commit, not rebase.** CLAUDE.md mandates merge commits ("**NEVER** squash
   merge — always use regular merge commits"). Rebasing the feature branch would rewrite
   history that has been pushed and visible. Merging `main` into the feature branch
   produces an extra merge commit on the feature branch, then a clean fast-forward of
   `main` — exactly the result the project has chosen for everything else.

3. **Use the approving user's signature for the merge commit.** They are the person
   approving; the commit log should reflect that, not the agent's identity nor a
   service account.

4. **Push-driven retry stays for conflicts only.** Without it, a real-conflict workflow
   would still need two clicks. The new field gates retry to *only* the conflict path
   and is set/cleared synchronously inside the same handler, not at random points in
   the lifecycle.

5. **No new endpoint, no Cancel-approval UI.** The earlier draft added both. Neither
   is needed once the server resolves the merge itself.

6. **Spec-approve sync: upstream is authoritative.** A Helix-managed mirror should not
   block on upstream divergence — upstream is, by definition, the canonical source of
   truth for the base branch. Treating "diverged" as "force-update local to upstream"
   matches what every other CI system does and matches user expectations. The
   `BranchDivergenceError` path stays for "local is strictly ahead" (genuinely anomalous)
   and for the explicit Force Sync button.

## Edge cases to cover in tests

- Fast-forward path: agent pushed clean, no upstream advance — unchanged behavior,
  task lands in `done`.
- Diverged-no-conflict: agent's branch and `main` both advanced touching different
  files — server-side merge produces merge commit, task lands in `done`. **This is the
  case that currently shows the bad toast.**
- Diverged-with-conflict: same file edited on both sides — task held with
  `PendingMergeConflict=true`, agent prompt names the file, agent push triggers retry.
- External-push-fails after local merge: rollback to `oldDefaultBranchRef` (existing
  behavior preserved), error returned to user, no zombie merge commit on `main`.
- Concurrent reviewers click Accept simultaneously: atomic status transition prevents
  double-merge / double-PR. Pattern: `TransitionSpecTaskStatus` already used by
  `ApproveSpecs` (`spec_driven_task_service.go:1294-1305`).
- Spec-approve against an external repo whose `main` advanced upstream: `SyncBaseBranch`
  fast-forwards (no error), `ApproveSpecs` proceeds.
- Spec-approve when local `main` is genuinely ahead of upstream (anomaly): error
  surfaces as today, with `BranchDivergenceError` and Force Sync guidance.

## Notes for the implementing agent

- Existing prior art for "spawn temp working clone of bare repo, do work, push back":
  `api/pkg/services/git_repository_service.go:1173-1242`. Reuse the pattern, don't
  reinvent.
- gitea library exposes most of what's needed (`giteagit.OpenRepository`,
  `gitcmd.NewCommand` for raw git plumbing). The merge itself is most easily done
  via `gitcmd.NewCommand("merge", ...)` inside the temp working clone — gitea's
  high-level API doesn't have a clean `Merge` wrapper.
- Honor `s.gitRepositoryService.GetRepoLock(repo.ID)` for the entire flow as today.
- `agent_rebase_required.tmpl` (in `api/pkg/prompts/templates/`) needs a small change:
  the closing line that says "tell the user they need to click Accept again" must be
  removed — the merge will complete automatically on the agent's next push.
- Related historical context: `001849_fix-pr-merge-issues-when` covers post-merge UI
  state; `2026-01-27-helix-specs-sync-divergence.md` covers the helix-specs branch's
  own sync (different code path — do not conflate).
- Don't add backwards-compatibility shims for the old "click Accept again" path. Per
  CLAUDE.md "NO FALLBACKS — one approach, fix properly, no dead code paths" — delete
  the old branch outright once the new one lands.
