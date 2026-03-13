# Requirements: SpectTask Human Attention Queue

## Problem

When managing multiple spectasks across projects, there's no way to know when something needs your attention without manually checking. The two main events that require human action are:

1. **Specs pushed** — agent finished writing/updating design docs, human needs to review
2. **Agent stopped after writing code** — the current interaction completed (via the WebSocket sync protocol's `message_completed` event), human needs to check the work

Neither of these reliably surfaces to the user today:

- **Specs pushed** does trigger a status transition (`spec_generation` → `spec_review`), so in theory it's detectable. But the existing `GlobalNotifications` bell icon **only renders on the Projects list page** (`Projects.tsx` is the sole page passing `notifications={true}` to `Page.tsx`). Once you're in Kanban, task detail, or split screen — where you actually work — the bell disappears.

- **Agent interaction completed** is not surfaced at all. When the agent finishes an interaction during `implementation`, `handleMessageCompleted` in `websocket_external_agent_sync.go` marks the interaction as complete and publishes updates to the frontend. But there's no notification, no Slack message, nothing. The task stays in `implementation` status (correctly — the agent may get another message). The user has to be watching the session to notice.

Additionally, there are failure states (`spec_failed`, `implementation_failed`) that need attention but aren't surfaced in the current notification UI at all.

## Key Insight: Events, Not Statuses

The user's original request names **events** — "specs pushed", "agent has stopped" — not status values. The correct model is an event log, rather than polling for tasks in specific statuses. Some events correspond to status transitions, but "agent interaction completed" doesn't change any status and shouldn't.

## Events That Require Human Attention

| Event | How to detect | Status change? |
|-------|--------------|----------------|
| Specs pushed (every commit) | `processDesignDocsForBranch` in `git_http_server.go` — sets `DesignDocsPushedAt`, transitions to `spec_review` | Yes |
| Agent interaction completed | `handleMessageCompleted` in `websocket_external_agent_sync.go` — fires when AI finishes responding via WebSocket sync protocol, and session is linked to a spectask (`helixSession.Metadata.SpecTaskID != ""`) | **No** — task stays in `implementation` |
| Spec generation failed | Status → `spec_failed` | Yes |
| Implementation failed | Status → `implementation_failed` | Yes |
| PR ready for merge | Status → `pull_request` (external repo) | Yes |

## User Stories

### US1: Global Attention Queue

As a user, I want to see an always-visible queue of events needing my attention, so I can quickly context-switch without hunting through projects.

**Acceptance Criteria:**
- Reuse the existing `GlobalNotifications` bell icon + badge — keep the same `IconButton`/`Badge`/`Bell` code, same position in the top bar
- Make it render on every page (remove the `notifications` prop gate in `Page.tsx` so it's not limited to `Projects.tsx`)
- Replace the small `Popover` with a proper slide-out `Drawer` for the queue body
- Swap the data source from status-polling to the new attention events API
- Shows attention events from all projects, sorted by time (newest first)
- Each item shows: event type, task name, project name, how long ago
- Clicking an item navigates to the task detail page or opens in split screen
- Badge count visible at all times showing unacknowledged items
- Items can be dismissed individually or all at once

### US2: Detect "Agent Interaction Completed"

As a user, I want to be notified when the agent finishes its current interaction during implementation, so I know work is ready for my review.

**Acceptance Criteria:**
- System detects `message_completed` events for spectask-linked sessions
- This generates an attention event without changing the task's status (task stays in `implementation`)
- Event fires every time an interaction completes, not just once
- Does NOT fire for non-spectask sessions (e.g., exploratory sessions without a task)

### US3: Browser Push Notifications

As a user, I want browser notifications when attention events fire, so I'm alerted even when the tab is in the background.

**Acceptance Criteria:**
- Permission requested tastefully (inline prompt in the queue drawer, not on page load)
- Fires a browser `Notification` for each new attention event
- Notification shows: event type + task name + project name
- Clicking the notification focuses the app and navigates to the task
- User can disable from the queue UI
- No duplicate notifications for already-acknowledged events

### US4: Slack Notifications (Per-Project)

As a user, I want Slack alerts when tasks need my attention, posted to the project's configured Slack channel.

**Acceptance Criteria:**
- Uses the **existing per-project Slack trigger** (`SlackTrigger` on each app with `ProjectUpdates: true` and `ProjectChannel`)
- NOT a global env var — each project controls its own Slack channel via the existing Slack bot integration
- New attention events are posted as replies to the task's existing Slack thread (reuse `SlackThread` lookup + `postProjectUpdateReply` pattern)
- If no Slack bot is configured for the project, no Slack message — silent skip
- Events posted: every spec commit push, every agent interaction completed, failures
- Different emoji per event type (📋 specs pushed, 🛑 agent done, ❌ failed)

### US5: Beautiful Queue UI

As a user, I want the queue to look great and feel native to the Helix app.

**Acceptance Criteria:**
- Slide-out drawer (right side) triggered by the existing bell icon (reuse current `GlobalNotifications` button code)
- Groups items by type: failures at top, then agent-stopped, then reviews/PRs
- Visual distinction between new (unseen) and acknowledged items
- Dismiss/snooze individual items (snooze = hide for 1h)
- Responsive — works on mobile viewport
- Consistent with existing Helix dark theme / `lightTheme` system

## Out of Scope

- Email notifications for these events (existing email system is separate)
- Customizable notification rules per user (v2)
- Teams webhook integration (follow same pattern as Slack later)