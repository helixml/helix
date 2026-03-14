# Requirements: Multi-Repo Pull Request Tracking

## Problem

A spec task can only track a single `PullRequestID` (string field on `SpecTask`). When agent code changes span multiple repos (e.g., helix + zed + qwen-code), PRs created on secondary repos are invisible to the orchestrator. The task auto-closes only when the default repo's PR merges, leaving secondary PRs untracked.

## User Stories

**US-1:** As an agent, when I push a feature branch to any repo in a project, that repo is recorded on the task so the orchestrator knows to track a PR there.

**US-2:** As an agent, when I open a PR on any project repo, that PR (id, url, state) is stored on the task independently of other repos.

**US-3:** As the orchestrator, I only transition a task to `done` when all tracked PRs are merged (or, for repos with no PR, the feature branch is merged via git).

**US-4:** As a user viewing a task, I see a list of all PRs across all repos, each with a link and its current merge status.

**US-5:** As an operator, existing single-repo tasks continue to work without migration or data loss.

## Acceptance Criteria

- [ ] Pushing to a feature branch on repo B (not the default repo) records repo B in the task's multi-repo tracking data
- [ ] `ensurePullRequest` creates PRs for all tracked repos and stores each PR's id/url/state
- [ ] Orchestrator merge detection checks all tracked repos; task reaches `done` only when every PR is merged
- [ ] Task detail view lists all PRs with repo name, PR link, and merged/open status
- [ ] Tasks with only a single repo continue to behave as today (backward compatible)
- [ ] Old tasks with `pull_request_id` set but no multi-repo data are handled gracefully (treated as single-repo)
