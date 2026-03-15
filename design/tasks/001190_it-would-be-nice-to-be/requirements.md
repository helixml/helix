# Requirements: Pin Projects

## User Stories

**As a user**, I want to pin important projects so they appear at the top of my projects board, making them easier to find.

**As a user**, I want to unpin a project to remove it from the pinned section.

**As a user**, I expect my pinned projects to be personal (not shared with other users) and to persist across sessions.

## Acceptance Criteria

1. A pin icon is visible on each project card in the projects board.
2. Clicking the pin icon on an unpinned project pins it; clicking it again unpins it.
3. Pinned projects appear in a distinct "Pinned" section at the top of the projects board, above the regular project list.
4. Unpinned projects continue to appear in the standard projects list below.
5. Pin state is stored per-user in the database and persists across sessions and devices.
6. A project can be pinned by multiple users independently.
7. The pin action is reflected immediately in the UI (optimistic update).
8. If a pinned project is deleted, it silently disappears from the pinned section.
