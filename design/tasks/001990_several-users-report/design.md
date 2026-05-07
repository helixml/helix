# Design: Auto-resolve "Branch has diverged" on Approve

## Approach

Capture the user's approval intent on the **first** click. Trigger the agent rebase as
today, but on the agent's next push **automatically retry the merge server-side** and
finalize the task. The user clicks once and either gets the success toast or a clean
failure — never "click again".

## Affected Code

### Backend
- `api/pkg/server/spec_task_workflow_handlers.go`
  - `approveImplementation` (around line 29) — record approval intent before the
    fast-forward attempt, and on `MergeBranchFastForward` failure return a friendlier
    response that the frontend can render as informational rather than as a CTA.
- `api/pkg/services/git_http_server.go`
  - `handleFeatureBranchPush` (line 955) — when the pushed branch belongs to a task in
    `implementation_review` *with approval intent recorded*, run the same finalize-merge
    code path that `approveImplementation` runs after a successful fast-forward.
- `api/pkg/types/simple_spec_task.go`
  - Add a single boolean / timestamp on `SpecTask` to mark "approval recorded, awaiting
    rebase". Reuse `ImplementationApprovedAt` if possible — it already carries the
    semantic; today it's only set on success. The new flag is needed only if we must
    distinguish "approved and merged" from "approved, awaiting agent". A minimal
    `AwaitingMergeRetry bool` (or `MergePendingSince *time.Time` for observability)
    keeps the existing `ImplementationApprovedAt` semantics intact.

### Frontend
- `frontend/src/services/specTaskWorkflowService.ts:18-46` (`useApproveImplementation`)
  - Replace the `Branch has diverged ... Click Accept again` warning with an info
    snackbar: *"Reconciling with `main` — task will merge automatically when the agent
    finishes."* Drop the "Click again" guidance entirely.
- `frontend/src/components/tasks/SpecTaskActionButtons.tsx`
  - When the task is in `implementation_review` *and* the new "awaiting merge retry"
    flag is set, hide/disable the **Accept** button and show a small inline status
    ("Reconciling with `main`…") plus a "Cancel approval" link.
- `frontend/src/components/tasks/TaskCard.tsx`
  - Same status badge treatment so the card list reflects the in-flight reconcile.

### Cancel path
- New endpoint `POST /api/v1/spec-tasks/{id}/cancel-pending-merge` (or reuse
  `PATCH /spec-tasks/{id}` to clear the flag) — clears the new flag, leaves status
  as `implementation_review`.

## Server-side flow (proposed)

```
approveImplementation:
  ... existing auth + checks ...
  set ImplementationApprovedBy / At / AwaitingMergeRetry = true   // NEW: record intent up-front
  persist
  try fast-forward merge
    success path:  push (external) → status=done, AwaitingMergeRetry=false → return done
    failure path:  send agent rebase prompt → status=implementation_review (intent kept)
                   → return task to UI; UI renders informational toast

handleFeatureBranchPush (agent pushes rebased branch):
  if task.Status == implementation_review && task.AwaitingMergeRetry:
    re-run the post-fast-forward portion of approveImplementation:
      acquire repo lock
      try fast-forward again (now main is an ancestor of feature)
      external: push merged main → if push OK, status=done; else rollback + clear flag + record error
      internal: status=done
    notify the user via existing task-update polling (status flip is enough; no extra channel)
    if retry merge fails for any reason: clear AwaitingMergeRetry, set task.metadata.error,
      keep status implementation_review (so the user gets a fresh, actionable error next click)
```

## Key Decisions

1. **Reuse fast-forward only.** No change to merge strategy. The agent already produces
   a fast-forwardable branch after the rebase prompt — we just have to detect the push
   and finalize, instead of asking the user to click.

2. **Single source of truth for "approval intent".** A new `AwaitingMergeRetry` flag on
   `SpecTask` is preferable to overloading `ImplementationApprovedAt` because the latter
   today implies the merge actually happened (used by the audit log and "merged at" UI).
   Splitting intent from completion keeps the existing semantics intact.

3. **Push-driven retry, not polling.** `handleFeatureBranchPush` already runs on every
   push to a feature branch. Hooking the retry there is free and synchronous with the
   event that unblocks the merge. No new goroutine or timer needed.

4. **Idempotency.** Both endpoints (`approve-implementation` and the implicit retry path)
   must tolerate being called when the task is already `done`. The existing status switch
   at the top of `approveImplementation` already accepts `implementation_review` and
   `implementation`; we just need to ensure the retry path no-ops if `AwaitingMergeRetry`
   is false or if status has moved past `implementation_review`.

5. **No background "did the agent give up?" recovery.** The agent's session may die or the
   rebase may stall. We don't add a watchdog in this task; if the user wants to bail,
   they use the cancel-pending-merge affordance. A watchdog can come later if needed.

6. **Conflicts are still possible.** If the agent's rebase actually conflicts (rare in
   practice — feature branches usually add new files), the agent will either fail to
   push, or push something that still doesn't fast-forward. The retry attempt will fail
   cleanly: clear the flag, surface the error, let the user decide. We do not try to
   resolve conflicts server-side.

## Edge cases to cover in tests

- Two reviewers click **Accept** simultaneously — second click no-ops, no double-merge.
- Agent pushes multiple times during rebase — only the push that produces a
  fast-forwardable feature branch finalizes the merge; earlier pushes are ignored
  (the retry's fast-forward attempt simply fails and leaves the flag set).
- External-repo push of merged `main` fails after the local fast-forward succeeds — same
  rollback path as today (`oldDefaultBranchRef`), plus clear the flag so the user can
  retry.
- User clicks **Cancel approval** while a push is in flight — the retry handler must
  re-read the task before merging and bail if the flag was cleared.
- `LastPushAt` updates without a real branch divergence (agent pushed during normal
  implementation) — retry handler ignores tasks not in `implementation_review`.

## Notes for the Implementing Agent

- The relevant prompt template is `api/pkg/prompts/templates/agent_rebase_required.tmpl`.
  Its current closing line tells the user to click Accept again — update it to match the
  new auto-finalize behavior ("the merge will happen automatically once you push").
- The repo lock pattern used in the merge path (`s.gitRepositoryService.GetRepoLock`) is
  the same lock the retry will need; honor it.
- `detachContext` is used in `approveImplementation` so DB writes survive client
  disconnects — the retry path runs in `handleFeatureBranchPush`'s normal context, so no
  detachment work is needed there.
- Existing related design notes: `001849_fix-pr-merge-issues-when` (post-merge UI) and
  `2026-01-27-helix-specs-sync-divergence.md` (related but different code path). Skim
  for context, do not duplicate logic.
- Frontend invalidates `["spec-tasks", id]` on the approve mutation; the existing query
  re-fetch on status change will pick up `done` after the retry. No new socket needed.
