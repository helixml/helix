# Requirements: Auto-Start Option on New Spec Task Form

## Background

Projects have a global "Auto-start backlog tasks" setting (`auto_start_backlog_tasks`). When disabled, tasks sit in the backlog indefinitely until manually started. Users sometimes want a specific task to start immediately regardless of this project-wide setting — e.g. urgent work in a project that normally uses manual task control.

## User Stories

**US1:** As a user creating a task, I want to mark it to start immediately even if the project's auto-start is off, so that I don't have to manually move it to planning after creation.

**US2:** As a user, I want the auto-start option to be opt-in per task (defaulting to off), so that the project's existing manual-control behaviour is preserved unless I explicitly override it.

## Acceptance Criteria

1. The New Spec Task form has a checkbox labeled **"Start immediately"** (or similar) that defaults to unchecked.
2. When checked, the created task bypasses the project's `auto_start_backlog_tasks` check and immediately begins spec generation (or implementation if "Just Do It" mode is also on).
3. When unchecked, task creation behaviour is unchanged — the task lands in backlog and waits for project auto-start or manual trigger.
4. The checkbox is visible regardless of whether the project's auto-start is enabled or disabled.
5. The option is also wired through the API (`auto_start` field in `CreateTaskRequest`).
