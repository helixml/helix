# Requirements: Show Only Active Branches When Creating Project Tasks

## User Stories

- As a project user, I want the “continue on an existing branch” selector to show only active branches from the primary repository, so I do not accidentally continue work on a completed or deleted branch.
- As a project user, I want a previously merged branch to reappear after new commits are pushed to it, because the branch has become active again.

## Acceptance Criteria

- The existing-branch selector on the project task creation form excludes the repository's default branch and Helix's `helix-specs` branch.
- A branch is active when its current tip contains at least one commit not reachable from the repository's current default-branch tip.
- A branch whose current tip is already reachable from the default branch is omitted. This covers normal merge commits and fast-forward merges without relying on task status or branch age.
- If new commits are added to a previously merged branch, it is shown again.
- Branches deleted from an external upstream are not shown, even when a stale local ref remains in Helix's repository mirror.
- The selector retains its current alphabetical sorting, search, empty-state text, and submission behavior.
- Other repository screens and callers that need the complete branch list continue to receive it.
- Failures to determine active branches are reported through the existing API error handling; the form must not silently present an unfiltered list.
- Backend and frontend tests cover active, merged, reactivated, deleted, default, and `helix-specs` branches.

## Out of Scope

- Automatically deleting merged branches.
- Changing branch options for repositories other than the project's primary repository.
- Defining activity by a fixed “recently merged” time window; commit reachability is the source of truth.

## Open Questions

- Should a squash- or rebase-merged branch be hidden using pull-request provider metadata? Git ancestry alone cannot prove that its changes were merged because its tip is not part of the default branch's history. The initial implementation can reliably identify merge-commit and fast-forward merges; supporting squash/rebase merges requires provider-specific data or a patch-equivalence heuristic.
