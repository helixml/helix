# Design: Default to Chat View on Mobile

## Problem

`SpecTaskDetailContent.tsx` defaults `currentView` to `"desktop"` for all screen sizes (line 304). On mobile, the desktop view is less useful — chat is what users interact with. The split-view logic already uses `useIsBigScreen({ breakpoint: "md" })` to distinguish mobile from desktop, but the initial view state doesn't account for screen size.

## Solution

Make `getInitialView()` (line 294) screen-size-aware: return `"chat"` on mobile, `"desktop"` on desktop — unless a URL `view` param overrides it.

### Key Decision: Where to check screen size

The `getInitialView` function runs during `useState` initialization, before hooks are available. Two options:

1. **Direct `window.matchMedia` call inside `getInitialView`** — Simple, no hook dependency issues. The MUI `md` breakpoint is 900px, so we match against `(max-width: 899.95px)`.
2. **Move default logic into the existing `useEffect`** — Would require changing `useState` initial value to something neutral, adding complexity.

**Choice: Option 1** — a single `window.matchMedia` check in `getInitialView` is the simplest change. This mirrors what `useIsBigScreen` does internally (MUI's `useMediaQuery` also calls `window.matchMedia`). The 900px threshold is already established as the mobile breakpoint in this component.

### Codebase Patterns Found

- `useIsBigScreen({ breakpoint: "md" })` is already used at line 153 for split-view switching
- `SpecTasksPage.tsx` line 86 uses `useMediaQuery(theme.breakpoints.down("md"))` for the same mobile detection
- The useEffect at line 530 already handles the mobile case correctly: `const newView = isBigScreen ? "desktop" : "chat"` — this aligns with our change

### Affected Code

- **File**: `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
- **Function**: `getInitialView()` (lines 294-305)
- **Change**: Add `window.matchMedia` check to return `"chat"` on small screens
