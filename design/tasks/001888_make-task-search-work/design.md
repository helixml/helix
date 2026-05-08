# Design: Mobile Task Search

## Current Architecture

### Mobile Kanban Layout
- **Breakpoint**: `useMediaQuery(theme.breakpoints.down("md"))` — below 960px
- **Layout**: Single column displayed at a time + `MobileColumnSidebar` (24px right edge strip)
- **Bottom nav**: `SpecTasksMobileBottomNav` — fixed at bottom with "New Task", "Chat", and overflow menu buttons
- **Toolbar**: Entire `searchFilter` + label/assignee toolbar is hidden with `display: { xs: "none", md: "flex" }` (line 1421 of `SpecTaskKanbanBoard.tsx`)

### Search Mechanics (already working)
- `searchFilter` state lives in `SpecTaskKanbanBoard` (line 659)
- `filterTasks()` uses `matchesAllTokens()` from `utils/searchUtils.ts` — splits query on whitespace, AND-matches against task name, description, and implementation_plan
- `filteredTasks` memo (line 868) applies search + label + assignee filters before distributing tasks to columns
- `searchFilter` is passed as prop to each `DroppableColumn` — the "No matching tasks" empty state already handles it

## Approach: Expandable Search Bar Above Kanban (Mobile-Only)

Add a compact, mobile-only search row that sits between the page header and the kanban board area. This avoids modifying the bottom nav (which has different responsibilities) and keeps search contextually close to the content being filtered.

### Why This Approach
- **Simple**: No new components needed — just show the existing `TextField` on mobile with adjusted styling
- **Discoverable**: Always visible, no hidden gestures
- **Non-intrusive**: Compact single row, doesn't steal significant screen space
- **Consistent**: Uses the same `searchFilter` state and `matchesAllTokens` filtering

### Implementation

**Change the toolbar display logic** in `SpecTaskKanbanBoard.tsx`:

Currently (line 1418-1428):
```tsx
<Box sx={{
  display: { xs: "none", md: "flex" },  // ← hidden on mobile
  ...
}}>
  {/* Title, New Task button, search, label filter, assignee filter */}
</Box>
```

Split into two sections:
1. **Desktop toolbar** — unchanged, stays `display: { xs: "none", md: "flex" }`
2. **Mobile search bar** — new `Box` with `display: { xs: "flex", md: "none" }` containing just the search field

The mobile search bar should:
- Show a full-width search `TextField` with `SearchIcon` start adornment and `ClearIcon` end adornment
- Be compact: height ~40px, small padding
- Sit just above the kanban board area (inside the same flex container)
- Use the same `searchFilter` / `setSearchFilter` state that already drives filtering

### Mobile Column Sidebar Task Counts

The `MobileColumnSidebar` currently receives `columns` which contain the already-filtered tasks (line 1676-1679). So task counts in the sidebar will automatically reflect search results — no additional work needed. The `columns` memo at line 896 already uses `filteredTasks`.

### Label/Assignee Filters (Stretch Goal)

If implementing label/assignee filters on mobile:
- Add a filter icon button next to the search field
- On tap, show a bottom sheet (`SwipeableDrawer` from MUI with `anchor="bottom"`) with the same `Autocomplete` controls for labels and assignees
- Show a badge on the filter icon when filters are active
- This is additive — the search bar works standalone first

## Key Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Add mobile-only search bar (new Box with `display: { xs: "flex", md: "none" }`) |

## Codebase Patterns Discovered

- Mobile detection: `useMediaQuery(theme.breakpoints.down("md"))` at 960px
- Responsive display toggle: `display: { xs: "none", md: "flex" }` / `display: { xs: "flex", md: "none" }`
- Search utility: `matchesAllTokens()` from `utils/searchUtils.ts` — always use this, never raw `.includes()`
- The `MobileColumnSidebar` is absolutely positioned (right: 0, top: 0, bottom: 56px, width: 24px)
- Bottom nav is fixed at bottom, 56px tall, z-index 1100
- Column task counts derive from `filteredTasks` so they auto-update with search
