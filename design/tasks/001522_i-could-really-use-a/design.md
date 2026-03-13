# Design: SpectTask Human Attention Queue

## Overview

Add a global "attention queue" that surfaces spectasks needing human action across all projects, with browser push notifications and Slack alerts. This builds on the existing `GlobalNotifications` component but replaces its limited popover with a proper slide-out drawer and adds proactive notification channels.

## Codebase Context

### What Exists Today

| Component | Location | What it does |
|-----------|----------|-------------|
| `GlobalNotifications` | `frontend/src/components/system/GlobalNotifications.tsx` | Bell icon in top bar, polls tasks in `spec_review`, `implementation_review`, `pull_request` statuses. Shows a small MUI `Popover` with task list. Tracks seen/unseen via `localStorage`. |
| `Page` component | `frontend/src/components/system/Page.tsx` | App shell — renders `<GlobalNotifications>` when `notifications` prop is true. This is the global layout wrapper. |
| Slack webhook (janitor) | `api/pkg/janitor/utils.go` | `sendSlackNotification(webhookURL, message)` — simple webhook POST. Used for session errors and admin alerts. |
| Agent notifications config | `api/pkg/config/config.go` | `AGENT_NOTIFICATIONS_SLACK_*` env vars — webhook URL, bot token, channel. Currently used for agent progress updates. |
| SpecTask statuses | `api/pkg/types/simple_spec_task.go` | All status constants including `spec_failed`, `implementation_failed` which are NOT currently surfaced in notifications. |
| Orchestrator | `api/pkg/services/spec_task_orchestrator.go` | Manages status transitions. This is where we hook in server-side notifications. |
| WebSocket events | `frontend/src/contexts/streaming.tsx` | Session-scoped WebSocket for streaming responses. NOT suitable for global task events (it's per-session). |
| Existing polling | `GlobalNotifications` polls every 30s; `SpecTasksPage` polls tasks every 3.7s for workspace view. |

### Key Decisions

**1. Polling vs WebSocket for queue updates**

Decision: **Keep polling, reduce interval to ~10s for the queue.** The existing `GlobalNotifications` already polls. Adding a dedicated WebSocket channel for task status events would be cleaner but is a larger change — the current WS infrastructure is session-scoped. Polling every 10s is fine for "human needed" events (they're low frequency). The React Query cache deduplicates requests across components.

**2. Replace GlobalNotifications vs build alongside**

Decision: **Replace `GlobalNotifications` entirely.** The new queue is a strict superset. The existing component is ~300 lines with no external dependents beyond `Page.tsx`. We keep the same bell icon location but swap the popover for a slide-out drawer.

**3. Where to trigger Slack notifications**

Decision: **In the orchestrator, on status transitions.** The orchestrator already manages all transitions (`handleSpecGeneration` → `spec_review`, feature branch push → `implementation_review`, error → `spec_failed`). Add a `notifyHumanNeeded()` call at each transition point. This guarantees exactly-once per transition (the orchestrator is the single writer).

**4. Browser notifications: when to request permission**

Decision: **Show an inline prompt inside the queue drawer on first open**, not a browser popup on page load. Users hate unsolicited permission requests. When they open the drawer, show a small banner: "Enable desktop notifications to get alerted when tasks need your attention" with an Enable button. Store preference in localStorage.

**5. Queue drawer vs modal vs embedded panel**

Decision: **Right-side slide-out drawer** (MUI `Drawer` with `anchor="right"`). This overlays on top of any page (Kanban, detail, split screen) without disrupting layout. Triggered by the bell icon (same position as today). Drawer width: ~400px. This pattern is already familiar from the chat panel in `SpecTasksPage`.

## Architecture

### Frontend

```
Page.tsx (app shell)
  └── AttentionQueue (replaces GlobalNotifications)
        ├── AttentionQueueButton — bell icon + badge count (always visible in top bar)
        ├── AttentionQueueDrawer — right-side MUI Drawer
        │     ├── BrowserNotificationBanner — permission request prompt
        │     ├── QueueHeader — title, "mark all seen", item count
        │     ├── QueueSection (failures) — grouped, red accent
        │     ├── QueueSection (reviews) — grouped, orange accent
        │     ├── QueueSection (PRs) — grouped, blue accent
        │     └── QueueItem — task name, project, status, time ago, dismiss/snooze
        └── useBrowserNotifications hook — manages Notification API lifecycle
```

**Data flow:**
1. `useAttentionQueue` hook — single React Query polling all projects' tasks (reuses the same API: `v1SpecTasksList`), filters to human-needed statuses, sorted by `status_updated_at`.
2. Seen/snoozed state in localStorage (same pattern as today, extended with snooze timestamps).
3. When new unseen items appear, fire browser `Notification` if permission granted.
4. Click on queue item → `account.orgNavigate('project-task-detail', ...)` (same as today).

**Human-needed statuses** (superset of today's `REVIEW_STATUSES`):
- `spec_review` — specs pushed, review needed
- `implementation_review` — agent wrote code, review needed  
- `pull_request` — external PR, merge needed
- `spec_failed` — spec generation failed, triage needed
- `implementation_failed` — implementation failed, triage needed

### Backend (Slack Notifications)

```
spec_task_orchestrator.go
  └── on status transition to human-needed status
        └── notifyHumanAttentionNeeded(task, newStatus)
              └── spec_task_human_notifier.go (new file)
                    ├── sendSlackNotification() — uses existing janitor webhook pattern
                    └── formatSlackMessage() — emoji + task name + project + link
```

**New file: `api/pkg/services/spec_task_human_notifier.go`**

Responsibilities:
- Accept a task + new status
- Look up project name from store
- Format a Slack message with appropriate emoji
- POST to the configured webhook (reuse `sendSlackNotification` from `janitor/utils.go`, or extract to shared package)
- Guard against duplicate sends (idempotency key = `taskID + status`)

**Configuration:** Reuses existing `AGENT_NOTIFICATIONS_SLACK_*` env vars. No new config needed. If webhook URL is empty, skip silently.

### Notification Message Format

**Slack:**
```
📋 Spec review needed: "Add user auth" (Project: helix-app)
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx

🔧 Code ready for review: "Fix login bug" (Project: helix-api)  
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx

❌ Spec generation failed: "Refactor DB layer" (Project: helix-core)
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx
```

**Browser notification:**
- Title: `"Helix: Spec review needed"` (or `"Code ready for review"`, `"Task failed"`)
- Body: `"Add user auth" — Project: helix-app`
- Icon: Helix favicon
- On click: `window.focus()` + navigate to task

## File Changes

| File | Change |
|------|--------|
| `frontend/src/components/system/GlobalNotifications.tsx` | **Delete** — replaced by AttentionQueue |
| `frontend/src/components/system/AttentionQueue.tsx` | **New** — queue button + drawer + all UI |
| `frontend/src/hooks/useAttentionQueue.ts` | **New** — React Query hook for fetching/filtering human-needed tasks across all projects |
| `frontend/src/hooks/useBrowserNotifications.ts` | **New** — manages `Notification` API permission + firing |
| `frontend/src/components/system/Page.tsx` | **Edit** — swap `GlobalNotifications` import for `AttentionQueue` |
| `api/pkg/services/spec_task_human_notifier.go` | **New** — Slack notification on human-needed transitions |
| `api/pkg/services/spec_task_orchestrator.go` | **Edit** — call notifier on status transitions |

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Polling N projects = N API calls | The existing pattern already does this. For users with many projects, we can add a dedicated backend endpoint later (`/api/v1/attention-queue`) that returns pre-filtered tasks across all projects in one call. For now, N is small. |
| Browser notification spam | Deduplicate by tracking `taskID + status` in localStorage. Only fire once per transition. |
| Slack webhook rate limits | One message per status transition. Spectask transitions are low-frequency (minutes/hours apart). Not a concern. |
| Drawer conflicts with other UI overlays | MUI Drawer uses z-index layering. Test with chat panel open, modals, etc. |