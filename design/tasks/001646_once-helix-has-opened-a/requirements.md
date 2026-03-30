# Requirements: Prevent Duplicate PR Creation

## Problem

Helix opens duplicate PRs when a user manually changes the PR externally:

1. **Rename a PR title** — a simple title rename on GitHub causes Helix to open a second PR.
2. **Close a PR** — closing the PR causes Helix to open a new duplicate PR on the next polling cycle (every 30 seconds).

The exact mechanism for the rename case needs reproduction to confirm, but the close case has a clear root cause: the GitHub `ListPullRequests` call only fetches open PRs, so a closed PR is invisible to Helix's deduplication check, and the polling loop creates a new one.

## User Stories

- **As a user**, when I close a Helix-created PR on GitHub, I expect Helix to respect that decision and not open a new duplicate PR.
- **As a user**, when I rename a Helix-created PR on GitHub, I expect Helix to not open a second PR.

## Acceptance Criteria

- [ ] Closing a Helix PR on GitHub does not cause Helix to open a new PR automatically.
- [ ] After closing a PR, if Helix is instructed to open a PR again (e.g., by explicit user action), it may do so — but not silently via polling.
- [ ] Renaming a PR title on GitHub does not result in a duplicate PR from Helix.
- [ ] Tasks in `pull_request` status with all PRs closed continue to be tracked but do not produce new PRs.
- [ ] No regression for the normal flow: when Helix first creates a PR for a task, deduplication still works correctly (no duplicate on first creation).
