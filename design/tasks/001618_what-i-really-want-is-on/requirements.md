# Requirements: Navigation History in Notification Panel

## Context

The existing global notification system (`GlobalNotifications.tsx`) shows a sliding panel (opened via the bell icon in the top-right AppBar) listing "attention events" — things that need the user's action. The user wants two enhancements to this panel:

1. **Deduplicate alerts by page** — show only the most recent attention event per spec task/page rather than multiple events for the same task.
2. **Recent pages section** — below the alerts, show a list of pages the user has visited recently that are *not* already represented by an active alert, so the user can quickly jump back to them.

## User Stories

**US-1:** As a user, I want the alerts section to show at most one notification per spec task so I'm not overwhelmed when a task has generated multiple events (e.g., both `agent_interaction_completed` and `specs_pushed` for the same task).

**US-2:** As a user, I want a "Recently visited" section below the alerts in the notification panel so I can jump back to spec task detail pages and design review pages I've looked at recently — even if they don't have an active alert.

**US-3:** As a user, I want the recently visited list to be deduplicated (one entry per page, most recent visit shown at the top) so it stays clean.

**US-4:** As a user, pages that already have an active alert should not appear in the "Recently visited" section to avoid duplicating the same page in two places.

## Acceptance Criteria

- [ ] The alerts section groups/deduplicates by `spec_task_id` (or URL if no task ID), keeping only the most recent event per page.
- [ ] The "Recently visited" section appears below the alerts in the same sliding panel, with a clear section heading (e.g., "Recently visited").
- [ ] Recently visited entries come from client-side navigation history tracked in `localStorage`.
- [ ] Pages already shown in the alerts section are excluded from "Recently visited".
- [ ] Each recently visited entry shows a readable title and is clickable to navigate.
- [ ] The recently visited list shows at most 10 entries (since alerts already take up space).
- [ ] The recently visited list is hidden when empty.
- [ ] History persists across page refreshes via `localStorage`.
