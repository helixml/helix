# Requirements: Don't Download Sessions for Stopped Desktops

## Problem

The Kanban (specs) page downloads the full session object (`GET /api/v1/sessions/{id}`) for every task card every 3 seconds via `useSandboxState` in `ExternalAgentDesktopViewer.tsx`. This happens regardless of whether the desktop is stopped, absent, or has never been started. Session objects are very large, causing hundreds of MB of unnecessary data transfer just from viewing the page.

## Root Cause

`useSandboxState` polls the full session endpoint to check `config.external_agent_status`. It does this on a 3-second interval for every task that has a `planning_session_id` and isn't in `completed` phase or merged to main — even when the desktop is known to be stopped.

**File:** `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:85`

## User Stories

- As a user viewing the Kanban board, I should not be downloading hundreds of MB of data just by looking at the page.
- As a user with many spec tasks, the Kanban page should remain performant and lightweight regardless of how many tasks exist.

## Acceptance Criteria

1. When a task's desktop is in a stopped/absent state, `useSandboxState` stops polling (or never starts) until the user explicitly starts the desktop.
2. The Kanban page does not make repeated `GET /api/v1/sessions/{id}` calls for desktops that are not running.
3. Tasks that have active/running desktops continue to poll normally.
4. Newly started desktops resume polling without a page refresh.
