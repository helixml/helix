# Requirements: Red Dot Notification Indicator on Kanban Cards

## User Stories

**As a user viewing the Kanban board,**
I want to see a red dot on task cards that have unread agent-completion notifications,
so that I can quickly spot which tasks need my attention without opening the notification panel.

**As a user viewing the Kanban board,**
I want the red dot to disappear automatically when I see the card (it scrolls into view),
so that it behaves the same way as the notification dropdown (viewing = acknowledged).

## Acceptance Criteria

1. A red dot badge appears at the top of a Kanban card when there is an unread `AttentionEvent` linked to that task (i.e., `acknowledged_at` is null).
2. The red dot is visually distinct — a small solid red circle, positioned prominently at the top of the card (e.g., top-right corner overlay or top of the card header).
3. When the card becomes visible in the viewport (scrolled into view), the system automatically acknowledges the linked `AttentionEvent` via the API (`PUT /api/v1/attention-events/{id}` with `{acknowledge: true}`).
4. After acknowledgment, the red dot disappears without requiring any user click.
5. This behavior mirrors the notification dropdown: "viewing = acknowledged." No extra click is required.
6. If multiple unread events are linked to a single task, acknowledging on view applies to all of them.
7. The red dot is only shown for `event_type = "agent_interaction_completed"` events (same events that trigger notifications for agent completion).

## Out of Scope

- No changes to the notification dropdown itself.
- No per-column aggregate badge (only per-card).
- No changes to the existing orange attention dot logic (that remains as-is).
