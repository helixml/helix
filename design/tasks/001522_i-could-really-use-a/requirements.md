# Requirements: SpectTask Human Attention Queue

## Problem

When managing multiple spectasks across projects, there's no way to know when something needs your attention without manually checking. The two main events that require human action are:

1. **Specs pushed** — agent finished writing/updating design docs, human needs to review
2. **Agent stopped after writing code** — agent session ended during implementation, human needs to check the work

Neither of these reliably surfaces to the user today:

- **Specs pushed** does trigger a status transition (`spec_generation` → `spec_review`), so in theory it's detectable. But the existing `GlobalNotifications` bell icon **only renders on the Projects list page** (`Projects.tsx` is the sole page passing `notifications={true}` to `Page.tsx`). Once you're in Kanban, task detail, or split screen — where you actually work — the bell disappears.

- **Agent stopped after writing code** is not reflected in any status transition at all. When the agent pushes to a feature branch during `implementation`, `handleFeatureBranchPush` in `git_http_server.go` explicitly records the push (`last_push_at`) but does NOT transition the status — the task stays in `implementation`. When the agent container stops or disconnects, nothing in the spectask model changes. There is no event, no status change, nothing. The user has to notice on their own.

Additionally, there are failure states (`spec_failed`, `implementation_failed`) that need attention but aren't surfaced in the current notification UI at all.

## Key Insight: Events, Not Statuses

The user's original request names **events** — "specs pushed", "agent has stopped" — not status values. The correct model is an event log that the frontend and Slack can subscribe to, rather than polling for tasks in specific statuses. Some of these events do correspond to status transitions, but others (like "agent stopped during implementation") don't map to any status today and shouldn't require one.

## Events That Require Human Attention

| Event | How to detect | Status change? |
|-------|--------------|----------------|
| Specs pushed (new or updated) | `DesignDocsPushedAt` set, status → `spec_review` | Yes |
| Agent stopped during implementation | Agent container stopped/disconnected while task is in `implementation` | **No** — task stays in `implementation`, only `last_push_at` and agent status change |
| Spec generation failed | Status → `spec_failed` | Yes |
| Implementation failed | Status → `implementation_failed` | Yes |
| PR ready for merge | Status → `pull_request` (external repo) | Yes |

## User Stories

### US1: Global Attention Queue

As a user, I want to see an always-visible queue of events needing my attention, so I can quickly context-switch without hunting through projects.

**Acceptance Criteria:**
- Queue is accessible from every page in the app (global overlay/drawer) — not gated behind a prop like today
- Shows attention events from all projects, sorted by time (newest first)
- Each item shows: event type, task name, project name, how long ago
- Clicking an item navigates to the task detail page or opens in split screen
- Badge count visible at all times showing unacknowledged items
- Items can be dismissed individually or all at once

### US2: Detect "Agent Stopped During Implementation"

As a user, I want to be notified when an agent stops working on a task (container stopped, session ended, idle timeout) during implementation, even though no status transition occurs.

**Acceptance Criteria:**
- System detects when an agent's container/session is no longer running while task is in `implementation` status
- This generates an attention event without changing the task's status (the task rightfully stays in `implementation` — the agent might be restarted)
- Event includes context: was there a recent push? How long was the agent running?
- Does NOT fire repeatedly for the same stopped agent — one event per stop

### US3: Browser Push Notifications

As a user, I want browser notifications when attention events fire, so I'm alerted even when the tab is in the background.

**Acceptance Criteria:**
- Permission requested tastefully (inline prompt in the queue drawer, not on page load)
- Fires a browser `Notification` for each new attention event
- Notification shows: event type + task name + project name
- Clicking the notification focuses the app and navigates to the task
- User can disable from the queue UI
- No duplicate notifications for already-acknowledged events

### US4: Slack Notifications

As a user, I want Slack alerts when tasks need my attention, so I'm notified in my primary communication tool.

**Acceptance Criteria:**
- Uses existing `AGENT_NOTIFICATIONS_SLACK_*` config (webhook URL, channel)
- Sends Slack message for each attention event
- Message includes: emoji per event type, task name, project name, link to task
- One message per event — not per poll cycle
- Configurable per-project (can disable for noisy projects)

### US5: Beautiful Queue UI

As a user, I want the queue to look great and feel native to the Helix app.

**Acceptance Criteria:**
- Slide-out drawer (right side) triggered by persistent bell icon in the top bar
- Groups items by type: failures at top, then agent-stopped, then reviews/PRs
- Visual distinction between new (unseen) and acknowledged items
- Dismiss/snooze individual items (snooze = hide for 1h)
- Responsive — works on mobile viewport
- Consistent with existing Helix dark theme / `lightTheme` system

## Out of Scope

- Email notifications for these events (existing email system is separate)
- Customizable notification rules per user (v2)
- Teams webhook integration (follow same pattern as Slack later)
- Filtering the queue by project (v2 — for now show everything)