# Implementation Tasks

## Data Model

- [ ] Add `RepoPREntry` struct to `api/pkg/types/simple_spec_task.go` (fields: RepoID, RepoName, PRID, PRURL, State, MergedAt)
- [ ] Add `RepoPRs []RepoPREntry` JSONB field to `SpecTask` struct (keep existing `PullRequestID`/`PullRequestURL` fields)
- [ ] Add GORM auto-migration for the new `repo_prs` column

## Backend — Branch Push Tracking

- [ ] In `handleFeatureBranchPush` (`git_http_server.go`), upsert a `RepoPREntry` (with empty PRID) for the pushed repo into `task.RepoPRs` and save

## Backend — PR Creation

- [ ] Refactor `ensurePullRequest` to accept a specific repo (not assume default repo only)
- [ ] On implementation approval, call `ensurePullRequest` for every repo in `task.RepoPRs`
- [ ] Update each `RepoPREntry` with PRID, PRURL, and State after PR creation/discovery
- [ ] Remove the guard that skips storing PR data for non-default repos

## Backend — Merge Detection

- [ ] Refactor `processExternalPullRequestStatus` to iterate `task.RepoPRs` and update each entry's state
- [ ] Only transition task to `done` when all `RepoPRs` entries are `merged` (or branch merged without PR)
- [ ] Refactor `handleMainBranchPush` to mark the specific repo's `RepoPREntry` as merged, then check all entries before setting `done`
- [ ] Extend `detectExternalPRActivity` / `checkTaskForExternalPRActivity` to check all repos in `RepoPRs`, not just the default repo

## Backend — Backward Compatibility

- [ ] When `RepoPRs` is empty, fall back to existing single `PullRequestID` logic in all orchestrator paths
- [ ] Ensure API serialises `RepoPRs` in task JSON response

## Frontend

- [ ] In `SpecTaskActionButtons.tsx`, when `task.repo_prs` is non-empty render a list of PR rows (repo name + link + status chip) instead of single button
- [ ] Fall back to existing single-PR button when `repo_prs` is empty
- [ ] In `SpecTaskDetailContent.tsx`, display the same multi-PR list in the task detail panel
- [ ] Add TypeScript type for `RepoPREntry` to the frontend task types
