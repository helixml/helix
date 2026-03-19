# Design: SpectTask Human Attention Queue

## Overview

Add a global "attention queue" that surfaces events needing human action across all projects, with browser push notifications and Slack alerts. This is event-driven — not status-polling — because the two main triggers ("specs pushed" and "agent interaction completed") don't both map to clean status transitions.

## Codebase Context

### What Exists Today

| Component | Location | What it does |
|-----------|----------|-------------|
| `GlobalNotifications` | `frontend/src/components/system/GlobalNotifications.tsx` | Bell icon in top bar. Polls tasks in `spec_review`, `implementation_review`, `pull_request` statuses. Shows a small MUI `Popover`. Tracks seen/unseen via `localStorage`. **Effectively invisible** — see below. |
| `Page` component | `frontend/src/components/system/Page.tsx` | App shell — renders `<GlobalNotifications>` only when `notifications` prop is true. |
| Per-project Slack bot | `api/pkg/trigger/slack/slack_bot.go` | Full Slack bot per project. Uses `SlackTrigger` config on the app: `BotToken`, `ProjectUpdates`, `ProjectChannel`. Subscribes via `store.SubscribeForTasks()` to get notified on any spectask create/update for that project. |
| Slack project updates | `api/pkg/trigger/slack/slack_project_updates.go` | Posts threaded Slack messages on task status changes. Creates a `SlackThread` per task, posts replies for each status update. Has emoji, color, dedup logic (`hasProjectUpdateReply`). |
| `SubscribeForTasks` | `api/pkg/store/store_spec_tasks.go` | Pub/sub that fires on every `CreateSpecTask` / `UpdateSpecTask` call. The Slack bot subscribes to this per-project. |
| `handleMessageCompleted` | `api/pkg/server/websocket_external_agent_sync.go` | Fires when AI finishes responding via WebSocket sync protocol. Knows the spectask via `helixSession.Metadata.SpecTaskID`. This is the "agent stopped after writing code" event. |
| `processDesignDocsForBranch` | `api/pkg/services/git_http_server.go` | Fires when design docs are pushed. Sets `DesignDocsPushedAt`, transitions to `spec_review`, updates the spectask in the store (which triggers `SubscribeForTasks`). |
| SpecTask statuses | `api/pkg/types/simple_spec_task.go` | All status constants including `spec_failed`, `implementation_failed` which are NOT surfaced in the current notification UI. |
| Orchestrator | `api/pkg/services/spec_task_orchestrator.go` | Manages status transitions + polls task states on a loop. |
| WebSocket events | `frontend/src/contexts/streaming.tsx` | Session-scoped WebSocket for streaming responses. NOT suitable for global task events. |
| External agent status | `types.SessionMetadata.ExternalAgentStatus` | String field: `"running"`, `"stopped"`, `"terminated_idle"`. On session metadata, not spectask. |
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

When the agent finishes an interaction during implementation, `handleMessageCompleted` in `websocket_external_agent_sync.go` fires:

```go
// websocket_external_agent_sync.go ~line 1851
// Update SpecTaskZedThread activity if this is a spectask session
if helixSession.Metadata.SpecTaskID != "" {
    go apiServer.updateSpecTaskZedThreadActivity(context.Background(), acpThreadID)
}
```

It knows the spectask ID, it knows the interaction just completed, but it doesn't emit any notification. The task stays in `implementation` (correctly — the agent might get another message). There's no signal to the human that work is ready for review.

### Existing Per-Project Slack Integration

The Slack integration is **not** a global env var. It's per-project, configured via `SlackTrigger` on each app:

```go
// types.go
type SlackTrigger struct {
    Enabled        bool   `json:"enabled,omitempty"`
    AppToken       string `json:"app_token"`
    BotToken       string `json:"bot_token"`
    ProjectUpdates bool   `json:"project_updates,omitempty"`  // ← this enables task updates
    ProjectChannel string `json:"project_channel,omitempty"`  // ← target channel
}
```

The bot subscribes via `store.SubscribeForTasks()` which fires on every `UpdateSpecTask`. The `postProjectUpdate` method creates a Slack thread per task and posts status change replies. This already fires for status transitions like `spec_generation` → `spec_review`.

**What's missing from the existing Slack integration:**
1. **Spec commit pushes** — the status transition to `spec_review` does trigger a Slack update, but it looks like a generic status change, not "specs are ready for your review"
2. **Agent interaction completed** — `handleMessageCompleted` doesn't call `UpdateSpecTask`, so `SubscribeForTasks` never fires. No Slack message at all.

## Key Decisions

**1. Events model: new `attention_events` table**

Decision: **Introduce an `attention_events` database table.** Backend code emits events when human attention is needed. The frontend polls this single table. This is a small, focused table — not a general-purpose event bus. It stores only "human needed" events with a clear lifecycle (created → acknowledged → dismissed).

Status polling doesn't work because "agent interaction completed" produces no status change, and you can't distinguish "newly entered this status" from "been in this status for hours."

**2. Slack: reuse the existing per-project Slack bot, don't use global env vars**

Decision: **Hook into the existing `SlackBot.postProjectUpdate` flow.** The `AGENT_NOTIFICATIONS_SLACK_*` global env vars are the wrong thing — Slack notifications must be per-project, using each project's configured `SlackTrigger`. We add new attention events as threaded replies in the existing task Slack threads.

For events triggered by `SubscribeForTasks` (status changes like `spec_review`, failures), the existing Slack integration already fires — we just need to make the messages more descriptive (e.g., "📋 Specs ready for your review" instead of generic "Status update: Spec Review").

For events NOT triggered by `SubscribeForTasks` (agent interaction completed), we need a new notification path. The `handleMessageCompleted` handler will emit an attention event, and separately notify the Slack bot directly (or update the spectask's `updated_at` to trigger the subscription).

**3. Agent-stopped detection: hook into `handleMessageCompleted`, not container monitoring**

Decision: **Emit an `agent_interaction_completed` attention event inside `handleMessageCompleted`** when the session is linked to a spectask (`helixSession.Metadata.SpecTaskID != ""`). This is precise — it fires exactly when the AI finishes responding, which is the moment the user's original prompt describes ("agent has stopped after writing code").

This is NOT the same as container stop detection. The container may keep running (idle timeout), and that's fine. What matters is "the agent finished its current task and the human should look."

This applies to **every phase with an active agent session**, not just implementation. Any phase except backlog can have a running container (`planning_session_id` is set across spec generation, review, revision, implementation, etc.).

**4. Refactor GlobalNotifications in-place, render unconditionally**

Decision: **Refactor the existing `GlobalNotifications.tsx` in-place** — keep the bell icon, badge, and button code exactly as they are. Swap the `Popover` for a `Drawer`, swap the data source from status-polling to the attention events API. Make `Page.tsx` render it unconditionally (remove the `notifications` prop gate). No new component file for the bell — reuse what's there.

**6. Kanban card visual treatment: widen existing code, don't add new**

Decision: **`TaskCard.tsx` already has the exact amber-dot-goes-away-on-click pattern we need.** The `useAgentActivityCheck` hook tracks `agent_work_state` from the backend (`"idle"` / `"working"` / `"done"`). It shows a green pulsing dot when working, an amber dot when idle/done (needs attention), and `markAsSeen()` on card click dismisses the amber dot. However, it's currently gated to only `planning` and `implementation` phases in two places:

1. The `enabled` arg: `showProgress && !!task.planning_session_id` where `showProgress = task.phase === "planning" || task.phase === "implementation"`
2. The rendering JSX: `task.phase === "planning" || task.phase === "implementation"` guards on both the green and amber dot

Fix: widen `enabled` to `!!task.planning_session_id` (any phase with a session), remove the phase guards from the rendering JSX, and sort cards with `needsAttention` to the top of their Kanban column.

**5. Browser notifications: permission requested in the queue drawer**

Decision: **Show an inline prompt inside the queue drawer on first open**, not a browser popup on page load. When they open the drawer, show a small banner: "Enable desktop notifications?" with an Enable button. Store preference in localStorage.

**6. Queue drawer UI**

Decision: **Right-side slide-out drawer** (MUI `Drawer` with `anchor="right"`). Overlays on top of any page (Kanban, detail, split screen) without disrupting layout. Triggered by the bell icon — rendered on every page now. Width: ~400px.

## Architecture

### Backend — Attention Events

#### Database Schema

```sql
CREATE TABLE attention_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,          -- who should see this (project owner / org members)
    organization_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    spec_task_id TEXT NOT NULL,
    event_type TEXT NOT NULL,       -- specs_pushed, agent_interaction_completed, spec_failed, etc.
    title TEXT NOT NULL,            -- "Specs ready for review"
    description TEXT,               -- context: "Agent pushed design docs for 'Add user auth'"
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at TIMESTAMP,      -- user saw it (opened drawer while this was visible)
    dismissed_at TIMESTAMP,         -- user explicitly dismissed
    snoozed_until TIMESTAMP,        -- hidden until this time
    idempotency_key TEXT UNIQUE,    -- prevents duplicate events
    metadata JSONB                  -- commit hash, session ID, interaction ID, etc.
);

CREATE INDEX idx_attention_events_user_active
    ON attention_events (user_id, organization_id)
    WHERE dismissed_at IS NULL;
```

#### Event Types

| Event Type | Trigger Point | Idempotency Key |
|------------|--------------|-----------------|
| `specs_pushed` | `git_http_server.go: processDesignDocsForBranch` | `taskID:specs_pushed:commitHash` |
| `agent_interaction_completed` | `websocket_external_agent_sync.go: handleMessageCompleted` (any phase with a spectask session, not just implementation) | `taskID:agent_interaction_completed:interactionID` |
| `spec_failed` | Orchestrator on status → `spec_failed` | `taskID:spec_failed` |
| `implementation_failed` | Orchestrator on status → `implementation_failed` | `taskID:implementation_failed` |
| `pr_ready` | `checkTaskForExternalPRActivity` on status → `pull_request` | `taskID:pr_ready:prID` |

#### Event Emission Flow

```
handleMessageCompleted (websocket_external_agent_sync.go)
    ↓ session has SpecTaskID?
    ↓ yes
    attentionService.EmitEvent("agent_interaction_completed", task, metadata)
        ├── Insert into attention_events (idempotent on key)
        ├── Notify per-project Slack bot (if configured)
        │     └── Post threaded reply to task's Slack thread
        └── Return (frontend polls attention_events on its own)

processDesignDocsForBranch (git_http_server.go)
    ↓ design docs pushed, status → spec_review
    attentionService.EmitEvent("specs_pushed", task, metadata)
        ├── Insert into attention_events
        ├── Slack: already handled by SubscribeForTasks (status changed)
        │     but we enhance the message: "📋 Specs ready for your review"
        └── Return

Orchestrator status transitions → spec_failed / implementation_failed / pull_request
    attentionService.EmitEvent(eventType, task, metadata)
        ├── Insert into attention_events
        ├── Slack: already handled by SubscribeForTasks
        └── Return
```

#### Slack Integration Detail

For events that DO trigger `UpdateSpecTask` (specs_pushed, failures, pr_ready): The existing `SubscribeForTasks` → `postProjectUpdate` flow already fires. We enhance `buildProjectUpdateReplyAttachment` to use richer messaging when we can detect the specific event (e.g., status just changed to `spec_review` → "📋 Specs ready for your review").

For `agent_interaction_completed`: This does NOT update the spectask, so `SubscribeForTasks` doesn't fire. The `AttentionService.EmitEvent` method needs to directly post to the task's Slack thread. It does this by:
1. Looking up the project's Slack app (if any) with `ProjectUpdates: true`
2. Looking up the existing `SlackThread` for this spectask
3. Posting a threaded reply: "🛑 Agent finished working — ready for your review"

If no Slack bot is configured for the project, this is a silent no-op.

### Backend — New Files & Changes

**New files:**
- `api/pkg/types/attention_event.go` — `AttentionEvent` struct, event type constants
- `api/pkg/store/store_attention_events.go` — CRUD: create (upsert on idempotency key), list (active only), update (acknowledge/dismiss/snooze), bulk dismiss, cleanup expired
- `api/pkg/services/attention_service.go` — `AttentionService` with `EmitEvent()`: creates DB row, posts to Slack if applicable
- `api/pkg/server/attention_event_handlers.go` — HTTP handlers for list/acknowledge/dismiss

**Modified files (backend):**
- `api/pkg/server/websocket_external_agent_sync.go` — in `handleMessageCompleted`, after the existing `updateSpecTaskZedThreadActivity` call, emit `agent_interaction_completed` attention event
- `api/pkg/services/git_http_server.go` — in `processDesignDocsForBranch`, emit `specs_pushed` attention event
- `api/pkg/services/spec_task_orchestrator.go` — on transitions to `spec_failed`, `implementation_failed`, `pull_request`, emit corresponding attention events
- `api/pkg/server/server.go` — register attention event API routes
- `api/pkg/trigger/slack/slack_project_updates.go` — enhance reply messages to be more descriptive for review/failure statuses; add a public method for posting attention-event-specific replies that `AttentionService` can call for non-status-change events like `agent_interaction_completed`

**Modified files (frontend — Kanban):**
- `frontend/src/components/tasks/TaskCard.tsx` — widen `useAgentActivityCheck` enabled condition from `showProgress && !!task.planning_session_id` to `!!task.planning_session_id`; remove `task.phase === "planning" || task.phase === "implementation"` guards from green/amber dot rendering JSX so dots show in any phase with a session
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — sort cards with `needsAttention` (from `agent_work_state`) to the top of their Kanban column

### Frontend

```
Page.tsx (app shell)
  └── GlobalNotifications (REFACTORED in-place, rendered UNCONDITIONALLY)
        ├── Existing bell icon + Badge — keep current IconButton/Badge/Bell code as-is
        ├── AttentionQueueDrawer — replaces the small Popover with a right-side MUI Drawer, ~400px
        │     ├── BrowserNotificationBanner — permission prompt (shown when permission is "default")
        │     ├── QueueHeader — "Needs Attention", count, "Dismiss All" button
        │     ├── QueueSection (failures) — red accent, collapsible
        │     ├── QueueSection (agent stopped) — amber accent
        │     ├── QueueSection (specs & PRs) — blue accent
        │     └── AttentionEventItem — event title, task name, project, time ago, dismiss/snooze
        └── useBrowserNotifications hook — Notification API lifecycle
```

**Data flow:**
1. `useAttentionEvents` hook — polls `GET /api/v1/attention-events?active=true` every ~10s via React Query
2. When drawer opens, PATCH events as acknowledged
3. When new unacknowledged events appear, fire browser `Notification` if permitted
4. Click on item → `account.orgNavigate('project-task-detail', ...)` (same as today)

**New files:**
- `frontend/src/hooks/useAttentionEvents.ts` — React Query hook for the new API
- `frontend/src/hooks/useBrowserNotifications.ts` — Notification API wrapper

**Modified files:**
- `frontend/src/components/system/GlobalNotifications.tsx` — **refactor in-place**: keep the existing bell icon / `IconButton` / `Badge` / `Bell` code exactly as-is; swap the data source from status-polling to `useAttentionEvents` hook; replace the `Popover` with a `Drawer`; add queue sections and browser notification wiring
- `frontend/src/components/system/Page.tsx` — render `GlobalNotifications` **unconditionally** (remove the `{notifications && ...}` prop gate so it appears on every page, not just `Projects.tsx`)

### API Endpoints

```
GET   /api/v1/attention-events?active=true     — list unresolved events for current user
PATCH /api/v1/attention-events/:id             — acknowledge, dismiss, or snooze an event
POST  /api/v1/attention-events/dismiss-all     — bulk dismiss all active events
```

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| `attention_events` table grows unbounded | Auto-expire dismissed events after 7 days (background cleanup in orchestrator). Active events are tiny in volume. |
| `agent_interaction_completed` fires too often (every interaction, not just "done coding") | This is intentional — the user asked for every time the agent stops. Idempotency key includes interaction ID so each completion is a distinct event. User can dismiss/snooze from the queue. |
| Polling 10s for attention events adds load | Single query on an indexed table with small result set. Negligible compared to existing 3.7s task polling. |
| Slack posting for `agent_interaction_completed` needs access to SlackBot internals | Expose a public `PostAttentionEvent(taskID, message)` method on the Slack trigger system, or have `AttentionService` look up the project's Slack app and thread directly via the store. |
| Drawer z-index conflicts with chat panel, modals | MUI Drawer handles layering. Test with chat panel open on split screen view. |
| Swagger/OpenAPI needs updating for new endpoints | Add swagger annotations to handlers, run `./stack update_openapi` to regenerate client. |

## Implementation Notes

### What was built
- `api/pkg/types/attention_event.go` — `AttentionEvent` struct + 5 event type constants + `BuildAttentionEventIdempotencyKey` helper
- `api/pkg/system/uuid.go` — added `atev_` prefix and `GenerateAttentionEventID()`
- `api/pkg/store/store_attention_events.go` — GORM CRUD with `ON CONFLICT DO NOTHING` for idempotent creates, active-only listing, bulk dismiss, expired cleanup
- `api/pkg/store/store_mocks.go` — manually added mock methods (mockgen not available in build env)
- `api/pkg/store/postgres.go` — added `AttentionEvent` to AutoMigrate
- `api/pkg/store/store.go` — added 6 methods to Store interface
- `api/pkg/services/attention_service.go` — `EmitEvent()` with project lookup, idempotency, fire-and-forget Slack via per-project bot token
- `api/pkg/server/attention_event_handlers.go` — 3 HTTP handlers with swagger annotations
- `api/pkg/server/server.go` — routes registered, AttentionService wired into server + GitHTTPServer + orchestrator
- `api/pkg/server/websocket_external_agent_sync.go` — `handleMessageCompleted` emits `agent_interaction_completed` for spectask sessions
- `api/pkg/services/git_http_server.go` — `processDesignDocsForBranch` emits `specs_pushed` on every design doc commit
- `api/pkg/services/spec_task_orchestrator.go` — subscribes to `spec_failed`/`implementation_failed` statuses, emits `pr_ready` on external PR detection, periodic cleanup
- `frontend/src/hooks/useAttentionEvents.ts` — React Query hook polling every 10s, mutation wrappers
- `frontend/src/hooks/useBrowserNotifications.ts` — Notification API wrapper with permission flow + localStorage opt-out
- `frontend/src/components/system/GlobalNotifications.tsx` — refactored in-place: bell icon preserved, Popover → Drawer, grouped event sections
- `frontend/src/components/system/Page.tsx` — removed `{notifications && ...}` gate so bell renders on every page
- `frontend/src/components/tasks/TaskCard.tsx` — widened attention dot to all phases with a session
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — `sortWithAttentionFirst` bubbles attention cards to top of each column

### Decisions made during implementation
- Changed PATCH route to PUT for attention event updates because `useApi` hook doesn't expose a `patch` method
- Used `api.get`/`api.put`/`api.post` from `useApi` instead of generated client methods (swagger not regenerated yet)
- For Slack notifications: the `AttentionService` directly calls the Slack API using the per-project bot token found via `ListApps` → `SlackTrigger`, rather than going through the `SlackBot` class (which would require complex wiring). It looks up existing `SlackThread` records and posts threaded replies.
- Failure event detection uses `SubscribeForTasks` in the orchestrator (a second subscription filtered to failure statuses) rather than hooking individual status-change sites — this catches failures regardless of where they originate.
- `sortWithAttentionFirst` is defined outside the `useMemo` to avoid dependency issues; it's a pure function so this is safe.
- Browser notification dedup uses both a `shownRef` in the hook and the Notification API's `tag` parameter for belt-and-suspenders dedup.

### Gotchas discovered
- `helix-4` is a symlink to `helix` — the editor only resolves the canonical path `helix/`
- `Project.UserID` not `Project.Owner` — the Go struct field name differs from the conceptual "owner"
- `mockgen` is not installed in the build environment — mock methods had to be added manually to `store_mocks.go`
- The `GlobalNotifications` component's `organizationId` prop comes from `Page.tsx` but is no longer used for data fetching (the attention events API filters by the authenticated user server-side). The prop is kept for interface compatibility.