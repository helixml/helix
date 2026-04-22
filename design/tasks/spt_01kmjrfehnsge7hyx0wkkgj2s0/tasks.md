# Implementation Tasks

- [x] Add `ToolTypeGitHub` constant to `api/pkg/types/types.go` alongside `ToolTypeAzureDevOps`
- [x] Add `ToolGitHubConfig` struct to `api/pkg/types/types.go` with `PersonalAccessToken string` and `BaseURL string` (for GHE) fields
- [x] Add `GitHub *ToolGitHubConfig` field to `ToolConfig` struct in `api/pkg/types/types.go`
- [x] Add `AssistantGitHub` struct and add it to `AssistantSkills` and flat state in `api/pkg/types/types.go`
- [x] Create `api/pkg/agent/skill/github/pull_request_diff.go` with `GitHubPullRequestDiffTool` and `NewPullRequestDiffSkill(token, baseURL string)` — reads `GitHubRepositoryContext` from ctx, calls `client.PullRequests.ListFiles()`, returns formatted diff
- [x] Create `api/pkg/agent/skill/github/pr_create_review_comment.go` with `GitHubCreateReviewCommentTool` and `NewCreateReviewCommentSkill(token, baseURL string)` — accepts `content`, optional `file_path` + `line_number`; uses PR Review Comments API for inline, Issues API for general; prefixes with `[Helix]`
- [x] Register GitHub skills in `api/pkg/controller/inference_agent.go`: add `if assistantTool.ToolType == types.ToolTypeGitHub` block that appends `NewPullRequestDiffSkill` and `NewCreateReviewCommentSkill`
- [x] Regenerate OpenAPI client (`swag init` + `swagger-typescript-api`)
- [x] Add GitHub tool option to the agent tool configuration UI — `GitHubSkill.tsx` component with PAT and optional BaseURL fields
- [x] Wire up GitHub tool in Skills.tsx (BASE_SKILLS entry, isSkillEnabled, renderSkillDialog)
- [x] Wire up GitHub tool in types.ts, utils/app.ts (flatten), hooks/useApp.ts (unflatten), pages/ProjectSettings.tsx
- [x] Verify `go build` passes for types, agent, controller, server, trigger packages
- [x] Verify `cd frontend && yarn build` passes
- [x] Create PR description
