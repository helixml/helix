# Requirements: Screenshot Attachments in Task Creation

## User Stories

### US-1: Upload screenshots when creating a task
**As a** Helix user creating a new task,
**I want to** attach screenshots to my task prompt,
**So that** the AI agent has visual context (error messages, UI mockups, current state) to understand what I need done.

### US-2: Paste screenshots from clipboard
**As a** user,
**I want to** paste a screenshot directly from my clipboard (Ctrl+V / Cmd+V) into the task creation form,
**So that** I can quickly share what I'm looking at without saving a file first.

### US-3: View attached screenshots
**As a** user viewing a task,
**I want to** see the attached screenshots rendered inline in the task description,
**So that** I can review the visual context that was provided.

## Acceptance Criteria

1. The task creation form (`NewSpecTaskForm.tsx`) shows an image upload area near the prompt textarea.
2. Users can drag-and-drop image files or click to browse.
3. Users can paste images from clipboard into the form (Ctrl+V / Cmd+V).
4. Uploaded images are stored in the Helix filestore at a task-scoped path.
5. Markdown image references (`![](url)`) are automatically appended to the prompt text sent to the API.
6. Images display inline when viewing the task detail page (already supported by react-markdown).
7. Supported formats: PNG, JPEG, GIF, WebP.
8. Maximum file size: 10MB per image.
9. Multiple images can be attached to a single task.
10. Upload progress is shown to the user.

## Out of Scope

- Image editing/annotation (crop, highlight, etc.)
- Video attachments
- Attaching images to existing tasks after creation (future enhancement)
- OCR or AI-based image analysis at upload time
