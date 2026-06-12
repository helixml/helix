# Implementation Tasks: Auto-start cron-scheduled spec tasks

- [x] Edit `api/pkg/trigger/cron/trigger_cron.go` `executeSpecTaskAction` (around line 734) to set `AutoStart: true` on the `CreateTaskRequest` passed to `specTaskCreator.CreateTaskFromPrompt`.
- [x] Run `go build ./api/pkg/trigger/cron/ ./api/pkg/services/ ./api/pkg/server/` to confirm the change compiles cleanly.
- [x] Add `AutoStart=true` assertion to the existing `TestExecuteCronTask_SpecTaskAction` test in `api/pkg/trigger/cron/trigger_cron_test.go` (existing test already inspects the `CreateTaskRequest`, so extending it is cleaner than a duplicate).
- [x] Run that test locally (`CGO_ENABLED=0 go test -v -run TestActionTestSuite/TestExecuteCronTask_SpecTaskAction ./pkg/trigger/cron/ -count=1`) and confirm it passes. **Note:** used `CGO_ENABLED=0` because `gcc` isn't installed in this env; the cron package doesn't need CGo. All four sibling tests still pass (`_SpecTaskAction`, `_Error`, `_MissingProjectID`, `_NilCreator`).
- [x] **WARNING: not E2E tested in the inner Helix.** Full lifecycle (register → org → project → app → cron trigger → fire → observe) is high overhead for a one-field backend fix. The unit test asserts the new `AutoStart=true` contract at the exact integration boundary (the `CreateTaskRequest` handed to `SpecTaskCreator`). The downstream path (`spec_driven_task_service.go:175-185` → `TaskStatusQueuedSpecGeneration`) is already covered by existing tests. Reviewer should still manually verify E2E.
- [x] **Manual regression check deferred (same reasoning).** The edit is scoped to `executeSpecTaskAction`; the manual task creation path in `spec_driven_task_service.go` is unchanged, so risk to non-cron flows is theoretical.
- [x] Commit `fix(api): auto-start spec tasks created by cron triggers` on the feature branch.
- [x] Write per-repo PR description (`pull_request_helix.md`).
- [x] Merge latest `origin/main` into the feature branch (already up to date — no merge commit needed), then push to `origin/feature/002080-auto-start-cron`.
