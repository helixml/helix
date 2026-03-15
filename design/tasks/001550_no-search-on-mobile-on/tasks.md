# Implementation Tasks

- [ ] In `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`, add a mobile-only search bar `Box` (with `display: { xs: 'flex', md: 'none' }`) after the desktop header and before the error alert, wiring it to the existing `searchFilter` state and `setSearchFilter` setter
- [ ] Verify on mobile that typing in the search bar filters task cards across the visible kanban column
- [ ] Verify the clear (×) button resets the filter and shows all tasks
- [ ] Verify on desktop the original search field in the header still works (no regression)
