# Requirements: Prevent Duplicate PR Creation

## Problem

Helix opens duplicate PRs when a user manually changes the PR state:

1. **Rename a PR title** — if the rename is done by closing the PR and reopening with a new title, the closed PR becomes invisible to Helix and a duplicate is created.
2. **Close a PR** — Helix immediately creates a new PR on the next polling cycle (every 30 seconds).

## User Stories

- **As a user**, when I close a Helix-created PR on GitHub, I expect Helix to respect that decision and not open a new duplicate PR.
- **As a user**, when I rename a PR (by closing it and reopening with a different title), I expect Helix not to create an additional PR.

## Acceptance Criteria

- [ ] Closing a Helix PR on GitHub does not cause Helix to open a new PR automatically.
- [ ] After closing a PR, if Helix is instructed to open a PR again (e.g., by explicit user action), it may do so — but not silently via polling.
- [ ] Renaming a PR by close+reopen does not result in a third duplicate PR from Helix.
- [ ] Tasks in `pull_request` status with all PRs closed continue to be tracked but do not produce new PRs.
- [ ] No regression for the normal flow: when Helix first creates a PR for a task, deduplication still works correctly (no duplicate on first creation).
