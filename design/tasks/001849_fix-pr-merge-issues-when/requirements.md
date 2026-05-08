# Requirements

## User Stories

### 1. Merged task shows correct status message
**As a** user viewing a task card after its PR has been merged,
**I want** to see "Task finished — Merged to default branch"
**So that** I know the task completed successfully without confusing error messages.

**Acceptance Criteria:**
- When a PR is merged and the task moves to `done` status, any previous error in `task.metadata.error` is cleared
- The task card shows the green success alert ("Task finished / Merged to default branch"), not the red error banner
- This works for all merge detection paths: PR status polling, external PR detection, and branch-merge fallback

### 2. Closed/deleted PRs don't trigger duplicate PR creation
**As a** user who closes a PR on GitHub (intentionally or accidentally),
**I want** Helix to respect that decision and not create a replacement PR,
**So that** I don't end up with duplicate PRs on my repository.

**Acceptance Criteria:**
- When an existing PR for a spec task is closed on GitHub, Helix does not create a new PR for the same branch
- When an existing PR is deleted on GitHub, Helix does not create a new PR for the same branch
- The task's `RepoPullRequests` retains the closed/merged PR record for reference
- PR updates (new commits pushed to the branch) do not trigger duplicate PR creation
