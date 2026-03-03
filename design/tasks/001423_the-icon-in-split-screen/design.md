# Design: Search/Filter Bar in Split Screen '+' Menu

## Summary

Add a search/filter text field to the task picker menu in `TaskPanel` (TabsView.tsx). The field auto-focuses on menu open and filters the task list in real-time.

## Architecture

### Component Changes

**File**: `helix/frontend/src/components/tasks/TabsView.tsx`

**Location**: Inside the `<Menu>` component rendered by `TaskPanel`, before the "Create New Task" option.

### State

Add one new state variable to `TaskPanel`:
```tsx
const [taskSearchQuery, setTaskSearchQuery] = useState("");
```

Reset on menu close:
```tsx
onClose={() => {
  setMenuAnchor(null);
  setTaskSearchQuery("");
}}
```

### Filtering Logic

```tsx
const filteredTasks = useMemo(() => {
  if (!taskSearchQuery.trim()) return unopenedTasks;
  const query = taskSearchQuery.toLowerCase();
  return unopenedTasks.filter(task => {
    const title = task.user_short_title || task.short_title || task.name || "";
    return title.toLowerCase().includes(query);
  });
}, [unopenedTasks, taskSearchQuery]);
```

### UI Component

Use MUI `TextField` with `SearchIcon` startAdornment (existing pattern from `Skills.tsx`):

```tsx
<Box sx={{ px: 2, py: 1 }}>
  <TextField
    size="small"
    fullWidth
    placeholder="Search tasks..."
    value={taskSearchQuery}
    onChange={(e) => setTaskSearchQuery(e.target.value)}
    autoFocus
    InputProps={{
      startAdornment: (
        <InputAdornment position="start">
          <SearchIcon sx={{ fontSize: 18 }} />
        </InputAdornment>
      ),
    }}
  />
</Box>
```

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| `autoFocus` on TextField | Matches Chrome's "search tabs" behavior where user can type immediately |
| Filter only by title | Keeps implementation simple; title is what users see in the list |
| Case-insensitive search | Standard UX expectation |
| Human Desktop always visible | It's a singleton, not a task—shouldn't be filtered out |
| Reset query on close | Fresh state each time matches user expectations |

## Existing Patterns

This follows the established pattern in `Skills.tsx` lines 1215-1235 for search bars in list views.