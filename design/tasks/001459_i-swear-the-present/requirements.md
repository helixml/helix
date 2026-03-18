# Requirements: Fix HELIX_PRIMARY_REPO_NAME Incorrect Value

## Problem

When a "Helix Setup" terminal session starts, the `HELIX_PRIMARY_REPO_NAME` environment variable is set to `docs` instead of `helix-4`, even though the project's primary repository is correctly configured as `helix-4` in the UI. This causes the working directory to land in the wrong repo on startup.

## Root Cause (Confirmed)

Two bugs combine to cause this:

**Bug 1 (Primary):** In `session_handlers.go` (~line 1869), `agent.PrimaryRepositoryID` is unconditionally set to `projectRepos[0].ID` without ever consulting `project.DefaultRepoID`:

```go
// BUGGY - ignores DefaultRepoID entirely
if len(projectRepos) > 0 {
    agent.PrimaryRepositoryID = projectRepos[0].ID
}
```

All other code paths (`spec_driven_task_service.go`, `project_handlers.go`) correctly check `project.DefaultRepoID` first and only fall back to `[0]` when it's empty.

**Bug 2 (Secondary):** `ListGitRepositories` orders by `git_repositories.created_at DESC`. The `docs` repository record was created more recently than `helix-4`, so it sorts first, making `projectRepos[0]` be `docs`.

## User Stories

**As a developer**, when my Helix workspace terminal opens, I want `HELIX_PRIMARY_REPO_NAME` to match the repository marked as Primary in my project settings, so that my terminal starts in the correct working directory.

**As a developer**, I want the primary repo selection logic to be consistent across all code paths (session creation, task creation, spec tasks), so behavior is predictable regardless of how a session is started.

## Acceptance Criteria

- [ ] After restart, the Helix Setup terminal's `HELIX_PRIMARY_REPO_NAME` reflects the project's configured primary repository (`helix-4`), not the first-sorted repo
- [ ] Changing the primary repo in project settings is reflected on next session start
- [ ] The fallback to `projectRepos[0]` only occurs when `project.DefaultRepoID` is genuinely unset
