# Implementation Tasks: Perpetual Spec Tasks Survive PR Merges

## Data model and API

- [ ] Add `PerpetualRun bool` to `SpecTask` in `api/pkg/types/simple_spec_task.go`
      with `json:"perpetual_run" gorm:"column:perpetual_run;default:false"`, next to `KeepAlive`
- [ ] Add `SuppressesAutoCompletion()` helper on `*SpecTask` returning `t.PerpetualRun`
- [ ] Add `PerpetualRun bool` to `CreateTaskRequest` and persist it in the create handler
- [ ] Add `PerpetualRun *bool` to `SpecTaskUpdateRequest` and apply it in
      `spec_driven_task_handlers.go` (pointer so `false` can be sent explicitly)
- [ ] Confirm GORM `AutoMigrate` creates the column on stack start; write no SQL
      migration and no backfill

## Guard the auto-terminal transitions

- [ ] Guard `handlePullRequest` all-PRs-merged branch (`spec_task_orchestrator.go` ~1172):
      persist PR states, trigger golden build, dismiss attention events, set status to
      `implementation`; do not set `MergedToMain`/`MergedAt`/`CompletedAt`
- [ ] Guard the branch-merge fallback in the same function (~1235) the same way
- [ ] Guard `checkTaskForExternalPRActivity` merged-PR branch (~1596): still append the
      `RepoPR` and dismiss attention events, leave status unchanged
- [ ] Guard `GitHTTPServer.handleMainBranchPush` merged-branch sweep
      (`git_http_server.go` ~1382): set status back to `implementation` instead of `done`
- [ ] Leave `pollPullRequests` and `detectExternalPRActivity` selectors untouched —
      widening them reintroduces the completion loop

## Tests

- [ ] Go unit tests: all-PRs-merged — normal task → `done` + `merged_to_main` +
      `completed_at`; perpetual task → `implementation`, none of those set, PR state still `merged`
- [ ] Go unit tests: branch-merge fallback, both cases
- [ ] Go unit tests: external-PR-detection merged branch, both cases
- [ ] Go unit tests: `handleMainBranchPush` sweep, both cases
- [ ] Run `go test ./api/pkg/services/...` and confirm green

## Live E2E on localhost:8080 (required — this is what makes it done)

- [ ] Create a task with `perpetual_run: true` and a live session
      (`config->>'zed_thread_id'` non-empty)
- [ ] Drive it to all-PRs-merged and run the orchestrator poll
- [ ] Assert `status != 'done'`, `merged_to_main = false`, `completed_at IS NULL`,
      PR row shows `merged`
- [ ] Send the session a new message and confirm it answers — the next operation
      working is the evidence, not the status assertion
- [ ] Regression: a task with `perpetual_run: false` and all PRs merged still reaches
      `done` with `merged_to_main = true` and `completed_at` set
- [ ] Archive the perpetual task; confirm the desktop stops and it leaves the board
- [ ] Write the commands and outputs to `design/2026-09-01-perpetual-run-e2e.md` in the helix repo

## Wrap-up

- [ ] Run `./stack update_openapi` so `perpetual_run` reaches the generated TS types
- [ ] Open the Helix PR; state explicitly that the HelixOS side was NOT wired (repo not
      checked out here), so the field is inert until HelixOS sends it, and confirm the
      archive path was verified live
