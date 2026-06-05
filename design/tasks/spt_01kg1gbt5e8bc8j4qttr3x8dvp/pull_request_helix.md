# feat(api,frontend): attach screenshots and documents to spec tasks

## Summary

Users can now attach screenshots, PDFs, and text documents to a spec task — both at creation time and from the task detail page while the task is still in `backlog`, `spec_generation`, `spec_review`, or `spec_revision`. When spec generation starts, the attachments are committed into the `helix-specs` branch under `design/tasks/<task-dir>/attachments/`, so they appear inside the agent's sandbox container at a stable, well-known path. The planning prompt gains an `## Attachments` section listing each file with its in-container path, MIME type, size, and optional caption, and instructing the agent to read them before exploring further.

## Why this approach

Zero new transport. The `helix-specs` repo is already cloned into the agent's workspace by the startup script and is the same place `prepopulateClonedSpecs()` writes cloned design docs via `WithExternalRepoWrite()`. Reusing that path means attachments are visible to the agent, browsable in the existing helix-specs viewer, gated by the same `PublicDesignDocs` flag, and automatically present in cloned tasks. The 10 MB × 10 file cap keeps git history healthy without needing git-LFS for v1.

## Changes

### Backend (Go)
- New `SpecTaskAttachment` type (`api/pkg/types/simple_spec_task.go`) + `spec_task_attachments` table (GORM AutoMigrate, no SQL migration needed).
- New store CRUD: `Create/Get/Update/Delete/ListSpecTaskAttachment(s)` + `DeleteSpecTaskAttachmentsByTaskID`.
- New filestore helpers: `GetSpecTaskAttachmentsPrefix`, `FilestoreSpecTaskAttachment{Upload,Download,Delete,sDeleteAll}`.
- New REST endpoints (all auth-gated by `authorizeUserToProjectByID`):
  - `POST /api/v1/spec-tasks/{taskId}/attachments` — multipart upload, `files[]` field + optional `caption`. Enforces 10 MB/file, 10 files/task, MIME allowlist, content-sniff, SVG `<script>` rejection. Returns 409 once task is past `spec_review`.
  - `GET /api/v1/spec-tasks/{taskId}/attachments` — list.
  - `DELETE /api/v1/spec-tasks/{taskId}/attachments/{attId}` — same status gate.
  - `GET /api/v1/spec-tasks/{taskId}/attachments/{attId}/content` — streams bytes. Allows anonymous reads when `task.PublicDesignDocs == true`.
- `StartSpecGeneration` now stages attachments into helix-specs (idempotent via `CommittedSHA`) and the planning prompt gains an `## Attachments` section pointing the agent at the in-workspace paths.
- `prepopulateClonedSpecs` extended to copy attachments from source → cloned task in the same commit window.
- `deleteSpecTask` garbage-collects attachment rows and the filestore prefix.
- Unit tests for filename sanitisation, MIME sniff, SVG-script rejection, status lock, allowlist, and prompt rendering.

### Frontend (React/TS)
- `frontend/src/services/specTaskAttachmentsService.ts` — React-Query hooks (`useSpecTaskAttachments`, `useUploadSpecTaskAttachments`, `useDeleteSpecTaskAttachment`) and shared constants.
- `frontend/src/components/tasks/TaskAttachmentsPanel.tsx` — grid of cards (thumbnail for images, generic icon for PDFs/text), lightbox for images, new-tab open for documents, remove confirmation. Hidden buttons in read-only statuses.
- `NewSpecTaskForm.tsx` — Attach files button + chip strip below the prompt. Files uploaded after task creation; failure surfaces a hint to retry from the detail page.
- `SpecTaskDetailContent.tsx` — mounts the panel below the description block.

## Screenshots

![New task form with Attach files button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kg1gbt5e8bc8j4qttr3x8dvp/screenshots/01-new-task-form-with-attach-button.png)

![New task form with attached PNG chip](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kg1gbt5e8bc8j4qttr3x8dvp/screenshots/02-new-task-form-with-attached-png.png)

![Detail page rendering the image thumbnail](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kg1gbt5e8bc8j4qttr3x8dvp/screenshots/03-task-detail-with-attachment.png)

![Detail page with image + text attachment](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kg1gbt5e8bc8j4qttr3x8dvp/screenshots/04-task-detail-multiple-attachments.png)

## Test plan

- [x] `go build ./...` clean.
- [x] `go test ./api/pkg/server/ ./api/pkg/services/ -run "TestBuildAttachmentsSection|TestSanitiseAttachmentFilename|TestDetectAttachmentMime|TestSvgContainsScript|TestSpecTaskAttachmentsLocked|TestAllowedMimeTypes|TestBuildPlanningPrompt"` passes.
- [x] `yarn tsc` clean.
- [x] End-to-end in the inner Helix: register, create task with PNG attached, see chip in form, find row in DB, see thumbnail on detail page, add a second file, delete one (blob + row removed).
- [ ] Reviewer: start planning on a task with attachments and confirm the agent reads `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/<file>` before exploring.
- [ ] Reviewer: clone a task that has attachments, confirm the cloned task's prompt lists them and they're present in the new task's helix-specs commit.
