# Implementation Tasks: Stage Spec-Task Attachments on Upload to Fix Planning Race

- [x] Create a feature branch off `main` in `helix` (conventional-commit workflow)
- [x] Add `StageUploadedAttachments(ctx, taskID)` to `SpecDrivenTaskService` in `api/pkg/services/spec_task_attachments.go`: load task+project+attachments, call the idempotent `commitAttachmentsToHelixSpecs` to stage into `helix-specs`
- [x] In `StageUploadedAttachments`, when `task.PlanningSessionID != ""` and `EnqueueMessageToAgent != nil`, enqueue a non-interrupt "attachment added — check `design/tasks/<taskDir>/attachments/`" note (use `GetTaskDirName`)
- [x] Call `StageUploadedAttachments` at the end of `uploadSpecTaskAttachments` (after rows are created): re-load the task for a fresh `PlanningSessionID`, use a detached context, warn (do not fail the 201) on staging error
- [x] Verify no dead code / no fallback paths introduced; confirm `commitAttachmentsToHelixSpecs` idempotency (skips rows with `CommittedSHA` set) still holds for the double-stage case
- [x] Add a service test in `api/pkg/services` reproducing the RACING sequence: attachment uploaded when a planning session already exists → assert file committed to `helix-specs` at `design/tasks/<taskDir>/attachments/<name>`, `CommittedSHA` set, and enqueuer invoked once
- [x] Add test cases: backlog (no session) → committed but NOT notified; double-call → no double-commit, no re-notify
- [x] `go build ./pkg/server/ ./pkg/services/ ./pkg/store/ ./pkg/types/` and run the new test with `CGO_ENABLED=1`
- [x] Live E2E in inner Helix (`localhost:8080`): register/login, create a task with an attachment via the UI, confirm the file appears in the sandbox workspace at `design/tasks/<taskDir>/attachments/` and the agent references it; confirm `committed_sha` is set in the DB
- [x] Commit (conventional format) and push the branch `feature/002235-stage-spec-task` (merged latest `main` first, build green). NOTE: PR is created by the platform when the user clicks "Open PR"; Drone CI runs at that point — not queryable from this sandbox yet. Local `go build` + `go vet` (my files clean) + the new `CGO_ENABLED=1` service tests all pass.
- [x] Update these design docs with implementation learnings and push to `helix-specs`
