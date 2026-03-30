# Design: GitHub PR Review Support

## Current State

`processGitHubPullRequest` in `api/pkg/trigger/project/helix_code_review.go` already:
- Creates a GitHub client using repo credentials (PAT / OAuth / GitHub App)
- Fetches PR details (head/base SHAs, branches)
- Sets `GitHubRepositoryContext` in the session context
- Calls `runReviewSession` to start the PR reviewer agent

What's missing: the PR reviewer agent has no GitHub-specific tools to read the diff or post comments. ADO provides three dedicated Go tools (`CreateThread`, `ReplyToComment`, `PullRequestDiff`). GitHub has none — only generic YAML-based OpenAPI skills that lack the context-awareness needed.

## Architecture

Follow the exact same pattern as ADO tools:

```
types.ToolTypeGitHub → inference_agent.go registers GitHub skills
                         ↓
    skill/github/pull_request_diff.go   (reads GitHubRepositoryContext from ctx)
    skill/github/pr_create_review_comment.go  (reads GitHubRepositoryContext from ctx)
```

The `GitHubRepositoryContext` (already defined in `api/pkg/types/github.go`) carries:
- `Owner`, `RepositoryName`, `PullRequestID`, `HeadSHA`, `BaseSHA`

This context is set by `processGitHubPullRequest` before the review session runs, so tools can use it without extra user config.

## Key Decisions

**Follow ADO pattern, not auto-injection.** The ADO tools are configured on the agent app (org URL + PAT in tool config). GitHub tools will work the same way — user adds a GitHub tool to the PR reviewer agent with a PAT (or GitHub App config). This keeps the pattern consistent and avoids threading repo credentials through the session.

**Two tools, not three.** ADO has `CreateThread`, `ReplyToComment`, and `PullRequestDiff`. For GitHub:
- `GitHubPullRequestDiff` — equivalent to ADO's PullRequestDiff
- `GitHubCreateReviewComment` — handles both inline (file+line) and general PR comments in one tool (GitHub's APIs make this natural — fall back to Issues API when no file/line given)

No `ReplyToComment` equivalent for MVP (GitHub's reply-to-review-comment is a separate API; not critical for initial release).

**Inline comments use the PR Review Comments API.** `POST /repos/{owner}/{repo}/pulls/{pull_number}/comments` with `commit_id`, `path`, and `position` (diff position). This is the correct GitHub API for inline review comments, equivalent to ADO thread_create with file/line context.

**Diff uses `PullRequests.ListFiles` + raw patch.** `client.PullRequests.ListFiles()` returns files with `Patch` field containing the unified diff. No need for a separate raw diff API call.

## Files to Create/Modify

### New files
- `api/pkg/agent/skill/github/pull_request_diff.go` — `GitHubPullRequestDiffTool` and `NewPullRequestDiffSkill()`
- `api/pkg/agent/skill/github/pr_create_review_comment.go` — `GitHubCreateReviewCommentTool` and `NewCreateReviewCommentSkill()`

### Modified files
- `api/pkg/types/types.go` — add `ToolTypeGitHub ToolType = "github"` and `ToolConfigGitHub` struct (PAT + optional BaseURL for GHE), add `GitHub *ToolConfigGitHub` to `AssistantToolConfig`
- `api/pkg/controller/inference_agent.go` — add `ToolTypeGitHub` block alongside the existing `ToolTypeAzureDevOps` block
- `frontend/src/components/...` — add GitHub tool option in agent tool configuration (search for where ADO tool type is rendered)

## Tool Design

### GitHubPullRequestDiffTool

Parameters: none (reads from context)

Execution:
1. Read `GitHubRepositoryContext` from ctx
2. Create GitHub client from tool credentials (PAT / App)
3. Call `client.PullRequests.ListFiles(ctx, owner, repo, prID, nil)`
4. Return formatted string: PR title + each file's patch

### GitHubCreateReviewCommentTool

Parameters:
- `content` (required) — comment text
- `file_path` (optional) — relative path
- `line_number` (optional) — 1-based line number in the new file

Execution:
1. Read `GitHubRepositoryContext` from ctx
2. If `file_path` + `line_number` given: use PR Review Comments API with `commit_id=HeadSHA`, `path`, `line` (GitHub supports `line` parameter since 2019, simpler than `position`)
3. If no file/line: use Issues Comments API (`client.Issues.CreateComment`)

Prefix comments with `[Helix]` (consistent with ADO tool behaviour).

## Codebase Patterns Learned

- All Go agent skills live in `api/pkg/agent/skill/<provider>/` with a constructor `New*Skill()` returning `agent.Skill`
- Skills are registered in `api/pkg/controller/inference_agent.go` in the tool-type switch
- Context is passed via `context.Context` using typed setters/getters in `api/pkg/types/`
- The `go-github/v57` library is already imported in `api/pkg/agent/skill/github/client.go` — use the same version
- ADO tools prefix comments with `[Helix]` — do the same
- Tool credentials in `AssistantToolConfig` parallel the `GitHub` struct in `GitRepository` — keep field names consistent
