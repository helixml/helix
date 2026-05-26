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

- `processExternalPullRequestStatus` branch-merge fallback (line 863-906) — runs when no PRs are tracked or all are closed, checks `IsBranchMerged`. Has a separate edge case where a task branch with zero new commits trivially "merges" (head is its own ancestor). Not addressed here; should be a separate change.
- `detectExternalPRActivity` external-merge detection (line 1125-1152) — operates only on `spec_review` / `implementation` status tasks, not `pull_request`. Not part of this incident.
- `detectExternalPRActivity` branch-merge fallback (line 1170+) — same edge case as above.

A broader rework that removes all four auto-transitions in favour of an explicit `mark_task_complete` path was started on a branch (commit `701447f2b`, "Backend: kill auto-transitions to done") but does not appear on any current branch. If that work resurfaces it supersedes this fix.

## Testing

Extracted a `GitService` interface for the six methods the orchestrator uses (`GetPullRequest`, `GetCIStatus`, `ListPullRequests`, `GetRepository`, `IsBranchMerged`, `IsCommitInBranch`). The concrete `*GitRepositoryService` satisfies this interface in production. Generated `MockGitService` via mockgen.

Three new gomock-based tests on `processExternalPullRequestStatus`:

- `TestProcessExternalPullRequestStatus_AllErrors_StaysInPullRequest` — the bug regression. Two tracked PRs, GetPullRequest errors for both, asserts task stays in `pull_request`. This test fails on `main` without the fix because the task transitions to `done`.
- `TestProcessExternalPullRequestStatus_AllMerged_TransitionsToDone` — happy path, guards against an over-eager fix that breaks legitimate transitions.
- `TestProcessExternalPullRequestStatus_MergedPlusError_StaysInPullRequest` — the symmetric case the fix specifically defends: one PR genuinely merged, one errored. Pre-fix this also wrongly transitions because `allMerged` was never cleared by the error.

All three pass against the fix.

The other three auto-transition sites (branch-merge fallback in `processExternalPullRequestStatus`, and both sites in `detectExternalPRActivity`) now have a path to being unit-tested via the same `GitService` interface, but are out of scope for this fix.
