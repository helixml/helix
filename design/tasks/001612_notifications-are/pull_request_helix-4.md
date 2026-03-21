# Fix notification read state and group related events

## Summary

Notifications were being marked as "read" (dimmed) automatically just by opening the panel. They should only go gray when explicitly clicked. Also, "spec ready" and "agent finished" events for the same task that arrive within 60 seconds of each other are now merged into a single notification item.

## Changes

- **Remove auto-acknowledge on panel open** — `handleDrawerOpen` no longer loops through events calling `acknowledge()`. Opening the panel is now a no-op for notification state.
- **Acknowledge on explicit click** — `handleNavigate` now calls `acknowledge(event.id)` before navigating, so clicking a notification is what marks it as read.
- **Group paired events** — New `groupEvents()` function pairs `specs_pushed` + `agent_interaction_completed` events for the same `spec_task_id` within 60 seconds into one "📋 Spec ready & agent finished" notification. Clicking or dismissing the grouped item acts on both underlying events.
- **No backend changes** — Pure frontend change; the `acknowledged_at` field, API endpoints, and database schema are all untouched.
