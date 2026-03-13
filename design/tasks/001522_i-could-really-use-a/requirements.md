# Requirements: SpectTask Human Attention Queue

## Problem

When managing multiple spectasks across projects, there's no consolidated view of what needs human attention right now. The existing bell notification shows pending reviews, but:
- It only covers review statuses, not agent-stopped/failed states
- It's a small popover, not a proper work queue
- No browser push notifications — you have to be looking at the app
- No Slack integration for "human needed" events
- No way to see the queue from every page (Kanban, detail, split screen)

## User Stories

### US1: Global Attention Queue
As a user, I want to see an always-visible queue of spectasks needing my attention, so I can quickly context-switch without hunting through projects.

**Acceptance Criteria:**
- Queue is accessible from every page in the app (global overlay/drawer)
- Shows tasks in these "human needed" states:
  - `spec_review` — specs pushed, need human review
  - `implementation_review` — agent wrote code, PR ready for review
  - `pull_request` — external PR awaiting merge
  - `spec_failed` — spec generation failed, needs triage
  - `implementation_failed` — implementation failed, needs triage
- Each item shows: task name, project name, status, time since status change
- Clicking an item navigates to the task (detail page or opens in split screen)
- Queue is sorted by `status_updated_at` (oldest first — FIFO)
- Badge count visible at all times showing total items needing attention

### US2: Browser Push Notifications
As a user, I want browser notifications when a task enters a "human needed" state, so I can be alerted even when the tab is in the background.

**Acceptance Criteria:**
- App requests `Notification.permission` on first load (with a tasteful prompt, not on page load)
- Fires a browser `Notification` when a task transitions to a human-needed status
- Notification title: task name; body: status description + project name
- Clicking the notification focuses the app tab and navigates to the task
- User can disable browser notifications from the queue UI
- Does NOT fire duplicate notifications for tasks already seen/acknowledged

### US3: Slack Notifications for Human-Needed Events
As a user, I want Slack alerts when tasks need my attention, so I'm notified in my primary communication tool.

**Acceptance Criteria:**
- Uses existing `AGENT_NOTIFICATIONS_SLACK_*` config (webhook URL, channel)
- Sends Slack message when a task transitions to any human-needed status
- Message includes: task name, project name, status, link to task in app
- Different emoji per event type (📋 spec review, 🔧 code review, ❌ failed)
- Does NOT spam — one message per status transition, not per poll cycle
- Configurable per-project (can disable for noisy projects)

### US4: Beautiful Queue UI
As a user, I want the queue to look great and feel native to the Helix app.

**Acceptance Criteria:**
- Slide-out drawer (right side) triggered by a persistent floating button or enhanced bell icon
- Groups items by urgency: failures at top, then reviews, then PRs
- Visual distinction between "new" (unseen) and "seen" items
- Dismiss/snooze individual items (snooze = hide for 1h)
- "Mark all as seen" action
- Responsive — works on mobile viewport
- Consistent with existing Helix dark theme / `lightTheme` system

## Out of Scope
- Email notifications for these events (existing email system is separate)
- Customizable notification rules per user (v2)
- Teams webhook integration (follow same pattern as Slack later)
- Filtering the queue by project (v2 — for now show everything)