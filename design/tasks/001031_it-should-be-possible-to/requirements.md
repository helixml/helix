# Requirements: Edit SpecTask Prompt Before Planning

## User Story

As a user, I want to easily edit the prompt/description of a SpecTask after creating it but before clicking "Start Planning", so that I can refine my request without having to delete and recreate the task.

## Problem

The edit functionality already exists (pencil icon in header), but it's too subtle and easy to miss. Users don't realize they can edit the prompt.

## Acceptance Criteria

1. **Discoverable Edit on Hover**
   - When hovering over the description text, show visual feedback that it's editable (cursor change, subtle highlight)
   - Clicking the description text enters edit mode directly
   - Only applies when `task.status === 'backlog'`

2. **Existing Edit Button Remains**
   - Keep the existing pencil icon button in the header as an alternative

3. **Edit Mode Behavior** (already implemented)
   - TextField for editing description
   - Save/Cancel buttons
   - Success/error feedback via snackbar

4. **Frozen After Planning Starts**
   - Once planning starts (status != 'backlog'), description is read-only
   - No hover effect, no click-to-edit
   - Users can interact with the agent via chat thread instead