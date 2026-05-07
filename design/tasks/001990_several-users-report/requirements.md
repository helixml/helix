# Requirements: Auto-resolve "Branch has diverged" on Approve

## Problem

When a user clicks **Accept** on a finished implementation, they often see:

> Branch has diverged - agent is rebasing. Click Accept again after rebase completes.

They then have to:

1. Wait an unknown amount of time for the agent to finish.
2. Notice the agent has finished (no signal in the UI).
3. Click **Accept** a second time.

If they click too early they see the same message again. If they don't notice the agent has finished, the task sits forever in `implementation_review` even though the user already told the system "merge this".

This happens for **internal Helix-hosted git** *and* **external GitLab** (and presumably GitHub/ADO/Bitbucket — the merge path is shared).

The exact toast lives in `frontend/src/services/specTaskWorkflowService.ts:25`, triggered by the
implementation-approval handler in `api/pkg/server/spec_task_workflow_handlers.go:262-315` when
`MergeBranchFastForward` fails because `main` advanced after the feature branch was created.

## Why It Happens

`approveImplementation` only does a *fast-forward* merge. If `main` has new commits the feature
branch doesn't contain (another task's PR landed, an external developer pushed, the operator
hot-fixed `main`), fast-forward is impossible. The handler:

- Reverts task status to `implementation_review`.
- Posts `agent_rebase_required.tmpl` to the agent over the chat channel.
- Returns `200` so the frontend renders the warning toast.
- Does **not** record the approval (`ImplementationApprovedBy`/`At` stay empty).

When the agent later pushes the rebased branch, `handleFeatureBranchPush`
(`api/pkg/services/git_http_server.go:955`) records `LastPushAt` but does nothing else — the
approval intent has been lost, so the user must re-click.

## User Stories

### 1. Approve survives a divergence without a second click

**As** a reviewer clicking **Accept** on a completed task,
**I want** the merge to complete on its own once the branch is reconciled,
**So that** I don't have to babysit the agent and click again.

**Acceptance Criteria:**
- Clicking **Accept** records that the user approved (`ImplementationApprovedBy`,
  `ImplementationApprovedAt` are set on the first click).
- If fast-forward fails, the UI toast is informational, not a call-to-action — e.g.
  *"Reconciling with `main` — task will merge automatically when the agent finishes."*
- When the agent finishes the rebase and pushes, the server retries the merge **without
  another click** and transitions the task to `done` (internal repo) or `pull_request`
  (external repo) as appropriate.
- A second human **Accept** click during the wait is a no-op (idempotent), not an error.
- The retry happens for both internal-merge and external-merge paths.

### 2. Cancel/abandon path

**As** a reviewer who changed their mind during the agent's rebase,
**I want** to be able to cancel or move the task back to `implementation_review`,
**So that** I'm not locked into a merge I no longer want.

**Acceptance Criteria:**
- A "Cancel approval" affordance appears while the task is in the
  rebase-and-merge waiting state. Clicking it clears the recorded approval
  intent and leaves the task in `implementation_review`.
- The agent rebase message is *not* recalled (the rebased branch is harmless to keep), but
  no automatic merge happens once the agent pushes.

### 3. Real merge conflict still surfaces clearly

**As** a reviewer whose branch genuinely conflicts with `main`,
**I want** a clear, actionable error rather than a silent retry loop,
**So that** I know to intervene.

**Acceptance Criteria:**
- If the post-push retry merge still fails (e.g. the agent's "rebase" didn't actually
  reconcile, or there are textual conflicts), the task is moved back to
  `implementation_review` with a visible error explaining what failed.
- The recorded approval intent is cleared so the next user click is fresh.
- Repeated retry/failure does not produce a loop of identical error toasts.

### 4. Spec-approval divergence (out of scope but tracked)

The same words ("branch has diverged") also appear when **approving a spec** against an
external repo whose `main` has moved — that path goes through `SyncBaseBranch` and
returns `BranchDivergenceError` formatted by `FormatDivergenceErrorForUser`, not the
toast in this task. **Not included** in this fix; tracked separately so we know the user
is conflating two distinct flows.

## Out of Scope

- Resolving real textual merge conflicts automatically.
- Changing the merge strategy from fast-forward to merge-commit/squash (history-shape
  decisions belong in their own task).
- Spec-approval divergence (see story 4).
- The "Force Sync" tooling for desynced local mirrors of external `main`.
