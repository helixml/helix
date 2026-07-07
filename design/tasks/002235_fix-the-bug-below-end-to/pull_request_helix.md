# fix(api): stage spec-task attachments on upload to fix planning race

## Summary

Spec-task attachments uploaded from the UI could be lost when the upload raced
`start-planning`. Staging into the `helix-specs` branch happened **only** at
planning time, so a slow upload that finished after the planning prompt was built
was never committed to `design/tasks/<taskDir>/attachments/` and the agent was
never told about it. (The CLI `--attach` path was immune because it uploads before
triggering planning.)

This makes attachment staging independent of the planning race: attachments are
staged into `helix-specs` **at upload time**, and if a planning session already
exists, the agent is notified about the late arrival via the existing prompt
queue.

## Changes

- **`api/pkg/services/spec_task_attachments.go`** — add `StageUploadedAttachments`:
  stages any un-staged attachments into `helix-specs` by reusing the idempotent
  `commitAttachmentsToHelixSpecs`, and — only when a planning session already
  exists and something new actually arrived — enqueues a non-interrupt
  "attachment added" note (built from `BuildAttachmentsSection`) so the agent
  reads it.
- **`api/pkg/server/spec_task_attachments_handlers.go`** — after creating the
  attachment rows, `uploadSpecTaskAttachments` calls `StageUploadedAttachments`
  (detached context so a client disconnect can't abort the commit). Staging
  failure is non-fatal: the row + blob exist and planning-time staging remains a
  backstop.
- **`api/pkg/services/spec_task_attachments_stage_test.go`** — new integration
  tests against a real git repo covering the racing upload (stage + notify), the
  backlog case (stage, no notify), and idempotency (no double-commit, no
  re-notify).

## Why it closes the race

`PlanningSessionID` is the discriminator across all request orderings:

- **Upload before planning** (CLI, or UI wins): file staged now; planning lists it
  in the prompt (commit skipped as idempotent).
- **Upload after the planning prompt was built** (the bug): file staged now **and**
  a session exists → the agent is notified.
- **Upload while still in backlog** (no session yet): file staged now; the planning
  prompt lists it later.

## Testing

- **Unit/integration** (`api/pkg/services`, real git, `CGO_ENABLED=1`): all three
  scenarios pass.
- **Live E2E in dev Helix**: created a task with an attachment via the real UI on
  a project with a local repo. Backlog upload → `committed_sha` set and file
  present at `design/tasks/000001_e2e-test-verify/attachments/e2e-attach-proof.png`.
  Then, with a planning session present, a second upload → file staged **and** a
  `prompt_history_entries` row queued (status `pending`, `interrupt=false`) with
  the "A new attachment was added…" note. Confirmed via DB, git tree, and API
  logs (`Committed attachments to helix-specs branch` + `[QUEUE] Enqueued agent
  message`).

## Screenshots

See `design/tasks/002235_fix-the-bug-below-end-to/screenshots/` on the helix-specs
branch (`02-attachments-staged.png`).
