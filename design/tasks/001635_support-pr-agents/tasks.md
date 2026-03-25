# Implementation Tasks

- [x] Create `api/pkg/types/github.go` with `GitHubRepositoryContext` struct and `SetGitHubRepositoryContext` / `GetGitHubRepositoryContext` context helpers (mirror `api/pkg/types/azure_devops.go`)
- [x] Add `getGitHubClient()` method to `HelixCodeReviewTrigger` in `api/pkg/trigger/project/helix_code_review.go` (mirror auth priority from `services.getGitHubClient`: App > OAuth > PAT > password)
- [x] Implement `processGitHubPullRequest()` on `HelixCodeReviewTrigger`: get client, parse URL, find PR, fetch PR details, set `GitHubRepositoryContext` on ctx, call `runReviewSession()`
- [x] Add `case types.ExternalRepositoryTypeGitHub:` to the switch in `ProcessGitPushEvent()` calling `processGitHubPullRequest()`
- [x] Verify `go build ./api/pkg/trigger/project/...` passes with no errors
- [ ] Test end-to-end: create a Helix project with a GitHub repo, enable PR reviews, set a reviewer app, push to a task feature branch, confirm a review session is created
