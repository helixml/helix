# Design: Fix Task Description Not Being Used When Launching

## Overview

Fix the `BacklogTableView` component to consistently use the `description` field (with fallback to `original_prompt`) for both display and editing of task descriptions.

## Current Architecture

### Field Semantics (Backend)

The backend correctly distinguishes between two fields:

```go
// In spec_driven_task_service.go, CreateTaskFromPrompt:
task := &types.SpecTask{
    Name:           generateTaskNameFromPrompt(req.Prompt),
    Description:    req.Prompt,      // User-editable
    OriginalPrompt: req.Prompt,      // Immutable original
    ...
}

// In StartSpecGeneration:
userPrompt := task.Description
if userPrompt == "" {
    userPrompt = task.OriginalPrompt  // Fallback
}
```

### Problem in Frontend

`BacklogTableView.tsx` uses the wrong field:

```tsx
// WRONG - Loading from original_prompt
const handlePromptClick = (task: SpecTask) => {
  setEditingPrompt(task.original_prompt || "");  // Should use description
};

// WRONG - Displaying original_prompt
{task.original_prompt || "(No prompt)"}  // Should use description
```

## Solution

### Approach

Simple field substitution - use `description` with fallback to `original_prompt` in all locations.

### Pattern to Use

```tsx
// Consistent pattern across codebase (matches SpecTaskDetailContent):
task.description || task.original_prompt || ""
```

### Changes Required

**File: `helix/frontend/src/components/tasks/BacklogTableView.tsx`**

1. **Line ~139** - `handlePromptClick` function:
   ```tsx
   // Before:
   setEditingPrompt(task.original_prompt || "");
   
   // After:
   setEditingPrompt(task.description || task.original_prompt || "");
   ```

2. **Line ~91** - Search filter:
   ```tsx
   // Before:
   (task.original_prompt || "").toLowerCase().includes(searchLower)
   
   // After:
   (task.description || task.original_prompt || "").toLowerCase().includes(searchLower)
   ```

3. **Line ~367** - Display text:
   ```tsx
   // Before:
   {task.original_prompt || "(No prompt)"}
   
   // After:
   {task.description || task.original_prompt || "(No prompt)"}
   ```

## Design Decisions

### Why Not a Helper Function?

A helper function like `getTaskDescription(task)` could reduce duplication, but:
- The pattern `task.description || task.original_prompt` is already used in `SpecTaskDetailContent.tsx`
- Only 3 occurrences in BacklogTableView need changing
- Inline fallback is clearer and consistent with existing code

### Why Not Change Backend?

The backend behavior is correct:
- `original_prompt` preserves the user's original intent for reference
- `description` allows editing without losing the original
- `StartSpecGeneration` correctly prefers `description`

## Testing

1. Create a new task with description "Original"
2. Edit in BacklogTableView, change to "Edited"
3. Save and verify:
   - Table shows "Edited"
   - Clicking to edit shows "Edited"
   - Starting planning uses "Edited"
4. Check SpecTaskDetailContent also shows "Edited"