# Requirements: Pulsing green dot for running agents on Kanban cards

## Problem

The red notification dot in the corner of a task card on the Kanban board
indicates that the agent finished and needs attention. However, this dot can
remain stale: if the user starts the agent again (e.g. via a follow-up prompt or
"Continue") without first dismissing the old attention event, the red dot stays
visible while the agent is once again actively working.

When the user is scanning the Kanban board, the red dot draws their attention
and invites a click — but if the agent is actually still running, the click is
wasted (the user just sees the agent churning). The user wants to *skip* tasks
where the agent is currently working without having to open them.

## User Story

> As a user reviewing the Kanban board, I want to see at a glance which tasks
> have an agent actively working on them right now, so I can skip them and
> focus only on tasks that genuinely need my attention.

## Acceptance Criteria

1. **Agent currently working** — When a task's `agent_work_state === "working"`,
   the card shows a **pulsing green dot** in the top-right corner (the same
   position as today's red dot).
   - Tooltip: "Agent is running"
   - The pulse is subtle (gentle opacity/scale animation, not distracting on a
     board with many cards).

2. **Green dot takes precedence over red dot** — If the agent is currently
   working AND there is a stale unacknowledged attention event, only the green
   dot is shown. The red dot is hidden until the agent stops.

3. **Red dot still works when agent is idle** — When the agent is not working
   (`agent_work_state` is `"idle"`, `"done"`, or absent) and there are unread
   attention events, the existing red dot continues to behave exactly as it
   does today.

4. **Click behaviour unchanged** — Clicking the card still opens the task
   detail and acknowledges any pending attention events. The dot itself is
   not separately clickable.

5. **Polling-driven** — The dot state updates automatically as the existing
   task list polling (~3s) refreshes `agent_work_state`. No new endpoints or
   websocket subscriptions are required.

6. **Both Kanban surfaces** — The change applies to `TaskCard.tsx` which is
   the card used by the main `SpecTaskKanbanBoard`. `AgentKanbanBoard` uses a
   different `SortableTaskCard`; out of scope for this task unless it shares
   the same indicator pattern (see design.md).

## Out of Scope

- Changing how attention events are created, persisted, or auto-cleared on the
  backend. (A separate design question — the user did not ask for it here.)
- Changing the "phase" colored dot inside the status row at the top-left of
  the card (planning=amber, implementation=green, etc.) — that is a different
  indicator.
- Sandbox lifecycle indicators ("Starting…", "Sandbox stopped") — already
  handled by the implementation label and unaffected here.
