# Implementation Tasks

## Frontend
- [x] Create `ImageAttachments.tsx` component with drag-and-drop (react-dropzone) and thumbnail preview grid
- [x] Add clipboard paste handler to `ImageAttachments` that intercepts Ctrl+V image data and triggers upload
- [x] Wire up file uploads using `useUploadFilestoreFiles()` to store images at `/users/{userId}/task-attachments/{sessionId}/{filename}`
- [x] Add upload progress indicator and error handling for each image
- [x] Add remove button ("X") on each thumbnail that deletes the file from the filestore and removes it from the list, so wrong images can be discarded before submitting
- [x] Integrate `ImageAttachments` into `NewSpecTaskForm.tsx` below the prompt textarea
- [x] On form submit, send uploaded image filestore paths in `AttachmentPaths` field and append markdown image references to the prompt text
- [x] Add file size validation (max 10MB) and file type filtering (PNG, JPEG, GIF, WebP)

## Backend
- [x] Add `AttachmentPaths []string` field to `CreateTaskRequest` and `SpecTask` model in `simple_spec_task.go`
- [x] In `spec_driven_task_service.go`, when building the planning Interaction: if attachments exist, read images from filestore, base64-encode them, and use `PromptMessageContent` with `ImageURLPart` entries instead of plain `PromptMessage`
- [x] Regenerated OpenAPI spec and TypeScript API client to include `attachment_paths` field
- [x] Wired filestore into SpecDrivenTaskService via SetFileStore() for reading uploaded images

## Deferred
- [ ] Copy attached images into the agent workspace directory — deferred because images are already sent inline via multimodal prompt (base64) and the agent can access the filestore API via USER_API_TOKEN if needed

## Verification
- [x] Verify image upload appears as thumbnail in form with remove button
- [x] Verify remove button deletes image from UI and filestore
- [ ] Verify images render correctly on the task detail page via existing react-markdown rendering (requires creating an actual task with images)
- [ ] Verify the LLM receives images as multimodal content (requires running agent with attached images)
