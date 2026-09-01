# Requirements: Perpetual Spec Tasks Survive PR Merges

## Background

A HelixOS bot's long-running session was marked `done` on 2026-09-01 while it was
actively working. The bot's desktop was up, its agent was streaming a response, and
it went on to process 111 queued messages after Helix had already declared it
finished.

Cause: `SpecTaskOrchestrator.handlePullRequest`
(`api/pkg/services/spec_task_orchestrator.go`, ~line 1172) moves a task to
`TaskStatusDone` with `MergedToMain = true` once every tracked PR is merged. Both
tracked PRs were genuinely merged, so the orchestrator did exactly what it was
written to do. **The logic is right for a one-shot feature task and wrong for a
perpetual bot run.** HelixOS models each bot as one persistent, long-lived spec task;
landing a branch is normal mid-session work for it, not the end of its life.
"All PRs merged" is not evidence that a perpetual run has finished.

Downstream, HelixOS derives bot liveness from this status and cannot defend itself:
`mapRemoteWorkflowStatus(done)` → `completed` → in `TerminalDispatchedStatuses()` →
`currentBotSpecTask` returns nil → the bot page shows no live run and "Run now"
dispatches a *second* session instead of reattaching. HelixOS already added a guard
for the sibling case (`if MergedToMain && !workInProgress(status)`), but that guard
cannot help here because this time the remote **status itself** is `done`. The fix
must be on the Helix side.

## User Stories

### US-1: Mark a spec task as a perpetual run
**As** an orchestrator (HelixOS) creating a long-lived bot session,
**I want** to declare at creation time that a spec task is a perpetual run,
**So that** Helix knows the task has no natural completion point.

Acceptance criteria:
- [ ] `SpecTask` has a new persisted boolean field, `perpetual_run`, defaulting to
      `false`.
- [ ] `POST /api/v1/spec-tasks` (`CreateTaskRequest`) accepts `perpetual_run` and
      persists it on the created task.
- [ ] `PUT /api/v1/spec-tasks/{id}` (`SpecTaskUpdateRequest`) accepts
      `perpetual_run` as a pointer so it can be explicitly set to `false`, letting a
      user convert a perpetual run into a normal task.
- [ ] The field is returned on every spec task read path and appears in the
      generated OpenAPI/TypeScript types.
- [ ] The marker is **not** derived from `type = 'bot_run'` or any other sniffing.
      `type` is a free-form string set by API callers (`bot_run`, `hypothesis`,
      `findai_jim`, `candidate_search`, `social_pulse` are all in use) and Helix's Go
      code must not start depending on HelixOS conventions.

### US-2: A perpetual run is not completed by a PR merge
**As** a bot whose session runs indefinitely,
**I want** merging a PR to be recorded but not to end my run,
**So that** my session stays live and my owner's UI keeps showing me as running.

Acceptance criteria:
- [ ] When all tracked PRs on a perpetual task are merged, the task does **not**
      transition to `done`, does **not** set `merged_to_main`, `merged_at`, or
      `completed_at`.
- [ ] The merge is still fully recorded: `repo_pull_requests` entries have their
      `PRState` updated to `merged`, CI status polling still runs, CI transition
      notifications still fire, and PR attention events are still dismissed.
- [ ] Polling is not short-circuited — the guard sits only on the terminal
      transition, not on the poll itself.
- [ ] After the merge is recorded, the task returns to a work-in-progress status so
      that (a) the PR poll loop stops re-firing on already-merged PRs, and (b)
      HelixOS's existing `workInProgress(status)` guard keeps evaluating true.
- [ ] The session behind the task remains live and answers a new message afterwards.

### US-3: Every auto-terminal path respects the marker
**As** a maintainer,
**I want** all automatic "you're done" transitions to honour the same marker,
**So that** the bug isn't just moved to a different code path.

Acceptance criteria — the following sites must not auto-complete a perpetual task:
- [ ] `handlePullRequest`, the `allMerged && len(task.RepoPullRequests) > 0` branch
      (~line 1172).
- [ ] `handlePullRequest`, the branch-merge fallback `!anyOpen && task.BranchName != ""`
      (~line 1235), which also sets `MergedToMain` + `Done`.
- [ ] `checkTaskForExternalPRActivity`, the merged-PR branch (~line 1596).
- [ ] `GitHTTPServer.handleMainBranchPush`, the merged-branch sweep in
      `api/pkg/services/git_http_server.go` (~line 1382).
- [ ] If any of these cannot sensibly apply the marker, the reason is stated in the
      PR description rather than the site being silently skipped.

### US-4: Normal tasks are unchanged
**As** a user of the core product,
**I want** one-shot feature tasks to complete exactly as they do today,
**So that** the fix costs nothing in the common case.

Acceptance criteria:
- [ ] A non-perpetual task whose PRs all merge still transitions to `done` with
      `merged_to_main = true`, `merged_at`, and `completed_at` set.
- [ ] The golden Docker cache build still triggers on merge.
- [ ] No behaviour change of any kind for tasks with `perpetual_run = false`, which
      is every existing task.

### US-5: A perpetual run can be ended deliberately
**As** a bot owner,
**I want** an explicit way to terminate a perpetual run,
**So that** "never auto-completes" does not mean "can never be stopped".

Acceptance criteria:
- [ ] Archiving still terminates a perpetual run: `PATCH /spec-tasks/{id}/archive`
      stops the desktop and the external agent, and the task disappears from the
      default board. This is HelixOS's existing "terminate a bot" mechanism
      (`archiveBotSpecTasks`) and must keep working unchanged.
- [ ] A user-initiated status change to `done` via the update handler is still
      allowed. Only *automatic* transitions are suppressed.
- [ ] Clearing `perpetual_run` via the update endpoint restores normal
      auto-completion behaviour.

### US-6: No bulk repair
**As** an operator,
**I want** no data migration that rewrites existing task statuses,
**So that** genuinely finished tasks are not reopened.

Acceptance criteria:
- [ ] No backfill migration is written. Of 132 `bot_run` tasks on meta, the 9 in
      `done` and 17 in `pull_request` have no session activity in the last 24h; the
      single affected task was already repaired by hand. There is nothing to
      backfill, and a migration that reopened finished tasks would be worse than the
      bug.

## Testing Requirements

Per `CLAUDE.md`, this must be tested end-to-end against the inner Helix at
`localhost:8080`. "It compiles" and "unit tests pass" are not acceptable evidence.

- [ ] **Live E2E**: create a spec task with `perpetual_run = true` and a live agent
      session (`config->>'zed_thread_id'` is a non-empty UUID); drive it to
      all-PRs-merged (either a real PR or the same state written through the store)
      and run the orchestrator poll. Assert the task does not go to `done`, does not
      set `merged_to_main`/`completed_at`, and that the session **still accepts and
      answers a new message afterwards**. The next operation working is the point.
- [ ] **Live E2E regression**: a normal task with all PRs merged still transitions to
      `done` exactly as before.
- [ ] **Live E2E**: archiving a perpetual run still terminates it (desktop stopped).
- [ ] **Go unit tests** around the orchestrator transitions covering both the
      perpetual and normal cases at each of the four sites in US-3.

## Constraints and Gotchas Discovered

- **Do not change the poll selectors carelessly.** `pollPullRequests`
  (`spec_task_orchestrator.go:1296`) only lists tasks in `pull_request` status, and
  `detectExternalPRActivity` (`:1422`) filters to `task.BranchName != "" &&
  !task.HasAnyPR()`. Those two filters are why the hand-repaired task stays stable
  after being moved back to `implementation` with `repo_pull_requests` populated.
  Widening either selector reintroduces the completion loop.
- **`KeepAlive` is the precedent to copy** for the new field
  (`api/pkg/types/simple_spec_task.go:412`): a plain `bool` with
  `gorm:"default:false"`, a `*bool` on `SpecTaskUpdateRequest`, handled in
  `spec_driven_task_handlers.go`. Columns are added by GORM `AutoMigrate`
  (`api/pkg/store/postgres.go:170`), not by a hand-written SQL migration.
- **`JustDoItMode` maps to column `yolo_mode`** — a reminder that JSON name and
  column name can diverge here. The new field should use a matching column name to
  avoid adding to that confusion.
- **Tasks enter `pull_request` status** from `spec_task_orchestrator.go:1548`,
  `spec_driven_task_service.go:1865`, and `spec_task_workflow_handlers.go:167`.

## Open Questions

1. **Status after a suppressed completion.** The design proposes moving a perpetual
   task back to `implementation` once its PRs are merged (this is the state the
   operator hand-repaired the live task to, and it stops the PR poll loop from
   re-firing). The alternative is leaving it in `pull_request`, which keeps polling
   already-merged PRs forever and burns GitHub API quota. Confirm `implementation`
   is the wanted resting state.
2. **`merged_to_main` on a perpetual run.** The spec suppresses it entirely, on the
   grounds that the flag means "this task's work landed and it is over". HelixOS's
   existing `if MergedToMain && !workInProgress(status)` guard would tolerate it
   being set, but suppressing it is the more honest model. Confirm nothing else in
   HelixOS reads `merged_to_main` as a plain "did a merge happen" signal.
3. **Second external PR on a perpetual run.** `detectExternalPRActivity` filters to
   `!task.HasAnyPR()`, so once a perpetual task has any tracked PR, a *later*
   externally-created PR will never be auto-detected. This is pre-existing behaviour
   and out of scope here, but it is a real limitation for long-lived runs. Should it
   be fixed in a follow-up?
4. **HelixOS is not checked out on this machine.** The task description says the
   `helixml/helixos` repo is available locally, but it is not — the local repos are
   `kodit`, `helix-next`, `docs`, `qwen-code`, `zed`, `helix`, and `helix-next` is a
   different codebase (no `api/internal/bridge`, no `mapRemoteWorkflowStatus`). The
   plan therefore delivers the Helix-side API change only, and the HelixOS PR that
   *sets* `perpetual_run` cannot be written here. If HelixOS can be made available,
   that wiring is a small follow-up PR on that repo.
5. **UI surfacing.** Should the board/task detail show a visible "perpetual" badge,
   or is the field API-only for now? The plan assumes API-only plus generated types,
   with no new UI controls.
