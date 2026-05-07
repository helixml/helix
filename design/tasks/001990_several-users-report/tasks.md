# Implementation Tasks

## Backend

- [ ] Add `AwaitingMergeRetry bool` (and optional `MergePendingSince *time.Time`) to `SpecTask` in `api/pkg/types/simple_spec_task.go`; add a GORM AutoMigrate column.
- [ ] In `api/pkg/server/spec_task_workflow_handlers.go::approveImplementation`, set `ImplementationApprovedBy/At` and `AwaitingMergeRetry=true` *before* attempting `MergeBranchFastForward` (so the intent survives a fast-forward failure).
- [ ] On `MergeBranchFastForward` failure in the same handler, keep the recorded approval intent (do not clear it) and include a clear `metadata.error_kind = "awaiting_rebase"` (or similar) in the response so the frontend can render the new informational state.
- [ ] On `MergeBranchFastForward` success, clear `AwaitingMergeRetry` along with the existing `MergedToMain`/`MergedAt` writes.
- [ ] Extract the post-fast-forward "finalize merge" code (push to upstream for external repos, set `MergedToMain`/`MergedAt`/`Status=done`, trigger golden build) into a private helper so it can be reused by the retry path.
- [ ] In `api/pkg/services/git_http_server.go::handleFeatureBranchPush`, after recording `LastPushAt`, if the task is in `implementation_review` with `AwaitingMergeRetry=true`, acquire the repo lock and re-attempt `MergeBranchFastForward` + finalize via the helper above.
- [ ] On retry failure (still not fast-forwardable, or external push fails), clear `AwaitingMergeRetry`, write `metadata.error` with the reason, and leave status at `implementation_review`.
- [ ] Add `POST /api/v1/spec-tasks/{spec_task_id}/cancel-pending-merge` (or extend the spec-task PATCH) to clear `AwaitingMergeRetry` and `ImplementationApprovedBy/At` while keeping status `implementation_review`.
- [ ] Update `api/pkg/prompts/templates/agent_rebase_required.tmpl` — replace the closing "tell the user they need to click Accept again" line with text that matches auto-finalize ("once you push, the merge will complete automatically").
- [ ] Run `./stack update_openapi` to regenerate the TS API client after handler changes.
- [ ] Add Go unit tests in `api/pkg/server/` covering: first-click records intent, fast-forward-fail keeps intent, push-driven retry merges and finalizes, push-driven retry no-ops when intent is cleared, cancel endpoint clears intent, double-Accept is idempotent.

## Frontend

- [ ] Update `frontend/src/services/specTaskWorkflowService.ts::useApproveImplementation` to detect the new "awaiting rebase" response and show an info snackbar — *"Reconciling with `main` — merge will complete automatically when the agent finishes."* — instead of the current warning with "Click Accept again".
- [ ] In `frontend/src/components/tasks/SpecTaskActionButtons.tsx`, when `task.AwaitingMergeRetry` is true, hide the Accept button and render an inline "Reconciling with `main`…" status with a "Cancel approval" link that calls the new cancel endpoint.
- [ ] In `frontend/src/components/tasks/TaskCard.tsx`, render the same reconciling status badge so the cards list reflects the in-flight state.
- [ ] Add a hook `useCancelPendingMerge(specTaskId)` mirroring the existing mutation hooks in `specTaskWorkflowService.ts`.
- [ ] `cd frontend && yarn build` before committing.

## Testing

- [ ] End-to-end: in the inner Helix, create a task on a local repo, let it implement, push a commit to `main` directly (simulate divergence), click Accept, verify info toast and that the task auto-merges to `done` once the agent's rebase pushes.
- [ ] Repeat the e2e flow against an external GitLab repo (or external GitHub if GitLab unavailable) — verify external push of merged `main` happens and task lands at `done`.
- [ ] Cancel-approval e2e: trigger the divergence flow, click Cancel approval mid-flight, verify intent is cleared and a fresh Accept click works after the rebase push.

## Follow-ups (not part of this task)

- [ ] Investigate the spec-approval divergence path (`SyncBaseBranch` → `BranchDivergenceError`) referenced in requirements story 4 — needs its own task.
- [ ] Consider a watchdog that surfaces a stalled rebase (agent crashed, no push in N minutes) — defer until we see it bite users.
