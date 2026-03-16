# Requirements: Labels for Spec Tasks

## Context

`SpecTask` already has a `Labels []string` field (JSON array in PostgreSQL). The field is persisted but has no dedicated store methods, no API endpoints for label management, and no UI support. This task adds the full vertical slice: store → API → UI.

## User Stories

**US1 – Add labels to a task**
As a user, I can add one or more string labels to a spec task so I can categorize work.

**US2 – Remove labels from a task**
As a user, I can remove a label from a spec task when it no longer applies.

**US3 – View available labels**
As a user, I can see all labels currently used within my project so I can reuse existing labels.

**US4 – Filter tasks by label**
As a user, I can filter the task list by one or more labels so I only see relevant tasks.

## Acceptance Criteria

- [ ] Labels are free-form strings (no predefined set required).
- [ ] A task can have zero or more labels.
- [ ] Adding/removing a label does not require re-saving the entire task form.
- [ ] `GET /api/v1/projects/{projectId}/labels` returns all unique labels in use across that project's tasks.
- [ ] `POST /api/v1/spec-tasks/{taskId}/labels` adds a label (idempotent if it already exists).
- [ ] `DELETE /api/v1/spec-tasks/{taskId}/labels/{label}` removes a label.
- [ ] Task list API (`GET /api/v1/spec-tasks`) accepts an optional `labels` query param (comma-separated) and returns only tasks that have ALL specified labels.
- [ ] The UI shows a label input/autocomplete on the task detail view.
- [ ] The UI shows a label filter control on the task list view.
- [ ] Labels are displayed as chips/badges on task cards in the list.
