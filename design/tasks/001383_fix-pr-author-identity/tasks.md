# Implementation Tasks

## Backend -- Service Layer

- [ ] Define `OAuthRequiredError` type (with `ProviderType` field) in `api/pkg/types/` or inline in the service package
- [ ] Add `userID string` parameter to `GitRepositoryService.CreatePullRequest` in `git_repository_service_pull_requests.go` and thread it to provider-specific helpers
- [ ] Update `getGitHubClient(ctx, repo, userID)`: if `userID != ""`, look up acting user's GitHub OAuth connection; if found use it, if NOT found return `OAuthRequiredError` (do NOT fall back to repo creds). If `userID == ""` (agent path), use existing fallback chain: GitHub App -> repo `OAuthConnectionID` -> PAT -> password
- [ ] Update `getGitLabClient(ctx, repo, userID)` with same two-mode resolution for GitLab
- [ ] Update `git_http_server.go` (agent-triggered PR path, line 1068) to pass `""` as `userID`

## Backend -- Handler Layer

- [ ] Update `createGitRepositoryPullRequest` handler in `git_repository_handlers.go` to pass `user.ID` to `CreatePullRequest`; handle `OAuthRequiredError` by returning HTTP 422 with `{"error": "oauth_required", "message": "...", "provider_type": "github"}`
- [ ] Update `ensurePullRequestForTask` in `spec_task_workflow_handlers.go` to accept `userID string` and pass it to `CreatePullRequest`; update `approveImplementation` to pass `user.ID`
- [ ] When `OAuthRequiredError` is returned from PR creation, propagate it as HTTP 422 to the frontend; do NOT advance the task to `pull_request` status -- leave it in `implementation` so the user can retry after OAuth setup

## Frontend

- [ ] In `specTaskWorkflowService.ts`, detect `oauth_required` error code in the mutation `onError` handler
- [ ] When `oauth_required` is received, show a message prompting the user to connect their GitHub/GitLab account and redirect to the OAuth connection flow
- [ ] After OAuth connection is established, the user retries "Open PR" -- no special handling needed beyond the existing button

## Tests

- [ ] Write a unit test: repo has `OAuthConnectionID` = user A's connection; `CreatePullRequest` called with user B's ID who also has a GitHub OAuth connection -> user B's token is used
- [ ] Write a unit test: `CreatePullRequest` called with a `userID` that has NO OAuth connection -> returns `OAuthRequiredError`, does NOT fall back to repo-level credentials
- [ ] Write a unit test: `CreatePullRequest` called with empty `userID` (agent path) -> falls back to repo-level credentials normally
- [ ] Run `go build ./pkg/services/ ./pkg/server/` to confirm no compile errors
- [ ] Run `cd frontend && yarn build` to confirm no frontend compile errors
