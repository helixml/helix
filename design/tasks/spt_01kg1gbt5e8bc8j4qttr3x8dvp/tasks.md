# Implementation Tasks: Attach Screenshots and Documents to Spec Tasks

## Backend — data + storage

- [x] Add `SpecTaskAttachment` type in `api/pkg/types/simple_spec_task.go` (matches schema in design.md).
- [x] Add a `store_spec_task_attachments.go` CRUD layer (Create/Get/List by task / Delete / DeleteByTaskID) and register the table for GORM AutoMigrate.
- [x] Regenerate store mocks so `MockStore` covers the new methods.
- [~] Add `FilestoreSpecTask*` helpers in `api/pkg/controller/filestore.go`.

## Backend — staging into helix-specs

- [ ] Create `api/pkg/services/spec_task_attachments.go` with `StageAttachmentsToHelixSpecs(ctx, task)` that uses `WithExternalRepoWrite()` to commit `attachments/*` into `design/tasks/<task-dir>/attachments/`, then sets `CommittedSHA` on each row.
- [ ] Make staging idempotent (skip rows with `CommittedSHA != ""`).
- [ ] Call `StageAttachmentsToHelixSpecs` from `StartSpecGeneration()` *before* `BuildPlanningPrompt`; on failure, mark the task failed with a descriptive message.
- [ ] Extend `prepopulateClonedSpecs()` to also copy `attachments/*` from source → destination task dir, and create matching `SpecTaskAttachment` rows for the cloned task.

## Backend — prompt

- [ ] Add `AttachmentsSection` to `PlanningPromptData` and the planning prompt template (guarded by `{{if .AttachmentsSection}}`).
- [ ] Implement `BuildAttachmentsSection(attachments, taskDirName)` in `spec_task_prompts.go` producing the markdown shown in design.md.
- [ ] Wire it into `BuildPlanningPrompt` callers (load attachments from store; pass into the prompt).
- [ ] Add an "if attachments are present, read them first" sentence to the existing Visual Testing section of the prompt.

## Backend — HTTP

- [ ] Create `api/pkg/server/spec_task_attachments_handlers.go` with the four endpoints from design.md (POST upload, GET list, GET content, DELETE).
- [ ] All handlers must call `authorizeUserToProjectByID` against the parent task's project.
- [ ] Upload handler: enforce 10 MB per file, 10 files per task, MIME allowlist, magic-bytes sniff via `http.DetectContentType`, SVG `<script>` rejection.
- [ ] Upload + delete: reject (`409 Conflict`) when task status is past `spec_review`.
- [ ] Content handler: support `?presigned=true` returning a 5-minute signed URL using the existing `filestore.VerifySignature` infra.
- [ ] Public-design-docs gating: when `task.PublicDesignDocs == true`, allow anonymous reads of the content endpoint (consistent with how design docs are gated today).
- [ ] Register routes in `api/pkg/server/server.go`.
- [ ] Add swagger annotations and regenerate the typed client: `./stack update_openapi`.

## Backend — tests

- [ ] Unit tests for size/MIME/per-task-cap validation in `spec_task_attachments_test.go`.
- [ ] Unit tests for staging idempotency (re-running is a no-op when `CommittedSHA` is set).
- [ ] HTTP suite tests in `spec_task_attachments_handlers_test.go` covering: happy path, auth denial, status-gated mutation rejection, MIME spoof rejection.
- [ ] Verify `go build ./pkg/server/ ./pkg/store/ ./pkg/types/ ./pkg/services/` passes locally.

## Frontend — service layer

- [ ] Create `frontend/src/services/taskAttachmentsService.ts` with React-Query hooks: `useTaskAttachments(taskId)`, `useUploadTaskAttachments()`, `useDeleteTaskAttachment()`.
- [ ] All calls go through the generated `api.getApiClient()` (no raw `fetch`/`api.post`).
- [ ] Invalidate `['task-attachments', taskId]` after mutations.

## Frontend — UI

- [ ] Add a drag-and-drop attachment area to `NewSpecTaskForm.tsx`, reusing `components/widgets/FileUpload.tsx`. Show pending list with thumbnails + size + remove button.
- [ ] Enforce client-side: accepted MIME list, 10 MB/file, 10 files/task; show inline rejection reasons.
- [ ] Wire submit: create task → upload attachments → optionally start planning. Surface upload errors with a "Retry upload" affordance.
- [ ] Create `frontend/src/components/tasks/TaskAttachmentsPanel.tsx`: list rows with thumbnail / filename / size / vertical-dot menu (View, Download, Remove). Lightbox for images; new tab for PDFs/text.
- [ ] Mount the panel inside `SpecTaskDetailContent.tsx` above the description block.
- [ ] Disable add/remove when `task.status` is past `spec_review` (read-only mode).
- [ ] `yarn build` cleanly in `frontend/`.

## End-to-end verification

- [ ] In the inner Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, complete onboarding, create a task with one PNG and one PDF attached.
- [ ] Confirm the planning session's first prompt contains the `## Attachments` section with correct absolute paths.
- [ ] Confirm files exist in the container at `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/` (use `helix spectask exec` or the in-task terminal).
- [ ] Clone the task; confirm attachments are inherited and the cloned task's prompt lists them.
- [ ] Delete an attachment from the detail page while task is in `backlog`; confirm filestore blob is removed AND a removal commit lands on helix-specs.

## Docs

- [ ] Add a short "Attaching Files" section to the spec-task user-facing docs in `docs/` (one screenshot, ~3 paragraphs).
- [ ] No CLAUDE.md changes needed unless we discover new project conventions during implementation.
