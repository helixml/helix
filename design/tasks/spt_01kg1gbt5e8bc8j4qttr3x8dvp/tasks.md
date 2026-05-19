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

- [x] Unit tests for filename sanitisation, MIME sniff, SVG-script rejection, status lock, allowlist in `spec_task_attachments_validation_test.go`.
- [x] Unit tests for `BuildAttachmentsSection` + `humanSize` in `spec_task_attachments_prompt_test.go`.
- [x] `go build ./...` clean.
- [x] All new tests pass: `go test ./api/pkg/server/ ./api/pkg/services/ -run "TestBuildAttachmentsSection|TestSanitiseAttachmentFilename|..."`.

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

- [x] Inner Helix at `http://localhost:8080`: registered, onboarded, created task with PNG attached via `NewSpecTaskForm` — see `screenshots/02-new-task-form-with-attached-png.png`.
- [x] Task created (backlog status) with attachment row in DB and blob in `/filestore/dev/spec-tasks/<task-id>/attachments/`.
- [x] Detail page renders `TaskAttachmentsPanel` with image thumbnail (served via `/api/v1/spec-tasks/{id}/attachments/{att}/content`) — see `screenshots/03-task-detail-with-attachment.png`.
- [x] Add another file (TXT) from detail page; renders generic icon — see `screenshots/04-task-detail-multiple-attachments.png`.
- [x] Delete from detail page: confirmation dialog, blob removed from filestore, DB row gone, panel returns to empty state.
- [ ] Full agent end-to-end (start planning, agent reads attachment from `/home/retro/work/helix-specs/...`) — left for runtime verification; the staging path commits via `WithExternalRepoWrite` which is the same plumbing used for `prepopulateClonedSpecs`, so behaviour is consistent with existing flows.

## Docs

- [ ] User-facing docs in `docs/` — skip for now; the UI is self-explanatory (button + helper text describe limits). Add if users ask.
- [x] No CLAUDE.md changes — no new project conventions introduced.

## Implementation Notes

- The dev environment runs Vite HMR for the frontend (`helix-frontend-1` on port 8081), so the production `yarn build` blocked by a `dist/` owned-by-root permission is a pre-existing issue, not regression. `yarn tsc` is clean and the dev server picks up changes immediately.
- Air auto-rebuilt the API after the changes — verified the new `/api/v1/spec-tasks/{taskId}/attachments` endpoint returns 401 (auth required) instead of 404 (route missing).
- Multipart upload uses raw axios (not the generated client) because the typed client signature only allows a single `File` per call, whereas the backend accepts an array under `files[]`. Listing/delete go through the generated client per CLAUDE.md.
- The content endpoint is mounted on `subRouter` (no-auth), not `authRouter`, so it can serve public-design-docs reads. The handler itself does the auth check: anonymous read is allowed iff `task.PublicDesignDocs == true`; otherwise requires `ActionGet` on the project.
- `BuildPlanningPrompt` got a 5th parameter (`attachmentsSection`). One existing test (`spec_task_prompts_test.go`) needed updating to pass an extra empty string.
