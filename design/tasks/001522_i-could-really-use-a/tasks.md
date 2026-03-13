# Implementation Tasks

## Phase 1: Backend — Attention Events Infrastructure

- [ ] Create `api/pkg/types/attention_event.go` — `AttentionEvent` struct with fields: `ID`, `UserID`, `OrganizationID`, `ProjectID`, `SpecTaskID`, `EventType`, `Title`, `Description`, `CreatedAt`, `AcknowledgedAt`, `DismissedAt`, `SnoozedUntil`, `IdempotencyKey`, `Metadata` (JSONB). Define event type constants: `specs_pushed`, `agent_stopped`, `spec_failed`, `implementation_failed`, `pr_ready`.
- [ ] Create `api/pkg/store/store_attention_events.go` — GORM migrations + store methods: `CreateAttentionEvent` (upsert on idempotency key), `ListAttentionEvents` (filter by user_id, org, active-only with `dismissed_at IS NULL AND (snoozed_until IS NULL OR snoozed_until < NOW())`), `UpdateAttentionEvent` (set acknowledged_at / dismissed_at / snoozed_until), `BulkDismissAttentionEvents`, `CleanupExpiredAttentionEvents` (delete dismissed events older than 7 days)
- [ ] Create `api/pkg/services/attention_service.go` — `AttentionService` struct with `EmitEvent(ctx, eventType, task, metadata)` method: looks up project name from store, builds idempotency key (`taskID:eventType:qualifier`), calls `CreateAttentionEvent`, fires Slack webhook if configured (non-blocking goroutine). Reuse `sendSlackNotification` from `janitor/utils.go` for Slack POST. Format messages with emoji per event type (📋 specs_pushed, 🛑 agent_stopped, ❌ spec_failed/implementation_failed, 🔀 pr_ready).
- [ ] Create `api/pkg/server/attention_event_handlers.go` — HTTP handlers: `GET /api/v1/attention-events` (list active events for current user), `PATCH /api/v1/attention-events/:id` (acknowledge/dismiss/snooze), `POST /api/v1/attention-events/dismiss-all` (bulk dismiss). All require auth via `getRequestUser`.
- [ ] Edit `api/pkg/server/server.go` — register the new attention event routes
- [ ] Add swagger annotations to attention event handlers, run `./stack update_openapi` to regenerate API client

## Phase 2: Backend — Emit Events at Trigger Points

- [ ] Edit `api/pkg/services/git_http_server.go` — in `processDesignDocsForBranch`, after transitioning task to `spec_review`, call `attentionService.EmitEvent(ctx, "specs_pushed", task, map{"commit": commitHash})`. Idempotency key: `taskID:specs_pushed:commitHash`
- [ ] Edit `api/pkg/services/spec_task_orchestrator.go` — in `handleImplementation`, after detecting agent is NOT running: if `task.LastPushAt != nil` (agent did work then stopped), call `attentionService.EmitEvent(ctx, "agent_stopped", task, map{"session_id": agent.SessionID})`. Idempotency key: `taskID:agent_stopped:sessionID`. Do NOT change the task status — it stays in `implementation`.
- [ ] Edit `api/pkg/services/spec_task_orchestrator.go` — wherever status transitions to `spec_failed` or `implementation_failed`, emit the corresponding attention event. Idempotency key: `taskID:spec_failed` / `taskID:implementation_failed`
- [ ] Edit `api/pkg/services/spec_task_orchestrator.go` — in `checkTaskForExternalPRActivity`, when task moves to `pull_request`, emit `pr_ready` event. Idempotency key: `taskID:pr_ready:prID`
- [ ] Add expired event cleanup to the orchestrator's periodic loop (call `CleanupExpiredAttentionEvents` once per hour)
- [ ] Write unit tests for `attention_service.go` — verify idempotency (second emit for same key is a no-op), verify Slack message format per event type, verify event creation with correct fields
- [ ] Write unit tests for `store_attention_events.go` — verify CRUD, idempotency key upsert, active-only filtering, bulk dismiss, cleanup

## Phase 3: Frontend — Attention Queue UI

- [ ] Create `frontend/src/hooks/useAttentionEvents.ts` — React Query hook: polls `GET /api/v1/attention-events?active=true` every 10s via `api.getApiClient()`, returns events sorted by `created_at` desc, exposes `acknowledge`, `dismiss`, `snooze`, `dismissAll` mutation wrappers
- [ ] Create `frontend/src/hooks/useBrowserNotifications.ts` — wraps browser `Notification` API: tracks `Notification.permission` state, `requestPermission()`, `fireNotification(title, body, onClick)`, localStorage opt-out flag (`helix_browser_notif_disabled`). Only fires for events not yet acknowledged.
- [ ] Create `frontend/src/components/system/AttentionQueue.tsx` — replaces `GlobalNotifications.tsx`:
  - `AttentionQueueButton` — bell icon + red badge count in top bar (reuse existing icon position)
  - `AttentionQueueDrawer` — MUI `Drawer` anchor="right", ~400px wide
  - `QueueHeader` — title "Needs Attention", event count, "Dismiss All" button
  - `QueueSection` — collapsible group per category: Failures (red), Agent Stopped (amber), Reviews & PRs (blue)
  - `AttentionEventItem` — event title, task name, project name, relative time ("3m ago"), dismiss button, snooze (1h) button
  - `BrowserNotificationBanner` — inline prompt when `Notification.permission === "default"`, with Enable/Dismiss buttons
- [ ] Wire browser notifications — when `useAttentionEvents` returns new unacknowledged events, call `useBrowserNotifications.fireNotification()`. Clicking browser notification focuses tab and navigates via `account.orgNavigate('project-task-detail', ...)`.
- [ ] Edit `frontend/src/components/system/Page.tsx` — replace `GlobalNotifications` import with `AttentionQueue`, render it **unconditionally** (remove the `{notifications && ...}` prop gate so queue button appears on every page)
- [ ] Delete `frontend/src/components/system/GlobalNotifications.tsx`

## Phase 4: Verification

- [ ] Test agent-stopped detection: start a task, let agent push code, stop the agent container manually, verify attention event is created and appears in the queue — and that the task status stays `implementation`
- [ ] Test specs-pushed detection: push design docs to helix-specs branch, verify `specs_pushed` event appears in queue
- [ ] Test idempotency: trigger same event twice (e.g., orchestrator polls twice while agent is stopped), verify only one event row exists
- [ ] Test Slack messages: verify format renders correctly in Slack (links, emoji, line breaks) with configured webhook
- [ ] Test drawer overlay on Kanban view, task detail page, and split screen view — verify z-index stacking and no layout disruption
- [ ] Test browser notifications in Chrome and Firefox — verify permission flow, notification content, click-to-navigate
- [ ] Test with multiple projects — verify queue aggregates events across all projects correctly
- [ ] `cd frontend && yarn build` — verify no build errors
- [ ] `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/ ./pkg/services/` — verify no build errors