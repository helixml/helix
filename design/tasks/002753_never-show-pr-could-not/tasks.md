# Implementation Tasks: Never Show "PR Could Not Be Created" When Work Landed on an Internal Repo

- [ ] Read prior art before touching code: `git show 77726b750`, `70ab4b28a`, `19d7d4f55`
- [ ] Confirm Open Question 1: check `project_repositories` for `code-chris-outreach-01kvtnnpgaz5eg525et25tx62f` and verify the task's repo set really does include it; adjust the resolver's candidate set if not
- [ ] Create `api/pkg/services/spec_task_repo_landing.go` with `RepoLandingState`, `RepoLanding`, `TaskLanding`, and `ResolveTaskLanding` (classify each repo via `GetDivergence` on `LocalPath`, matching `ensurePullRequestForRepo`)
- [ ] Add the derived predicates `WorkRepos()`, `PullRequestPossible()`, `AllWorkLanded()`, `DivergedRepos()`
- [ ] Move the body of `HelixAPIServer.shouldOpenPullRequest` into `services.RepoSupportsPullRequests`, delete the old method, and update its call sites
- [ ] Delete `projectHasExternalRepo` from `spec_task_orchestrator.go`
- [ ] In `handlePullRequest`: clear `task.Metadata["error"]` and record the merge whenever `AllWorkLanded()` is true, before the timeout block
- [ ] In `handlePullRequest`: choose the timeout message in order — diverged → internal-only → original; on resolver error use the original message and clear nothing
- [ ] In `approveImplementation`: decide the workflow branch from `landing.PullRequestPossible()` instead of `shouldOpenPullRequest(project.DefaultRepoID)`
- [ ] In the non-PR branch of `approveImplementation`: fast-forward every internal work repo via the existing `mergeInternalRepoBranch`
- [ ] Unit tests for all six cases in design.md's test-plan table
- [ ] `cd api && go build ./...` and run the new tests with `CGO_ENABLED=1`
- [ ] E2E in the inner Helix at `http://localhost:8080`: external-default project + `bot_run` task landing on an internal repo; push, let it auto-merge, wait past the 5-minute window, confirm no error banner
- [ ] E2E regression: external-repo task with an unpushed branch still shows the original message
- [ ] Check whether a diverged branch in `tryAutoMergeBotRun` is still a log-line-only silent failure; report it in the PR (do not retroactively merge the five stranded `chris-outreach` branches, and do not touch that repo)
- [ ] Commit with conventional-commit messages on a feature branch, push, open a PR against `helixml/helix` main, report full PR URLs, and record any `NOT tested: <what and why>` in the PR body — do not merge
