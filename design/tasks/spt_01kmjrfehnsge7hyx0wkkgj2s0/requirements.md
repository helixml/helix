# Requirements: GitHub PR Review Support

## Background

Helix has a PR review agent feature that auto-reviews pull requests when code is pushed to a feature branch. This is fully functional for Azure DevOps (ADO), but GitHub repos cannot use it. The trigger chain fires for GitHub repos (`processGitHubPullRequest` exists and sets GitHub context), but the PR review agent has no GitHub-specific tools to read the diff or post review comments. This means GitHub users see no activity from the PR review agent.

## User Stories

**As a developer using a GitHub repository with a Helix project:**

1. I can configure a PR review agent that automatically reviews my pull requests on GitHub, just like ADO users can.
2. When I (or an implementation agent) pushes to a feature branch that has an open PR, the review agent posts review comments on that PR.
3. The review agent can see the full diff of the PR to understand what changed.
4. The review agent can post inline comments on specific files and lines, not just general comments.

## Acceptance Criteria

1. A new `GitHub` tool type exists in the agent tool configuration UI (alongside the existing AzureDevOps tool type).
2. A GitHub tool configured with a PAT (or GitHub App credentials) provides these skills to the review agent:
   - `GitHubPullRequestDiff` — fetches the list of changed files and their diffs
   - `GitHubCreateReviewComment` — posts a review comment on the PR (optionally on a specific file + line)
3. When `processGitHubPullRequest` triggers a review session and the PR reviewer agent has a GitHub tool configured, the agent successfully posts comments on the GitHub PR.
4. The tool reads the PR target (owner, repo, PR number) from the `GitHubRepositoryContext` in the session context — no duplicate configuration required.
5. Inline file/line comments use the GitHub Pull Request Review Comments API (`POST /repos/{owner}/{repo}/pulls/{pull_number}/comments`).
6. General (non-inline) PR comments fall back to the Issues comments API (`POST /repos/{owner}/{repo}/issues/{issue_number}/comments`).
7. Go build passes: `go build ./pkg/...`
8. Frontend build passes: `cd frontend && yarn build`
