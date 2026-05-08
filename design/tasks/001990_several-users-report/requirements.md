# Requirements: Server-side reconcile so "Branch has diverged" stops happening

## What users see

Approving a finished implementation often pops:

> Branch has diverged - agent is rebasing. Click Accept again after rebase completes.

They have to wait, watch for the agent, then click **Accept** again. If they click too soon
they get the same toast. If they don't notice the agent finished, the task sits in
`implementation_review` indefinitely. Reported on **internal Helix-hosted git** *and*
**external GitLab**.

A second, related complaint surfaces when the user **approves a spec** while the project's
`main` has moved upstream. That path doesn't show the same toast — it shows the
`FormatDivergenceErrorForUser` block ("Cannot sync base branch… use Force Sync to
overwrite local with upstream"). The user reports both as "branch has diverged" because
that's the phrase they see; both must be addressed.

## Root cause

Two distinct fast-forward-only assumptions:

- **Implementation approve** (`api/pkg/server/spec_task_workflow_handlers.go:262-315`):
  `MergeBranchFastForward` is fast-forward-only. The moment `main` advances after the
  feature branch was created (another task lands, an external dev pushes, an operator
  hot-fixes), fast-forward is impossible. The handler asks the agent to rebase via chat
  prompt and tells the user to click again.
- **Spec approve** (`api/pkg/services/spec_driven_task_service.go:1233-1241`):
  `SyncBaseBranch` performs only a fast-forward of the *local mirror* of `main` from
  upstream. The moment Helix's local copy diverges from upstream `main` (e.g. Helix
  pushed a previous merge directly while upstream was reorganised), the sync errors out
  and the user is told to "Force Sync" or fix it externally.

Both paths refuse to do the obvious thing: a real merge.

The project's own merge policy (CLAUDE.md) is **always merge commits, never squash**.
That policy is incompatible with fast-forward-only. The current code is enforcing a
linear-history rule the rest of the project doesn't follow.

## User Stories

### 1. Implementation approve survives a divergence with one click

**As** a reviewer clicking **Accept** on a completed task,
**I want** the merge to complete on its own,
**So that** I don't have to watch the agent or click again.

**Acceptance Criteria:**
- Single click on **Accept** results in either the task transitioning to `done`
  (internal repo) / `pull_request` (external repo), or a clear, actionable error.
- No "Click Accept again" wording exists anywhere in the success or recoverable-error
  paths.
- When `main` has advanced and the feature branch is no longer fast-forwardable, the
  server performs a real merge of `main` into the feature branch (or the equivalent
  rebase) **server-side** in the bare repo, then completes the merge. The agent is **not**
  involved unless there is a true textual conflict.
- For external repos, the merged result is pushed upstream as part of the same
  request. If the push fails, the local merge is rolled back (existing behavior preserved).

### 2. Spec approve survives a local-mirror divergence

**As** a user approving a spec on a project whose external `main` has moved,
**I want** Helix to reconcile its local mirror with upstream and proceed,
**So that** approval is not blocked by a "Force Sync" instruction every time the
external repo gets a normal push.

**Acceptance Criteria:**
- When `SyncBaseBranch` detects local `main` is behind upstream and not diverged
  (upstream is strictly ahead), it fast-forwards local `main` to upstream and proceeds —
  no error.
- When local `main` and upstream `main` have *both* moved (true divergence), Helix
  treats upstream as authoritative for the base branch (it's the project's source of
  truth) and updates the local mirror to upstream's commit, logging a warning. The
  spec-approval flow proceeds.
- "Force Sync" remains available as the manual escape hatch for cases where the
  operator explicitly does not want this behavior, but it's no longer the *only* way
  through.

### 3. Real conflicts still surface clearly

**As** a reviewer whose feature branch genuinely conflicts with `main`,
**I want** a clear, actionable error,
**So that** I know the agent (or I) need to resolve files by hand.

**Acceptance Criteria:**
- When the server-side merge has actual textual conflicts, the task is held in
  `implementation_review`, the conflicted files are surfaced in the error response, and
  the agent is sent a prompt that names the conflicting files.
- The agent's rebase prompt is updated: it no longer instructs the user to click
  Accept again; it instructs the agent to resolve and push, and the server's next push
  handler completes the merge automatically. (See story 1 — push-driven retry is the
  fallback path, not the primary one.)
- If a retry on push still fails, the task is left in `implementation_review` with the
  error preserved; subsequent user clicks are accepted (idempotent) and re-attempt the
  merge.

### 4. Two reviewers / double-clicks are safe

**As** an operator,
**I want** approval to be idempotent,
**So that** simultaneous clicks or accidental double-submits don't corrupt state.

**Acceptance Criteria:**
- Concurrent calls to `approveImplementation` for the same task do not double-merge
  or open duplicate PRs. The atomic status transition pattern already used by
  `ApproveSpecs` (`TransitionSpecTaskStatus`) is the model.
- A click on a task already in `done` is a no-op returning the current task.

## Out of Scope

- Changing the *user-visible* merge result (merge commit vs. fast-forward vs. squash) —
  story 1 deliberately allows whichever git operation reconciles cleanly. We do *not*
  introduce a UI for choosing strategy.
- Conflict resolution UI (file-level pickers etc.).
- Replacing the agent rebase prompt entirely — it stays for the conflict path.
- Auto-merging arbitrary upstream pushes back into in-flight feature branches before
  approval (story 1 only triggers reconcile *at approve time*).
