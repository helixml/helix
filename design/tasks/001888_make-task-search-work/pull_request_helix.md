# Add task search bar to mobile kanban view

## Summary
The kanban board's search/filter toolbar was hidden on mobile (`display: { xs: "none", md: "flex" }`), making it impossible for mobile users to search or filter tasks. This adds a mobile-only search bar that uses the existing filtering logic.

## Changes
- Added a mobile-only search bar (`display: { xs: "flex", md: "none" }`) in `SpecTaskKanbanBoard.tsx`, positioned between the header and kanban board
- Full-width `TextField` with search icon and clear button, wired to the existing `searchFilter`/`setSearchFilter` state
- Uses existing `matchesAllTokens` filtering — no new logic needed
- Column task counts in the mobile sidebar automatically update since they derive from `filteredTasks`

## Screenshots
![Mobile search bar visible](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001888_make-task-search-work/screenshots/01-mobile-search-visible.png)
![No matching tasks state](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001888_make-task-search-work/screenshots/02-mobile-search-no-results.png)

## Testing
- Verified search bar appears on mobile viewport (375px) and is hidden on desktop (1280px)
- Verified search filters tasks correctly (matching and non-matching queries)
- Verified "No matching tasks" empty state appears
- Verified sidebar column counts update with filtered results
- Verified clear button resets search and restores tasks
- Verified search persists when switching columns via mobile sidebar
- Verified no desktop regression — desktop toolbar unchanged
