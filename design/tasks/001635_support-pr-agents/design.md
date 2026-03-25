# Design: GitHub PR Agent Support

## Current Architecture

The trigger chain on git push:
1. `api/pkg/services/git_http_server.go` detects a push to a task branch
2. Calls `trigger.Manager.ProcessGitPushEvent()` → `HelixCodeReviewTrigger.ProcessGitPushEvent()`
3. `api/pkg/trigger/project/helix_code_review.go:61-66` — switch on `repo.ExternalType`
4. Only `ExternalRepositoryTypeADO` is handled; all others return "unsupported external repository type"
5. For ADO: fetches PR details, sets `AzureDevopsRepositoryContext` on ctx, calls `runReviewSession()`
6. `runReviewSession()` creates a Helix session and runs the configured reviewer app

## What Needs to Change

### 1. `api/pkg/types/github.go` — New file

Add a `GitHubRepositoryContext` struct and context helpers, mirroring `api/pkg/types/azure_devops.go`:

```go
type GitHubRepositoryContext struct {
    RemoteURL     string
    Owner         string
    RepositoryName string
    PullRequestID int
    SourceBranch  string  // head ref
    TargetBranch  string  // base ref
    HeadSHA       string
    BaseSHA       string
}
```

Plus `SetGitHubRepositoryContext` / `GetGitHubRepositoryContext` using a typed context key.

### 2. `api/pkg/trigger/project/helix_code_review.go` — Main change

**Add GitHub case to the switch** (line 62):
```go
case types.ExternalRepositoryTypeGitHub:
    return h.processGitHubPullRequest(ctx, specTask, project, repo, commitHash)
```

**Implement `processGitHubPullRequest()`** following the ADO pattern:
- Create a GitHub client using the repo's auth config (same priority order as `git_repository_service_pull_requests.go:getGitHubClient()`: GitHub App > OAuth > PAT > password)
- Parse owner/repo from `repo.ExternalURL` using existing `github.ParseGitHubURL()`
- Find the PR for this task (`specTask.GetPRForRepo()` / `GetFirstOpenPR()`)
- Fetch PR details via `client.GetPullRequest()`
- Set `GitHubRepositoryContext` on ctx
- Call `runReviewSession()`

**Implement `getGitHubClient()`** on `HelixCodeReviewTrigger`:
- The trigger has `store store.Store` already (needed for OAuth lookup)
- Mirror the logic from `services.GitRepositoryService.getGitHubClient()`: check App auth, then OAuth connection, then PAT, then password

### 3. Imports

Add `"github.com/helixml/helix/api/pkg/agent/skill/github"` to `helix_code_review.go` imports.

## Key Existing Utilities

| What | Where |
|------|-------|
| GitHub client creation with multi-auth | `api/pkg/agent/skill/github/client.go` — `NewClientWithPAT`, `NewClientWithGitHubApp`, etc. |
| URL parsing | `github.ParseGitHubURL(url)` in same package |
| PR fetching | `client.GetPullRequest(ctx, owner, repo, number)` |
| ADO context pattern to copy | `api/pkg/types/azure_devops.go` |
| Auth priority logic to mirror | `api/pkg/services/git_repository_service_pull_requests.go:getGitHubClient()` |

## What Does NOT Change

- `trigger.Manager` — no new trigger type needed; `ProcessGitPushEvent` already delegates to `helixCodeReview`
- `runReviewSession()` — reused as-is
- All ADO code paths
- GitLab / Bitbucket (still unsupported, out of scope)

## Design Decisions

- **Duplicate auth logic vs. inject service**: The trigger already has `store` access. Duplicating the `getGitHubClient()` auth logic in the trigger keeps the code self-contained and avoids a circular dependency between `trigger/project` and `services`. The auth logic is short (~30 lines).
- **No new context type for the agent skill**: The `GitHubRepositoryContext` provides structured PR metadata on the context. If the GitHub agent skill needs to read it, it can use `GetGitHubRepositoryContext()`. The review prompt already includes task name + commit hash; adding structured context allows future skill improvements without re-triggering.
- **PR lookup**: Reuses existing `specTask.GetPRForRepo()` / `GetFirstOpenPR()` — same as ADO.
