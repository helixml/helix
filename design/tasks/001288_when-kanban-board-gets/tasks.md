# Implementation Tasks

## Phase 1: Memoization (Quick Wins)

- [ ] Wrap `TaskCard` component with `React.memo()` and custom comparison function
- [ ] Add `isVisible` prop to `TaskCard` interface
- [ ] Disable `useTaskProgress` polling when `isVisible=false`
- [ ] Disable `useSandboxState` polling in `ExternalAgentDesktopViewer` when parent not visible

## Phase 2: Virtualization

- [ ] Add `react-window` to `frontend/package.json`
- [ ] Replace map loop in `DroppableColumn` with `VariableSizeList`
- [ ] Calculate dynamic card heights (cards with screenshots are taller)
- [ ] Pass `isVisible={true}` to virtualized cards (react-window only renders visible)
- [ ] Add 2-item overscan buffer for smooth scrolling

## Phase 3: Screenshot Optimization

- [ ] Add visibility check before screenshot fetch in `ScreenshotViewer`
- [ ] Skip screenshot polling entirely for cards in "backlog" and "completed" columns
- [ ] Reduce screenshot polling from 1.7s to 3s for non-focused cards

## Phase 4: Usage Data Quantization (Server-Side)

- [ ] In `usage_handlers.go`: Auto-select aggregation level based on time range (5min → hourly → daily)
- [ ] Cap max data points to ~50 regardless of time range
- [ ] In `store_usage_metrics.go`: Add early-exit to `fillInMissing*` functions when limit reached
- [ ] Consider removing empty data points from response (don't fill gaps with zeros)

## Phase 5: Testing & Verification

- [ ] Create test script to generate 100 tasks via API
- [ ] Measure scroll FPS before/after with Chrome DevTools
- [ ] Verify network requests decrease when scrolling (off-screen cards stop polling)
- [ ] Check memory usage stays stable over 5 minutes of use
- [ ] Verify usage endpoint returns <50 data points for week-old tasks