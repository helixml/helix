# Implementation Tasks

- [ ] Add mobile-only search bar in `SpecTaskKanbanBoard.tsx` — new `Box` with `display: { xs: "flex", md: "none" }` containing a full-width `TextField` with `SearchIcon`/`ClearIcon` adornments, wired to existing `searchFilter`/`setSearchFilter` state. Place it just before the kanban board `Box` (before line 1607).
- [ ] Style the mobile search bar: compact height (~40px), horizontal padding matching kanban content, bottom margin 1, no border-bottom (keep it lightweight)
- [ ] Test on mobile viewport: emulate 375px width (iPhone), verify search filters tasks in the visible column, persists across column switches, clear button works, and "No matching tasks" empty state appears correctly
- [ ] (Stretch) Add filter icon button next to search field that opens a MUI `SwipeableDrawer` (anchor="bottom") with label and assignee `Autocomplete` controls, with a badge indicating active filter count
