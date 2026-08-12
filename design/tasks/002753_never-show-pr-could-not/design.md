# Design: Never Show "PR Could Not Be Created" When Work Landed on an Internal Repo

## What I found in the codebase (read this before starting)

Discovery notes — these save the next agent the search:

- **`ListGitRepositories{ProjectID}` goes through a junction table.**
  `api/pkg/store/git_repository.go:83` does
  `INNER JOIN project_repositories ON project_repositories.repository_id = git_repositories.id`.
  The `git_repositories.project_id` column (`types/git_repositories.go:146`) is *not* what
  the filter uses. `GetProjectsForRepository` (`store/project_repository.go:141`) reads the
  same junction in reverse — that is how `handleFeatureBranchPush` finds a task from a push.
- **There is already a proven per-repo "did the work land here?" test.**
  `ensurePullRequestForRepo` (`spec_task_workflow_handlers.go:532-577`) does exactly this,
  and it works for internal *and* external repos because external repos also have a
  `LocalPath` mirror:
  1. `gitRepositoryService.ListBranches(repo.ID)` → does `task.BranchName` exist here?
  2. `services.GetDivergence(repo.LocalPath, "refs/heads/"+branch, "refs/heads/"+default)`
     → `ahead == 0` means "no commits ahead", i.e. contained in the default branch.
  **Reuse this. Do not invent a second notion of "the task's repos".**
- **The workspace repo set is the project repo set.** `spec_driven_task_service.go:562`
  lists `ProjectID: task.ProjectID` and feeds it to `zedAgent.SetRepoContext(projectRepos,
  project.DefaultRepoID)` (`types/types.go:2117`). Same call, same set. So "the task's
  repos" == project repos via the junction. (See Open Question 1 in requirements.md.)
- **`tryAutoMergeBotRun` (`git_http_server.go:1302`) is the working reference** for
  "guard on the pushed repo, not the project default" — `70ab4b28a` fixed that already.
  It is passed the pushed repo by `handleFeatureBranchPush` (`git_http_server.go:1082`).
- **`shouldOpenPullRequest`** (`spec_task_workflow_handlers.go:899`) is the canonical
  "can this repo host a PR?" predicate (GitHub / GitLab / ADO). It lives in package
  `server`; the orchestrator lives in package `services`, and `server` imports `services`,
  so the shared helper must live in `services`.
- **Only two code paths write `pull_request`** other than a genuine detected PR:
  `approveImplementation` (`spec_task_workflow_handlers.go:163`) and the real-PR detectors
  (`spec_task_orchestrator.go:1432`, `spec_driven_task_service.go:1943`). Only the first
  is in scope.

## Architecture

One new file, `api/pkg/services/spec_task_repo_landing.go`, exporting a single resolver
used by both the orchestrator (package `services`) and the workflow handler (package
`server`).

```go
// RepoLandingState describes where a task's branch stands in one repo.
type RepoLandingState string

const (
    RepoLandingAbsent   RepoLandingState = "absent"   // branch not in this repo
    RepoLandingMerged   RepoLandingState = "merged"   // ahead == 0: contained in default branch
    RepoLandingPending  RepoLandingState = "pending"  // ahead > 0, behind == 0: FF still possible
    RepoLandingDiverged RepoLandingState = "diverged" // ahead > 0 && behind > 0: cannot FF
)

type RepoLanding struct {
    Repo    *types.GitRepository
    State   RepoLandingState
    CanOpenPR bool // RepoSupportsPullRequests(repo)
}

type TaskLanding struct{ Repos []RepoLanding }

// ResolveTaskLanding lists the task's repos (same resolution as workspace setup:
// ListGitRepositories{ProjectID: task.ProjectID}) and classifies each one.
func ResolveTaskLanding(ctx context.Context, s store.Store, branches BranchLister, task *types.SpecTask) (TaskLanding, error)
```

Derived predicates on `TaskLanding` — this is where the whole fix lives:

| Predicate | Meaning |
|---|---|
| `WorkRepos()` | repos whose state is `merged`, `pending` or `diverged` |
| `PullRequestPossible()` | `true` if any **work repo** has `CanOpenPR`; if there are **no** work repos, `true` if any repo in the set has `CanOpenPR` (preserves the genuine "agent never pushed to the external repo" diagnosis) |
| `AllWorkLanded()` | there is ≥1 work repo and every work repo is `merged` |
| `DivergedRepos()` | work repos in state `diverged` |

`RepoSupportsPullRequests(repo)` moves into `services` (the body of today's
`shouldOpenPullRequest`); `HelixAPIServer.shouldOpenPullRequest` is **deleted** and its
three call sites updated to call the `services` function. No wrapper, no dead code.

### Why this shape

The correct question is *"can the repo this task's branch actually went to host a PR?"*,
not *"does this project contain any external repo anywhere"*. Classifying every repo in
the task's set once, and then asking that question of the repos that actually have the
branch, answers it without inventing a new repo-resolution and without a fallback chain.

For the HelixOS shape: `helixos` and `helix-next` are `absent` (the bot never pushed
there), `chris-outreach` is `merged` ⇒ `WorkRepos() = [chris-outreach]`,
`PullRequestPossible() = false`, `AllWorkLanded() = true`. For a real external failure:
no work repos at all ⇒ `PullRequestPossible() = true` ⇒ original message survives.

## Defect fixes

### A + C — `handlePullRequest` (`spec_task_orchestrator.go:1031`)

Replace `projectHasExternalRepo` (delete it) with a single decision block, evaluated
**before** the existing `hasErr` guard:

1. `AllWorkLanded()` ⇒ **clear** `task.Metadata["error"]`, set `MergedToMain`/`MergedAt`
   if unset, persist, and skip the timeout error entirely. This alone fixes the reported
   row, whose `merged_to_main` was already `t` eight minutes before the error was written.
   Runs on every poll, not just after the timeout — that is the self-healing mechanism
   for existing stale rows (US-4), so no migration is needed.
2. Past the 5-minute window with no PRs, pick the message in this order (first match wins):
   - `len(DivergedRepos()) > 0` → *"The feature branch `<branch>` in `<repo>` has diverged
     from `<default>` and cannot be fast-forwarded. The agent must merge `<default>` into
     `<branch>` and push again."*
   - `!PullRequestPossible()` → the existing internal-repo message from `77726b750`
     (unchanged wording).
   - otherwise → the existing original message (unchanged wording).

   Keep the `if _, hasErr := ...; !hasErr` write-once guard: step 1 is now the eraser.

**Error policy on resolution failure:** if `ResolveTaskLanding` errors (store or git
failure), fall back to the *original* message, exactly as `projectHasExternalRepo`
returned `true` on error today. A transient failure must not assert something false; it
degrades to the pre-existing behaviour. Explicitly: it must **not** clear an error.

### B — `approveImplementation` (`spec_task_workflow_handlers.go:143`)

Today the whole workflow branch comes from `GetGitRepository(project.DefaultRepoID)`.
Change the condition from `s.shouldOpenPullRequest(repo)` to
`landing.PullRequestPossible()`, computed from the same resolver. The default repo is
still used for `DefaultBranch`, `ensurePullRequestsForAllRepos`, and the push
instruction — only the *decision* moves off it.

When `PullRequestPossible()` is false, the existing internal path runs; extend it to
fast-forward **every internal work repo** by reusing `mergeInternalRepoBranch`
(`spec_task_workflow_handlers.go:~740`) rather than only the default repo. That is a
generalisation of existing code, not a new path.

Net effect for the bug: an autonomous run whose work landed by fast-forward on an
internal repo is never put into `pull_request` waiting for a merge that already happened.

## Decisions and trade-offs

- **Shared helper in `services`, not `server`** — `server` imports `services`, not the
  reverse. Putting it anywhere else forces duplication, which is what caused this bug.
- **Branch containment as ground truth, not `merged_to_main`** — the flag is a cache that
  was already wrong once. `GetDivergence` against the repo's default branch is the fact.
  `merged_to_main` is written *from* the fact, never read as the sole input.
- **No new DB column.** A `last_push_repo_id` was considered (record the repo a push went
  to). Rejected: the branch-containment check already answers it for every repo, and a
  task can push to several repos, so a single "last" column would be another partial truth
  to drift.
- **No frontend change.** `SpecTaskDetailContent.tsx:754`, `TaskCard.tsx:1220` and
  `SpecTaskActionButtons.tsx:870` render `task.metadata.error` verbatim; once the source
  stops writing (and starts clearing) the string, the banner is gone. Hiding it in the UI
  would leave the bad state in the row.
- **No migration.** Requirement US-4's clearing logic resolves existing rows the next time
  the orchestrator evaluates them.

## Gotchas

- Internal repos with `LocalPath == ""` and repos whose default branch is unset cannot be
  classified — treat as `absent`, and let the error path degrade to the original message
  rather than asserting "landed".
- The orchestrator's PR poll runs every ~1 minute and the timeout window is 5 minutes off
  `StatusUpdatedAt`; the E2E test has to wait it out (~6 min).
- `ListBranches` on an external repo can hit the provider — the divergence check is local
  (`repo.LocalPath`) and should be preferred; only fall back to `ListBranches` when the
  local path is missing, matching `ensurePullRequestForRepo`'s ordering.

## Test plan

Go unit tests in `api/pkg/services/spec_task_orchestrator_test.go` (existing gomock
`store.MockStore` suite pattern, see `spec_task_orchestrator_test.go:1003` for the
`ListGitRepositories` expectation style) plus a new
`spec_task_repo_landing_test.go`:

| Case | Repo set | Expected |
|---|---|---|
| HelixOS shape | 2 external `absent` + 1 internal `merged` | no error written; existing error cleared; `merged_to_main` true |
| Genuine failure | 1 external, no work repos | original message written verbatim |
| All-internal project | 1 internal `absent` | `77726b750` internal-repo message |
| Diverged | 1 internal `diverged` | diverged message naming branch, repo and remedy |
| Stale error + landed | internal `merged`, `metadata.error` preset | error key absent after the poll |
| Resolver error | store returns error | original message; existing error untouched |

E2E in the inner Helix per requirements.md §Verification — this is the evidence that
matters; the unit tests are not a substitute.
