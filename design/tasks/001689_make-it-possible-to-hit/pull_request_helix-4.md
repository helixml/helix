# Add Enter key shortcut and plus button to spec task details page

## Summary
The spec task details page was missing the "Enter to create new task" shortcut and the plus button that already exist on the main specs list page. This adds both, matching the existing pattern exactly.

## Changes
- `SpecTaskDetailPage.tsx`: add `createDialogOpen` state, Enter/Escape key handler, plus icon button in top-right toolbar, and slide-in `NewSpecTaskForm` panel
- Pressing bare Enter (no modifiers, not in an input/textarea) toggles the new task panel
- Plus icon button in the top right (alongside "Open in Split Screen") also toggles the panel
- On task creation, panel closes and user stays on the current detail page
