# Implementation Tasks: Stage Spec-Task Attachments on Upload to Fix Planning Race

- [ ] Create a feature branch off `main` in `helix` (conventional-commit workflow)
- [ ] Add `StageUploadedAttachments(ctx, taskID)` to `SpecDrivenTaskService` in `api/pkg/services/spec_task_attachments.go`: load task+project+attachments, call the idempotent `commitAttachmentsToHelixSpecs` to stage into `helix-specs`
- [ ] In `StageUploadedAttachments`, when `task.PlanningSessionID != ""` and `EnqueueMessageToAgent != nil`, enqueue a non-interrupt "attachment added — check `design/tasks/<taskDir>/attachments/`" note (use `GetTaskDirName`)
- [ ] Call `StageUploadedAttachments` at the end of `uploadSpecTaskAttachments` (after rows are created): re-load the task for a fresh `PlanningSessionID`, use a detached context, warn (do not fail the 201) on staging error
- [ ] Verify no dead code / no fallback paths introduced; confirm `commitAttachmentsToHelixSpecs` idempotency (skips rows with `CommittedSHA` set) still holds for the double-stage case
- [ ] Add a service test in `api/pkg/services` reproducing the RACING sequence: attachment uploaded when a planning session already exists → assert file committed to `helix-specs` at `design/tasks/<taskDir>/attachments/<name>`, `CommittedSHA` set, and enqueuer invoked once
- [ ] Add test cases: backlog (no session) → committed but NOT notified; double-call → no double-commit, no re-notify
- [ ] `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/` and run the new test with `CGO_ENABLED=1`
- [ ] Live E2E in inner Helix (`localhost:8080`): register/login, create a task with an attachment via the UI, confirm the file appears in the sandbox workspace at `design/tasks/<taskDir>/attachments/` and the agent references it; confirm `committed_sha` is set in the DB
- [ ] Commit (conventional format), push the branch, open/verify PR, and check CI (Drone) is green — fix and re-check if red
- [ ] Update these design docs with any implementation learnings and push to `helix-specs`
