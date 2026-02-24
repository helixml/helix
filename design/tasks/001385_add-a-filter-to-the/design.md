# Design: Kanban Board Search Filter

## Architecture

This is a **frontend-only** change. The filter operates on data already fetched by the existing `useSpecTasks` hook.

## Component Changes

### 1. SpecTasksPage.tsx

Add search state and pass it to `SpecTaskKanbanBoard`:

```tsx
const [searchFilter, setSearchFilter] = useState('');

// In topbar, add search input before view mode toggle
<TextField
  size="small"
  placeholder="Search tasks..."
  value={searchFilter}
  onChange={(e) => setSearchFilter(e.target.value)}
  InputProps={{
    startAdornment: <SearchIcon />,
    endAdornment: searchFilter && (
      <IconButton size="small" onClick={() => setSearchFilter('')}>
        <ClearIcon />
      </IconButton>
    ),
  }}
/>

// Pass to kanban board
<SpecTaskKanbanBoard
  ...existingProps
  searchFilter={searchFilter}
/>
```

### 2. SpecTaskKanbanBoard.tsx

Add `searchFilter` prop and filter tasks before column assignment:

```tsx
interface SpecTaskKanbanBoardProps {
  // ...existing props
  searchFilter?: string;
}

// Filter function
const filterTasks = (tasks: SpecTaskWithExtras[], filter: string): SpecTaskWithExtras[] => {
  if (!filter.trim()) return tasks;
  const lowerFilter = filter.toLowerCase();
  return tasks.filter(task => 
    task.name?.toLowerCase().includes(lowerFilter) ||
    task.description?.toLowerCase().includes(lowerFilter) ||
    task.implementation_plan?.toLowerCase().includes(lowerFilter)
  );
};

// Apply filter before column useMemo
const filteredTasks = useMemo(
  () => filterTasks(tasks, searchFilter || ''),
  [tasks, searchFilter]
);

// Update columns useMemo to use filteredTasks instead of tasks
const columns: KanbanColumn[] = useMemo(() => {
  // Replace all `tasks.filter(...)` with `filteredTasks.filter(...)`
}, [filteredTasks, ...otherDeps]);
```

### 3. DroppableColumn (in SpecTaskKanbanBoard.tsx)

Add empty state message when filter produces no results:

```tsx
{column.tasks.length === 0 && searchFilter && (
  <Typography sx={{ color: 'text.secondary', fontSize: '0.75rem', textAlign: 'center', py: 2 }}>
    No matching tasks
  </Typography>
)}
```

## Data Flow

```
User types → searchFilter state (SpecTasksPage)
          → prop to SpecTaskKanbanBoard
          → filterTasks() applied
          → columns useMemo recalculates
          → UI re-renders with filtered tasks
```

## Existing Patterns Used

- **Search input style**: Follows `Skills.tsx` pattern with `TextField`, `SearchIcon`, and clear button
- **Filter logic**: Similar to `filteredSkills` in `Skills.tsx` - case-insensitive `includes()` check
- **State management**: Local `useState` in page component, same as existing `showArchived`, `showMetrics` toggles

## UI Placement

The search input goes in the topbar, positioned before the view mode toggle (kanban/workspace/audit buttons). This keeps it visible and accessible without cluttering the board itself.

## Performance Considerations

- Filter runs on every keystroke but operates on already-loaded array (typically <100 tasks)
- `useMemo` prevents unnecessary re-filtering when other state changes
- No debouncing needed given small data size

## Implementation Notes

### Files Modified
1. `frontend/src/pages/SpecTasksPage.tsx` - Added searchFilter state and TextField in topbar
2. `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` - Added filtering logic

### Key Decisions
- Search input placed before view mode toggle buttons in topbar for visibility
- Filter hidden on mobile (`display: { xs: "none", md: "flex" }`) to save space
- TextField width set to 200px to fit alongside other topbar elements
- Used existing MUI SearchIcon and ClearIcon from @mui/icons-material

### Gotchas
- The `columns` useMemo depends on `filteredTasks` (not `tasks`) to apply the filter
- DroppableColumn needs `searchFilter` prop to show appropriate empty state message ("No matching tasks" vs "No tasks")
- Filter state lives in SpecTasksPage and is passed down as prop - this means it persists when switching view modes (kanban/workspace) within the same session