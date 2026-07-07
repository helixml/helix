# Design: Stage Spec-Task Attachments on Upload to Fix Planning Race

## Summary

Make attachment staging into `helix-specs` happen at **upload time**, not only at
planning time. When the upload lands after the planning prompt was already built,
also notify the agent via the existing session prompt-queue. This removes the
race entirely: once an upload succeeds, the file is in the repo and (if planning
already started) the agent is told.

## Current architecture (what exists today)

- **Upload handler** — `api/pkg/server/spec_task_attachments_handlers.go`,
  `uploadSpecTaskAttachments`. Writes the blob to the filestore and creates the
  `spec_task_attachments` DB row. **Does not touch the repo.**
- **Planning-time staging** — `api/pkg/services/spec_task_attachments.go`:
  - `stageAttachmentsAndBuildPromptSection` (called from
    `StartSpecGeneration` in `spec_driven_task_service.go:380`) lists
    attachments, calls `commitAttachmentsToHelixSpecs`, and returns the prompt
    section.
  - `commitAttachmentsToHelixSpecs` is **idempotent**: it filters to rows whose
    `CommittedSHA == ""`, reads each blob via `ReadAttachmentBlob`, commits them
    inside one `WithExternalRepoWrite` session under
    `design/tasks/<taskDir>/attachments/`, then persists each `CommittedSHA`.
- **Queue path (sender)** — `SpecDrivenTaskService.EnqueueMessageToAgent`
  (`SpecTaskMessageEnqueuer`), wired in `server.go:594` to
  `enqueueSpecTaskAgentMessage`. It enqueues a message onto the task's
  `PlanningSessionID` prompt queue (async, delivered when idle). It **errors if
  `PlanningSessionID == ""`** (no session yet).
- **Service wiring** — `ReadAttachmentBlob` (server.go:610) and
  `EnqueueMessageToAgent` (server.go:594) are already set on
  `specDrivenTaskService`, and the upload handler (on `HelixAPIServer`) can reach
  `s.specDrivenTaskService`.

Everything the fix needs is already wired — we are moving an existing capability
earlier in the lifecycle, not building new infrastructure.

## The fix

### 1. Add an exported staging entrypoint on the service

Add a method on `SpecDrivenTaskService` (in `spec_task_attachments.go`), e.g.:

```go
// StageUploadedAttachments stages any un-staged attachments for the task into
// helix-specs immediately (reusing the idempotent commitAttachmentsToHelixSpecs),
// and — if a planning session already exists — enqueues a note so the agent knows
// a new attachment arrived after its prompt was built. Safe to call at any point
// in the task lifecycle.
func (s *SpecDrivenTaskService) StageUploadedAttachments(ctx context.Context, taskID string) error
```

It loads the task + project + attachments, and:
- If the project has a repo configured, calls `commitAttachmentsToHelixSpecs`
  (idempotent — a no-op for already-committed rows).
- If `task.PlanningSessionID != ""` **and** `EnqueueMessageToAgent != nil`,
  enqueues an "attachment added" note (interrupt = `false`, notify the task
  creator) pointing the agent at `design/tasks/<taskDir>/attachments/`.
  - If `PlanningSessionID == ""`, do nothing extra: the planning prompt is built
    later and will list the (now-committed) attachment.

Reuse `GetTaskDirName(task)` for the directory name in the note.

### 2. Call it from the upload handler

At the end of `uploadSpecTaskAttachments`, after all rows are created and before
writing the 201 response, re-load the task (to get a fresh `PlanningSessionID` —
the one loaded at the top of the handler may be stale) and call
`s.specDrivenTaskService.StageUploadedAttachments(ctx, taskID)`.

Use a **detached context** (see `detachContext` used by `startPlanning`) so a
client disconnect after the multipart body is received doesn't abort the git
commit. Staging failure is logged as a warning and does **not** fail the upload
response — the row + blob already exist, and planning-time staging remains a
backstop.

### Why this closes the race (all orderings)

| Ordering | What happens |
|---|---|
| Upload before planning (CLI, or UI wins) | Upload stages the file; row exists. Planning later lists it in the prompt (commit is skipped as idempotent). Agent sees it in prompt. ✅ |
| Upload after planning session created + prompt built (the bug) | Upload stages the file **and** `PlanningSessionID` is set → enqueue note. Agent is told and reads the dir. ✅ |
| Upload while task still in backlog / queued, no session yet | Upload stages the file; `PlanningSessionID == ""` so no note. When planning runs, the row + file already exist → listed in prompt. ✅ |

The two branches (stage-now + conditional-notify) together cover every interleave
because the planning session ID is the discriminator: if it's set, the prompt was
(or is being) built and we must notify; if it's empty, the prompt hasn't been
built yet and the normal path will include the attachment.

## Key decisions

- **Reuse `commitAttachmentsToHelixSpecs`, don't duplicate.** It is already
  idempotent by `CommittedSHA`, so calling it at both upload time and planning
  time is safe. No new commit logic, no fallback path.
- **Notify only when a session exists.** Avoids the `enqueue → error (no session)`
  case and avoids a spurious note when the attachment will be in the initial
  prompt anyway.
- **Non-interrupt note.** A late attachment shouldn't cancel in-flight agent
  work; delivering when idle (queue semantics) is correct.
- **Best-effort push, non-fatal to upload.** Matches existing planning-time
  semantics (`FailOnPushError: false`) and keeps the upload endpoint responsive
  and robust; planning-time staging is still a backstop if the push fails.
- **Frontend ordering left as-is.** The UI already uploads after create; the race
  is against planning, which the server now handles authoritatively. No frontend
  change is required for correctness (optional defense-in-depth only).

## Files to change

| File | Change |
|---|---|
| `api/pkg/services/spec_task_attachments.go` | Add `StageUploadedAttachments`; build the "attachment added" note. Reuse `commitAttachmentsToHelixSpecs` + `GetTaskDirName`. |
| `api/pkg/server/spec_task_attachments_handlers.go` | After creating rows, re-load task and call `StageUploadedAttachments` with a detached context; warn (don't fail) on error. |

## Testing strategy

1. **Service/unit test (racing sequence).** In `api/pkg/services`, with a
   mock store + a fake `EnqueueMessageToAgent` capturing calls and a real temp
   git repo (mirror `git_integration_test.go` setup for `LocalPath` /
   `WithExternalRepoWrite`):
   - Simulate the bug: task already has a `PlanningSessionID` set and the
     attachment row exists with `CommittedSHA == ""`. Call
     `StageUploadedAttachments`. Assert (a) the file exists on the `helix-specs`
     branch at `design/tasks/<taskDir>/attachments/<name>`, (b) `CommittedSHA`
     is now non-empty, (c) the enqueuer was invoked once with a message
     referencing the attachments dir.
   - Backlog case: `PlanningSessionID == ""` → file committed, enqueuer **not**
     invoked.
   - Idempotency: calling twice does not double-commit and does not re-notify a
     row already committed.
   Run with `CGO_ENABLED=1` per CLAUDE.md (tree-sitter).
2. **Live E2E in inner Helix** (mandatory, per CLAUDE.md): register/login at
   `localhost:8080`, create a task with an attachment through the UI (the racing
   path), then verify via DB (`committed_sha` set) and by inspecting the
   `helix-specs` branch / sandbox workspace that the file is present at
   `design/tasks/<taskDir>/attachments/`, and that the agent references it.
3. **CI**: push the branch, then check Drone (`gh pr checks` / Drone MCP tools).

## Notes / gotchas learned during discovery

- `auto_start` (set by `NewSpecTaskForm`) means the UI create request itself puts
  the task into `queued_spec_generation`; the orchestrator then builds the prompt
  asynchronously. So "start-planning" in the root-cause doc is equivalently the
  `auto_start` path — the race is create/queue vs the slow upload either way.
- The upload handler's `task` is fetched at the top of the request; re-fetch
  before deciding whether to notify so `PlanningSessionID` is current.
- `enqueueSpecTaskAgentMessage` returns an error when there is no planning
  session — that's why the notify branch is gated on `PlanningSessionID != ""`.
- Attachments are locked read-only past `spec_review`
  (`specTaskAttachmentsLocked`); staging only ever runs for uploads that pass
  that gate, so no interaction with locked states.
