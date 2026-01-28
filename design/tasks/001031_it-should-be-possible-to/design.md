# Design: Edit SpecTask Prompt Before Planning

## Current State

The `SpecTaskDetailContent.tsx` component already has:
- Edit mode toggle (`isEditMode` state)
- Form data state (`editFormData` with `name`, `description`, `priority`)
- Save handler that calls `updateSpecTask.mutateAsync()` with name, description, priority
- Edit button visible only when `task.status === 'backlog'`

**Problem**: The edit form UI only shows fields for `description` and `priority`. There is no TextField for editing the `name` field, even though the save handler already sends it to the backend.

## Solution

Add a TextField for the task name in the edit mode section of `SpecTaskDetailContent.tsx`. The backend API already supports updating the `name` field via `TypesSpecTaskUpdateRequest`.

## Implementation Details

### Location
`helix/frontend/src/components/tasks/SpecTaskDetailContent.tsx` in the `renderDetailsContent()` function.

### Changes Required

1. **Add Name TextField** - Insert a new form field above the Description field in edit mode:
   ```tsx
   {/* Name/Prompt - only in edit mode */}
   {isEditMode && (
     <Box sx={{ mb: 3 }}>
       <Typography variant="subtitle2" color="text.secondary" gutterBottom>
         Task Prompt
       </Typography>
       <TextField
         fullWidth
         multiline
         rows={2}
         value={editFormData.name}
         onChange={(e) => setEditFormData(prev => ({ ...prev, name: e.target.value }))}
         placeholder="Task prompt/name"
       />
     </Box>
   )}
   ```

2. **Display Name in Read Mode** - Optionally show the name when not in edit mode (currently only shown in header). This is optional since the task name is already displayed in the page header.

### Data Flow

```
User clicks Edit → isEditMode = true → editFormData populated from task
User edits name TextField → editFormData.name updated
User clicks Save → updateSpecTask.mutateAsync({ name, description, priority })
Backend updates task → Query invalidated → UI refreshes
```

## Key Decisions

1. **Edit `name` not `original_prompt`**: The `name` field is the user-editable title. The `original_prompt` field preserves the original request for history/debugging purposes and should remain immutable.

2. **Reuse existing infrastructure**: No new state, handlers, or API calls needed. The edit form already handles `name` in `editFormData` and sends it in the update request.

3. **Multiline TextField**: Using `rows={2}` for prompts since they can be longer than typical titles.