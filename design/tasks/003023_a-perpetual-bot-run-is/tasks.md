# Implementation Tasks: Perpetual Spec Tasks Survive PR Merges

## Data model and API

- [ ] Add `PerpetualRun bool` to `SpecTask` in `api/pkg/types/simple_spec_task.go`
      with `json:"perpetual_run" gorm:"column:perpetual_run;default:false"`, next to `KeepAlive`
- [ ] Add `HoldsLifecycleInImplementation()` and `SuppressesAutoCompletion()` helpers
      on `*SpecTask`, both returning `t.PerpetualRun`
- [ ] Add `PerpetualRun bool` to `CreateTaskRequest` and persist it in the create handler
- [ ] Add `PerpetualRun *bool` to `SpecTaskUpdateRequest` and apply it in
      `spec_driven_task_handlers.go` (pointer so `false` can be sent explicitly)
- [ ] Confirm GORM `AutoMigrate` creates the column on stack start; write no SQL
      migration and no backfill

## Group A — stop perpetual runs advancing out of `implementation`

- [ ] `spec_task_workflow_handlers.go:167` (approveImplementation, external repo): still
      record approval, create PRs, send push instruction, sync PR descriptions — but
      leave `Status = implementation`. **This is the observed trigger.**
- [ ] `spec_task_orchestrator.go:1548` (externally-opened PR detected): still append the
      `RepoPR` and emit the `pr_ready` attention event; leave status
- [ ] `spec_driven_task_service.go:1865` (PR created via push detection): still append
      the `RepoPR`; leave status
- [ ] `spec_task_workflow_handlers.go:214` and `:281` (internal repo → `implementation_review`):
      still stamp `RebaseRequestedAt`/`ImplementationApprovedBy` and send the
      push/rebase instruction; leave status

## Group B — stop perpetual runs reaching `done`

- [ ] `spec_task_orchestrator.go` ~1172 (all PRs merged): persist PR states, trigger
      golden build, dismiss attention events; skip status/`MergedToMain`/`MergedAt`/`CompletedAt`
- [ ] `spec_task_orchestrator.go` ~1235 (branch-merge fallback): same
- [ ] `spec_task_orchestrator.go` ~1596 (`checkTaskForExternalPRActivity` merged PR):
      still append the `RepoPR` and dismiss attention events; skip terminal writes
- [ ] `git_http_server.go` ~1382 (`handleMainBranchPush` sweep): skip perpetual tasks
- [ ] `git_http_server.go` ~1235 (`tryAutoMergeAfterRebase` — NOT in the original brief):
      still merge and push upstream; skip terminal writes
- [ ] `spec_task_workflow_handlers.go` ~360 (internal-repo merge success — NOT in the
      original brief): still merge, push, record approval, dismiss attention events,
      trigger golden build; skip terminal writes

## Group C — keep PR data fresh (ONLY after Group B is green)

- [ ] Widen `pollPullRequests` (`spec_task_orchestrator.go:1297`) to also select
      `perpetual_run = true` tasks that hold at least one PR in a work-in-progress status
- [ ] Widen the `RefreshPullRequestStatus` status gate (`:1338`) to match
- [ ] Leave `detectExternalPRActivity`'s `!task.HasAnyPR()` filter untouched

## Tests

- [ ] Group A unit tests, perpetual/normal pair per site: normal advances to
      `pull_request`/`implementation_review`; perpetual stays `implementation` AND still
      gets its `RepoPR` / `ImplementationApprovedBy` recorded
- [ ] Group B unit tests, perpetual/normal pair per site: normal reaches `done` +
      `merged_to_main` + `completed_at`; perpetual has none set but PR state still `merged`
      (reuse the `git_http_server_auto_merge_test.go` harness for the B5 site)
- [ ] Group C unit test: perpetual task in `implementation` holding a PR is polled;
      non-perpetual task in `implementation` is not
- [ ] Run `go test ./api/pkg/services/... ./api/pkg/server/...` and confirm green

## Live E2E on localhost:8080 (required — this is what makes it done)

- [ ] Create a perpetual task with a live session (`config->>'zed_thread_id'` non-empty)
- [ ] Reproduce the four-minute sequence: approve-implementation → PR opens → merge PR →
      run orchestrator poll
- [ ] Assert at each step: `status = 'implementation'`, `merged_to_main = false`,
      `completed_at IS NULL`, tracked PR reads `merged`
- [ ] Send the session a new message and confirm it answers — the next operation
      working is the evidence, not the status assertion
- [ ] Open and merge a SECOND PR on the same task; confirm it survives again
- [ ] Regression: same flow on a non-perpetual task still reaches `done` with
      `merged_to_main = true` and `completed_at` set
- [ ] Archive the perpetual task; confirm the desktop stops and it leaves the board
- [ ] Write commands and outputs to `design/2026-09-01-perpetual-run-e2e.md` in the helix repo

## Wrap-up

- [ ] Run `./stack update_openapi` so `perpetual_run` reaches the generated TS types
- [ ] Open the Helix PR. State in the description: (a) the whipper connection — the
      false `done` also ended HelixOS whip epochs, since `WhipStopped` fires on "the run
      reached a terminal status"; (b) that the HelixOS side was NOT wired (repo not
      checked out here), so the field is inert until HelixOS sends it; (c) that the
      archive path was verified live; (d) that two terminal sites beyond the brief's
      list were found and guarded
