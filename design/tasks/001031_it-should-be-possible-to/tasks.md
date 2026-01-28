# Implementation Tasks

- [ ] Add Name/Prompt TextField in `SpecTaskDetailContent.tsx` `renderDetailsContent()` function
  - Insert above the Description field, inside the `isEditMode` conditional
  - Use multiline TextField with `rows={2}`
  - Bind to `editFormData.name` with onChange handler
  - Label as "Task Prompt" to match creation form terminology

- [ ] Verify save handler already includes `name` in update request (existing code - just confirm)

- [ ] Test the feature:
  - Create a new SpecTask
  - Click Edit button (should appear for backlog status tasks)
  - Edit the prompt text
  - Click Save and verify the change persists
  - Click Cancel and verify changes are discarded