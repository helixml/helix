# Requirements: Stage Spec-Task Attachments on Upload to Fix Planning Race

## Background

When a user creates a spec task with an attachment from the UI, three
independent HTTP requests race:

1. `POST /spec-tasks/from-prompt` — task created (with `auto_start`, it goes
   straight to `queued_spec_generation`).
2. Planning kicks off (either the `auto_start` path or an explicit
   `POST /spec-tasks/{id}/start-planning`), and the orchestrator builds the
   planning prompt.
3. `POST /spec-tasks/{id}/attachments` — the slow file upload (a 2.25 MB PNG
   took ~1.5 s and lost the race).

Attachment staging into the `helix-specs` repo happens **only** at planning time
(`stageAttachmentsAndBuildPromptSection` → `commitAttachmentsToHelixSpecs`). When
the upload loses the race, `ListSpecTaskAttachments` returns zero rows at staging
time, so:

- nothing is committed → `committed_sha` stays `""`;
- the file never reaches `design/tasks/<taskDir>/attachments/`;
- the prompt's attachments section is empty → the agent is never told.

The CLI (`helix spectask start --attach`) is immune because it uploads *before*
triggering planning. The bug lives purely in the racing UI/API ordering.

Full root cause: `attachments/2026-07-07-spec-task-attachment-upload-race.md`.

## User Stories

### US-1: Attachment always reaches the agent's workspace
**As a** user creating a spec task with a screenshot/attachment from the UI,
**I want** the attachment to always land in the agent's workspace,
**so that** the agent can see it regardless of how the create/plan/upload
requests are ordered or how slow the upload is.

**Acceptance criteria:**
- [ ] Uploading an attachment stages it into the `helix-specs` branch at
  `design/tasks/<taskDir>/attachments/<name>` immediately, independent of
  planning timing.
- [ ] The attachment's `committed_sha` is set to a non-empty commit SHA after a
  successful upload.
- [ ] This holds for the racing sequence (create → start-planning → slow upload),
  not just the CLI ordering.

### US-2: The agent is made aware of late-arriving attachments
**As a** user who added an attachment after the planning prompt was already
built,
**I want** the agent to be told the attachment exists,
**so that** the file is not silently sitting in the workspace unnoticed.

**Acceptance criteria:**
- [ ] When an attachment is uploaded after a planning session already exists for
  the task, an "attachment added" note is enqueued to that session (via the
  existing queue path) telling the agent where to find it.
- [ ] When no planning session exists yet, the normal planning-time prompt path
  already lists the attachment (the row and committed file exist before the
  prompt is built), so no separate note is needed.
- [ ] The note is delivered non-interrupt (respects the prompt queue; delivered
  when the agent is idle rather than cancelling in-flight work).

### US-3: No regressions / no double-staging
**As a** maintainer,
**I want** the fix to reuse the existing idempotent staging code,
**so that** double-staging (upload-time + planning-time) is safe and no new dead
code paths are introduced.

**Acceptance criteria:**
- [ ] `commitAttachmentsToHelixSpecs` remains idempotent — attachments with a set
  `committed_sha` are skipped, so upload-time + planning-time staging never
  double-commits the same bytes.
- [ ] Existing CLI `--attach` behaviour is unchanged.
- [ ] Read-only enforcement (attachments locked past `spec_review`) is unchanged.

## Non-Goals

- Reworking the frontend request ordering is optional defense-in-depth only; the
  server fix must stand on its own. (The UI already uploads after create; the
  race is against planning, not against create.)
- No change to attachment validation, size limits, MIME sniffing, or storage.

## Testing Requirements (mandatory)

- [ ] Reproduce the **racing** sequence (create → start-planning/queued → slow
  upload), NOT the CLI path. Assert the file lands at
  `design/tasks/<taskDir>/attachments/<name>` in the repo AND the agent is told
  about it (enqueue invoked when a planning session exists).
- [ ] Live end-to-end in this inner Helix (`localhost:8080`) per CLAUDE.md:
  register/login, create a task with an attachment via the UI, and confirm the
  file appears in the sandbox workspace and the agent references it.
- [ ] Follow CLAUDE.md: feature branch, conventional commits, no fallbacks, clean
  up dead code, check CI (Drone) after push.

## Open Questions

1. **Synchronous vs async staging in the upload handler.** Staging runs a git
   commit + push (`WithExternalRepoWrite`), which can add ~1 s to the upload
   response. Design proposes staging synchronously (with a detached context) so
   the 201 response guarantees the commit was attempted and the flow is
   deterministic/testable. Acceptable, or should staging run in a background
   goroutine so the upload returns immediately? (Async makes the E2E harder to
   assert deterministically.)
2. **Push failure semantics.** Existing planning-time staging uses
   `FailOnPushError: false` (best-effort push, warn on failure). Keep the same
   best-effort semantics for upload-time staging, or should an upload-time push
   failure surface as a non-fatal warning only? Assumption: keep best-effort to
   match existing behaviour.
3. **Notify wording.** The enqueued note will instruct the agent to check
   `design/tasks/<taskDir>/attachments/`. Any preferred phrasing, or is a short
   generic "a new attachment was added, read it before continuing" sufficient?
