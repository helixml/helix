# Implementation Tasks: Attach Screenshots and Documents to Spec Tasks

## Backend — data + storage

- [x] Add `SpecTaskAttachment` type in `api/pkg/types/simple_spec_task.go` (matches schema in design.md).
- [x] Add a `store_spec_task_attachments.go` CRUD layer (Create/Get/List by task / Delete / DeleteByTaskID) and register the table for GORM AutoMigrate.
- [x] Regenerate store mocks so `MockStore` covers the new methods.
- [x] Add `FilestoreSpecTask*` helpers in `api/pkg/controller/filestore.go`.

## Backend — staging into helix-specs

- [x] Create `api/pkg/services/spec_task_attachments.go` that commits `attachments/*` to helix-specs via `WithExternalRepoWrite()`.
- [x] Make staging idempotent (skip rows with `CommittedSHA != ""`).
- [x] Call staging from `StartSpecGeneration()` before `BuildPlanningPrompt`; log + continue on failure (don't fail the task — attachments are best-effort).
- [x] Extend `prepopulateClonedSpecs()` to copy `attachments/*` from source → destination task dir.

## Backend — prompt

- [x] Add `AttachmentsSection` to `PlanningPromptData` and the planning prompt template.
- [x] Implement `BuildAttachmentsSection(attachments, taskDirName)` in `spec_task_prompts.go`.
- [x] Wire it into the `BuildPlanningPrompt` caller (loads attachments via staging step).

## Backend — HTTP

- [x] Create `api/pkg/server/spec_task_attachments_handlers.go` with upload/list/content/delete endpoints.
- [x] Auth via `authorizeUserToProjectByID` against the parent task's project.
- [x] Upload: 10 MB/file, 10 files/task, MIME allowlist, content-type sniff, SVG `<script>` rejection.
- [x] Upload + delete: reject (`409 Conflict`) when task status is past `spec_review`.
- [x] Content endpoint: public if `task.PublicDesignDocs`, otherwise auth-gated. Mounted on unauthenticated subrouter so anonymous reads work.
- [x] Register routes in `api/pkg/server/server.go`.
- [x] Add swagger annotations and regenerate the typed client: `./stack update_openapi`.
- [x] Hook `DeleteSpecTask` to garbage-collect attachment rows + filestore prefix.

## Backend — tests

- [ ] Unit tests for size/MIME/per-task-cap validation in `spec_task_attachments_test.go`.
- [ ] Unit tests for staging idempotency (re-running is a no-op when `CommittedSHA` is set).
- [ ] HTTP suite tests in `spec_task_attachments_handlers_test.go` covering: happy path, auth denial, status-gated mutation rejection, MIME spoof rejection.
- [ ] Verify `go build ./pkg/server/ ./pkg/store/ ./pkg/types/ ./pkg/services/` passes locally.

## Frontend — service layer

- [x] Create `frontend/src/services/specTaskAttachmentsService.ts` with React-Query hooks (`useSpecTaskAttachments`, `useUpload…`, `useDelete…`).
- [x] List/delete go through the generated client; multi-file upload uses raw axios + FormData (the typed client only supports one file per call).
- [x] Invalidate `[SPEC_TASK_ATTACHMENTS_KEY, taskId]` after mutations.

## Frontend — UI

- [x] Add an "Attach files" picker + chip strip to `NewSpecTaskForm.tsx`. (Used native file input, not react-dropzone — simpler, matches existing patterns.)
- [x] Enforce client-side: accepted MIME list, 10 MB/file, 10 files/task; show inline rejection reasons.
- [x] Wire submit: create task → upload attachments → optionally start planning. Surface upload errors with a "retry from detail page" hint.
- [x] Create `frontend/src/components/tasks/TaskAttachmentsPanel.tsx`: grid of cards with thumbnail / filename / size / remove button. Lightbox for images; new tab for PDFs/text.
- [x] Mount the panel inside `SpecTaskDetailContent.tsx` below the description block.
- [x] Hide add/remove when `task.status` is past `spec_review` (read-only mode).
- [x] `yarn tsc` clean. (Production `yarn build` blocked by pre-existing dist/ root permissions — not from this change.)

## End-to-end verification

- [ ] In the inner Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, complete onboarding, create a task with one PNG and one PDF attached.
- [ ] Confirm the planning session's first prompt contains the `## Attachments` section with correct absolute paths.
- [ ] Confirm files exist in the container at `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/` (use `helix spectask exec` or the in-task terminal).
- [ ] Clone the task; confirm attachments are inherited and the cloned task's prompt lists them.
- [ ] Delete an attachment from the detail page while task is in `backlog`; confirm filestore blob is removed AND a removal commit lands on helix-specs.

## Docs

- [ ] Add a short "Attaching Files" section to the spec-task user-facing docs in `docs/` (one screenshot, ~3 paragraphs).
- [ ] No CLAUDE.md changes needed unless we discover new project conventions during implementation.
