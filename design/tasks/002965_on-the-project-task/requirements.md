# Requirements: Show Only Active Branches When Creating Project Tasks

## User Stories

- As a project user, I want the “continue on an existing branch” selector to show only active branches from the primary repository, so I do not accidentally continue work on a completed or deleted branch.
- As a project user, I want a previously merged branch to reappear after new commits are pushed to it, because the branch has become active again.

## Acceptance Criteria

- The existing-branch selector on the project task creation form excludes the repository's default branch and Helix's `helix-specs` branch.
- A branch is considered inactive only when its current tip exactly matches a non-first parent of a merge commit reachable from the default branch.
- A newly created branch remains visible even when its tip is identical to the default-branch tip and it has no unique commits yet.
- A branch without exact merge-parent evidence remains visible.
- If new commits are added to a previously merged branch, it is shown again.
- Branches deleted from an external upstream are not shown, even when a stale local ref remains in Helix's repository mirror.
- The selector retains its current alphabetical sorting, search, empty-state text, and submission behavior.
- Failures to determine active branches are reported through the existing API error handling; the form must not silently present an unfiltered list.
- Backend and frontend tests cover active, merged, reactivated, deleted, default, and `helix-specs` branches.

## Out of Scope

- Automatically deleting merged branches.
- Changing branch options for repositories other than the project's primary repository.
- Defining activity by a fixed “recently merged” time window.
- Detecting squash, rebase, or fast-forward merges, which cannot be distinguished reliably from fresh branches using the local commit graph alone.

## Open Questions

None.
