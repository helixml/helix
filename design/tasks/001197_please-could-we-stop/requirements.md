# Requirements: Stop Screenshot Requests for Stopped Sessions

## Problem Statement

When a spec task's session is stopped, the frontend continues to poll for screenshots, spamming the browser console with failed request warnings. This creates noise in developer tools and wastes network resources.

## User Stories

1. **As a developer**, I want the browser console to be clean of repetitive failed screenshot warnings, so I can focus on actual errors and debug information.

2. **As a user**, I want the frontend to stop making unnecessary API requests when the desktop is paused/stopped, so my browser uses less bandwidth and CPU.

## Acceptance Criteria

1. **No screenshot polling when session is stopped**: When `external_agent_status === 'stopped'` or the sandbox state is `'absent'`, screenshot polling must not run.

2. **No console spam**: The `[DesktopStreamViewer] Screenshot fetch failed` and `[DesktopStreamViewer] Screenshot fetch error` warnings should not appear repeatedly for stopped sessions.

3. **Resume works correctly**: When a session is resumed (started again), screenshot polling should restart normally.

4. **No regression in "keep mounted" behavior**: The stream mode's "keep mounted to prevent fullscreen exit" behavior should continue working - we just need to stop the polling, not unmount components.

## Out of Scope

- Changes to the API/backend behavior
- Changes to the video streaming (only screenshot polling is affected)
- UI/UX changes to the "paused" overlay