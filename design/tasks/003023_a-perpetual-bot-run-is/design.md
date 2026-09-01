# Design: Perpetual Spec Tasks Survive PR Merges

## Approach

Add one first-class boolean to `SpecTask` — `PerpetualRun` — set by the API caller at
creation time, and apply a single rule everywhere the task lifecycle advances:

> **A perpetual run stays in `implementation`. Nothing else about a merge changes.**

PRs are still created, tracked, CI-polled, notified on, and displayed. Merges still
warm the golden cache and still dismiss attention events. Only the *status
transitions* are suppressed — both the advance out of `implementation` and the flip
to `done`.

The addendum evidence is why the rule has to cover both. Guarding only the completion
path leaves the task parked in `pull_request`, which is still wrong (the bot is
implementing, not awaiting a merge) and still exposed: `pull_request` is one poll
cycle away from `done`, and `implementation_review` is directly selected by the
main-branch-push sweep. The observed reopen survived four minutes precisely because
the *advancing* path fired first.

Rejected alternatives:

- **Sniffing `type = 'bot_run'`** — out of bounds. `type` is a free-form string owned
  by API callers. No Helix Go code reads its values and starting now would bake a
  HelixOS convention into the Helix product.
- **Inferring from session liveness** — fragile and racy. A bot idle for an hour is
  still perpetual; a finished task with a warm desktop is still finished.
- **Fixing it in HelixOS** — impossible. The remote status *is* `done`; the existing
  `MergedToMain && !workInProgress(status)` guard has nothing left to disagree with.
- **A "reopen the task" tool** — explicitly ruled out. The bug recurs on every PR the
  bot opens; a reopen buys minutes.

## Decision: a new field, not `keep_alive`

**Decision: add `perpetual_run`. Do not overload `keep_alive`.** This was a close
call and the reasoning belongs in the PR description.

The operator had `keep_alive = true` on the affected task and expected it to mean
"this run does not end" ("I toggled keep-alive on. It should last forever, right?").
Helix read it as "keep the box". The box did stay up — `Up 7 hours`, a turn completed
at 03:03:15, 111 queued approvals delivered through it. Only the status disagreed,
and HelixOS renders liveness from the status.

What `keep_alive` gates today is only ever the container, never the workflow:

- `spec_task_orchestrator.go:1693` (`handleDone`) — "Task in done status but
  keep_alive is set - leaving desktop running".
- `external-agent/gc_reaper.go:152` — don't GC the workspace. Its doc comment
  reasons explicitly and at length about workspace retention, and deliberately
  refuses to time-bound it.
- `spec_driven_task_handlers.go:1291-1307` — turning it *off* on an already-`Done`
  task stops the desktop then.

So `done + keep_alive = true` is a state the current design deliberately supports:
the task finishes, the box stays up for someone to poke at. Coherent for a feature
task. Incoherent for a bot.

### Why not reuse it

The case for reuse is real: it is already the operator's "don't stop this"
declaration, already coupled to done-handling, needs no new column, no new API
surface, and — the strongest point — **no HelixOS change to set it**, so it would fix
production immediately, whereas a new field is inert until HelixOS is wired.

It loses on three grounds:

1. **The regression lands on the core path.** Someone sets `keep_alive` on an
   ordinary feature task purely to keep the desktop for debugging after it lands.
   Under reuse that task never completes — it sits in `implementation` forever,
   never shows done, never clears from the board. The original brief is explicit
   that not regressing normal tasks is "the behaviour that matters most in the
   product". Compare the failure modes: with a separate field the worst case is the
   fix stays inert until someone sets the flag — a known, visible, fixable gap. With
   reuse the worst case is silent, hits a different user, and hits the common case.
2. **It is a casual, transient toggle.** `showKeepAlive={isDesktopRunning}`
   (`SpecTaskDetailContent.tsx:2668`) — the button only appears while the desktop is
   up, one click in the session toolbar next to Restart. Attaching permanent
   workflow semantics to a button people press to stop a box sleeping is exactly the
   "toggle silently changed what it means" failure the addendum warns about.
3. **It is one-way.** Conflating them is easy; separating them later breaks whoever
   came to depend on the conflated meaning.

### But the operator's reading was reasonable, so fix what caused it

Deciding against reuse does not mean the misreading was the operator's fault. Two
consequences, both in scope:

- **`perpetual_run` gets a UI toggle too**, not just an API field. That gives the
  operator a working lever today instead of waiting on the HelixOS PR, which is the
  reuse camp's best argument and the main thing a separate field otherwise gives up.
- **Narrow the `keep_alive` labels.** "Keep Alive ON — won't auto-sleep"
  (`SpecTaskViewToolbar.tsx:314-317`) and "Keep Alive enabled — container won't
  auto-sleep" (`SpecTaskDetailContent.tsx:1036`) never say *what* won't auto-sleep,
  which is how it got read as "the run". They should name the desktop and say the
  task still completes normally. `keep_alive`'s behaviour does not change — only its
  labelling, which was under-specified before this bug and is what made the two
  concepts look like one.

Related: a perpetual run's workspace must not be reaped either, so
`gc_reaper.go:152` treats `perpetual_run` as live alongside `keep_alive`. Note that
`handleDone`'s keep-alive branch is unreachable for a perpetual run — it never
reaches `done` — so no change is needed there.

## Data Model

`api/pkg/types/simple_spec_task.go`, on `SpecTask`, next to `KeepAlive`:

```go
// PerpetualRun marks a task with no natural completion point — a long-lived
// agent session (e.g. a HelixOS bot) that opens and lands branches as ordinary
// mid-session work. Such a task stays in `implementation` for its whole life:
// PRs are tracked and merges recorded, but no automatic transition may advance
// it to pull_request/implementation_review or complete it. Only archiving or an
// explicit user status change ends it.
//
// Distinct from KeepAlive, which is purely about the container: KeepAlive says
// "don't stop the desktop when this task finishes", and `done + keep_alive` is
// a supported state. PerpetualRun says "this task does not finish".
PerpetualRun bool `json:"perpetual_run" gorm:"column:perpetual_run;default:false"`
```

Column created by GORM `AutoMigrate` (`api/pkg/store/postgres.go:170`), same as
`KeepAlive`. **No SQL migration file and no backfill** (US-8).

Request plumbing mirrors `KeepAlive` exactly:
- `CreateTaskRequest.PerpetualRun bool`
- `SpecTaskUpdateRequest.PerpetualRun *bool` (pointer so `false` can be sent)

Two grep-able predicates keep the rule in one place and name the two halves:

```go
// HoldsLifecycleInImplementation reports whether workflow transitions that would
// advance this task out of implementation must be skipped.
func (t *SpecTask) HoldsLifecycleInImplementation() bool { return t.PerpetualRun }

// SuppressesAutoCompletion reports whether transitions to a terminal status must
// be skipped for this task.
func (t *SpecTask) SuppressesAutoCompletion() bool { return t.PerpetualRun }
```

They return the same thing today. They are separate because the two halves are
conceptually distinct and a future rule may want to split them; a reader at a call
site should see which half they are in.

## Site Census

Ten paths move a task through its lifecycle. Every one is guarded. Two of the
terminal ones were **not** in the original brief and were found by auditing
`grep -n "TaskStatusDone" api/pkg/services/git_http_server.go
api/pkg/server/spec_task_workflow_handlers.go`.

### Group A — advancing out of `implementation`

| # | Site | Today | Perpetual behaviour |
|---|------|-------|---------------------|
| A1 | `spec_task_workflow_handlers.go:167` — `approveImplementation`, external repo | records approval, `Status = pull_request` | Record `ImplementationApprovedBy`/`At`, create the PRs, send the push instruction, sync PR descriptions. **Leave `Status = implementation`.** *The observed trigger.* |
| A2 | `spec_task_orchestrator.go:1548` — `checkTaskForExternalPRActivity`, externally-opened PR | appends `RepoPR`, `Status = pull_request` | Append the `RepoPR`, emit the `pr_ready` attention event. Leave status. |
| A3 | `spec_driven_task_service.go:1865` — PR created via push detection | appends `RepoPR`, `Status = pull_request` | Append the `RepoPR`. Leave status. |
| A4 | `spec_task_workflow_handlers.go:214` (nothing pushed yet) and `:281` (rebase required) — internal repo | `Status = implementation_review` + `RebaseRequestedAt` | Still stamp `RebaseRequestedAt` and `ImplementationApprovedBy`, still send the push/rebase instruction. Leave status. |

A4 matters more than it looks: `implementation_review` is the exact status the
`handleMainBranchPush` sweep (B4) selects on, so parking there is a live route to a
false `done` even with the PR path guarded.

### Group B — transitions to `done`

All six write the same shape today: `Status = Done; MergedToMain = true;
MergedAt = &now; CompletedAt = &now`. Each keeps everything except that shape.

| # | Site | Perpetual behaviour |
|---|------|---------------------|
| B1 | `spec_task_orchestrator.go` ~1172 — `allMerged && len(RepoPullRequests) > 0` | Persist updated PR states, trigger the golden build, dismiss attention events. No status/merge/completion writes. |
| B2 | `spec_task_orchestrator.go` ~1235 — branch-merge fallback `!anyOpen && BranchName != ""` | Log the detected merge, dismiss attention events. No terminal writes. |
| B3 | `spec_task_orchestrator.go` ~1596 — `checkTaskForExternalPRActivity` merged PR | Still append the `RepoPR` (the merge is recorded), dismiss attention events. No terminal writes. |
| B4 | `git_http_server.go` ~1382 — `handleMainBranchPush` sweep | Skip. Selects `status == implementation_review`, which A4 now prevents a perpetual task from reaching — guarded anyway, belt and braces, since a task could have been marked perpetual while already parked there. |
| B5 | `git_http_server.go` ~1235 — **`tryAutoMergeAfterRebase`** *(not in the brief)* | Fires automatically when the agent's rebase push lands and FF succeeds. Perform the merge and the upstream push as normal; skip the terminal writes and `DismissTaskAttentionEvents` stays. |
| B6 | `spec_task_workflow_handlers.go` ~360 — **`approveImplementation` internal-repo server-side merge success** *(not in the brief)* | Perform the merge and upstream push, record `ImplementationApprovedBy`/`At`, dismiss attention events, trigger the golden build. Skip the terminal writes. For a perpetual run "Accept" means "land this branch", not "kill my bot". |

B6 is user-initiated, which is worth being explicit about: the deliberate-termination
routes stay archive and an explicit `PUT status: done` (US-7). Approve-implementation
is not one of them — the evidence shows it running against the live bot at 02:49:14
and being the first domino.

### Group C — keeping PR data fresh

A perpetual task now never enters `pull_request`, but `pollPullRequests`
(`spec_task_orchestrator.go:1297`) selects exactly `status = pull_request`, and
`RefreshPullRequestStatus` (`:1338`) gates on the same. Left alone, a perpetual run's
PR state and CI status would freeze at creation — which contradicts "PRs must keep
being tracked and displayed".

So both are widened to also admit `perpetual_run = true` tasks that hold at least one
PR and are in a work-in-progress status.

**Sequencing is load-bearing.** This change lands *only after* every guard in Group B
is in place and tested. Widening the selector first re-creates the exact
poll → `allMerged` → `done` loop this work exists to break — that pairing is why the
brief warns about these two filters. Implement B first, get its unit tests green,
then C.

`detectExternalPRActivity`'s `BranchName != "" && !task.HasAnyPR()` filter is **not**
touched. It is the other half of what makes a hand-reopened task stable, and the
limitation it imposes on perpetual runs (a second *externally*-created PR is never
auto-detected) is pre-existing and flagged as Open Question 3.

## Termination Routes

Two, both already implemented and unchanged by this work:

1. **Archive** — `PATCH /api/v1/spec-tasks/{taskId}/archive`
   (`spec_driven_task_handlers.go:1401`) stops the planning session's desktop and any
   running `SpecTaskExternalAgent`, then sets `archived = true`. This is what
   HelixOS's `archiveBotSpecTasks` already calls. No guard touches it — but it must be
   re-verified live (US-7).
2. **Explicit user status change** — `PUT /api/v1/spec-tasks/{id}` with
   `status: done`. A human saying "this is finished" is authoritative.

Clearing `perpetual_run` restores normal lifecycle behaviour from the next transition.

## Testing Strategy

### Go unit tests

Table-driven, one perpetual/normal pair per site, ten pairs total, against the
existing orchestrator and git-http-server test harnesses (see
`api/pkg/services/git_http_server_auto_merge_test.go` for the B5 harness):

- Group A: normal advances to `pull_request` / `implementation_review`; perpetual
  stays `implementation` **and** still has the `RepoPR` appended /
  `ImplementationApprovedBy` recorded.
- Group B: normal reaches `done` + `merged_to_main` + `completed_at`; perpetual has
  all three unset, with `RepoPullRequests[*].PRState == "merged"` still updated.
- Group C: a perpetual task in `implementation` holding a PR is selected by the
  poller; a non-perpetual task in `implementation` is not.

### Live E2E on `localhost:8080` (the part that makes this done)

Per `CLAUDE.md`, use `http://localhost:8080` — never `api:8080`, which is the outer
stack running this agent.

1. Create a project + task with `perpetual_run: true, just_do_it_mode: true`; start
   it so a real session exists with a non-empty `config->>'zed_thread_id'`.
2. Reproduce the four-minute sequence deliberately: call approve-implementation so a
   PR opens, merge the PR, then run the orchestrator poll. Assert at each step that
   `status` is still `implementation`, `merged_to_main = false`,
   `completed_at IS NULL`, and the tracked PR reads `merged`.
3. **Send the session a new message and confirm it answers.** The next operation
   working is the evidence; a status assertion alone is not.
4. **Do it a second time** — open and merge another PR on the same task. The bug
   recurs per-PR, so surviving one merge proves nothing about the third.
5. Regression: same flow on a `perpetual_run: false` task — must reach `done` with
   `merged_to_main = true` and `completed_at` set.
6. Archive the perpetual task; confirm the desktop container stops and it leaves the
   default board.

Record commands and outputs in `design/2026-09-01-perpetual-run-e2e.md` in the helix
repo, per the CLAUDE.md debugging-notes rule.

## HelixOS Interaction

Two things to state in the Helix PR description:

1. **The whipper connection.** HelixOS's whipper (`api/internal/whipper/`, helixos PR
   #161) is the keep-alive meant to stop this class of run dying, and its epoch ends
   when "the run reached a terminal status" (`WhipStopped` doc comment in
   `api/internal/types/whip.go`). So the false `done` was also silently killing
   keep-alive — a second, independent reason this fix matters. Say so explicitly or
   the connection is lost.
2. **Whether HelixOS was wired.** Setting `perpetual_run: true` from HelixOS's
   dispatcher (`api/internal/bridge/`) is a separate PR on `helixml/helixos`. **That
   repo is not checked out on this machine** (local repos are `kodit`, `helix-next`,
   `docs`, `qwen-code`, `zed`, `helix`; `helix-next` is a different codebase with no
   `api/internal/bridge`). So the Helix PR must state plainly that the HelixOS side
   was **not** wired and that the field is inert for bot runs until it is.

## Notes for Future Agents

- Spec-task columns here are added by GORM `AutoMigrate`, not by files in
  `api/pkg/store/migrations/` — those are reserved for renames, drops, and data fixes.
- `JustDoItMode` persists to the column `yolo_mode`. JSON name ≠ column name is
  normal in this file; pin the column name explicitly when adding a field.
- Pattern for "optional boolean the user can turn off": `bool` on the model, `bool` on
  the create request, `*bool` on the update request. See `KeepAlive`,
  `PublicDesignDocs`.
- The lifecycle is spread across four files —
  `services/spec_task_orchestrator.go`, `services/git_http_server.go`,
  `services/spec_driven_task_service.go`, `server/spec_task_workflow_handlers.go`.
  `grep -n "TaskStatusDone\|TaskStatusPullRequest\|TaskStatusImplementationReview"`
  across those four is the way to find every transition; the census above was built
  that way and found two sites a careful hand-written brief had missed.
