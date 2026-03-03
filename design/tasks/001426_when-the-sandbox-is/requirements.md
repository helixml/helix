# Requirements: Session State After Sandbox Restart

## Problem Statement

When the sandbox is restarted, sessions that were running get into a broken UX state. The UI appears to be constantly trying to connect, reconnecting, and timing out in an endless loop. The fix is simple: just set the session to the stopped state, and the existing UI will show a prominent "Start" button in the middle of the page.

## User Stories

### US-1: Clear Session State After Sandbox Restart
**As a** user with an active session  
**When** the sandbox restarts (maintenance, crash, etc.)  
**I want** my session to show as stopped  
**So that** I see the existing "Start" button UI instead of an endless connection loop

## Acceptance Criteria

1. **AC-1**: When sandbox disconnects, sessions should transition to stopped state
2. **AC-2**: The existing stopped-state UI (with prominent Start button) should display
3. **AC-3**: No endless "connecting... reconnecting... timeout" loop
4. **AC-4**: The existing reconciler should automatically restart sessions with `desired_state=running` when sandbox reconnects

## Current Behavior (Bug)

1. Session has `container_name` set in DB (from previous run)
2. Sandbox restarts, container no longer exists
3. Frontend polls session, sees `container_name` set but container is gone
4. UI shows connecting/reconnecting/timeout loop instead of stopped state

## Expected Behavior (Fix)

1. When sandbox disconnects, clear stale container metadata (`container_name`, `container_id`, `container_ip`)
2. Set `external_agent_status` to "stopped"
3. Keep `desired_state` unchanged (so reconciler can restart when sandbox returns)
4. Frontend sees stopped state → shows existing UI with Start button (already works correctly)