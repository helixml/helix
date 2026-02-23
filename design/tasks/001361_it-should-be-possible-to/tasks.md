# Implementation Tasks

## Frontend Changes

- [ ] Add image attachment state to `NewSpecTaskForm.tsx` (`attachedImages: {path: string, name: string, preview: string}[]`)
- [ ] Add drag & drop zone using `FileUpload` component pattern below the task description
- [ ] Upload images to filestore on drop/select: `POST /api/v1/filestore` with path `task-attachments/pending/{uuid}/{filename}`
- [ ] Show thumbnail previews with remove button for each attached image
- [ ] Pass attachment paths in `CreateTaskRequest` when submitting form
- [ ] Regenerate API client after backend changes: `./stack update_openapi`

## Backend API Changes

- [ ] Add `Attachments []string` field to `CreateTaskRequest` in `api/pkg/types/simple_spec_task.go`
- [ ] Add `Attachments []string` field to `SpecTask` struct with `gorm:"type:json"` tag
- [ ] Run database migration to add `attachments` column to `spec_tasks` table
- [ ] Update `createTaskFromPrompt` handler to save attachments to task record
- [ ] Move files from `task-attachments/pending/{uuid}/` to `task-attachments/{task_id}/` on task creation

## Sandbox File Transfer

- [ ] In `StartSpecGeneration()`: copy attachment files from filestore to sandbox at `/home/retro/work/attachments/{task_id}/`
- [ ] In `StartJustDoItMode()`: same file copy logic
- [ ] Use existing `uploadFileToSandbox` pattern or direct file copy to container

## Agent Prompt Updates

- [ ] Add attachments section to `approvalPromptTemplate` in `agent_instruction_service.go`
- [ ] Add attachments section to planning prompt in `StartSpecGeneration`
- [ ] Format: list each image path so agent knows where to find them
- [ ] Include note that Zed can display images with `read_file` tool

## Testing

- [ ] Test drag & drop image upload in NewSpecTaskForm
- [ ] Test image preview and removal
- [ ] Verify images appear in sandbox filesystem after task starts
- [ ] Verify agent prompt includes image paths
- [ ] Test with PNG, JPEG, and GIF formats