# Implementation Tasks

- [ ] In `ensurePullRequestForRepo`, before calling `ListPullRequests`, check `task.RepoPullRequests` for an existing entry matching this `repoID`; if found, fetch the PR state by ID and return it without creating a new PR
- [ ] In `handlePullRequest` (orchestrator), skip the `o.ensurePRs(...)` call if all entries in `task.RepoPullRequests` have `PRState == "closed"`
- [ ] Write a unit test: task with one closed RepoPR entry → `ensurePullRequestForRepo` returns existing PR, does not call `CreatePullRequest`
- [ ] Write a unit test: orchestrator polling with all PRs closed → `ensurePRs` is not called
- [ ] Manually verify: close a Helix PR on GitHub, wait 60 seconds, confirm no duplicate PR is opened
