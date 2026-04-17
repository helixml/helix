# Rename "Just Do It" to "Skip spec" and add toggle to backlog cards

## Summary
The "Just do it" terminology was confusing to users. This renames it to "Skip spec" everywhere and adds a toggle directly on backlog task cards so users can enable/disable it without opening edit mode.

## Changes
- **SpecTaskActionButtons.tsx**: Renamed backlog button from "Just Do It" → "Start Implementation" and "Retry" → "Retry Implementation". Added a "Skip spec" Switch toggle next to the action button that calls the update API to flip `just_do_it_mode`.
- **NewSpecTaskForm.tsx**: Renamed the creation form checkbox from "Just Do It" to "Skip spec". Updated placeholder and helper text to describe skipping spec generation.
- **SpecTaskDetailContent.tsx**: Updated the edit-mode checkbox label from "Skip planning" to "Skip spec" for consistency.

## Screenshots

### Backlog card with "Skip spec" ON → shows "Start Implementation"
![Skip spec ON](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001848_the-button-just-do-it/screenshots/02-backlog-skip-spec-on.png)

### Backlog card with "Skip spec" OFF → shows "Start Planning"
![Skip spec OFF](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001848_the-button-just-do-it/screenshots/03-backlog-skip-spec-off.png)
