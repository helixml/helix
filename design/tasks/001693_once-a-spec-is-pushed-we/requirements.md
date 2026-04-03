# Spec Title Becomes Task Name After Spec Push

## Background

Tasks currently derive their name from the prompt at creation time (`GenerateTaskNameFromPrompt` in `spec_driven_task_service.go`). Once a spec is pushed, the first line of `requirements.md` becomes the authoritative name. The prompt field in the task form should lock at that point, establishing a clear single source of truth per stage.

## User Stories

**As a planner**, when I push spec documents, I want the task name to automatically update to the title from `requirements.md`, so I don't have to manually rename it.

**As a reviewer**, I want the prompt to be read-only once the spec is in review, so the spec title is the fixed reference point and edits go through the spec document itself.

## Acceptance Criteria

- [ ] When a spec is pushed and the task transitions to `spec_review`, the task's `name` is updated to the first non-empty line of `requirements.md` (stripped of leading `#` and whitespace)
- [ ] The task name from the spec title is applied regardless of its length (no 60-char truncation applied to spec titles; keep as-is or apply a generous limit like 200 chars)
- [ ] In `spec_review` and all later statuses, the prompt/description field in the task detail form is rendered as read-only (non-editable)
- [ ] In `backlog` and `spec_generation` statuses, the prompt field remains editable (existing behavior unchanged)
- [ ] If `requirements.md` has no usable first line, the name is left unchanged
