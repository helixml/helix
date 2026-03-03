# Requirements: Session State After Sandbox Restart

## Problem Statement

When the sandbox is restarted, sessions that were running get into a broken UX state. The UI appears to be constantly trying to connect, reconnecting, and timing out in an endless loop. Users don't realize the session has actually stopped and that clicking the restart button would fix it.

## User Stories

### US-1: Clear Session State After Sandbox Restart
**As a** user with an active session  
**When** the sandbox restarts (maintenance, crash, etc.)  
**I want** my session to show as "Stopped" with a clear "Restart" button  
**So that** I understand the session has stopped and know exactly what action to take

### US-2: No Confusing Connection Loop
**As a** user viewing a session after sandbox restart  
**When** the container no longer exists  
**I want** the UI to immediately show a stopped/paused state  
**So that** I'm not watching a spinner that looks like it's endlessly trying to reconnect

## Acceptance Criteria

1. **AC-1**: When sandbox disconnects, sessions should show "Stopped/Paused" UI immediately (not a connecting spinner)
2. **AC-2**: The "Restart Desktop" button should be clearly visible and prominent
3. **AC-3**: No endless "connecting... reconnecting... timeout" loop in the UI
4. **AC-4**: The existing reconciler should automatically restart sessions with `desired_state=running` when sandbox reconnects

## Current Behavior (Bug)

1. Session has `container_name` set in DB (from previous run)
2. Sandbox restarts, container no longer exists
3. Frontend polls session, sees `container_name` set but container is gone
4. UI shows connecting/reconnecting/timeout loop - appears broken
5. User doesn't realize clicking "Restart" would fix the issue
6. Clicking restart actually works, but UX doesn't make this obvious

## Expected Behavior (Fix)

1. When sandbox disconnects, clear stale container metadata (`container_name`, `container_id`, `container_ip`)
2. Set `external_agent_status` to "stopped" explicitly
3. Keep `desired_state` unchanged (so reconciler can restart when sandbox returns)
4. Frontend immediately shows clean "Paused" UI with prominent "Restart" button - no spinner/connection loop