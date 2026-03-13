# Design: SpectTask Human Attention Queue

## Overview

Add a global "attention queue" that surfaces events needing human action across all projects, with browser push notifications and Slack alerts. This is event-driven — not status-polling — because the two main triggers ("specs pushed" and "agent stopped after writing code") don't both map to clean status transitions.

## Codebase Context

### What Exists Today

| Component | Location | What it does |
|-----------|----------|-------------|
| `GlobalNotifications` | `frontend/src/components/system/GlobalNotifications.tsx` | Bell icon in top bar. Polls tasks in `spec_review`, `implementation_review`, `pull_request` statuses. Shows a small MUI `Popover`. Tracks seen/unseen via `localStorage`. **Effectively invisible** — see below. |
| `Page` component | `frontend/src/components/system/Page.tsx` | App shell — renders `<GlobalNotifications>` only when `notifications` prop is true. |
| Slack webhook (janitor) | `api/pkg/janitor/utils.go` | `sendSlackNotification(webhookURL, message)` — simple webhook POST. Used for session errors and admin alerts. |
| Agent notifications config | `api/pkg/config/config.go` | `AGENT_NOTIFICATIONS_SLACK_*` env vars — webhook URL, bot token, channel. |
| SpecTask statuses | `api/pkg/types/simple_spec_task.go` | All status constants including `spec_failed`, `implementation_failed` which are NOT surfaced in notifications. |
| Orchestrator | `api/pkg/services/spec_task_orchestrator.go` | Manages status transitions + polls task states on a loop. |
| Git HTTP server | `api/pkg/services/git_http_server.go` | Handles git pushes. `processDesignDocsForBranch` sets `DesignDocsPushedAt` and transitions to `spec_review`. `handleFeatureBranchPush` records `last_push_at` but does NOT transition status. |
| WebSocket events | `frontend/src/contexts/streaming.tsx` | Session-scoped WebSocket for streaming responses. NOT suitable for global task events. |
| External agent status | `types.SessionMetadata.ExternalAgentStatus` | String field: `"running"`, `"stopped"`, `"terminated_idle"`. Set on session metadata, not on the spectask itself. |
| Existing polling | `GlobalNotifications` polls every 30s; `SpecTasksPage` polls tasks every 3.7s for workspace view. |

### Why You Never See the Bell

`Page.tsx` accepts an optional `notifications` boolean prop. When true, it renders `<GlobalNotifications>`. Only ONE page passes it:

- `Projects.tsx` → `notifications={true}` ✅
- `SpecTasksPage.tsx` → not set ❌
- `SpecTaskDetailPage.tsx` → not set ❌
- `SpecTaskReviewPage.tsx` → not set ❌
- `TeamDesktopPage.tsx` → not set ❌

The bell only shows on the projects list — the one page where you're least likely to need it.

### The "Agent Stopped" Gap

When an agent pushes code to a feature branch during `implementation`, `handleFeatureBranchPush` runs:

```go
// git_http_server.go line ~940
case types.TaskStatusImplementation:
    // Record the push but don't transition status or send prompt automatically
    now := time.Now()
    task.LastPushCommitHash = commitHash
    task.LastPushAt = &now
```

The push is recorded, but status stays `implementation`. When the agent container later stops or disconnects, nothing changes on the spectask either — `ExternalAgentStatus` lives on the session metadata, not the task. There is no event, no notification, nothing. The human has to notice on their own.

## Key Decision: Events, Not Status Polling

The current `GlobalNotifications` polls for tasks in specific statuses. This doesn't work because:

1. "Agent stopped during implementation" produces no status change
2. Status polling can't distinguish "newly entered this status" from "been in this status for hours"
3. It requires the frontend to reconstruct event semantics from snapshots

**Decision: Introduce an `attention_events` table in the database.** Backend code emits events when human attention is needed. The frontend polls this single table. Slack notifications fire at event creation time.

This is a small, focused table — not a general-purpose event bus. It stores only "human needed" events with a clear lifecycle (created → acknowledged → dismissed).

## Attention Events

### Event Types

| Event Type | Trigger Point | What Happened |
|------------|--------------|---------------|
| `specs_pushed` | `git_http_server.go: processDesignDocsForBranch` | Agent pushed new/updated design docs. Task moves to `spec_review`. |
| `agent_stopped` | Orchestrator's `handleImplementation` loop | Agent container is no longer running while task is in `implementation` and has a `last_push_at` (i.e., it did some work). |
| `spec_failed` | Orchestrator when status → `spec_failed` | Spec generation errored out. |
| `implementation_failed` | Orchestrator when status → `implementation_failed` | Implementation errored out. |
| `pr_ready` | `checkTaskForExternalPRActivity` | External PR detected, task moved to `pull_request`. |

### Database Schema

```sql
CREATE TABLE attention_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,          -- who should see this (task/project owner)
    organization_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    spec_task_id TEXT NOT NULL,
    event_type TEXT NOT NULL,       -- specs_pushed, agent_stopped, spec_failed, etc.
    title TEXT NOT NULL,            -- human-readable: "Specs ready for review"
    description TEXT,               -- context: "Agent pushed 3 design docs"
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at TIMESTAMP,      -- user saw it (opened drawer)
    dismissed_at TIMESTAMP,         -- user explicitly dismissed
    snoozed_until TIMESTAMP,        -- hidden until this time
    idempotency_key TEXT UNIQUE,    -- "task_id:event_type:trigger" to prevent dupes
    metadata JSONB                  -- extra context (commit hash, session ID, etc.)
);

CREATE INDEX idx_attention_events_user_active
    ON attention_events (user_id, organization_id)
    WHERE dismissed_at IS NULL;
```

**Idempotency key examples:**
- `tsk_abc:specs_pushed:commit_def` — one event per spec push commit
- `tsk_abc:agent_stopped:ses_xyz` — one event per session stop
- `tsk_abc:spec_failed` — one event per failure (no extra qualifier needed)

### Detecting "Agent Stopped"

The orchestrator already polls tasks in `implementation` status via `handleImplementation`. Currently it just checks if the external agent is running and returns. We extend this:

```
handleImplementation(task):
    if task.ExternalAgentID != "":
        agent = store.GetSpecTaskExternalAgent(task.ID)
        if agent.Status == "running":
            // Agent running, all good
            return
        else:
            // Agent NOT running, but task is in implementation
            if task.LastPushAt != nil:
                // Agent did work and then stopped → human should look
                emitAttentionEvent("agent_stopped", task, agent.SessionID)
            // If no push, agent may not have started yet — don't alert
```

This fires once per stopped agent session (idempotency key includes session ID). If the user restarts the agent, a new session gets a new ID, so a future stop would be a new event.

## Architecture

### Backend

```
attention_events table (new)
    ↑ written by:
    ├── git_http_server.go          → specs_pushed events
    ├── spec_task_orchestrator.go   → agent_stopped, spec_failed, implementation_failed, pr_ready
    │
    ↓ read by:
    ├── GET /api/v1/attention-events          → frontend polling
    ├── PATCH /api/v1/attention-events/:id    → acknowledge/dismiss/snooze
    └── POST attention event hook             → Slack webhook (at creation time)
```

**New files:**
- `api/pkg/types/attention_event.go` — type definitions
- `api/pkg/store/store_attention_events.go` — CRUD operations
- `api/pkg/services/attention_service.go` — `EmitEvent()` with idempotency, Slack dispatch
- `api/pkg/server/attention_event_handlers.go` — HTTP handlers

**Modified files:**
- `api/pkg/services/git_http_server.go` — call `EmitEvent("specs_pushed", ...)` in `processDesignDocsForBranch`
- `api/pkg/services/spec_task_orchestrator.go` — call `EmitEvent` for agent_stopped, failures, PR detection
- `api/pkg/server/server.go` — register new routes

### Frontend

```
Page.tsx (app shell)
  └── AttentionQueue (replaces GlobalNotifications, rendered UNCONDITIONALLY)
        ├── AttentionQueueButton — bell icon + red badge count
        ├── AttentionQueueDrawer — right-side MUI Drawer, ~400px
        │     ├── BrowserNotificationBanner — permission prompt (shown when permission is "default")
        │     ├── QueueHeader — "Needs Attention", count, "Dismiss All" button
        │     ├── QueueSection (failures) — red accent, collapsible
        │     ├── QueueSection (agent stopped) — amber accent
        │     ├── QueueSection (reviews & PRs) — blue accent
        │     └── AttentionEventItem — event title, task name, project, time ago, dismiss/snooze
        └── useBrowserNotifications hook — Notification API lifecycle
```

**Data flow:**
1. `useAttentionEvents` hook — polls `GET /api/v1/attention-events?active=true` every ~10s via React Query
2. When drawer opens, PATCH events as acknowledged
3. When new unacknowledged events appear, fire browser `Notification` if permitted
4. Click on item → `account.orgNavigate('project-task-detail', ...)` (same as today)

**New files:**
- `frontend/src/components/system/AttentionQueue.tsx`
- `frontend/src/hooks/useAttentionEvents.ts` — React Query hook for the new API
- `frontend/src/hooks/useBrowserNotifications.ts` — Notification API wrapper

**Modified files:**
- `frontend/src/components/system/Page.tsx` — replace `GlobalNotifications` with `AttentionQueue`, render unconditionally (remove `notifications` prop gate)

**Deleted files:**
- `frontend/src/components/system/GlobalNotifications.tsx`

### Slack Notifications

Fired inside `attention_service.go` at event creation time (not polled). Reuses existing `AGENT_NOTIFICATIONS_SLACK_WEBHOOK_URL` config — no new env vars.

**Message format:**
```
📋 Specs ready for review: "Add user auth" (Project: helix-app)
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx

🛑 Agent stopped after coding: "Fix login bug" (Project: helix-api)
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx

❌ Spec generation failed: "Refactor DB layer" (Project: helix-core)
→ https://app.helix.ml/org/projects/prj_xxx/tasks/tsk_xxx
```

### API Endpoints

```
GET  /api/v1/attention-events?active=true    — list unresolved events for current user
PATCH /api/v1/attention-events/:id           — acknowledge, dismiss, or snooze an event
POST /api/v1/attention-events/:id/dismiss-all — bulk dismiss
```

## File Change Summary

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | **New** — `AttentionEvent` struct, event type constants |
| `api/pkg/store/store_attention_events.go` | **New** — `CreateAttentionEvent`, `ListAttentionEvents`, `UpdateAttentionEvent` |
| `api/pkg/services/attention_service.go` | **New** — `EmitEvent()` with idempotency + Slack dispatch |
| `api/pkg/server/attention_event_handlers.go` | **New** — HTTP handlers for list/acknowledge/dismiss |
| `api/pkg/server/server.go` | **Edit** — register attention event routes |
| `api/pkg/services/git_http_server.go` | **Edit** — emit `specs_pushed` event in `processDesignDocsForBranch` |
| `api/pkg/services/spec_task_orchestrator.go` | **Edit** — emit `agent_stopped`, failure, and PR events |
| `frontend/src/components/system/AttentionQueue.tsx` | **New** — full queue UI (button + drawer + items) |
| `frontend/src/hooks/useAttentionEvents.ts` | **New** — React Query hook for attention events API |
| `frontend/src/hooks/useBrowserNotifications.ts` | **New** — browser Notification API wrapper |
| `frontend/src/components/system/Page.tsx` | **Edit** — replace `GlobalNotifications` with `AttentionQueue`, render unconditionally |
| `frontend/src/components/system/GlobalNotifications.tsx` | **Delete** |

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| `attention_events` table grows unbounded | Auto-expire dismissed events after 7 days (background cleanup in orchestrator). Active events are tiny in volume. |
| Agent-stopped detection fires too eagerly (agent is just between sessions) | Only fire when `last_push_at` is set (agent did real work). Idempotency key includes session ID — won't re-fire for same session. |
| Polling 10s for attention events adds load | Single query on an indexed table with small result set. Negligible compared to existing 3.7s task polling. |
| Slack webhook rate limits | Events are low-frequency (minutes/hours apart). One message per event via idempotency. |
| Drawer z-index conflicts with chat panel, modals | MUI Drawer handles layering. Test with chat panel open on split screen view. |
| Swagger/OpenAPI needs updating for new endpoints | Add swagger annotations to handlers, run `./stack update_openapi` to regenerate client |