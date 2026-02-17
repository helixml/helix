# Implementation Tasks

## Phase 1: Usage Data Quantization (Server-Side) - HIGHEST PRIORITY

- [x] In `usage_handlers.go`: Auto-select aggregation level based on time range (5min → hourly → daily)
- [x] Cap max data points to ~50 regardless of time range
- [x] Cap min data points to ~20 (e.g., 2-day-old task should show 20 hourly points, not 2 daily points)
- [x] ~~In `store_usage_metrics.go`: Add early-exit to `fillInMissing*` functions~~ (not needed - aggregation level selection handles this)
- [x] ~~Consider removing empty data points~~ (keep zeros so chart shows "no activity" periods accurately)

## Phase 2: Memoization (Quick Wins)

- [x] Wrap `TaskCard` component with `React.memo()` and custom comparison function
- [x] Add `isVisible` prop to `TaskCard` interface
- [x] Disable `useTaskProgress` polling when `isVisible=false`
- [x] Disable `useSandboxState` polling in `ExternalAgentDesktopViewer` when parent not visible

## Phase 3: Virtualization

- [ ] Add `react-window` to `frontend/package.json`
- [ ] Replace map loop in `DroppableColumn` with `VariableSizeList`
- [ ] Calculate dynamic card heights (cards with screenshots are taller)
- [ ] Pass `isVisible={true}` to virtualized cards (react-window only renders visible)
- [ ] Add 2-item overscan buffer for smooth scrolling

## Phase 4: Screenshot Optimization

- [ ] Add visibility check before screenshot fetch in `ScreenshotViewer`
- [ ] Skip screenshot polling entirely for cards in "backlog" and "completed" columns
- [ ] Reduce screenshot polling from 1.7s to 3s for non-focused cards

## Phase 5: Testing & Verification (via Chrome MCP)

- [ ] Baseline: `performance_start_trace` + `list_network_requests` on Kanban with 20+ tasks
- [ ] After Phase 1: Verify usage endpoint payloads dropped from ~200KB to ~5KB via `list_network_requests`
- [ ] After Phase 2-4: `performance_start_trace` during scroll to check for jank
- [ ] Verify off-screen cards stop polling via `list_network_requests` filtering
- [ ] Memory leak check: `evaluate_script` for `performance.memory.usedJSHeapSize` before/after 2 min scroll