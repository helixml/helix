# Design: Compact Spark Lines & Task List in Kanban Cards

## Current State

Two components contribute to vertical height in task cards:

1. **UsagePulseChart** (`frontend/src/components/tasks/UsagePulseChart.tsx`)
   - Container height: 50px with `mt: 0.5, mb: 0.5` (4px + 4px)
   - Chart height: 50px with `margin: { top: 5, bottom: 5 }` internal padding
   - Total vertical footprint: ~58px

2. **TaskProgressDisplay** (inline in `frontend/src/components/tasks/TaskCard.tsx`, lines 256-445)
   - Wrapper: `mt: 1.5, mb: 0.5` (12px + 4px)
   - Progress header: `py: 0.75` (6px top + 6px bottom)
   - Task list container: `py: 0.5` (4px top + 4px bottom)
   - Each task item: `py: 0.5, gap: 0.75` (4px top + 4px bottom, 6px gap)
   - With 4 visible tasks, total: ~100-110px

## Changes

### 1. UsagePulseChart — reduce height from 50px to 30px

In `UsagePulseChart.tsx`:
- Container `height: 50` → `height: 30`
- Chart `height={50}` → `height={30}`
- Remove `mt: 0.5, mb: 0.5` margins (let the card content padding handle spacing)
- Reduce chart internal margin from `{ top: 5, bottom: 5 }` to `{ top: 2, bottom: 2 }`

Saves ~28px.

### 2. TaskProgressDisplay — tighten spacing

In the `TaskProgressDisplay` component within `TaskCard.tsx`:
- Wrapper `mt: 1.5` → `mt: 1` (saves 4px)
- Progress header `py: 0.75` → `py: 0.5` (saves 4px)
- Task list container `py: 0.5` → `py: 0.25` (saves 2px)
- Per-task item `py: 0.5` → `py: 0.25` (saves ~8px across 4 items)
- Per-task item `gap: 0.75` → `gap: 0.5` (saves ~4px across 4 items)
- Status icon container: `width/height: 16` → `14`, icon `fontSize: 14` → `12` (slightly smaller indicators)
- Spinner: `width/height: 14` → `12`

Saves ~22px total.

### 3. Card content spacing adjustments

In `TaskCard.tsx` card content area:
- Labels row `mb: 1` → `mb: 0.5` (saves 4px) when followed by chart/checklist

## Files Changed

- `frontend/src/components/tasks/UsagePulseChart.tsx` — height and margin reduction
- `frontend/src/components/tasks/TaskCard.tsx` — TaskProgressDisplay spacing adjustments

## Codebase Notes

- The project uses MUI's `sx` prop with the 8px spacing scale (`1` = 8px, `0.5` = 4px, `0.25` = 2px)
- `UsagePulseChart` uses `@mui/x-charts` LineChart component — the `height` prop and `margin` prop control its size
- TaskProgressDisplay shows a sliding window of tasks (1 before active, active, 2 after) with fade opacity — this logic is unchanged
- The component is memoized with `React.memo` — no performance concerns with spacing changes
