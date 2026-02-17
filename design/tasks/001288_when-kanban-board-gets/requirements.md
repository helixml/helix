# Requirements: Kanban Board Performance

## Problem Statement

When the Kanban board has many tasks (20+), the UI becomes unusably slow. Scrolling lags, interactions feel delayed, and the browser may become unresponsive.

## User Stories

1. As a user with 50+ tasks, I want the Kanban board to scroll smoothly at 60fps
2. As a user, I want task cards to respond to clicks within 100ms
3. As a user, I want the board to load in under 2 seconds regardless of task count

## Root Causes Identified

1. **No virtualization** - All task cards render in DOM even when off-screen
2. **Excessive re-renders** - `TaskCard` component not memoized, re-renders on any parent state change
3. **Multiple polling intervals** - Each task has its own `refetchInterval` (progress: 5s, screenshots: 1.7s)
4. **Screenshot polling per card** - `LiveAgentScreenshot` polls every 1.7s per task in "planning" or "implementation" phase
5. **Heavy card content** - Cards include `ExternalAgentDesktopViewer` which loads screenshots

## Acceptance Criteria

- [ ] Board with 100 tasks scrolls at 60fps
- [ ] Initial render time < 2s for 100 tasks
- [ ] No screenshot polling for off-screen cards
- [ ] Memory usage stays stable (no leaks from polling)
- [ ] Task click response < 100ms