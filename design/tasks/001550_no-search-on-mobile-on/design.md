# Design: No Search on Mobile — Spec Task View

## Architecture

### Where Search Lives

There are two places in the spec task view where search exists:

1. **Kanban board header** (`SpecTaskKanbanBoard.tsx` ~line 1338)
   — Already hidden on mobile via `display: { xs: "none", md: "flex" }` on the parent `Box`.
   — **No change needed here.**

2. **BacklogFilterBar** (`BacklogFilterBar.tsx`, used inside `BacklogTableView.tsx`)
   — Accessible on mobile: user taps the backlog column header → `backlogExpanded = true` → `BacklogTableView` renders → `BacklogFilterBar` renders with a visible search field.
   — **This is the change needed.**

### File Locations

| File | Change |
|------|--------|
| `frontend/src/components/tasks/BacklogFilterBar.tsx` | Hide the filter bar on mobile using `display: { xs: "none", md: "flex" }` on the root `Box` |

### Decision

Hide the entire `BacklogFilterBar` on mobile (not just the search field). Rationale:
- The priority filter is also a complex UI element that doesn't fit well on small screens.
- The table itself still shows all tasks unfiltered — mobile users can scroll.
- Consistent with the approach used in the kanban board header.

### Pattern Used

This project uses the MUI responsive `display` shorthand:
```jsx
<Box sx={{ display: { xs: "none", md: "flex" }, ... }}>
```
This is the standard pattern throughout the codebase for hiding elements on mobile.

## Codebase Notes

- `isMobile` is checked via `useMediaQuery(theme.breakpoints.down("md"))` in `SpecTaskKanbanBoard.tsx` (line 598). The `BacklogFilterBar` does not currently import `useMediaQuery`.
- The MUI `sx` responsive `display` prop is simpler and avoids adding a hook dependency to `BacklogFilterBar`.
- `BacklogTableView` is only rendered inside `SpecTaskKanbanBoard` (when `backlogExpanded === true`).
