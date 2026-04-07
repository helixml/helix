# Pull Request: Add screenshot attachments to task creation

**Branch:** `feature/001736-i-would-like-to-be-able`

## Summary

Allow users to attach screenshots when creating a new SpecTask so the AI agent has visual context (error messages, UI mockups, current state) to understand what needs to be done.

## Changes

### Frontend
- **New `ImageAttachments.tsx` component** with drag-and-drop (react-dropzone), clipboard paste (Ctrl+V), thumbnail preview grid with per-image remove button, upload progress, file size validation (10MB max), and file type filtering (PNG, JPEG, GIF, WebP)
- **Modified `NewSpecTaskForm.tsx`** to integrate the image attachments component, send `attachment_paths` in the create request, and append markdown image references to the prompt text

### Backend
- **Modified `simple_spec_task.go`** — added `AttachmentPaths []string` field to both `CreateTaskRequest` and `SpecTask` model (JSONB storage)
- **Modified `spec_driven_task_service.go`** — added `buildMultimodalPrompt()` method that reads images from filestore, base64-encodes them, and creates `PromptMessageContent` with `ImageURLPart` entries so external LLM providers receive the images inline
- **Modified `server.go`** — wired filestore into `SpecDrivenTaskService`
- **Regenerated OpenAPI spec and TypeScript client** to include the new `attachment_paths` field

## Testing

- Verified image upload appears as thumbnail in the form
- Verified remove button deletes image from UI and filestore
- TypeScript compilation passes (`npx tsc --noEmit`)

## Screenshots

See `screenshots/` directory:
- `01-new-task-form-with-image-upload.png` — form with uploaded image showing remove button
- `02-after-image-removed.png` — form after clicking remove, showing empty drop zone
