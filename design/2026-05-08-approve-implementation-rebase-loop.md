# Approve-implementation rebase loop

**Date:** 2026-05-08
**Status:** in progress
**Branch:** `worktree-fix-approve-rebase-loop`

## Problem

When a user clicks Accept on a spec task whose feature branch has diverged from
the default branch, the UI shows:

> Branch has diverged - agent is rebasing. Click Accept again after rebase completes.

and the task flips to `implementation_review`. Clicking Accept again does the
same thing, every time, regardless of whether the agent has finished rebasing.
Users get stuck in a loop with no signal that any progress is being made.

This affects internal-git repos and any external repo where
`shouldOpenPullRequest` returns false (i.e. any path that takes server-side
fast-forward merge in `approveImplementation` rather than the PR path).

## Reproduction (prime, 2026-05-08)

Setup:
1. Project `pri` on prime org `priya`, internal repo `code-pri-1773665635`,
   default branch `main`.
2. In the bare repo on disk, create a `feature/test-rebase-loop` branch and a
   divergent commit on `main` such that neither is an ancestor of the other.
3. Stage spec task `spt_01kkvcmf2nxf4ac8c9sxym2mgr` with
   `status=implementation`, `branch_name=feature/test-rebase-loop`,
   `last_push_at=now`.
4. Hit `POST /api/v1/spec-tasks/.../approve-implementation` three times in a
   row.

Result: every call returns `status=implementation_review`, `last_push_at`
unchanged, `status_updated_at` advancing each call. Backend logs show
`Fast-forward merge failed - asking agent to rebase` plus a fresh rebase prompt
queued for the agent on every click.

When the feature ref is then advanced to a true descendant of main (simulating
a successful agent rebase + push), the next click succeeds and the task
transitions to `done`. So the success path works — only the loop and the
missing progress signal need fixing.

## Root cause

Three defects in `approveImplementation`
(`api/pkg/server/spec_task_workflow_handlers.go:262-315`) and the push handler
(`api/pkg/services/git_http_server.go:975-1009`):

1. **No idempotency.** Every Accept click while in `implementation_review`
   re-runs the FF attempt and re-queues a rebase instruction at the agent. With
   `interrupt=false` these queue up and confuse the agent.
2. **No progress signal to the FE.** `handleFeatureBranchPush` only updates
   `LastPushAt` when status is `implementation`; pushes that arrive while the
   task sits in `implementation_review` (i.e. the rebase push) are ignored.
3. **No auto-retry on agent push.** Even when the agent does push a successful
   rebase, the server waits for the user to click Accept yet again. The user
   is the synchronisation primitive, but they have no signal telling them when
   the rebase landed.

## Design

Two layers, shipped together. Layer 3 (server-side three-way merge) is left
for follow-up.

### Layer 1 — backend

**a. New `RebaseRequestedAt *time.Time` field on `SpecTask`.**

Set when the FF-failure branch is taken; consulted on subsequent calls to
decide whether the rebase is "still pending" or "ready to retry."

In `approveImplementation`, after computing the merge result:

```
if mergeErr != nil {
    // Did we already ask for a rebase, and has the agent not pushed since?
    rebasePending :=
        specTask.RebaseRequestedAt != nil &&
        (specTask.LastPushAt == nil ||
         !specTask.LastPushAt.After(*specTask.RebaseRequestedAt))

    if rebasePending {
        // Idempotent: do not re-send prompt, just return current state.
        writeResponse(w, specTask, http.StatusOK)
        return
    }

    // First-time fail OR agent has pushed since: send prompt and stamp.
    specTask.Status = TaskStatusImplementationReview
    specTask.RebaseRequestedAt = &now
    ... (existing save + send rebase prompt code)
}
```

**b. Auto-retry FF in `handleFeatureBranchPush` when status is
`implementation_review`.**

```
case types.TaskStatusImplementationReview:
    // Update LastPushAt so FE can show progress + idempotency check works.
    task.LastPushCommitHash = commitHash
    task.LastPushAt = &now
    s.store.UpdateSpecTask(ctx, task)

    // Best-effort: try FF again. If it works, advance to done.
    s.tryAutoMergeAfterRebase(ctx, task)
```

`tryAutoMergeAfterRebase` is a new helper that:
- Re-fetches the task, the project's default repo.
- For external repos: `SyncAllBranches` then attempt FF.
- For internal repos: attempt FF directly.
- On success: stamp `ImplementationApprovedBy = "system"`,
  `ImplementationApprovedAt`, `MergedToMain`, `MergedAt`, `Status = done`,
  push to upstream if external. Trigger the same golden-build hook the
  user-driven path triggers.
- On failure: leave the task in `implementation_review`, log the conflict.
  Do NOT re-send a rebase prompt — the user can intervene.

This is the key change: most divergence cases resolve themselves without a
second user click. The user's first Accept becomes the only thing they need to
do.

### Layer 2 — frontend

`frontend/src/services/specTaskWorkflowService.ts:23-26` — the existing snackbar
copy "Click Accept again after rebase completes" goes away. Replacement:

- If response status is `implementation_review` and `rebase_requested_at` is
  set: show "Branch has diverged from main. Agent is rebasing — this will
  complete automatically." (info, not warning.)
- The Accept button on the task detail page is disabled while
  `status == implementation_review` and `last_push_at <= rebase_requested_at`.
  As soon as `last_push_at` advances past `rebase_requested_at` (and the auto-
  retry hasn't completed yet), enable Accept again as a manual escape hatch.

### Migration

`RebaseRequestedAt` is a new nullable `timestamp with time zone` column.
GORM AutoMigrate handles it (consistent with how `LastPushAt` was added).

## Out of scope

- **Server-side three-way merge** to eliminate the agent round-trip entirely
  for non-conflicting divergence. Worth doing later — uses
  `git merge-tree --write-tree` + `git commit-tree` on the bare repo. Would
  reduce the entire flow to a single click in the common case, with the agent
  only involved when there are real conflicts.
- **GitLab divergence handling**. GitLab repos go through the PR path
  (`shouldOpenPullRequest()=true`); divergence there surfaces in the GitLab UI
  as "MR can't be merged" and is the user's problem to fix in GitLab. Not the
  same loop.

## Test plan

Unit:
- New test in `spec_task_workflow_handlers_test.go` (or sibling) that
  - sets up a divergent feature branch
  - calls approve once → asserts status flipped, `RebaseRequestedAt` set
  - calls approve again immediately → asserts no second prompt was sent
    (assert via mock on the agent message sender call count)
- Test the auto-retry branch in `handleFeatureBranchPush`:
  - task in `implementation_review`, push arrives, branch is now FF-mergeable
    → task ends in `done`
  - same setup but branch still divergent → task stays in
    `implementation_review`, no extra rebase prompt sent

Manual on prime, repeating the repro from above:
- After fix: three Accept clicks while branch is divergent → first one returns
  `implementation_review` and sends the prompt; subsequent two return the
  same state but the API logs show NO further `asking agent to rebase` lines.
- Simulated agent rebase push → task auto-transitions to `done` without a
  user click.
