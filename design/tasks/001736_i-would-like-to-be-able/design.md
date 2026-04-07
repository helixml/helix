# Design: Screenshot Attachments in Task Creation

## Architecture Overview

This feature leverages the **existing filestore infrastructure** for storage and the **existing markdown rendering** for display. The main work is in the frontend task creation form.

```
User uploads image → Filestore API (POST /api/v1/filestore/upload)
                   → Stored at /users/{userId}/task-attachments/{uuid}/{filename}
                   → Markdown reference appended to prompt
                   → Rendered via react-markdown on task detail page
```

## Key Decisions

### 1. Storage: Reuse existing filestore (not a new attachments table)
**Why:** The filestore already handles upload, access control, and signed URLs. Adding a separate attachments table would duplicate infrastructure. Images are stored at a user-scoped path with a unique prefix per upload session, so they don't collide.

**Path format:** `/users/{userId}/task-attachments/{uploadSessionId}/{filename}`
- `uploadSessionId` is a UUID generated when the form opens, grouping all images for one task creation attempt.

### 2. Prompt integration: Append markdown image references to the prompt text
**Why:** The `OriginalPrompt` field is plain text/markdown. By appending `![screenshot](viewerUrl)` to the prompt, images are automatically:
- Sent to the AI agent as part of the prompt context
- Rendered in the task detail view (react-markdown already handles `![](url)`)
- No backend changes needed to the `CreateTaskRequest` or `SpecTask` model

### 3. Upload timing: Upload immediately on drop/paste, not on form submit
**Why:** Uploading eagerly provides immediate feedback (progress bar, preview thumbnail) and avoids a slow submit step. If the user abandons the form, orphaned files in the filestore are acceptable (can be cleaned up later via a background job if needed).

### 4. Clipboard paste: Listen for paste events on the form container
**Why:** Users frequently screenshot with OS tools and want to Ctrl+V directly. The paste handler extracts image blobs from `ClipboardEvent.clipboardData` and uploads them the same way as drag-and-drop.

## Frontend Changes

### `NewSpecTaskForm.tsx`
- Add an `ImageAttachments` component below the prompt textarea.
- This component uses `react-dropzone` (already a dependency) for drag-and-drop.
- Registers a `paste` event listener on the form for clipboard images.
- Shows thumbnails of uploaded images with a remove button.
- On form submit, appends `\n\n![screenshot-N](viewerUrl)` for each uploaded image to the prompt string before calling the API.

### New component: `ImageAttachments.tsx`
- Props: `onImagesChange(images: UploadedImage[])` callback
- Internal state: list of `{ id, filename, previewUrl, viewerUrl, uploading, error }`
- Uses `useUploadFilestoreFiles()` from `filestoreService.ts` for uploads
- Displays upload progress via existing `UploadingOverlay` pattern

## Backend Changes

**None required.** The existing filestore upload endpoint and viewer endpoint handle everything. The prompt field already accepts arbitrary text including markdown image syntax.

## Codebase Patterns & Notes

- **Frontend framework:** React 18 + MUI 5 + MobX + Vite
- **File upload hook:** `useUploadFilestoreFiles()` in `frontend/src/services/filestoreService.ts`
- **Existing dropzone component:** `frontend/src/components/widgets/FileUpload.tsx` — can be referenced for patterns but the task form needs a lighter inline version
- **Markdown rendering:** `react-markdown` with `remark-gfm` used in `DesignDocPage.tsx` and `Markdown.tsx` — images render automatically via `![](url)` syntax
- **Filestore viewer URL:** `GET /api/v1/filestore/viewer/{path}` — serves file content with auth checks
- **API client:** Auto-generated from OpenAPI spec in `frontend/src/api/api`
- **Task creation endpoint:** `POST /api/v1/spec-tasks/from-prompt` in `api/pkg/server/spec_driven_task_handlers.go`
- **Task model:** `SpecTask` in `api/pkg/types/simple_spec_task.go` — `OriginalPrompt` is a TEXT column
