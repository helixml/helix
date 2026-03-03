# Requirements: Session State After Sandbox Restart

## Problem Statement

When the sandbox is restarted, sessions that were running get into a state where they appear to be "broken" rather than cleanly "stopped". This creates a confusing UX where users don't know if something went wrong or if they can simply restart the session.

## User Stories

### US-1: Clear Session State After Sandbox Restart
**As a** user with an active session  
**When** the sandbox restarts (maintenance, crash, etc.)  
**I want** my session to show as "Stopped" not "Broken"  
**So that** I know I can safely resume it without worrying about data loss

### US-2: Easy Session Recovery
**As a** user with a stopped session  
**When** the sandbox comes back online  
**I want** to be able to resume my session with one click  
**So that** I can continue my work without manual intervention

## Acceptance Criteria

1. **AC-1**: When sandbox disconnects, sessions with `desired_state=running` should show "Stopped" in the UI (not spinner/error)
2. **AC-2**: The "Start Desktop" / "Resume" button should be visible and functional for stopped sessions
3. **AC-3**: The existing reconciler should automatically restart sessions with `desired_state=running` when sandbox reconnects
4. **AC-4**: Sessions should not show a loading/starting state indefinitely if the sandbox is offline

## Current Behavior (Bug)

1. Session has `container_name` set in DB (from previous run)
2. Sandbox restarts, container no longer exists
3. Frontend polls session, sees `container_name` set but `external_agent_status` shows "stopped"
4. Logic conflict: `hasContainer` is true (name exists) but status is "stopped"
5. UI may show incorrect state (spinner, or no clear "Resume" button)

## Expected Behavior (Fix)

1. When sandbox disconnects, clear stale container metadata (`container_name`, `container_id`, `container_ip`)
2. Set `external_agent_status` to "stopped" explicitly
3. Keep `desired_state` unchanged (so reconciler can restart when sandbox returns)
4. Frontend shows clean "Paused" UI with "Resume" button