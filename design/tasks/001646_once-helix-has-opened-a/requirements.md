# Requirements: Prevent Duplicate PR on Rename

## Problem

Once Helix has opened a PR, **manually renaming the PR title on GitHub** causes Helix to open a duplicate PR.

The close-PR scenario is addressed in a separate PR. This spec covers only the rename case.

## User Story

- **As a user**, when I rename a Helix-created PR title on GitHub, I expect Helix not to open a second PR.

## Acceptance Criteria

- [ ] Renaming a PR title on GitHub (PR stays open, same branch) does not cause Helix to open a duplicate PR.
- [ ] If the push-triggered path (`ensurePullRequest` in git_http_server.go) encounters an already-tracked PR for the same repo, it does not overwrite the user's chosen title back to Helix's version.
- [ ] No regression: when Helix first creates a PR for a task that has no existing PR, it still creates the PR correctly.
