# Requirements: Perpetual Spec Tasks Survive PR Merges

## Background

A HelixOS bot's long-running session was marked `done` on 2026-09-01 while it was
actively working. The bot's desktop was up, its agent was streaming a response, and
it went on to process 111 queued messages after Helix had already declared it
finished.

HelixOS models each bot as one persistent, long-lived spec task. Landing a branch is
normal mid-session work for such a run, not the end of its life. **"All PRs merged"
is not evidence that a perpetual run has finished.** The lifecycle logic is right for
a one-shot feature task and wrong for a perpetual one.

### The full cycle (corrected)

The background PR poller is only the second half. The first half is that the agent's
own normal work drives the task *into* `pull_request` status, and only then does the
poller finish it off. Observed on `spt_01m130cevad74edgzt40t2mmd7` after a manual
reopen to `implementation` — it survived **four minutes**:

```
02:45:xx  reopened by hand to `implementation`
02:48:16  HelixOS polled, recorded the run `implementing` — bot showed live
02:49:14  implementation_approved_by / implementation_approved_at written —
          the approve-implementation handler ran and set Status = pull_request
          (api/pkg/server/spec_task_workflow_handlers.go:167)
02:49:19  pollCIStatusForPR refreshed helixos#162's ci_updated_at
02:49:20  spec_task_orchestrator.go:1184 logged "PR merged! Moving task to done"
          — both tracked PRs were already merged, so the allMerged branch fired
02:54:18  HelixOS polled, run back to `completed`, bot showed dead again
```

So the cycle is: agent opens a PR (normal work) → `implementation` → `pull_request` →
PRs already merged or merge shortly after → `pull_request` → `done` +
`merged_to_main` → HelixOS maps `done` → `completed` (terminal) →
`currentBotSpecTask` returns nil, the bot reads as not running, "Run now" dispatches a
*second* session, and any active whip epoch is ended.

**This recurs every single time the bot opens a PR.** A manual reopen buys minutes,
not hours. A "reopen it" tool is not the remedy and must not be built.

HelixOS cannot defend itself: it already added a guard for the sibling case
(`MergedToMain && !workInProgress(status)`), but that guard has nothing to disagree
with when the remote **status itself** is `done`. The fix must be on the Helix side.

### Second, independent reason this matters

HelixOS's whipper (`api/internal/whipper/`, merged in helixos PR #161) is the
keep-alive meant to stop exactly this class of run dying. Its epoch is ended when
"the run reached a terminal status" (see the `WhipStopped` doc comment in
`api/internal/types/whip.go`). So the same false `done` also silently kills
keep-alive. This connection must be stated in the PR description so it is not lost.

## User Stories

### US-1: Mark a spec task as a perpetual run
**As** an orchestrator (HelixOS) creating a long-lived bot session,
**I want** to declare at creation time that a spec task is a perpetual run,
**So that** Helix knows the task has no natural completion point.

Acceptance criteria:
- [ ] `SpecTask` has a new persisted boolean, `perpetual_run`, defaulting to `false`.
- [ ] `POST /api/v1/spec-tasks` (`CreateTaskRequest`) accepts and persists it.
- [ ] `PUT /api/v1/spec-tasks/{id}` (`SpecTaskUpdateRequest`) accepts it as a pointer
      so it can be explicitly set to `false`, demoting a perpetual run to a normal task.
- [ ] The field is returned on every spec task read path and appears in the generated
      OpenAPI/TypeScript types.
- [ ] The marker is **not** derived from `type = 'bot_run'` or any other sniffing.
      `type` is a free-form string owned by API callers (`bot_run`, `hypothesis`,
      `findai_jim`, `candidate_search`, `social_pulse` are all in use); no Helix Go
      code reads its values today and none should start.

### US-2: Opening a PR does not move a perpetual run out of `implementation`
**As** a bot doing ordinary work,
**I want** opening a PR to be recorded without advancing my task's lifecycle,
**So that** the poller never gets the chance to finish me off.

This is the half the original brief missed, and it is arguably the more important of
the two. Guarding only the completion path leaves the task parked in `pull_request`,
which is still wrong: the bot is implementing, not awaiting a merge.

Acceptance criteria — a perpetual task must stay in `implementation` when:
- [ ] `approveImplementation` takes the external-repo branch
      (`spec_task_workflow_handlers.go:167`) — the observed trigger. The PR is still
      created, `ImplementationApprovedBy`/`At` are still recorded, the push
      instruction is still sent to the agent.
- [ ] `checkTaskForExternalPRActivity` detects an externally-opened PR
      (`spec_task_orchestrator.go:1548`) — the `RepoPR` is still appended and the
      `pr_ready` attention event still fires.
- [ ] A PR is created via push detection (`spec_driven_task_service.go:1865`) — the
      `RepoPR` is still appended.
- [ ] `approveImplementation` takes either internal-repo branch that parks a task in
      `implementation_review` (`spec_task_workflow_handlers.go:214` nothing-pushed-yet,
      `:281` rebase-required). `implementation_review` is the exact status the
      `handleMainBranchPush` sweep selects on, so parking there is a live route to a
      false `done`. The rebase instruction to the agent is still sent.

### US-3: A perpetual run is never auto-completed
**As** a bot whose session runs indefinitely,
**I want** merges to be recorded but never to end my run,
**So that** my session stays live and my owner's UI keeps showing me as running.

Acceptance criteria — none of these may set `Status = done`, `merged_to_main`,
`merged_at`, or `completed_at` on a perpetual task:
- [ ] `handlePullRequest`, all-PRs-merged branch (`spec_task_orchestrator.go` ~1172).
- [ ] `handlePullRequest`, branch-merge fallback `!anyOpen && BranchName != ""` (~1235).
- [ ] `checkTaskForExternalPRActivity`, merged-PR branch (~1596).
- [ ] `GitHTTPServer.handleMainBranchPush` merged-branch sweep
      (`git_http_server.go` ~1382).
- [ ] **`GitHTTPServer.tryAutoMergeAfterRebase`** (`git_http_server.go` ~1235) — not
      named in the brief. Fires automatically when the agent's rebase push lands and
      a fast-forward succeeds. Same terminal write.
- [ ] **`approveImplementation` internal-repo server-side merge success**
      (`spec_task_workflow_handlers.go` ~360) — not named in the brief. For a
      perpetual run, "Accept" means "land this branch", not "kill my bot".
- [ ] The merge is still fully recorded everywhere: `repo_pull_requests` entries reach
      `PRState = merged`, `MergeCommitHash` is still captured where it is captured
      today, CI status polling and its transition notifications still run, attention
      events are still dismissed, and the golden Docker cache build still triggers.
- [ ] Polling is not short-circuited — the guard sits on the transition, not the poll.

### US-4: PR state stays fresh for a perpetual run
**As** a bot owner,
**I want** my bot's PRs to keep showing accurate state and CI status,
**So that** "never leaves `implementation`" doesn't mean "PR data goes stale".

Acceptance criteria:
- [ ] The PR poller reaches perpetual tasks that hold PRs while in a work-in-progress
      status, so `PRState` and CI status keep refreshing. Both
      `pollPullRequests` (`spec_task_orchestrator.go:1297`) and the
      `RefreshPullRequestStatus` gate (`:1338`) need widening.
- [ ] This widening lands **only after** every terminal guard in US-3 is in place.
      Widening the selector first re-creates the exact loop this fix exists to break.
- [ ] `detectExternalPRActivity`'s `!task.HasAnyPR()` filter is left alone (see Open
      Question 3).

### US-5: Normal tasks are unchanged
**As** a user of the core product,
**I want** one-shot feature tasks to behave exactly as they do today,
**So that** the fix costs nothing in the common case.

Acceptance criteria:
- [ ] A non-perpetual task still advances `implementation` → `pull_request` on
      approval, and still reaches `done` with `merged_to_main`, `merged_at`, and
      `completed_at` when its PRs merge.
- [ ] The golden Docker cache build still triggers on merge.
- [ ] No behaviour change of any kind for `perpetual_run = false`, which is every
      existing task.

### US-6: A perpetual run can be ended deliberately
**As** a bot owner,
**I want** an explicit way to terminate a perpetual run,
**So that** "never auto-completes" does not mean "can never be stopped".

Acceptance criteria:
- [ ] Archiving still terminates it: `PATCH /spec-tasks/{id}/archive` stops the
      desktop and the external agent. This is HelixOS's existing `archiveBotSpecTasks`
      mechanism and must keep working unchanged.
- [ ] A user-initiated `PUT status: done` is still honoured. Only automatic and
      workflow-implied transitions are suppressed.
- [ ] Clearing `perpetual_run` restores normal lifecycle behaviour.

### US-7: No bulk repair
- [ ] No backfill migration is written. Of 132 `bot_run` tasks on meta, the 9 in
      `done` and 17 in `pull_request` have no session activity in the last 24h, and
      the single affected task was already repaired by hand. There is nothing to
      backfill, and a migration that reopened genuinely finished tasks would be worse
      than the bug.

## Testing Requirements

Per `CLAUDE.md`, tested end-to-end against the inner Helix at `localhost:8080`.
"It compiles" and "unit tests pass" are not acceptable evidence.

- [ ] **Live E2E, the four-minute reproduction**: create a perpetual task with a live
      agent session (`config->>'zed_thread_id'` a non-empty UUID); call
      approve-implementation so a PR opens; merge it; run the orchestrator poll.
      Assert the task stayed in `implementation` throughout, never set
      `merged_to_main`/`completed_at`, and that the PR is tracked with
      `PRState = merged`. Then **send the session a new message and confirm it
      answers.** The next operation working is the point.
- [ ] **Live E2E, repeat**: open and merge a *second* PR on the same task and confirm
      it survives again. The bug recurs per-PR, so surviving once is not enough.
- [ ] **Live E2E regression**: a normal task, same flow, still reaches `done` with
      `merged_to_main = true` and `completed_at` set.
- [ ] **Live E2E**: archiving a perpetual run still terminates it.
- [ ] **Go unit tests** covering perpetual and normal cases at all four status-advancing
      sites (US-2) and all six terminal sites (US-3).

## Constraints and Gotchas Discovered

- **Ten distinct code paths move a spec task through its lifecycle** — four into
  `pull_request`/`implementation_review`, six into `done`. Two of the terminal ones
  (`tryAutoMergeAfterRebase`, the internal-repo merge in `approveImplementation`) were
  not in the original brief. If you add an eleventh, it needs the same guard.
- **Do not widen the poll selectors before the terminal guards land.**
  `pollPullRequests` (`:1297`) selects only `status = pull_request`;
  `detectExternalPRActivity` (`:1422`) filters `BranchName != "" && !HasAnyPR()`.
  Those two filters are the only reason a hand-reopened task is stable at all.
- **`KeepAlive` is the precedent to copy** (`api/pkg/types/simple_spec_task.go:412`):
  plain `bool` on the model, `bool` on the create request, `*bool` on the update
  request. Columns are added by GORM `AutoMigrate` (`api/pkg/store/postgres.go:170`),
  not by hand-written SQL in `api/pkg/store/migrations/`.
- **`JustDoItMode` persists to the column `yolo_mode`** — JSON name and column name
  diverge here, so pin the column name explicitly in the new field's tag.

## Open Questions

1. **Is `implementation` the right permanent resting state?** The design keeps a
   perpetual run in `implementation` forever — it never enters `pull_request` or
   `implementation_review`. That is the simplest rule and matches "the bot is
   implementing, not awaiting a merge". The cost is that the board shows no visual
   difference between a bot mid-PR and a bot mid-edit. Acceptable, or is a
   perpetual-specific display state wanted?
2. **`merged_to_main` on a perpetual run.** Suppressed entirely here, on the grounds
   that it means "this task's work landed and it is over". HelixOS's
   `MergedToMain && !workInProgress(status)` guard would tolerate it being set, but
   suppressing is more honest. Confirm nothing in HelixOS reads it as a plain "did a
   merge happen" signal.
3. **Second external PR on a perpetual run.** `detectExternalPRActivity` filters
   `!task.HasAnyPR()`, so once a perpetual task holds any PR, a later
   externally-created PR is never auto-detected. Pre-existing and deliberately left
   alone here (touching it is how you reintroduce the loop). Follow-up?
4. **HelixOS is not checked out on this machine.** The brief says it is, but the local
   repos are `kodit`, `helix-next`, `docs`, `qwen-code`, `zed`, `helix` — and
   `helix-next` is a different codebase (no `api/internal/bridge`, no
   `mapRemoteWorkflowStatus`, no `internal/whipper`). So the HelixOS PR that *sets*
   `perpetual_run` cannot be written here, and the whipper interaction can only be
   described, not verified. Until that PR lands the Helix-side fix is inert in
   production. Can the repo be made available?
5. **UI surfacing.** Assumed API-only plus generated types, no new UI controls.
   Should the board show a "perpetual" badge?
