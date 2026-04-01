# Requirements: Desktop Auto-Shutdown After Idle Timeout

## User Story

As a platform operator, I want desktops to automatically shut down when they've been idle for one hour, so that compute resources aren't wasted on abandoned sessions.

## Definitions

- **Desktop**: An external agent (Zed instance) identified by `ExternalAgentID` in `SessionMetadata`, running in a container managed by the `HydraExecutor`.
- **Session**: A `Session` record with `ExternalAgentStatus = "running"` in its `SessionMetadata`.
- **Interaction**: A record in the `interactions` table linked to a session via `session_id`.
- **Last interaction time**: The maximum of `interactions.updated` across all sessions associated with a given desktop (grouped by `external_agent_id`).

## Acceptance Criteria

1. A background process checks for idle desktops periodically (every 5 minutes).
2. A desktop is considered idle if no interaction in any of its associated sessions has been created or updated in the last 60 minutes.
3. When a desktop is idle, `StopDesktop()` is called for the corresponding session, and `ExternalAgentStatus` is updated to `"terminated_idle"`.
4. Desktops that are already stopped or not in "running" status are skipped.
5. Each desktop stop is attempted independently — a failure stopping one desktop does not prevent others from being stopped.
6. Shutdown activity is logged at INFO level with the desktop's session ID, external agent ID, and idle duration.
