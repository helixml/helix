# Requirements: Notification Panel Count Fix

## Problem

The notification bell badge shows a count based on the **raw** (un-de-duplicated) API events,
but the panel itself shows de-duplicated groups. This creates a mismatch: the badge can show
"3 unread" while the panel only shows 1 notification item.

Additionally, the user asked: when clicking a grouped notification to acknowledge it, do all
underlying duplicate events get marked as acknowledged? **Answer: yes**, both the primary and
secondary events in a group are acknowledged (see grouped handler in `GlobalNotifications.tsx`
lines 579–581 — it calls `acknowledge(group.secondary.id)` then `handleNavigate(ev)` which
calls `acknowledge(group.primary.id)`). This is already correct behavior.

## User Stories

**US-1**: As a user, I want the notification badge count to reflect the number of notification
items I actually see in the panel (after de-duplication), not the raw count of underlying DB
records.

**US-2**: As a user, I want the unread badge count to show only de-duplicated items that haven't
been acknowledged, so the number I see matches what I need to act on.

## Acceptance Criteria

- AC-1: The badge `badgeContent` is derived from the de-duplicated groups (output of
  `deduplicateGroupsByTask(groupEvents(events))`), not from raw `query.data.length`.
- AC-2: A group counts as "unread" if any of its member events are unacknowledged.
- AC-3: `hasNew` reflects whether any de-duplicated group is unread.
- AC-4: `totalCount` shown in the gray badge (when all read) equals the number of de-duplicated
  groups, not the raw event count.
- AC-5: Acknowledging a grouped notification already marks both events — no change needed there.
