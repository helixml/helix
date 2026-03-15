# Design: Search on Mobile — Spec Task View

## Current State

In `SpecTaskKanbanBoard.tsx`, the kanban board header (which contains the "Search tasks..." `TextField`) is wrapped in a `Box` with `display: { xs: "none", md: "flex" }`. This hides search entirely on mobile.

The `searchFilter` state and `filterTasks` function already exist in `SpecTaskKanbanBoard` — the filter logic works, it just has no input on mobile.

## Approach: Compact Search Bar Above the Mobile Column

Add a dedicated mobile-only search row directly inside `SpecTaskKanbanBoard`, between the hidden desktop header and the kanban board body. It renders only on mobile (`display: { xs: "flex", md: "none" }`).

```
┌─────────────────────────────────────┐
│ 🔍 Search tasks...              [×] │  ← new mobile-only row
├────────────────────────────────────┤
│ Task card                      │ B │
│ Task card                      │ P │
│ Task card                      │ R │  ← existing mobile column view
│                                │ D │
│                                │PR │
│                                │ M │
└────────────────────────────────────┘
│ + New Task  💬 Chat  ···        │  ← existing bottom nav
```

This is the simplest approach because:
- Reuses the existing `searchFilter` state — no new state needed.
- Does not touch `SpecTasksMobileBottomNav` (bottom nav is already busy).
- No new components required — just a new `Box` with a `TextField` inside `SpecTaskKanbanBoard`.
- Consistent visual placement (same bar concept, just mobile-sized and full-width).

## Alternatives Considered

- **Search icon in bottom nav**: adds a 4th item to an already 3-item nav bar, changes the nav structure.
- **Search icon in topbar**: the topbar is managed by `SpecTasksPage`, not `SpecTaskKanbanBoard`; requires prop threading.
- **Floating search button (FAB)**: overlaps with content, more complex.

The compact inline bar above the column is the cleanest fit.

## File to Change

Only `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`.

Insert a new `Box` after the desktop header (line ~1370) and before the error alert / kanban board body:

```tsx
{/* Mobile search bar */}
<Box
  sx={{
    display: { xs: 'flex', md: 'none' },
    px: 1,
    py: 0.75,
    flexShrink: 0,
    borderBottom: '1px solid',
    borderColor: 'divider',
  }}
>
  <TextField
    size="small"
    fullWidth
    placeholder="Search tasks..."
    value={searchFilter}
    onChange={(e) => setSearchFilter(e.target.value)}
    InputProps={{
      startAdornment: (
        <InputAdornment position="start">
          <SearchIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
        </InputAdornment>
      ),
      endAdornment: searchFilter && (
        <InputAdornment position="end">
          <IconButton size="small" onClick={() => setSearchFilter('')} sx={{ padding: 0.25 }}>
            <ClearIcon sx={{ fontSize: 16 }} />
          </IconButton>
        </InputAdornment>
      ),
    }}
  />
</Box>
```

The existing `searchFilter` state is already passed to `DroppableColumn` in the mobile column view (line ~1453), so filtering will work automatically once the input is wired up.

## Codebase Notes

- Pattern `display: { xs: 'flex', md: 'none' }` (mobile-only) is the inverse of the existing `display: { xs: 'none', md: 'flex' }` (desktop-only) pattern used throughout this file and `SpecTasksPage.tsx`.
- `SearchIcon` and `ClearIcon` are already imported in `SpecTaskKanbanBoard.tsx` (lines 59 and ~61).
- `searchFilter` state is already defined at line ~624 and used for filtering at lines ~772-774.
