# Implementation Tasks: Generate Snappy LLM Titles for Spec Tasks

## Backend — store layer

- [x] Add `UpdateSpecTaskShortTitle(ctx, id, title string) (bool, error)` to the `Store` interface and the postgres implementation in `api/pkg/store/store_spec_tasks.go`. Returns `bool` so callers can tell whether the conditional UPDATE actually fired. Conditional `WHERE id = ? AND (short_title IS NULL OR short_title = '')` keeps the LLM draft from clobbering the agent's H1.
- [x] Hand-extend the gomock store mock (no regen needed — single method added matches the existing mock pattern).

## Backend — title generation

- [ ] Add `GenerateSpecTaskTitleAsync(ctx, taskID, ownerID, prompt string)` to `api/pkg/server/summary_service.go`. Reuse the existing rate limiter (`s.mu` / `s.pendingCount` / `s.maxConcurrent`), the 30-second timeout, `MaxTokens: 50`, `Temperature: 0.3`, and the same trim/truncate cleanup used by `updateSessionTitle`.
- [ ] Use the imperative-verb snappy-title system prompt from `design.md` §1. Cap result at 60 chars at a word boundary.
- [ ] Bail out (no DB write) when `KoditEnrichmentProvider` or `KoditEnrichmentModel` are empty, or when the LLM call errors / times out. Log at `Warn`, not `Error` — this is best-effort.

## Backend — wire into task creation

- [ ] Add a `summaryService *SummaryService` field to `SpecDrivenTaskService` and pass it through `NewSpecDrivenTaskService`. Keep it nilable so existing unit tests don't need to construct one.
- [ ] In `api/pkg/server/server.go` (wherever the spec-driven task service is constructed), pass the existing `SummaryService` instance.
- [ ] In `SpecDrivenTaskService.CreateTask` (`api/pkg/services/spec_driven_task_service.go`), after the successful `s.store.CreateSpecTask(...)`, call `s.summaryService.GenerateSpecTaskTitleAsync(ctx, task.ID, task.UserID, task.OriginalPrompt)` (guarded by a nil check).

## Backend — agent H1 path

- [ ] In `api/pkg/services/git_http_server.go` around lines 1660-1671, when `SpecTitleFromRequirements` returns a non-empty title and it differs from `task.ShortTitle`, set `task.ShortTitle = specTitle` alongside the existing `task.Name` update. Keep them in a single `UpdateSpecTask` call.

## Backend — tests

- [ ] Add a `summary_service_test.go` case for `GenerateSpecTaskTitleAsync` happy path (mock provider returns "Snappy spec-task titles" → assert `store.UpdateSpecTaskShortTitle` called with that string).
- [ ] Add a `summary_service_test.go` case for the no-config fallback (no kodit model → no store write).
- [ ] Update / add a `git_http_server_test.go` case that confirms a pushed `# Requirements: Foo` updates BOTH `Name` and `ShortTitle`.
- [ ] `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/services/ ./pkg/types/` must pass.

## Frontend — helper + display sites

- [ ] Create `frontend/src/components/tasks/taskTitle.ts` exporting `specTaskTitle(task)` that returns `user_short_title || short_title || name || 'Untitled task'`.
- [ ] Replace `task.name` with `specTaskTitle(task)` in: `TaskCard.tsx:838`, `TasksTable.tsx:131`, `CronTaskCard.tsx:99`, `EmptyTasksState.tsx:168`, `SpecTaskDetailContent.tsx:910`. Do not touch tooltip/title-history text — long `task.name`/`task.description` is the intended tooltip content.
- [ ] Collapse the existing inline `user_short_title || short_title || name` chains in `TabsView.tsx` (lines 381, 471, 808, 956, 1394, 1853, 1999, 2071) to use `specTaskTitle` (refactor, no behaviour change).
- [ ] `cd frontend && yarn build` must pass.

## Manual verification in inner Helix

- [ ] Create a new task with a long multi-sentence prompt; confirm within ~5 seconds the Kanban card shows a snappy ≤ 60-char title, not the truncated prompt.
- [ ] Disable the kodit enrichment model in system settings; confirm task creation still works and the card shows the old first-line title (fallback).
- [ ] Run planning end-to-end; confirm the H1 from `requirements.md` replaces the LLM-generated title.
- [ ] Double-click the tab to rename; confirm the user override sticks and isn't clobbered by either auto-generation path.
- [ ] Take a before/after screenshot of the Kanban board and attach it to the PR description.

## Wrap-up

- [ ] Run `./stack update_openapi` if any swagger annotation changed (none expected — `short_title` is already in the generated client).
- [ ] Open a PR using the conventional-commit format from `CLAUDE.md` (e.g. `feat(api): generate snappy LLM titles for spec tasks`).
- [ ] Watch the Drone CI build and fix any failures before requesting review.
