# Fix flash of stopped desktop state when clicking Start Planning

## Summary

When a task already had a `planning_session_id` from a previous run (in stopped state), clicking "Start Planning" would briefly show "Desktop Paused" with a start button for 1–2 seconds before correctly transitioning to "Starting Desktop". This looked broken to users.

**Root cause:** `planning_session_id` pointed to the old stopped session while the backend asynchronously created a new one. The desktop viewer polled that stale session, saw `status=stopped`, and rendered the paused state.

**Fix:** During `queued_spec_generation` task status, suppress the paused state via `effectiveIsDesktopPaused` and pass `initialSandboxState="starting"` to both `ExternalAgentDesktopViewer` instances so the UI shows "Starting Desktop..." immediately.

## Changes

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - Added `isQueuedForPlanning` and `effectiveIsDesktopPaused` derived state
  - Replaced `isDesktopPaused` with `effectiveIsDesktopPaused` in both toolbar "Start desktop" button conditions (desktop + mobile)
  - Added `initialSandboxState={isQueuedForPlanning ? "starting" : undefined}` to both `ExternalAgentDesktopViewer` instances
