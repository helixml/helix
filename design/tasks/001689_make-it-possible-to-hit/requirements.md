# Requirements: Enter Key + Plus Button on Spec Task Details Page

## User Stories

**As a user viewing a spec task detail page**, I want to press Enter to open a new spec task panel, so I can quickly create related tasks without navigating away.

**As a user viewing a spec task detail page**, I want a plus (+) icon in the top right of the page, so I can easily create a new spec task while reviewing the current one.

## Current Behavior

- `SpecTasksPage` (kanban/list view) already has both behaviors:
  - Pressing bare Enter toggles the right-side `NewSpecTaskForm` panel
  - A plus button in the toolbar opens the same panel
- `SpecTaskDetailPage` has neither — only an "Open in Split Screen" icon button in the top right

## Acceptance Criteria

1. On the spec task detail page, pressing bare Enter (no modifiers, not focused in an input/textarea) opens the `NewSpecTaskForm` panel
2. Pressing Enter again (or Escape) closes the panel
3. A plus icon button appears in the top-right toolbar of the spec task detail page, next to the existing "Open in Split Screen" button
4. Clicking the plus button opens the same `NewSpecTaskForm` panel
5. After a task is created, the panel closes and the user stays on the current detail page
6. The same guard conditions apply as in `SpecTasksPage`: ignore Enter if target is INPUT, TEXTAREA, contentEditable, or has tabindex
