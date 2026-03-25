# Add GitHub PR review support

## Summary

The PR agent (automated code review on push) previously only worked with Azure DevOps repositories. GitHub repositories hit an "unsupported external repository type" error and received no review. This PR adds GitHub support following the same pattern as the existing ADO implementation.

## Changes

- **`api/pkg/types/github.go`** (new) — `GitHubRepositoryContext` struct carrying PR metadata (owner, repo, PR number, head/base branch and SHAs) plus `SetGitHubRepositoryContext` / `GetGitHubRepositoryContext` context helpers
- **`api/pkg/trigger/project/helix_code_review.go`** — adds `case types.ExternalRepositoryTypeGitHub:` to the `ProcessGitPushEvent` switch, implements `processGitHubPullRequest()` (fetches PR via GitHub API, sets context, delegates to `runReviewSession()`), and `getGitHubClient()` (GitHub App > OAuth > PAT > password auth priority)

## Behaviour

When a git push occurs on a task feature branch linked to a GitHub repository, and `PullRequestReviewsEnabled=true` and `PullRequestReviewerHelixAppID` are set on the project, the configured reviewer app now runs a review session — identical to the existing ADO flow. ADO behaviour is unchanged.
