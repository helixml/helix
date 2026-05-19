# feat: snappy LLM-generated titles for spec tasks

## Summary

Spec tasks no longer display the truncated raw prompt on the Kanban card
and tab strip. When a task is created, an async LLM call (using the existing
kodit enrichment model that already powers session-title generation) writes
a concise ≤60-char title to `spec_tasks.short_title`. The frontend already
preferred `user_short_title || short_title || name`, so the new value slots
straight into the display chain — and once the planning agent pushes
`requirements.md`, the H1 from that file supersedes the LLM draft.

## Why

The current `GenerateTaskNameFromPrompt` just collapses whitespace in the
prompt and truncates to 60 chars. A prompt like *"Give spec tasks a
generated title, and show this on the UI…"* turned into a card title that
was hard to scan in a busy Kanban board. The schema already had a
`short_title` column intended for exactly this purpose but no code path was
populating it.

## Changes

**Backend**
- `api/pkg/store/`: new `UpdateSpecTaskShortTitle(ctx, id, title) (bool, error)`
  with a conditional `WHERE short_title IS NULL OR short_title = ''` UPDATE so
  late-arriving LLM drafts never clobber the agent-supplied H1. Mock updated.
- `api/pkg/server/summary_service.go`: new
  `GenerateSpecTaskTitleAsync` + `cleanGeneratedTitle` helper that reuses
  the existing rate limiter, 30s timeout, and kodit enrichment model.
  Imperative-verb system prompt; quote / "Title:" / "Task:" stripping;
  word-boundary truncation at 60 chars.
- `api/pkg/services/spec_driven_task_service.go`: new `TitleGenerator`
  interface + `SetTitleGenerator` setter. Wired from `server.go` after
  both services exist. Fired from `CreateTaskFromPrompt` after the row
  is stored, guarded by `!testMode`.
- `api/pkg/services/git_http_server.go`: when the planning agent pushes
  `requirements.md`, the parsed H1 now updates both `task.Name` and
  `task.ShortTitle` in a single `UpdateSpecTask` call.

**Frontend**
- `frontend/src/components/tasks/taskTitle.ts`: new
  `specTaskTitle(task, fallback?)` helper centralising the
  `user_short_title || short_title || name || fallback` chain.
- Used by `TaskCard.tsx`, `SpecTaskDetailContent.tsx`, and 8 spots in
  `TabsView.tsx` that previously duplicated the chain inline.
- `TasksTable.tsx` / `CronTaskCard.tsx` / `EmptyTasksState.tsx` are NOT
  touched — those render `TypesTriggerConfiguration` (recurring agent
  triggers), not spec tasks.

**Tests**
- `api/pkg/server/summary_service_title_test.go`: 11 cases covering
  `cleanGeneratedTitle` cleanup behaviour.
- `api/pkg/services/git_helpers_test.go`: 6 cases covering
  `SpecTitleFromRequirements`.

## Configuration

This relies on the existing `system_settings.kodit_enrichment_provider`
and `kodit_enrichment_model` columns. If they're empty, task creation
still works and the card falls back to `task.name` (the truncated
prompt) — no degraded path, no error.

## Screenshots

![Before — truncated prompt](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002036_give-spec-tasks-a/screenshots/01-before-truncated-prompt.png)
![After — snappy short_title](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002036_give-spec-tasks-a/screenshots/02-after-snappy-title.png)
![User override — user_short_title wins](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002036_give-spec-tasks-a/screenshots/03-user-override.png)

## Test plan

- [x] `CGO_ENABLED=0 go build ./...` passes
- [x] `go test -run TestCleanGeneratedTitle ./api/pkg/server/` passes
- [x] `go test -run TestSpecTitleFromRequirements ./api/pkg/services/` passes
- [x] `cd frontend && yarn tsc && yarn build` passes
- [x] Inner-Helix manual verification: end-to-end task creation, no-LLM
      fallback, `short_title` display, and `user_short_title` override
      all behave as expected (screenshots above)
- [ ] End-to-end LLM happy path — requires a configured
      `kodit_enrichment_model` (not available in the sandbox)
