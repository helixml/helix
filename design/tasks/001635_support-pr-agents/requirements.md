# Requirements: GitHub PR Agent Support

## Problem

The PR agent (code review on push) only works with Azure DevOps repositories. GitHub repositories hit an "unsupported external repository type" error in `helix_code_review.go:65`. Users with GitHub repos cannot use the PR review feature at all.

## User Stories

**As a developer using a GitHub repository**, I want the Helix PR agent to automatically review my pull request when I push changes to the feature branch, so I get the same automated code review experience as Azure DevOps users.

**As a project admin**, I want to configure a PR reviewer agent for GitHub-backed projects and have it trigger on git push, just like ADO.

## Acceptance Criteria

1. When a git push occurs on a task branch linked to a GitHub repo, and `PullRequestReviewsEnabled=true` and `PullRequestReviewerHelixAppID` is set, the PR agent runs a review session (no error).
2. The review session receives GitHub PR context (owner, repo, PR number, source/target branches, commit SHAs) so the agent has the information it needs to review.
3. If no GitHub authentication is configured on the repo, a clear error is returned (not a panic).
4. Azure DevOps behavior is unchanged.
5. GitLab and Bitbucket continue to return "unsupported" (they are not in scope for this task).
