# Design: Perpetual Spec Tasks Survive PR Merges

## Approach

Add one first-class boolean to `SpecTask` — `PerpetualRun` — set by the API caller at
creation time, and guard every *automatic* transition to a terminal status with it.
Nothing else changes: PR state, CI polling, notifications, and golden builds all
still run. Only the flip to `done` is suppressed.

This is deliberately the smallest honest model. HelixOS already knows a bot run is
perpetual when it dispatches it; it just has no way to say so. An explicit flag says
it. The rejected alternatives:

- **Sniffing `type = 'bot_run'`** — explicitly out of bounds. `type` is a free-form
  string owned by the API caller (`bot_run`, `hypothesis`, `findai_jim`,
  `candidate_search`, `social_pulse`…). No Helix Go code reads its values today and
  starting now would bake a HelixOS convention into the Helix product.
- **Inferring liveness from session activity** — fragile and racy. A bot idle for an
  hour is still perpetual; a finished task with a warm desktop is still finished.
  Liveness is the wrong signal for "does this task have an end".
- **Fixing it in HelixOS** — impossible. The remote status *is* `done`; HelixOS's
  existing `MergedToMain && !workInProgress(status)` guard has nothing left to
  disagree with.

## Data Model

`api/pkg/types/simple_spec_task.go`, on `SpecTask`, next to `KeepAlive`:

```go
// PerpetualRun marks a task with no natural completion point — a long-lived
// agent session (e.g. a HelixOS bot) that lands branches as ordinary mid-session
// work. Automatic terminal transitions (all-PRs-merged, branch-merged-to-main)
// are suppressed for these; only archiving or an explicit user status change
// ends them.
PerpetualRun bool `json:"perpetual_run" gorm:"column:perpetual_run;default:false"`
```

The column is created by GORM `AutoMigrate` (`api/pkg/store/postgres.go:170`), the
same way `KeepAlive` and every other spec-task boolean was added. **No SQL migration
file, and specifically no backfill** — see US-6.

Request plumbing, mirroring `KeepAlive` exactly:

- `CreateTaskRequest.PerpetualRun bool` — `json:"perpetual_run"`.
- `SpecTaskUpdateRequest.PerpetualRun *bool` — pointer so `false` can be sent
  explicitly, letting a user demote a perpetual run to a normal task.

A single grep-able predicate keeps the guard consistent and gives one place to change
the rule later:

```go
// SuppressesAutoCompletion reports whether automatic terminal transitions must
// be skipped for this task.
func (t *SpecTask) SuppressesAutoCompletion() bool { return t.PerpetualRun }
```

## The Four Guarded Sites

All four currently write the same shape: `Status = Done; MergedToMain = true;
MergedAt = &now; CompletedAt = &now`. Each keeps everything except that shape.

| # | Site | Current terminal write | Perpetual behaviour |
|---|------|------------------------|---------------------|
| 1 | `spec_task_orchestrator.go` ~1172 — `allMerged && len(RepoPullRequests) > 0` | Done + MergedToMain | Persist updated PR states, trigger golden build, dismiss attention events, set `Status = implementation`. No merge/completion timestamps. |
| 2 | `spec_task_orchestrator.go` ~1235 — `!anyOpen && BranchName != ""` branch-merge fallback | Done + MergedToMain | Log the detected merge, dismiss attention events, set `Status = implementation`. No merge/completion timestamps. |
| 3 | `spec_task_orchestrator.go` ~1596 — `checkTaskForExternalPRActivity` merged-PR branch | Appends `RepoPR`, then Done + MergedToMain | Still append the `RepoPR` (the merge is recorded), dismiss attention events, leave `Status` where it is — this path only fires for tasks already in `spec_review`/`implementation`. |
| 4 | `git_http_server.go` ~1382 — `handleMainBranchPush` merged-branch sweep | Done + MergedToMain + MergeCommitHash | Set `Status = implementation` (this path only selects tasks in `implementation_review`), record nothing terminal. |

Site 4 note: it selects `task.Status == types.TaskStatusImplementationReview`. A
perpetual run reaching `implementation_review` and having its branch land is the
"human approved my branch, it merged" case — for a bot that is mid-session work, so
dropping it back to `implementation` is the right resting state, same as sites 1 and 2.

### Why `implementation` and not "stay put"

Leaving a perpetual task in `pull_request` after its PRs merge would leave
`pollPullRequests` (which selects exactly `status = pull_request`) hitting GitHub for
already-merged PRs on every cycle, forever, once per perpetual bot. Moving to
`implementation` stops that, and it is the state the operator hand-repaired the live
task to — verified stable because:

- `pollPullRequests` only lists `pull_request` status → the task is no longer polled.
- `detectExternalPRActivity` filters `BranchName != "" && !task.HasAnyPR()` → the
  task is excluded because `repo_pull_requests` is populated.

**Do not widen either selector.** Doing so reintroduces the completion loop this fix
exists to break. If a future change needs perpetual tasks re-polled, it needs a
different mechanism, not a looser filter.

`implementation` is also a work-in-progress status on the HelixOS side, so the
existing `MergedToMain && !workInProgress(status)` guard in `bridge.go:766-786` stays
satisfied even if `merged_to_main` were ever set by another path.

### What is explicitly NOT suppressed

- CI status polling (`pollCIStatusForPR`) and its transition notifications.
- `RepoPullRequests[i].PRState` updates and persistence.
- `DismissTaskAttentionEvents` — the PR-ready nudge is stale once the PR merged.
- The golden Docker cache build. It is keyed on a merge to main, not on task
  completion, so a perpetual run's merge should still warm the cache.

## Termination Routes for a Perpetual Run

A perpetual run keeps exactly two ways to end, both already implemented and
unchanged by this work:

1. **Archive** — `PATCH /api/v1/spec-tasks/{taskId}/archive`
   (`spec_driven_task_handlers.go:1401`). It stops the planning session's desktop and
   any running `SpecTaskExternalAgent`, then sets `archived = true`. This is the
   mechanism HelixOS's `archiveBotSpecTasks` already uses to terminate a bot. The
   guard added here touches only the orchestrator and the git HTTP server, so this
   path is untouched — but it must be re-verified live (US-5).
2. **Explicit user status change** — `PUT /api/v1/spec-tasks/{id}` with
   `status: done`. Suppression applies to automatic transitions only; a human saying
   "this is finished" is authoritative.

Clearing `perpetual_run` via the update endpoint restores normal auto-completion for
the next poll cycle.

## Testing Strategy

### Go unit tests (`api/pkg/services/`)

Table-driven, one pair (perpetual / normal) per guarded site, against the existing
orchestrator test harness with a mocked store and git service:

- All PRs merged → normal goes `done` + `merged_to_main` + `completed_at`; perpetual
  goes `implementation` with all three unset and `RepoPullRequests[*].PRState ==
  "merged"`.
- Branch-merge fallback, external-PR-detection, and main-branch-push sweep: same
  pairing.

### Live E2E on `localhost:8080` (the part that makes this done)

Per `CLAUDE.md`, use `http://localhost:8080` — never `api:8080`, which is the outer
stack running this agent.

1. Create a project + task via the API with `perpetual_run: true` and
   `just_do_it_mode: true`; start it so a real session exists with a non-empty
   `config->>'zed_thread_id'`.
2. Put it in the all-PRs-merged state — either by opening and merging a real PR, or
   by writing `repo_pull_requests` with `PRState: merged` directly through the store
   — then trigger the orchestrator poll.
3. Assert via SQL/API: `status != 'done'`, `merged_to_main = false`,
   `completed_at IS NULL`, and the PR row shows `merged`. Then **send the session a
   new message and confirm it answers.** The next operation working is the evidence;
   a status assertion on its own is not.
4. Repeat step 2 on a task with `perpetual_run: false` and confirm it reaches `done`
   with `merged_to_main = true` and `completed_at` set — the regression that would
   hurt most.
5. Archive the perpetual task and confirm the desktop container stops and the task
   leaves the default board.

Record the commands and outputs in `design/2026-09-01-perpetual-run-e2e.md` in the
helix repo, per the CLAUDE.md debugging-notes rule.

## HelixOS Wiring

Setting `perpetual_run: true` from HelixOS's dispatcher (`api/internal/bridge/`) is a
separate PR on `helixml/helixos`, not part of the Helix PR. **The helixos repo is not
checked out on this machine** (local repos are `kodit`, `helix-next`, `docs`,
`qwen-code`, `zed`, `helix`; `helix-next` is a different codebase with no
`api/internal/bridge`). The Helix PR description must therefore state plainly that
the HelixOS side was **not** wired, and that until it is, the new field is inert for
bot runs — the Helix-side fix is a no-op in production until HelixOS starts sending
the flag.

## Notes for Future Agents

- Spec-task columns in this repo are added by GORM `AutoMigrate`, not by files in
  `api/pkg/store/migrations/`. Those SQL migrations are reserved for renames, drops,
  and data fixes.
- `JustDoItMode` persists to the column `yolo_mode`. JSON name ≠ column name is
  normal here; pin the column name explicitly in the tag when adding a field.
- The pattern for "optional boolean the user can turn off" is `bool` on the model,
  `bool` on the create request, `*bool` on the update request. See `KeepAlive` and
  `PublicDesignDocs`.
- Four separate code paths can auto-complete a task. If you add a fifth, it needs the
  same guard.
