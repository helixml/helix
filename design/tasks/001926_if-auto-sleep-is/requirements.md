# Requirements

## Background

When a spec task's pull request is merged on GitHub (or any tracked external repo), the orchestrator's PR poll loop detects the merge, transitions the task to `TaskStatusDone`, and `handleDone()` immediately calls `containerExecutor.StopDesktop()` — terminating the agent's desktop session.

The "Keep Alive" toggle (lock icon in the task detail view) was introduced to prevent the **idle-based** auto-shutdown of the desktop container. However, today it does **not** prevent the **PR-merge-triggered** shutdown. So a user who has explicitly turned the lock green (Keep Alive ON) loses their session as soon as the PR lands — which is exactly when they may want to keep poking at the agent (verify the merge result, follow-up commits, run the next task in the same session, etc.).

## User Stories

- **As a developer**, when I have enabled Keep Alive (green lock icon) on a task, I want my agent's desktop session to remain running after the PR is merged, so I can continue iterating without having to restart the session.

- **As a developer**, when Keep Alive is disabled (open lock icon — the default), I want existing behavior preserved: a merged PR transitions the task to Done and shuts down the desktop.

## Acceptance Criteria

1. **WHEN** a task has `keep_alive = true` **AND** all of its tracked PRs are detected as merged, **THEN** the desktop container MUST remain running.
2. **WHEN** a task has `keep_alive = true` **AND** the orchestrator detects the branch was merged to main directly (no PR / branch-merge fallback paths), **THEN** the desktop container MUST remain running.
3. **WHEN** a task has `keep_alive = false` (default), **THEN** existing PR-merge → task-done → desktop-stopped behavior MUST be unchanged.
4. The task's status MAY still transition to `TaskStatusDone` (the merge is a real event and should be recorded), and `MergedToMain`/`MergedAt`/`CompletedAt` MUST still be set correctly so PR-tracking UI continues to work.
5. **WHEN** the user toggles Keep Alive OFF on a task that is already in `TaskStatusDone` with a still-running desktop, **THEN** the desktop SHOULD be stopped (so the user has an explicit way to release the resources).
6. The lock-icon toggle in the task detail UI MUST remain visible and operable while the desktop is running, regardless of task status, so the user can disable Keep Alive after merge to release the desktop.
7. Behavior MUST be consistent across all three merge-detection code paths in `spec_task_orchestrator.go`:
   - `pollPullRequests` "all PRs merged" branch (around line 778)
   - `pollPullRequests` branch-merged-to-main fallback (around line 848)
   - `detectExternalPRActivity` merged-PR detection (around line 1063)
   - `checkBranchMergedNoPR` no-PR fallback (around line 1116)

## Out of Scope

- Adding any helix-side action that **performs** a PR merge. Merges happen on GitHub; we only detect them. The user's phrase "stop merging all PRs triggering shutting down the agent" refers to the merge-detection-triggered shutdown, not to a merge action initiated by helix.
- Changing the idle-based auto-sleep logic (already correctly gated by `keep_alive`).
- Changing the lock-icon UI itself (no new buttons or tooltips needed beyond what already exists).
