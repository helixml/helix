# Requirements: Notifications Read/Dismiss and Grouping

## Problem

Two issues with the current notification panel:

1. **Read state triggered too eagerly.** Opening the notification panel auto-marks all notifications as "read" (acknowledged). The user wants a notification to only turn gray/dimmed when they explicitly click on it.

2. **Duplicate notifications for the same event.** When an agent finishes, two notifications appear close together: "spec ready" (`specs_pushed`) and "agent finished work" (`agent_interaction_completed`) for the same task. These should be grouped into one notification.

## Definitions

| State | Visual | Trigger |
|-------|--------|---------|
| **Unread** | Bold title, full opacity | Default when created |
| **Read** | Normal weight, 65% opacity | User **clicks** the notification row (navigates) |
| **Dismissed** | Removed from list | User clicks X or "Dismiss all" |

## User Stories

**US-1:** As a user, opening the notification panel should not change any notification's state — I want to be able to glance at the list without marking anything as read.

**US-2:** As a user, a notification should only become "read" (dimmed) when I explicitly click on it to navigate.

**US-3:** As a user, when a "spec ready" and "agent finished work" notification arrive for the same task within a minute of each other, I want to see them as a single grouped notification — not two separate items.

## Acceptance Criteria

- [ ] Opening the panel does NOT call `acknowledge()` — no auto-read on drawer open
- [ ] Clicking a notification row calls `acknowledge()` for that event (and navigates)
- [ ] X button and "Dismiss all" continue to work exactly as before
- [ ] `specs_pushed` + `agent_interaction_completed` events with the same `spec_task_id` and `created_at` within 60 seconds of each other are rendered as a single grouped notification item
- [ ] A grouped notification navigates to the spec review page (same behavior as `specs_pushed`)
- [ ] A grouped notification is "read" when clicked and "dismissed" when its X is clicked — both underlying events are acknowledged/dismissed together
- [ ] If only one of the two event types is present (no pair), it continues to display individually as before
