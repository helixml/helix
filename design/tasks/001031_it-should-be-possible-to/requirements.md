# Requirements: Edit SpecTask Prompt Before Planning

## User Story

As a user, I want to edit the prompt/description of a SpecTask after creating it but before clicking "Start Planning", so that I can refine my request without having to delete and recreate the task.

## Acceptance Criteria

1. **Edit Button in Details Panel**
   - The edit button (pencil icon) is already visible in the details panel for tasks in `backlog` status
   - Location: `SpecTaskDetailContent.tsx` header action buttons area (lines ~1057 and ~1301 for different layouts)
   - Clicking it enables edit mode for the prompt/name field
   - **Note**: Editing is NOT available on the kanban card - only in the details panel

2. **Editable Prompt Field**
   - When in edit mode, the task name/prompt should be editable via a TextField
   - The TextField should be multiline to accommodate longer prompts
   - The current prompt value should be pre-populated

3. **Save/Cancel Actions**
   - Save button persists changes to the backend via existing `updateSpecTask` mutation
   - Cancel button reverts to the original value without saving
   - Success/error feedback via snackbar

4. **Only in Backlog Status**
   - Prompt editing should only be available when `task.status === 'backlog'`
   - Once planning starts, the prompt becomes read-only (existing behavior)

## Out of Scope

- Editing prompts on the kanban card view (must open details panel)
- Editing prompts after planning has started
- Editing the `original_prompt` field directly (we edit `name`/`description`)
- Changes to the backend API (already supports updating name/description)