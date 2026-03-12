# Requirements: Split Screen View — Buttons Change Wrong Panel

## Problem Statement

In the split screen (workspace) view, when a user has multiple task panels open side-by-side, clicking buttons inside one panel's detail view causes the **other** panel(s) to change state instead of (or in addition to) the panel that was clicked.

## Root Cause

`SpecTaskDetailContent` stores its internal view mode (desktop/chat/changes/details) by syncing with the **global URL router params** via `router.mergeParams({ view: newView })`. When multiple `SpecTaskDetailContent` instances exist in split screen (rendered by `TabsView`), they all:

1. **Write** to the same `router.params.view` when a toggle button is clicked
2. **Read** from the same `router.params.view` via a `useEffect` that syncs `currentView` state
3. Compete over the global `streaming.setCurrentSessionId()` — only one session can be "current" at a time

So clicking "Desktop" in panel A sets `router.params.view = "desktop"`, which triggers the `useEffect` in panel B to also switch to desktop view.

Similarly, the auto-switch `useEffect` (line ~503) that changes view based on `activeSessionId` fires `router.mergeParams` which cascades to all mounted instances.

## User Stories

1. **As a user** with two tasks open side-by-side in split screen, I want to click the "Desktop" toggle in the left panel without the right panel also switching to desktop view.

2. **As a user** viewing task details in one panel and a desktop stream in another, I want each panel to independently maintain its own view state (chat/desktop/changes/details).

3. **As a user** clicking "Start Planning", "Review Spec", or any action button inside one panel, I want only that panel's state to change — not any other open panel.

4. **As a user** with multiple active sessions in split panels, I want each panel's chat to independently subscribe to its own session's WebSocket stream without panels fighting over a single global session ID.

## Acceptance Criteria

- [ ] Clicking view toggle buttons (Desktop/Chat/File Diff/Details) in one split panel does NOT change the view in any other panel
- [ ] The auto-switch logic (details→desktop when session starts, →details when session stops) only affects the panel that owns that task
- [ ] Action buttons (Start Planning, Review Spec, Reject, Open PR, etc.) only affect the panel they belong to
- [ ] WebSocket streaming works correctly when multiple panels have active sessions — each panel shows its own session's chat
- [ ] Single-panel mode (no split) continues to work exactly as before
- [ ] The standalone `SpecTaskDetailPage` (non-workspace route) continues to work as before
- [ ] No regressions in mobile/small-screen layout where only one view shows at a time

## Affected Components

| File | Role |
|------|------|
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Core detail view — contains the buggy `router.mergeParams` and `streaming.setCurrentSessionId` calls |
| `frontend/src/components/tasks/TabsView.tsx` | Split screen container — renders multiple `SpecTaskDetailContent` instances |
| `frontend/src/contexts/streaming.tsx` | Global streaming context — `setCurrentSessionId` is single-valued |
| `frontend/src/contexts/router.tsx` | Router context — `mergeParams` writes to shared URL |
| `frontend/src/components/tasks/SpecTaskActionButtons.tsx` | Action buttons — these delegate to parent callbacks, not directly buggy but affected |