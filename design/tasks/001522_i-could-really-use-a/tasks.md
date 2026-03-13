# Implementation Tasks

## Phase 1: Frontend — Attention Queue UI

- [ ] Create `frontend/src/hooks/useAttentionQueue.ts` — React Query hook that polls all projects' tasks, filters to human-needed statuses (`spec_review`, `implementation_review`, `pull_request`, `spec_failed`, `implementation_failed`), sorts by `status_updated_at` oldest-first, deduplicates across projects
- [ ] Create `frontend/src/hooks/useBrowserNotifications.ts` — hook wrapping the browser `Notification` API: permission state, `requestPermission()`, `fireNotification(title, body, onClick)`, localStorage opt-out flag
- [ ] Create `frontend/src/components/system/AttentionQueue.tsx` — full replacement for `GlobalNotifications.tsx`:
  - `AttentionQueueButton` — bell icon + red badge count (reuse existing icon position in top bar)
  - `AttentionQueueDrawer` — MUI `Drawer` anchor="right", ~400px wide
  - `QueueHeader` — title ("Needs Your Attention"), item count, "Mark all seen" button
  - `QueueSection` — collapsible group per urgency tier: Failures (red), Reviews (orange), PRs (blue)
  - `QueueItem` — task name, project name, status chip, relative time ("3m ago"), dismiss button, snooze (1h) button
  - `BrowserNotificationBanner` — inline prompt shown when permission is `"default"`, with Enable/Dismiss buttons
- [ ] Extend localStorage seen/snoozed tracking — store `{ taskId, status, seenAt, snoozedUntil }` per item, expire after 8h (match existing pattern), filter snoozed items from visible queue
- [ ] Wire browser notifications — when `useAttentionQueue` detects new unseen items, call `useBrowserNotifications.fireNotification()` with task title + project name; clicking notification focuses tab and navigates via `account.orgNavigate('project-task-detail', ...)`
- [ ] Update `frontend/src/components/system/Page.tsx` — replace `GlobalNotifications` import/usage with `AttentionQueue`, and render it **unconditionally** (remove the `{notifications && ...}` prop gate so the queue button appears on every page, not just `Projects.tsx`)
- [ ] Delete `frontend/src/components/system/GlobalNotifications.tsx`

## Phase 2: Backend — Slack Notifications

- [ ] Create `api/pkg/services/spec_task_human_notifier.go` — `HumanAttentionNotifier` struct with `NotifyHumanNeeded(ctx, task, newStatus)` method:
  - Look up project name from store
  - Format Slack message with emoji per status type (📋 spec review, 🔧 code review, 🔀 PR, ❌ failed)
  - Include deep link to task (`APP_URL/org/projects/{projectId}/tasks/{taskId}`)
  - POST via `sendSlackNotification()` (extract from `janitor/utils.go` or import directly)
  - Idempotency: track `taskID+status` in an in-memory set (with TTL) to avoid re-sending on orchestrator re-polls
- [ ] Edit `api/pkg/services/spec_task_orchestrator.go` — inject `HumanAttentionNotifier`, call `NotifyHumanNeeded()` at each transition to a human-needed status:
  - `handleSpecGeneration` → when moving to `spec_review`
  - `handleFeatureBranchPush` (in `git_http_server.go`) → when moving to `implementation_review`
  - Error handlers → when moving to `spec_failed` or `implementation_failed`
  - `checkTaskForExternalPRActivity` → when moving to `pull_request`
- [ ] Reuse existing `AGENT_NOTIFICATIONS_SLACK_WEBHOOK_URL` config — no new env vars; if URL is empty, notifier is a no-op
- [ ] Write unit test for `spec_task_human_notifier.go` — verify message format, emoji selection, idempotency (second call for same task+status does not send)

## Phase 3: Polish & Verification

- [ ] Test drawer overlay on Kanban view, detail page, and split screen view — ensure it doesn't break layout or z-index stacking
- [ ] Test browser notifications in Chrome and Firefox — verify permission flow, notification content, click-to-navigate
- [ ] Test Slack messages — verify format renders correctly in Slack (links, emoji, line breaks)
- [ ] Test with multiple projects — verify queue aggregates tasks from all projects correctly
- [ ] Test edge cases: task status changes while drawer is open (should update live), snooze expiry, localStorage TTL cleanup
- [ ] `cd frontend && yarn build` — verify no build errors
- [ ] `cd api && go build ./pkg/services/ ./pkg/server/` — verify no build errors