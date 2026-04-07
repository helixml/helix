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
- [ ] Copy attached images into the agent workspace directory (`helix-specs/design/tasks/{taskDir}/screenshots/`) during workspace setup so the agent can access them locally

## Verification
- [~] Verify images render correctly on the task detail page via existing react-markdown rendering
- [ ] Verify the LLM receives images as multimodal content (check agent logs for image parts in prompt)
- [ ] Verify images are present in the agent's workspace directory inside the dev container
