# Implementation Tasks

## Backend — implementation approve (the main fix)

- [ ] Add `MergeBranchWithMergeCommit(ctx, repoPath, sourceBranch, targetBranch, signature)` to `api/pkg/services/git_helpers.go`. Use the temp-working-clone pattern from `api/pkg/services/git_repository_service.go:1173-1242`. Return a typed `MergeConflictError{ConflictedFiles []string}` when `git merge` reports conflicts; return nil on a clean merge after pushing the new tip back to the bare repo's `sourceBranch` ref.
- [ ] In `api/pkg/server/spec_task_workflow_handlers.go::approveImplementation`, when `MergeBranchFastForward` fails:
  - Call `MergeBranchWithMergeCommit` with the approving user's `git.Signature` (name + email from `user`).
  - On success: re-attempt `MergeBranchFastForward` (now an ancestor) and fall through to the existing finalize block (push upstream for external repos, set `MergedToMain`/`MergedAt`/`Status=done`, trigger golden build).
  - On `MergeConflictError`: set `Status=implementation_review` and `PendingMergeConflict=true`, send the agent the existing rebase prompt with conflicted-file names prepended, return `200` with the conflict info in the body.
- [ ] **Delete** the old "send rebase prompt + tell the user to click again" branch outright (no fallback per CLAUDE.md). The conflict path replaces it.
- [ ] Add `PendingMergeConflict bool` to `SpecTask` in `api/pkg/types/simple_spec_task.go`; add the GORM AutoMigrate column.
- [ ] In `api/pkg/services/git_http_server.go::handleFeatureBranchPush`, when the pushed branch belongs to a task with `Status==implementation_review && PendingMergeConflict`, re-run the same finalize logic from `approveImplementation` (extract it to a helper to avoid duplication). On success: clear `PendingMergeConflict`. On failure: write `metadata.error`, leave the flag set so a later push can retry.
- [ ] Update `api/pkg/prompts/templates/agent_rebase_required.tmpl`: remove the closing "tell the user they need to click Accept again" line; rephrase to "once you push, the merge will complete automatically".

## Backend — spec approve (story 2)

- [ ] In `api/pkg/services/git_repository_service_pull.go::SyncBaseBranch`, change the diverged branch from "return `BranchDivergenceError`" to "log a warning and force-update the local ref to upstream's commit" — upstream is authoritative for the project's base branch.
- [ ] Keep the "local strictly ahead of upstream" path returning `BranchDivergenceError` (genuine anomaly) and keep `FormatDivergenceErrorForUser` for that case + the explicit Force Sync UI button.
- [ ] In `api/pkg/services/spec_driven_task_service.go::ApproveSpecs`, the existing
  `if divergeErr := GetBranchDivergenceError(err); divergeErr != nil` branch becomes
  effectively dead for normal pushes; remove it (keep the generic error wrap).

## Backend — concurrency

- [ ] Wrap the new `approveImplementation` body in the same atomic status-transition pattern used by `ApproveSpecs` (`TransitionSpecTaskStatus`, `spec_driven_task_service.go:1294-1305`) so simultaneous clicks from two reviewers cannot double-merge.

## Backend — plumbing

- [ ] `./stack update_openapi` after handler/types changes, so the TS API client picks up `PendingMergeConflict`.
- [ ] Go unit tests in `api/pkg/server/`:
  - Fast-forward path still works (regression).
  - Diverged-no-conflict path: `MergeBranchWithMergeCommit` succeeds, task lands in `done`.
  - Diverged-with-conflict path: task held in `implementation_review` with `PendingMergeConflict=true`; conflicted file names appear in the agent prompt and response body.
  - Concurrent Accept: only one merge happens.
  - Push-driven retry on conflict path: clean push → finalize → done.
- [ ] Go unit test in `api/pkg/services/`: `SyncBaseBranch` upstream-ahead → fast-forward; diverged → force-update local; local-ahead → `BranchDivergenceError`.

## Frontend

- [ ] In `frontend/src/services/specTaskWorkflowService.ts::useApproveImplementation`, delete the `"Branch has diverged - agent is rebasing. Click Accept again..."` warning. Add an info-snackbar branch for responses carrying the new conflict info: *"Merge conflict detected — agent is resolving. Merge will complete automatically once the agent pushes a fix."*
- [ ] In `frontend/src/components/tasks/SpecTaskActionButtons.tsx` and `frontend/src/components/tasks/TaskCard.tsx`, when `task.PendingMergeConflict` is true, hide the **Accept** button and show "Resolving conflict…" inline.
- [ ] `cd frontend && yarn build` before committing.

## End-to-end testing in the inner Helix

- [ ] **Diverged-no-conflict** (the headline bug): create a task on a local repo, let the agent push a clean implementation, then push an unrelated commit to `main` directly to simulate divergence, click Accept, verify the task lands in `done` with **no second click and no warning toast**.
- [ ] Repeat against an external GitLab repo if available (or external GitHub) — verify the merged `main` is pushed upstream and the task lands at `done` (internal repo) / `pull_request` (external repo) as appropriate.
- [ ] **Diverged-with-conflict**: create a task that edits a file, then push a conflicting edit to that same file on `main`, click Accept, verify task is held with `PendingMergeConflict=true`, agent receives a prompt naming the conflicting file, and once the agent pushes a clean rebase the task auto-finalizes to `done`.
- [ ] **Spec approve**: in a project bound to an external repo, push a commit to upstream `main`, then approve a task's specs in Helix, verify approval succeeds without the "Force Sync" error.

## Follow-ups (not part of this task)

- [ ] Watchdog for stalled conflict-resolution (agent crashed mid-rebase, never pushes) — defer until we see it bite users.
- [ ] Conflict-resolution UI surfacing the file list and offering manual resolution — separate UX project.
