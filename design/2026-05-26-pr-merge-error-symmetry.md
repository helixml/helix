# PR-poll error path: symmetric flag treatment

## Symptom

Self-hosted deployment reports tasks moving from `pull_request` status to `done` (the "Merged" Kanban column) when their PRs on the upstream Git host have not actually been merged. Pattern correlates with controlplane pod restarts (Helm upgrades, node rotations).

## Root cause

`api/pkg/services/spec_task_orchestrator.go:processExternalPullRequestStatus` decides whether a task should transition to `done` by polling each tracked PR's state from the configured Git provider:

```go
anyOpen := false
allMerged := true          // <-- starts true
allClosed := true

for i, repoPR := range task.RepoPullRequests {
    pr, err := o.gitService.GetPullRequest(ctx, repoPR.RepositoryID, repoPR.PRID)
    if err != nil {
        log.Warn()...
        allClosed = false  // "can't confirm closed"
        continue           // allMerged left untouched
    }
    switch pr.State {
    case Open:    anyOpen = true; allMerged = false; allClosed = false
    case Merged:  allClosed = false
    case Closed:  allMerged = false
    case Unknown: allMerged = false; allClosed = false
    }
}

if allMerged && len(task.RepoPullRequests) > 0 {
    task.Status = TaskStatusDone
    ...
}
```

The error branch carefully clears `allClosed` but forgets `allMerged`. So if `GetPullRequest` errors for every tracked PR in a single poll cycle, the loop ends with `allMerged` still at its `true` default and the task wrongly transitions to `done`.

## Why pod restart triggers it

The PR-poll loop fires every 30s. On a freshly-rolled pod:

- DNS / TLS / outbound-connection pool caches are cold
- Provider auth-token caches are empty
- The first poll cycle is the highest-risk window for transient errors

Same pattern fires any time the Git provider returns 5xx / 429 / 404 / TLS hiccup, or a token quietly loses access to a project. The bug is provider-agnostic, but customer reports we have are all GitLab self-hosted (where self-signed certs, VPN paths, and bespoke proxies make transient errors more common than against github.com).

## Fix

One line, symmetric to the existing `allClosed = false`:

```go
if err != nil {
    log.Warn()...
    allMerged = false   // ADD: can't confirm merged either
    allClosed = false
    continue
}
```

Behaviour during a real Git-provider outage becomes "task stays in `pull_request` status", which is the correct conservative default. The next successful poll cycle re-evaluates against live state.

## Other PR→Done sites considered but out of scope

The same file has three other sites that auto-transition tasks to `done`:

- `processExternalPullRequestStatus` branch-merge fallback (line 863-906) — runs when no PRs are tracked or all are closed/errored, checks `IsBranchMerged`. **Important**: with this fix in place, `allMerged=false` and `allClosed=false` after all-errors, so the loop falls through to the fallback. For Robert's reported symptom (active PR branches with commits not yet in default) `IsBranchMerged` correctly returns false → no transition. But the fallback retains edge cases that could still wrongly transition:
  - Branch with zero new commits (HEAD == default HEAD → trivially "merged" because any commit is its own ancestor under `merge-base --is-ancestor`).
  - `LastPushCommitHash` happens to be in default's history for unrelated reasons (e.g. that commit landed via a different PR).
  - Local clone is stale and has an outdated view of default.

  Not addressed here. Should be a separate change. The covered tests in this PR include `TestProcessExternalPullRequestStatus_AllErrorsWithBranch_FallbackDoesNotTransition` which pins the safe production-typical case.

- `detectExternalPRActivity` external-merge detection (line 1125-1152) — operates only on `spec_review` / `implementation` status tasks, not `pull_request`. Not part of this incident.

- `detectExternalPRActivity` branch-merge fallback (line 1170+) — same edge case as the first bullet.

A broader rework that removes all four auto-transitions in favour of an explicit `mark_task_complete` path was started on a branch (commit `701447f2b`, "Backend: kill auto-transitions to done") but does not appear on any current branch. If that work resurfaces it supersedes this fix.

## Residual risk summary

This fix is **sufficient** for the customer-reported symptom (tasks moving from Pull Request to Merged after Helm upgrades / GitLab transient errors), confirmed by code-trace:

1. Customer's tasks have active PR branches with commits not yet in default → `IsBranchMerged` returns false in the fallback path.
2. The bug-trigger window (cold-pod transient errors against self-hosted GitLab) hits the `allMerged` path, which this fix closes.

It is **not** a complete defence against all paths that could wrongly transition a task to Done. The branch-merge fallback has its own edge cases that should be addressed in a follow-up — ideally by deleting auto-transitions entirely in favour of an explicit completion path (the `701447f2b` direction).

## Testing

Extracted a `GitService` interface for the six methods the orchestrator uses (`GetPullRequest`, `GetCIStatus`, `ListPullRequests`, `GetRepository`, `IsBranchMerged`, `IsCommitInBranch`). The concrete `*GitRepositoryService` satisfies this interface in production. Generated `MockGitService` via mockgen.

Three new gomock-based tests on `processExternalPullRequestStatus`:

- `TestProcessExternalPullRequestStatus_AllErrors_StaysInPullRequest` — the bug regression. Two tracked PRs, GetPullRequest errors for both, asserts task stays in `pull_request`. This test fails on `main` without the fix because the task transitions to `done`.
- `TestProcessExternalPullRequestStatus_AllMerged_TransitionsToDone` — happy path, guards against an over-eager fix that breaks legitimate transitions.
- `TestProcessExternalPullRequestStatus_MergedPlusError_StaysInPullRequest` — the symmetric case the fix specifically defends: one PR genuinely merged, one errored. Pre-fix this also wrongly transitions because `allMerged` was never cleared by the error.

All three pass against the fix.

The other three auto-transition sites (branch-merge fallback in `processExternalPullRequestStatus`, and both sites in `detectExternalPRActivity`) now have a path to being unit-tested via the same `GitService` interface, but are out of scope for this fix.
