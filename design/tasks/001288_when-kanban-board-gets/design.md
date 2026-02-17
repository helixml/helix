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
│       ├── UsagePulseChart - polls every 60s, returns HUGE payloads
│       └── LiveAgentScreenshot (if active)
│           └── ExternalAgentDesktopViewer
│               └── useSandboxState() - polls every 3s
│               └── ScreenshotViewer - polls every 1.7s
```

With 50 tasks across columns:
- 50 DOM nodes for cards always rendered
- Up to 50 concurrent polling intervals
- Screenshot fetching/decoding on main thread

### Usage Data Explosion (Major Issue)

`UsagePulseChart` calls `/api/v1/spec-tasks/{taskId}/usage` with `aggregationLevel: '5min'`.

**Problem**: The `from` time defaults to `specTask.PlanningStartedAt` or `specTask.CreatedAt`:

```go
// usage_handlers.go L150-160
switch {
case specTask.PlanningStartedAt != nil:
    from = *specTask.PlanningStartedAt
case specTask.StartedAt != nil:
    from = *specTask.StartedAt
default:
    from = specTask.CreatedAt
}
```

Then `fillInMissing5Minutes()` fills EVERY 5-minute bucket with a data point:

| Task Age | Data Points | Approx JSON Size |
|----------|-------------|------------------|
| 1 day    | 288         | ~30 KB           |
| 7 days   | 2,016       | ~200 KB          |
| 30 days  | 8,640       | ~860 KB          |

With 20 long-running tasks visible, each polling every 60s = **17 MB/minute** of usage data.

## Solution: Virtualization + Conditional Polling + Server-Side Quantization

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

### 5. Server-Side Usage Data Quantization (Critical)

Limit the usage endpoint to return max ~50 data points for the chart:

```go
// usage_handlers.go - add after determining from/to
maxPoints := 50
duration := to.Sub(from)
pointsAt5Min := int(duration.Minutes() / 5)

// Auto-select aggregation level based on time range
if pointsAt5Min > maxPoints {
    if int(duration.Hours()) <= maxPoints {
        aggregationLevel = AggregationLevelHourly
    } else {
        aggregationLevel = AggregationLevelDaily
    }
}
```

Also add a `LIMIT` clause to `fillInMissing*` functions to cap output.

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
4. `api/pkg/server/usage_handlers.go` - Auto-select aggregation level, cap max points
5. `api/pkg/store/store_usage_metrics.go` - Add limit to fillInMissing* functions

## Testing (Self-Verifiable via Chrome MCP)

Use Chrome MCP tools to measure before/after:

1. **Baseline measurement**: Navigate to Kanban board with 20+ tasks
   - `performance_start_trace` with reload
   - `list_network_requests` to count usage endpoint calls and payload sizes
   - `take_screenshot` to document current state

2. **After Phase 1 (usage quantization)**:
   - `list_network_requests` - verify usage payloads dropped from ~200KB to ~5KB
   - Compare total network transfer over 60s

3. **After Phase 2-4 (frontend optimizations)**:
   - `performance_start_trace` during scroll
   - Check for jank in trace results
   - `list_network_requests` - verify off-screen cards stop polling

4. **Memory leak check**:
   - `evaluate_script` to get `performance.memory.usedJSHeapSize`
   - Scroll up/down for 2 minutes
   - Compare heap size - should be stable