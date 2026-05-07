# Implementation Tasks

## Backend — data model & client methods

- [x] Extend `RepoPR` in `api/pkg/types/simple_spec_task.go` with `CIStatus`, `CIURL`, `CIUpdatedAt`, `CIHeadSHA` fields (all `omitempty`).
- [x] Create `api/pkg/services/ci_status.go` with `NormalizeCIStatus(provider, raw string) string` returning one of `"running"`, `"passed"`, `"failed"`, `"none"`. Tests cover all provider verdicts including unknown→failed.
- [x] Add `GetCIStatus(ctx, owner, repo, sha)` to `api/pkg/agent/skill/github/client.go` — combines `Repositories.GetCombinedStatus` and `Checks.ListCheckRunsForRef`, takes the worst conclusion.
- [x] Add `GetCIStatus(ctx, projectID, sha)` to `api/pkg/agent/skill/gitlab/client.go` — uses `Pipelines.ListProjectPipelines` filtered by SHA.
- [x] Add `GetCIStatus(ctx, project, repoID, commitID)` to `api/pkg/agent/skill/azure_devops/client.go` — uses Build API filtered by repository, matches by SourceVersion.
- [~] Bitbucket: add stub `GetCIStatus()` returning `("none", "", nil)` with a TODO comment.
- [x] In the ADO `GetCIStatus` implementation, treat 401/403 from the Build API as `ErrCIScopeMissing` (so existing PATs without `vso.build` don't break the UI). The orchestrator will treat this error as `"none"` and emit a one-time log warning.
- [ ] Update the ADO connection UI hint (`frontend/src/components/...` for git provider connection) to mention `vso.build` is required for CI status — locate the existing scope hint and amend.
- [ ] Unit tests for `normalizeCIStatus` covering each provider's known verdicts.

## Backend — notification & orchestrator

- [ ] Create `api/pkg/services/spec_task_ci_notifier.go` with `CINotifier` interface, mirroring the structure of `SpecTaskReviewNotifier`.
- [ ] Implement the production `CINotifier` that calls `sendChatMessageToExternalAgent()` with `interrupt: false` and persists a waiting interaction if the agent is offline.
- [ ] Generate a `gomock` mock for `CINotifier` (run `mockgen` consistent with existing patterns).
- [ ] Add `ci_passed` and `ci_failed` event-type constants in `api/pkg/services/attention_service.go`.
- [ ] In `pollPullRequests` (`api/pkg/services/spec_task_orchestrator.go`): for each tracked PR, after PR state update, call `client.GetCIStatus()` for the PR's head SHA.
- [ ] Reset cached `CIStatus = ""` if `RepoPR.CIHeadSHA` differs from the new head SHA (handles force-push / new commits).
- [ ] Detect `running → passed` transition: call `CINotifier.NotifyCIResult(...)` and `attention.EmitEvent("ci_passed")`.
- [ ] Detect `running → failed` transition: same with `"ci_failed"`.
- [ ] Persist updated `RepoPR` (with new `CIStatus`, `CIURL`, `CIUpdatedAt`, `CIHeadSHA`) back to the task row.
- [ ] Orchestrator tests for transition detection: no notify on first observation, notify on first transition, no double-notify on restart with same cached status, reset on new SHA.

## Frontend

- [ ] Run `./stack update_openapi` after Go struct changes to regenerate `frontend/src/api/api.ts` so `RepoPR` includes the new CI fields.
- [ ] Create `frontend/src/components/tasks/CIStatusIcon.tsx`: takes `prs` prop, computes worst status, renders one ~16px MUI icon (Sync animated for running / CheckCircle for passed / Cancel for failed / nothing for none), wrapped in a `<Tooltip>` listing each PR with its CI status + clickable link.
- [ ] Slot `<CIStatusIcon prs={task.repo_pull_requests} />` into the existing status row in `TaskCard.tsx` (around line 964) — between the phase label and the assignee avatar's `ml: 'auto'`. Verify card height is unchanged with measurement screenshots.
- [ ] Add `prevProps.task.repo_pull_requests === nextProps.task.repo_pull_requests` (or a shallow comparison) to the `TaskCard` memo comparator so PR/CI changes trigger re-renders.

## Verification

- [ ] `cd api && go build ./pkg/services/ ./pkg/types/ ./pkg/agent/skill/...` passes locally.
- [ ] `cd frontend && yarn build` passes (catches type errors).
- [ ] Inner-Helix E2E: register `test@helix.ml` / `helixtest`, complete onboarding, create a spec task wired to a real GitHub repo with Actions, push a branch, open a PR with a workflow that fails, watch the icon turn red within ~30s, confirm agent receives the chat message ("CI failed for PR…"), confirm the human sees the attention event red dot.
- [ ] Re-run with a passing workflow: icon turns green, agent receives "CI passed" message.
- [ ] Push a new commit to the same PR while CI is queued: confirm the icon resets and re-evaluates from `running`.
- [ ] Take before/after screenshots of the Kanban card to confirm the card is **not taller** than before.

## Follow-ups (separate tasks)

- [ ] Bitbucket CI status implementation.
- [ ] Webhook-driven CI updates for github.com (latency optimization).
- [ ] Per-project notification toggle (settings UI + project-level config).
