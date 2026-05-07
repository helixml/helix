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

- [x] Run `./stack update_openapi` after Go struct changes to regenerate `frontend/src/api/api.ts` so `RepoPR` includes the new CI fields.
- [x] Create `frontend/src/components/tasks/CIStatusIcon.tsx`: takes `prs` prop, computes worst status (failed > running > passed > none), renders one ~14px MUI icon, wrapped in a `<Tooltip>` listing each PR with its CI status + clickable link.
- [x] Slot `<CIStatusIcon prs={task.repo_pull_requests} />` into the existing status row in `TaskCard.tsx` — between the phase label and the assignee avatar (no extra row).
- [x] Add a `ciSignature(...)` shallow CI comparison to the `TaskCard` memo comparator so PR/CI status changes trigger re-renders.

## Verification

- [x] `cd api && go build ./...` passes locally (after merging origin/main).
- [x] `cd frontend && npx tsc --noEmit` passes (no type errors). `yarn build` blocked by `frontend/dist` permission (root-owned bind mount); type-check is the canonical gate.
- [x] Unit tests pass: `go test -run "TestNormalizeCIStatus|TestCITransition_" ./pkg/services/`.
- [x] Drop in-progress branch tasks (UI hint about ADO scope) — not blocking; can be picked up later. Note added in design.md OAuth/PAT scopes section.
- [x] Inner-Helix visual verification: inserted three test tasks (running / passed / failed) directly into the spec_tasks table with hand-crafted RepoPR JSON, navigated to the Kanban board, screenshot confirms all three icons render inline in the existing status row with the correct colours and animation. Card height unchanged. Screenshot at `screenshots/01-kanban-with-ci-icons.png`.
- [ ] (Deferred) Live end-to-end test against a real GitHub repo with Actions — needs a connected provider in the inner Helix; not blocking PR review since the unit tests cover the transition logic and the UI verification is in the screenshot above.

## Follow-ups (separate tasks)

- [ ] Bitbucket CI status implementation.
- [ ] Webhook-driven CI updates for github.com (latency optimization).
- [ ] Per-project notification toggle (settings UI + project-level config).
