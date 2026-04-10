# Requirements: "Keep Alive" Toggle for Spectasks

## Problem

Desktop spectasks auto-shutdown after 1 hour of idle time (no interactions). This destroys browser login sessions (e.g. LinkedIn, Gmail) that users need to persist across idle periods. Users currently have no way to prevent this.

## User Stories

### US-1: Prevent auto-sleep
**As a** user with an active desktop session (e.g. logged into LinkedIn),
**I want to** toggle "Keep Alive" on a spectask,
**so that** the container stays running even when I'm not actively interacting with it.

### US-2: See keep-alive status at a glance
**As a** user viewing a spectask detail page,
**I want to** see whether keep-alive is enabled,
**so that** I know if my session is protected from auto-shutdown.

### US-3: Disable keep-alive
**As a** user who no longer needs a persistent session,
**I want to** toggle keep-alive off,
**so that** the container resumes normal idle-shutdown behavior to save resources.

## Acceptance Criteria

1. A toggle button appears in the spectask detail page header toolbar (top-right area, near the existing start/stop/restart buttons).
2. The toggle is only visible/enabled when the desktop is running.
3. When enabled, the idle checker skips this spectask's container — it will not be auto-stopped.
4. When disabled (default), normal idle-shutdown behavior applies.
5. The setting persists across page reloads (stored in database on the SpecTask model).
6. The setting does NOT prevent manual stop — the user can still click Stop at any time.
7. The toggle state is visually obvious (icon change, color, or tooltip update).

## Naming Decision

**"Keep Alive"** is the recommended term:
- Clear and intuitive — users understand "keep this alive"
- Matches networking terminology (keepalive packets)
- Alternatives considered: "Lock" (ambiguous — could mean edit-locking), "Always On" (implies it auto-starts), "Pin" (too vague), "Don't Sleep" (informal)
