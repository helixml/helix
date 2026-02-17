# Design: Kanban Board Performance

## Architecture Overview

The Kanban board (`SpecTaskKanbanBoard.tsx`) renders columns with task cards (`TaskCard.tsx`). Each card can include live screenshots via `ExternalAgentDesktopViewer`.

## Current Problems

```
SpecTaskKanbanBoard
├── DroppableColumn (x5-6 columns)
│   └── TaskCard (N tasks, ALL rendered)
│       ├── useTaskProgress() - polls every 5s
│       ├── useAgentActivityCheck() - state tracking
│       └── LiveAgentScreenshot (if active)
│           └── ExternalAgentDesktopViewer
│               └── useSandboxState() - polls every 3s
│               └── ScreenshotViewer - polls every 1.7s
```

With 50 tasks across columns:
- 50 DOM nodes for cards always rendered
- Up to 50 concurrent polling intervals
- Screenshot fetching/decoding on main thread

## Solution: Virtualization + Conditional Polling

### 1. Virtualize Column Content

Use `react-window` for column scroll areas:

```tsx
// DroppableColumn with virtualization
import { VariableSizeList } from 'react-window';

<VariableSizeList
  height={columnHeight}
  itemCount={column.tasks.length}
  itemSize={(index) => CARD_HEIGHT}
  width={COLUMN_WIDTH}
>
  {({ index, style }) => (
    <div style={style}>
      <TaskCard task={column.tasks[index]} isVisible={true} />
    </div>
  )}
</VariableSizeList>
```

### 2. Memoize TaskCard

```tsx
const TaskCard = React.memo(function TaskCard({ task, ... }) {
  // existing code
}, (prevProps, nextProps) => {
  // Custom comparison - only re-render on meaningful changes
  return prevProps.task.id === nextProps.task.id &&
         prevProps.task.status === nextProps.task.status &&
         prevProps.task.updated_at === nextProps.task.updated_at;
});
```

### 3. Pause Polling for Off-Screen Cards

Add `isVisible` prop to TaskCard and conditionally enable polling:

```tsx
const { data: progressData } = useTaskProgress(task.id, {
  enabled: showProgress && isVisible,  // Only poll when visible
  refetchInterval: isVisible ? 5000 : false,
});
```

### 4. Throttle Screenshot Polling

When multiple cards are visible, stagger screenshot fetches:

```tsx
// In ScreenshotViewer - already has some optimization
// Add: when isStreaming context is true, slow to 10s (already exists!)
const effectiveInterval = isStreaming ? 10000 : refreshInterval;
```

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| `react-window` over `react-virtualized` | Smaller bundle, simpler API, sufficient for our needs |
| Keep DOM for visible + 2 buffer items | Smooth scroll without visible pop-in |
| Don't virtualize columns themselves | Only 5-6 columns, overhead not worth it |
| Keep existing polling intervals | Just disable when off-screen |

## Files to Modify

1. `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` - Add virtualization to DroppableColumn
2. `frontend/src/components/tasks/TaskCard.tsx` - Memoize, add `isVisible` prop
3. `frontend/package.json` - Add `react-window` dependency

## Testing

1. Create 100 test tasks via API
2. Measure scroll FPS with Chrome DevTools Performance tab
3. Verify network tab shows reduced polling when scrolled
4. Memory snapshot before/after scrolling to check for leaks