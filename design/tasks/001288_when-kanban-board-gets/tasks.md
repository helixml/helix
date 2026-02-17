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

## Phase 4: Testing & Verification

- [ ] Create test script to generate 100 tasks via API
- [ ] Measure scroll FPS before/after with Chrome DevTools
- [ ] Verify network requests decrease when scrolling (off-screen cards stop polling)
- [ ] Check memory usage stays stable over 5 minutes of use