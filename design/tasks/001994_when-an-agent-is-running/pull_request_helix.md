# Show pulsing green dot on Kanban cards while agent is running

## Summary

When an agent is currently working on a task, the card now shows a pulsing
**green** dot in the top-right corner instead of the existing static **red**
"agent finished" dot. This makes it easy to skip in-progress tasks while
scanning the Kanban board, even when a stale unacknowledged attention event
from a previous run is still on the card.

## Behaviour

| State | Indicator |
|---|---|
| `agent_work_state === "working"` | Pulsing green dot, tooltip "Agent is running" |
| Unacknowledged attention event, agent not working | Static red dot (unchanged) |
| Otherwise | No dot |

The green dot takes precedence over the red dot when both signals are
present. The red dot reappears once the agent goes idle again — attention
events are not auto-acknowledged.

## Changes

- `frontend/src/components/tasks/TaskCard.tsx`
  - New `pulseDot` keyframes (opacity 1.0 → 0.4 → 1.0 over 1.5s, ease-in-out)
  - New `isAgentWorking = task.agent_work_state === "working"` derivation
  - Top-right dot now renders green-pulsing when working, else red-static when
    `hasUnreadNotification`, else nothing

## Why this is small

`agent_work_state` is already populated on every task in the existing
`useSpecTasks` poll (~3.1s refetch), so no new endpoint, websocket, or schema
change was needed. The diff is ~20 lines in one file.

## Screenshots

![Three dot states (green pulsing / red static / none)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001994_when-an-agent-is-running/screenshots/01-dot-states-preview.png)

Note: this screenshot is from a standalone HTML preview, not the inner Helix —
see "Testing" below.

## Testing

- `cd frontend && yarn build` — clean.
- **Inner Helix end-to-end test was not possible in the implementation
  environment** (Docker stack still building when the change landed). A
  reviewer should spot-check in a running inner Helix:
  1. Start a task → green dot pulses while agent is working.
  2. Let agent finish → red dot appears.
  3. Send a follow-up prompt without dismissing → red dot is replaced by green
     pulsing dot, and reverts to red when the agent stops.
