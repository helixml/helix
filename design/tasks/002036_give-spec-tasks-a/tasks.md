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

- [x] Create `frontend/src/components/tasks/taskTitle.ts` exporting `specTaskTitle(task, fallback?)` that returns `user_short_title || short_title || name || fallback`. Accepts null/undefined task safely.
- [x] Replace `task.name` with `specTaskTitle(task)` for SpecTask display: `TaskCard.tsx:838`, `SpecTaskDetailContent.tsx:910`. ~~`TasksTable.tsx` / `CronTaskCard.tsx` / `EmptyTasksState.tsx`~~ — discovered these operate on `TypesTriggerConfiguration` (recurring agent triggers/cron tasks), NOT `TypesSpecTask`. Helper does not apply; reverted those edits.
- [x] Collapse 8 inline `user_short_title || short_title || name` chains in `TabsView.tsx` to `specTaskTitle()` calls. Behaviour-preserving refactor — `grep "short_title ||"` returns 0 matches.
- [x] `yarn tsc` (typecheck) and `yarn build` (vite production build) both pass. Had to `sudo chown -R retro:retro frontend/dist` first because the bind mount was root-owned (per CLAUDE.md: never rm -rf dist).

## Manual verification in inner Helix

- [x] **End-to-end task creation** in the inner Helix at `localhost:8080`: registered `test@helix.ml`, created `testorg` / `testproj` (Claude Code / claude-opus-4-6), submitted a long multi-sentence dark-mode prompt. Task row landed in DB; card initially showed truncated prompt because the kodit enrichment model is NOT configured in this sandbox (fallback path).
- [x] **Verified the no-LLM fallback (Story 2):** `kodit_enrichment_provider` and `kodit_enrichment_model` are empty in `system_settings`. Task creation succeeded; card showed first-line `name` value (see `01-before-truncated-prompt.png`). Exactly the documented graceful-degradation behaviour.
- [x] **Verified the display chain renders `short_title` (Story 1):** injected `short_title = 'Add dark mode toggle to settings'` via SQL → reloaded UI → card switched to the snappy title (see `02-after-snappy-title.png`). Confirms the frontend `specTaskTitle` helper picks up `short_title` over `name`.
- [x] **Verified user override beats auto-gen (Story 4):** set both `short_title` and `user_short_title` via SQL → UI rendered `user_short_title` ("Dark mode (my pick)") even with `short_title` populated (see `03-user-override.png`). Override correctly takes precedence.
- [~] **Planning H1 → ShortTitle (Story 3):** could not run end-to-end without an LLM provider configured. Behaviour is verified at the unit-test layer (`git_helpers_test.go` covers `SpecTitleFromRequirements`) and the new line in `git_http_server.go` is a straight assignment alongside the existing `Name` update.
- [~] **LLM happy path with provider configured:** not exercisable in this sandbox (no `provider_endpoints` row, no enrichment-model config). The runtime path is covered by the build (compiles, wires through `SetTitleGenerator`) and `TestCleanGeneratedTitle`. **Operator action required after deploy:** set `kodit_enrichment_provider`/`kodit_enrichment_model` in `system_settings` to activate.
- [x] Screenshots saved under `screenshots/`: 01-before, 02-after, 03-user-override.

## Wrap-up

- [x] No swagger changes needed — only used existing `short_title` field on `SpecTask`. Generated client already exposes it.
- [x] PR description written at `pull_request_helix.md`. Title uses the conventional `feat:` prefix.
- [ ] Watch the Drone CI build after pushing (depends on platform pipeline).
