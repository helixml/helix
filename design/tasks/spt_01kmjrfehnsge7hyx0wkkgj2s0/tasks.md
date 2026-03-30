# Implementation Tasks

- [ ] Add `ToolTypeGitHub` constant to `api/pkg/types/types.go` alongside `ToolTypeAzureDevOps`
- [ ] Add `ToolConfigGitHub` struct to `api/pkg/types/types.go` with `PersonalAccessToken string` and `BaseURL string` (for GHE) fields
- [ ] Add `GitHub *ToolConfigGitHub` field to `AssistantToolConfig` struct in `api/pkg/types/types.go`
- [ ] Create `api/pkg/agent/skill/github/pull_request_diff.go` with `GitHubPullRequestDiffTool` and `NewPullRequestDiffSkill(token, baseURL string)` — reads `GitHubRepositoryContext` from ctx, calls `client.PullRequests.ListFiles()`, returns formatted diff
- [ ] Create `api/pkg/agent/skill/github/pr_create_review_comment.go` with `GitHubCreateReviewCommentTool` and `NewCreateReviewCommentSkill(token, baseURL string)` — accepts `content`, optional `file_path` + `line_number`; uses PR Review Comments API for inline, Issues API for general; prefixes with `[Helix]`
- [ ] Register GitHub skills in `api/pkg/controller/inference_agent.go`: add `if assistantTool.ToolType == types.ToolTypeGitHub` block that appends `NewPullRequestDiffSkill` and `NewCreateReviewCommentSkill`
- [ ] Add GitHub tool option to the agent tool configuration UI (find where AzureDevOps tool type is rendered in `frontend/src/`) — show PAT and optional BaseURL fields
- [ ] Verify `go build ./pkg/...` passes
- [ ] Verify `cd frontend && yarn build` passes
- [ ] End-to-end test: configure a GitHub-backed Helix project with a PR reviewer agent that has a GitHub tool (PAT), create a task, push to a feature branch with an open PR, confirm review comments appear on the GitHub PR
