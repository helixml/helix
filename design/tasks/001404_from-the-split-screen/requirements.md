# Requirements: Back to Board View Navigation

## Problem Statement

When users are in the Split Screen (workspace) view, it's difficult to navigate back to the Kanban board view. Currently, users must either:
1. Click the small Kanban icon in the topbar (not obvious)
2. Navigate up to "All Projects" and back into the project

## User Stories

1. **As a user in Split Screen view**, I want an obvious way to return to the Board view so I can see my task overview without hunting for small icons.

## Acceptance Criteria

- [ ] A clearly visible "Board View" or "Back to Board" button/link is present within the Split Screen view
- [ ] Clicking it switches the view mode to `kanban` (same page, no full navigation)
- [ ] The button is visible without scrolling (above the fold)
- [ ] Works on both desktop and tablet sizes
- [ ] No disruption to existing topbar view toggle functionality

## Out of Scope

- Changing the existing topbar icons
- Mobile-specific changes (workspace view already disabled on mobile)
- Keyboard shortcuts