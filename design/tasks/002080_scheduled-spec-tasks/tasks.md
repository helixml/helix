# Implementation Tasks: Auto-start cron-scheduled spec tasks

- [ ] Edit `api/pkg/trigger/cron/trigger_cron.go` `executeSpecTaskAction` (around line 734) to set `AutoStart: true` on the `CreateTaskRequest` passed to `specTaskCreator.CreateTaskFromPrompt`.
- [ ] Run `go build ./api/pkg/trigger/cron/ ./api/pkg/services/ ./api/pkg/server/` to confirm the change compiles cleanly.
- [ ] Add or extend a unit test in `api/pkg/trigger/cron/` that calls `executeSpecTaskAction` with a fake `SpecTaskCreator` (gomock or a spy) and asserts the captured `CreateTaskRequest` has `AutoStart == true`. Test name suggestion: `TestExecuteSpecTaskAction_SetsAutoStart`.
- [ ] Run that test locally (`CGO_ENABLED=1 go test -v -run TestExecuteSpecTaskAction ./api/pkg/trigger/cron/ -count=1`) and confirm it passes.
- [ ] Manually verify end-to-end in the inner Helix at `http://localhost:8080`:
   - Register / log in (`test@helix.ml` / `helixtest`), complete onboarding.
   - Create a project; **leave Auto-Start Backlog Tasks OFF**.
   - Create a Helix app, attach a cron trigger with `action=spec_task` and a schedule one minute in the future.
   - Wait for the cron to fire; confirm the newly-created task lands in the `Queued Spec Generation` column (NOT `Backlog`) and is then progressed by the orchestrator.
- [ ] Regression check: from the project Kanban board, manually create a task without ticking auto-start. Confirm it still lands in `Backlog`.
- [ ] Commit with a conventional message, e.g. `fix(api): auto-start spec tasks created by cron triggers`.
- [ ] Push the branch, open the PR, and watch CI via `gh pr checks <num>` until green. If red, fetch failing-step logs via the Drone MCP tools and fix.
