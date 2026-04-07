# Design: Screenshot Attachments in Task Creation

## Architecture Overview

This feature leverages the **existing filestore infrastructure** for storage and the **existing markdown rendering** for display. The main work is in the frontend task creation form.

```
User uploads image → Filestore API (POST /api/v1/filestore/upload)
                   → Stored at /users/{userId}/task-attachments/{uuid}/{filename}
                   → Image paths saved in CreateTaskRequest.AttachmentPaths[]
                   → Rendered via react-markdown on task detail page (via viewer URLs)
                   → Sent as multimodal ImageURLPart in agent's planning prompt
                   → Copied to agent workspace for local file access
```

## Key Decisions

### 1. Storage: Reuse existing filestore (not a new attachments table)
**Why:** The filestore already handles upload, access control, and signed URLs. Adding a separate attachments table would duplicate infrastructure. Images are stored at a user-scoped path with a unique prefix per upload session, so they don't collide.

**Path format:** `/users/{userId}/task-attachments/{uploadSessionId}/{filename}`
- `uploadSessionId` is a UUID generated when the form opens, grouping all images for one task creation attempt.

### 2. Prompt integration: Two-layer approach (UI display + agent delivery)

Images need to reach **two consumers** with different capabilities:

**a) UI display (task detail page):** Markdown image references (`![screenshot](viewerUrl)`) are stored in the `OriginalPrompt` field. The existing react-markdown renderer displays them automatically.

**b) Agent LLM delivery:** The planning prompt builder (`spec_driven_task_service.go`) uses `PromptMessageContent` with `ImageURLPart` entries instead of plain `PromptMessage`. This sends images as multimodal content to the LLM so it can actually "see" them. The `MessageContent` struct and OpenAI conversion logic already exist in `types.go` but are currently unused for spec tasks.

**c) Agent local access:** Images are copied into the agent's workspace directory (`helix-specs/design/tasks/{taskDir}/screenshots/`) so the agent can reference them from disk if needed.

### 3. Upload timing: Upload immediately on drop/paste, not on form submit
**Why:** Uploading eagerly provides immediate feedback (progress bar, preview thumbnail) and avoids a slow submit step. If the user abandons the form, orphaned files in the filestore are acceptable (can be cleaned up later via a background job if needed).

### 4. Clipboard paste: Listen for paste events on the form container
**Why:** Users frequently screenshot with OS tools and want to Ctrl+V directly. The paste handler extracts image blobs from `ClipboardEvent.clipboardData` and uploads them the same way as drag-and-drop.

## Frontend Changes

### `NewSpecTaskForm.tsx`
- Add an `ImageAttachments` component below the prompt textarea.
- This component uses `react-dropzone` (already a dependency) for drag-and-drop.
- Registers a `paste` event listener on the form for clipboard images.
- Shows thumbnails of uploaded images, each with an "X" remove button.
- Clicking remove deletes the file from the filestore (via the existing `DELETE /api/v1/filestore/delete` endpoint) and removes the thumbnail from the list immediately.
- On form submit, only the remaining (non-removed) images are included in the prompt and `AttachmentPaths`.

### New component: `ImageAttachments.tsx`
- Props: `onImagesChange(images: UploadedImage[])` callback
- Internal state: list of `{ id, filename, previewUrl, viewerUrl, filestorePath, uploading, error }`
- Uses `useUploadFilestoreFiles()` from `filestoreService.ts` for uploads
- Uses the filestore delete API to remove images when the user clicks the remove button
- Displays upload progress via existing `UploadingOverlay` pattern

## Backend Changes

### API: Add `AttachmentPaths` to `CreateTaskRequest`
**File:** `api/pkg/types/simple_spec_task.go`

Add an `AttachmentPaths []string` field to `CreateTaskRequest`. The frontend sends the filestore paths of uploaded images (e.g. `/users/{userId}/task-attachments/{sessionId}/screenshot.png`). These paths are stored on the `SpecTask` record for later use when building the agent prompt.

### Planning prompt: Use multimodal `PromptMessageContent`
**File:** `api/pkg/services/spec_driven_task_service.go` (around line 439)

When creating the planning Interaction, if the task has attachment paths:
1. Read each image from the filestore via the `FileStore.OpenFile()` method.
2. Base64-encode the image data.
3. Build a `PromptMessageContent` with `ImageURLPart` entries using `data:image/png;base64,...` URIs alongside the text prompt.
4. Set `Interaction.PromptMessageContent` instead of `Interaction.PromptMessage`.

**Why base64 instead of signed URLs:** The LLM provider (Anthropic/OpenAI) needs to fetch the image. Signed filestore URLs point to the internal Helix API which may not be reachable from the external LLM provider. Base64 data URIs are self-contained and work regardless of network topology.

### Workspace delivery: Copy images to agent workspace
**File:** `api/pkg/services/spec_driven_task_service.go` or `api/pkg/external-agent/hydra_executor.go`

When setting up the agent workspace, copy attached images into the task's spec directory (`helix-specs/design/tasks/{taskDir}/screenshots/`). The agent can then reference these files locally for inclusion in design docs or further analysis. The filestore `OpenFile()` + workspace `WriteFile()` methods handle this.

## Codebase Patterns & Notes

- **Frontend framework:** React 18 + MUI 5 + MobX + Vite
- **File upload hook:** `useUploadFilestoreFiles()` in `frontend/src/services/filestoreService.ts`
- **Existing dropzone component:** `frontend/src/components/widgets/FileUpload.tsx` — can be referenced for patterns but the task form needs a lighter inline version
- **Markdown rendering:** `react-markdown` with `remark-gfm` used in `DesignDocPage.tsx` and `Markdown.tsx` — images render automatically via `![](url)` syntax
- **Filestore viewer URL:** `GET /api/v1/filestore/viewer/{path}` — serves file content with auth checks
- **API client:** Auto-generated from OpenAPI spec in `frontend/src/api/api`
- **Task creation endpoint:** `POST /api/v1/spec-tasks/from-prompt` in `api/pkg/server/spec_driven_task_handlers.go`
- **Task model:** `SpecTask` in `api/pkg/types/simple_spec_task.go` — `OriginalPrompt` is a TEXT column
- **Multimodal types:** `MessageContent`, `ImageURLPart` in `api/pkg/types/types.go` — already support image parts, converted to OpenAI format for LLM calls
- **Planning prompt builder:** `BuildPlanningPrompt()` in `api/pkg/services/spec_task_prompts.go`
- **Interaction creation:** `spec_driven_task_service.go` lines ~439-450 — where `PromptMessage` is set (change to `PromptMessageContent` for multimodal)
- **Container env vars:** `hydra_executor.go` — agent gets `USER_API_TOKEN` and `HELIX_API_URL` for API access
