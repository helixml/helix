# Implementation Tasks

- [x] In `ensurePullRequestForRepo` (spec_task_workflow_handlers.go), before `ListPullRequests`, check `task.GetPRForRepo(repo.ID)`; if a PR is already tracked, fetch it by PR ID and return it without creating a new one
- [x] In `ensurePullRequestForRepo`, add 422 "already exists" error handling after `CreatePullRequest` fails: re-list PRs and return the found PR instead of propagating the error (matching the behaviour in git_http_server.go's `ensurePullRequest`)
- [x] In `ensurePullRequest` (git_http_server.go), skip the `UpdatePullRequest` title-overwrite call so user-renamed PR titles are preserved
- [x] Write a unit test: task with a tracked RepoPR for a repo → `ensurePullRequestForRepo` returns that PR without calling `ListPullRequests` or `CreatePullRequest`
- [x] Write a unit test: `ensurePullRequestForRepo` receives 422 "already exists" from `CreatePullRequest` → recovers and returns the existing PR (no error)
- [ ] Manually verify: rename a Helix PR title on GitHub, wait 60 seconds, confirm no duplicate PR is opened
