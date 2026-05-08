# Add GitHub PR review support for PR review agent

## Summary
The PR review agent could only interact with Azure DevOps pull requests. This adds equivalent GitHub support so the agent can read PR diffs and post inline review comments on GitHub repos.

The trigger chain (`processGitHubPullRequest` in `helix_code_review.go`) already existed and set the `GitHubRepositoryContext`, but the agent had no GitHub-specific tools to act on it. Now it does.

## Changes
- **New `ToolTypeGitHub` and `AssistantGitHub` types** in `api/pkg/types/types.go` — PAT + optional BaseURL (for GitHub Enterprise)
- **New `GitHubPullRequestDiffTool`** (`api/pkg/agent/skill/github/pull_request_diff.go`) — fetches changed files and patches via `PullRequests.ListFiles()`
- **New `GitHubCreateReviewCommentTool`** (`api/pkg/agent/skill/github/pr_create_review_comment.go`) — posts inline review comments (file + line) or general PR comments; prefixes with `[Helix]`
- **Skill registration** in `api/pkg/controller/inference_agent.go` — `ToolTypeGitHub` block alongside existing ADO block
- **Frontend `GitHubSkill.tsx`** config dialog — PAT field + optional Base URL for GHE
- **Frontend wiring** — Skills.tsx, types.ts, useApp.ts, app.ts, ProjectSettings.tsx
- **Regenerated OpenAPI client** with new types

## Screenshots

![GitHub PRs skill in the GitHub category tab](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kmjrfehnsge7hyx0wkkgj2s0/screenshots/01-github-category-skills.png)

![GitHub PRs configuration dialog with PAT and Base URL fields](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kmjrfehnsge7hyx0wkkgj2s0/screenshots/02-github-prs-config-dialog.png)
