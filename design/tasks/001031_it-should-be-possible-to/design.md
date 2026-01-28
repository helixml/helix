# Design: Edit SpecTask Prompt Before Planning

## Current State

The edit functionality already exists in `SpecTaskDetailContent.tsx`:
- Edit button (pencil icon) in header, visible only for `backlog` status tasks
- Edit mode with TextField for description and Select for priority
- Save/Cancel buttons with backend integration via `updateSpecTask` mutation

**Problem**: The edit button is too subtle and users don't notice it.

## Solution

Make the description text itself clickable to enter edit mode, with hover feedback to indicate it's editable.

## Implementation Details

### Location
`helix/frontend/src/components/tasks/SpecTaskDetailContent.tsx` in the `renderDetailsContent()` function, around the Description section (line ~622).

### Changes Required

1. **Wrap description text in clickable Box** with hover styles (only for backlog status):

```tsx
{/* Description */}
<Box sx={{ mb: 3 }}>
  <Typography variant="subtitle2" color="text.secondary" gutterBottom>
    Description
  </Typography>
  {isEditMode ? (
    <TextField
      fullWidth
      multiline
      rows={4}
      value={editFormData.description}
      onChange={(e) => setEditFormData(prev => ({ ...prev, description: e.target.value }))}
      placeholder="Task description"
    />
  ) : (
    <Box
      onClick={task?.status === TypesSpecTaskStatus.TaskStatusBacklog ? handleEditToggle : undefined}
      sx={{
        cursor: task?.status === TypesSpecTaskStatus.TaskStatusBacklog ? 'pointer' : 'default',
        '&:hover': task?.status === TypesSpecTaskStatus.TaskStatusBacklog ? {
          backgroundColor: 'action.hover',
          borderRadius: 1,
          mx: -1,
          px: 1,
        } : {},
      }}
    >
      <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
        {task?.description || task?.original_prompt || 'No description provided'}
      </Typography>
    </Box>
  )}
</Box>
```

### Hover UX

- **Cursor**: Changes to `pointer` on hover (backlog only)
- **Background**: Subtle highlight using `action.hover` theme color
- **Padding**: Slight negative margin + padding to make the hover area feel natural
- **Click**: Triggers `handleEditToggle()` to enter edit mode

### Non-Backlog Tasks

- No cursor change
- No hover highlight
- Click does nothing
- Description displays as normal read-only text