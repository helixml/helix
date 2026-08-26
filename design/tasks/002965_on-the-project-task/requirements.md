# Requirements: Show Only Active Branches When Creating Project Tasks

## User Stories

- As a project user, I want the “continue on an existing branch” selector to show only active branches from the primary repository, so I do not accidentally continue work on a completed or deleted branch.
- As a project user, I want a previously merged branch to reappear after new commits are pushed to it, because the branch has become active again.

## Acceptance Criteria

- The existing-branch selector on the project task creation form excludes the repository's default branch and Helix's `helix-specs` branch.
- A branch is considered inactive only when Helix has affirmative evidence that the branch was merged and the branch's current tip still matches the merged head commit.
- A newly created branch remains visible even when its tip is identical to the default-branch tip and it has no unique commits yet.
- A branch with no known matching merge record remains visible; Git ancestry alone must not be used to hide it.
- If new commits are added to a previously merged branch, it is shown again.
- Branches deleted from an external upstream are not shown, even when a stale local ref remains in Helix's repository mirror.
- The selector retains its current alphabetical sorting, search, empty-state text, and submission behavior.
- Failures to determine active branches are reported through the existing API error handling; the form must not silently present an unfiltered list.
- Backend and frontend tests cover active, merged, reactivated, deleted, default, and `helix-specs` branches.

## Out of Scope

- Automatically deleting merged branches.
- Changing branch options for repositories other than the project's primary repository.
- Defining activity by a fixed “recently merged” time window; the current tip compared with recorded merge evidence is the source of truth.

## Open Questions

- Is conservative behavior acceptable for branches merged outside Helix with no pull-request metadata available? Such a merge cannot be distinguished reliably from a fresh branch at the same commit, so this design keeps the branch visible rather than hiding valid work.
