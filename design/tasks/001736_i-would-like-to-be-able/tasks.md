# Implementation Tasks

- [ ] Create `ImageAttachments.tsx` component with drag-and-drop (react-dropzone) and thumbnail preview grid
- [ ] Add clipboard paste handler to `ImageAttachments` that intercepts Ctrl+V image data and triggers upload
- [ ] Wire up file uploads using `useUploadFilestoreFiles()` to store images at `/users/{userId}/task-attachments/{sessionId}/{filename}`
- [ ] Add upload progress indicator and error handling for each image
- [ ] Add remove button on each thumbnail to delete an uploaded image
- [ ] Integrate `ImageAttachments` into `NewSpecTaskForm.tsx` below the prompt textarea
- [ ] On form submit, append markdown image references (`![screenshot](viewerUrl)`) to the prompt text before calling the API
- [ ] Verify images render correctly on the task detail page via existing react-markdown rendering
- [ ] Add file size validation (max 10MB) and file type filtering (PNG, JPEG, GIF, WebP)
