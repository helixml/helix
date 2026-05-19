# Implementation Tasks: Generate Snappy LLM Titles for Spec Tasks

## Backend — store layer

- [x] Add `UpdateSpecTaskShortTitle(ctx, id, title string) (bool, error)` to the `Store` interface and the postgres implementation in `api/pkg/store/store_spec_tasks.go`. Returns `bool` so callers can tell whether the conditional UPDATE actually fired. Conditional `WHERE id = ? AND (short_title IS NULL OR short_title = '')` keeps the LLM draft from clobbering the agent's H1.
- [x] Hand-extend the gomock store mock (no regen needed — single method added matches the existing mock pattern).

## Backend — title generation

- [x] Add `GenerateSpecTaskTitleAsync(ctx, taskID, ownerID, prompt string)` to `api/pkg/server/summary_service.go` — reuses the rate limiter, 30s timeout, MaxTokens 50, Temperature 0.3, and a new `cleanGeneratedTitle` helper that strips quotes, "Title:"/"Task:" prefixes, trailing punctuation, and truncates at a word boundary.
- [x] Imperative-verb snappy-title system prompt in place. Cap result at 60 chars at word boundary.
- [x] No-config bail-out: when `KoditEnrichmentProvider`/`KoditEnrichmentModel` are empty, returns `("", nil)` and no DB write happens. LLM/network errors logged at `Warn`.

## Backend — wire into task creation

- [x] Defined `TitleGenerator` interface in `services` package + setter `SpecDrivenTaskService.SetTitleGenerator`. Chose interface over passing `*SummaryService` to avoid `services → server` import cycle. Nilable, so existing tests are unaffected.
- [x] `server.go` calls `apiServer.specDrivenTaskService.SetTitleGenerator(apiServer.summaryService)` right after constructing both. `*SummaryService` satisfies the interface via duck typing.
- [x] In `SpecDrivenTaskService.CreateTaskFromPrompt`, after `s.store.CreateSpecTask(...)`, fires `s.titleGenerator.GenerateSpecTaskTitleAsync(...)` guarded by `s.titleGenerator != nil && !s.testMode`.

## Backend — agent H1 path

- [x] `git_http_server.go` now updates BOTH `task.Name` and `task.ShortTitle` from the parsed H1 in `requirements.md`, with the 100-char schema cap applied to `short_title`. Single `UpdateSpecTask` call.

## Backend — tests

- [x] Added `api/pkg/server/summary_service_title_test.go` exercising `cleanGeneratedTitle` (11 cases: quote stripping, "Title:"/"Task:" prefix stripping, trailing-punctuation trim, word-boundary truncation, hard truncation with ellipsis, empty/all-quotes edge cases).
- [x] Added `api/pkg/services/git_helpers_test.go` covering `SpecTitleFromRequirements` (6 cases including the "# Requirements: …" prefix path, bare `# Requirements` fallback, empty content). Skipped a full `processSpecsBranchPush` test — would need a live git harness for marginal value over the parser test plus the build-level wiring check.
- [x] `CGO_ENABLED=0 go build ./...` and the two `go test -run ...` invocations pass locally. CGo not available in this sandbox so we kept the new tests CGo-free.

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
