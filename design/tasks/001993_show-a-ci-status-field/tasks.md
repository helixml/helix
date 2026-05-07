# Implementation Tasks

## Backend — data model & client methods

- [x] Extend `RepoPR` in `api/pkg/types/simple_spec_task.go` with `CIStatus`, `CIURL`, `CIUpdatedAt`, `CIHeadSHA` fields (all `omitempty`).
- [x] Create `api/pkg/services/ci_status.go` with `NormalizeCIStatus(provider, raw string) string` returning one of `"running"`, `"passed"`, `"failed"`, `"none"`. Tests cover all provider verdicts including unknown→failed.
- [x] Add `GetCIStatus(ctx, owner, repo, sha)` to `api/pkg/agent/skill/github/client.go` — combines `Repositories.GetCombinedStatus` and `Checks.ListCheckRunsForRef`, takes the worst conclusion.
- [x] Add `GetCIStatus(ctx, projectID, sha)` to `api/pkg/agent/skill/gitlab/client.go` — uses `Pipelines.ListProjectPipelines` filtered by SHA.
- [x] Add `GetCIStatus(ctx, project, repoID, commitID)` to `api/pkg/agent/skill/azure_devops/client.go` — uses Build API filtered by repository, matches by SourceVersion.
- [x] Bitbucket: add stub `GetCIStatus()` returning `(nil, nil)` (treated as "none" by caller) with a TODO comment.
- [x] In the ADO `GetCIStatus` implementation, treat 401/403 from the Build API as `ErrCIScopeMissing`. The dispatcher in `git_repository_service_ci_status.go` catches it and degrades to `"none"` with a one-time warn log.
- [ ] Update the ADO connection UI hint (`frontend/src/components/...` for git provider connection) to mention `vso.build` is required for CI status — locate the existing scope hint and amend.

## Backend — unified dispatcher

- [x] Add `HeadSHA string` to `types.PullRequest`; populate in GitHub (`Head.SHA`), GitLab (`SHA`), ADO (`LastMergeSourceCommit.CommitId`).
- [x] Add `types.CIStatus { State, URL, HeadSHA }` and `GitRepositoryService.GetCIStatus(ctx, repoID, prID, headSHA)` dispatcher in new file `pkg/services/git_repository_service_ci_status.go`.
- [x] Unit tests for `NormalizeCIStatus` covering each provider's known verdicts.

## Backend — notification & orchestrator

- [x] Create `api/pkg/services/spec_task_ci_notifier.go` with `CINotifier` interface.
- [x] Implement the production `CINotifier` (`MessageSenderCINotifier`) that wraps the existing `SpecTaskMessageSender` callback (which already handles offline → waiting-interaction queue). Wired in `pkg/server/server.go` after orchestrator construction.
- [x] Skip generating a separate mock — wrote a small in-test `recordingCINotifier` instead (interface is one method; mockgen would be over-engineering).
- [x] Add `AttentionEventCIPassed` / `AttentionEventCIFailed` constants in `pkg/types/attention_event.go`; update `attention_service.go` `buildTitle` / `buildDescription` / `eventEmoji`.
- [x] Add `pollCIStatusForPR` to the orchestrator and call it from `processExternalPullRequestStatus` for each tracked PR.
- [x] Reset cached `CIStatus = ""` if `RepoPR.CIHeadSHA` differs from the new head SHA (handles force-push / new commits).
- [x] Detect `running → passed` transition: call `CINotifier.NotifyCIResult(...)` + `attention.EmitEvent(AttentionEventCIPassed)`.
- [x] Detect `running → failed` transition: same with `AttentionEventCIFailed`.
- [x] Persist updated `RepoPR` (with new `CIStatus`, `CIURL`, `CIUpdatedAt`, `CIHeadSHA`) back to the task row (via existing `updated` flag in `processExternalPullRequestStatus`).
- [x] Orchestrator tests for transition detection: first-observation silent, running→passed notifies once, running→failed notifies once with logs link, no notification on no-op transitions, nil-notifier doesn't panic.

## Frontend

- [~] Run `./stack update_openapi` after Go struct changes to regenerate `frontend/src/api/api.ts` so `RepoPR` includes the new CI fields.
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
