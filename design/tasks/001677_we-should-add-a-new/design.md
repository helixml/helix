# Design: PR Open Notification & Browser Notification Auto-Read

## Overview

Two related improvements to the notification system:
1. A new `pr_opened` attention event type, triggered when Helix creates a PR, with a click action that opens the external PR URL in a new tab.
2. Auto-acknowledging notifications when a user clicks a browser/desktop notification to navigate within Helix.

---

## Part 1: `pr_opened` Notification Type

### Existing Context

- Notification types: `api/pkg/types/attention_event.go` (lines 34-42)
- Event emission service: `api/pkg/services/attention_service.go` — `EmitEvent()` takes type, task, qualifier, metadata
- Existing `pr_ready` event: emitted in `spec_task_orchestrator.go` (lines 975-992) when the monitoring loop detects an open PR; includes `pr_id` and `pr_url` in metadata
- PR creation: `api/pkg/server/spec_task_workflow_handlers.go` — `ensurePullRequestForRepo()` (lines 560-609) calls the git provider and returns PR URL; multi-provider dispatch at `api/pkg/services/git_repository_service_pull_requests.go`
- Frontend types: `frontend/src/hooks/useAttentionEvents.ts` (lines 24-29)
- Frontend panel: `frontend/src/components/system/GlobalNotifications.tsx` — `AttentionEventItem` component renders events; `handleNavigate()` (lines 400-427) drives click behavior
- Browser notifications: `frontend/src/hooks/useBrowserNotifications.ts` — `fireNotification()` (lines 90-119)

### Backend Changes

**New constant** in `api/pkg/types/attention_event.go`:
```go
AttentionEventPROpened = "pr_opened"
```

**New title/description** in `api/pkg/services/attention_service.go` switch block:
```go
case types.AttentionEventPROpened:
    title = "Pull request opened"
    description = fmt.Sprintf("A pull request was opened for %s", task.Name)
```

**Trigger location**: emit `pr_opened` in `ensurePullRequestForRepo()` (or its caller in `spec_task_workflow_handlers.go`) immediately after a PR is successfully created — alongside the existing `pr_ready` logic in the orchestrator. Use `prID` as the idempotency qualifier.

> Note: `pr_ready` (in the orchestrator) covers detection of already-open PRs (including externally-created ones). `pr_opened` is specifically for when Helix created the PR. Both events carry `pr_url` in metadata.

### Frontend Changes

**TypeScript type** — add `'pr_opened'` to the union in `useAttentionEvents.ts`.

**Icon** — add `GitPullRequest` (from lucide-react) to the `eventIcon()` switch case for `pr_opened`. Use `GitMerge` for `pr_ready` (already exists), `GitPullRequest` for `pr_opened` to visually distinguish them.

**Color** — use indigo (`#6366f1`) for `pr_opened` left border, distinct from purple (`#8b5cf6`) used by `pr_ready`.

**Click behavior** — in `handleNavigate()` (GlobalNotifications.tsx), add a case for `pr_opened`:
- Read `pr_url` from `event.metadata`
- If present, call `window.open(pr_url, '_blank', 'noopener,noreferrer')`
- If missing (edge case), fall back to navigating to the task detail page

> Consider applying the same external-link behavior to `pr_ready` events that have a `pr_url` in metadata, since they also represent a linkable PR.

**Browser notification** — in the browser notification firing logic (GlobalNotifications.tsx lines 343-387), add a `pr_opened` case with appropriate title/body. The click callback should open the external URL rather than navigating within Helix (consistent with the panel behavior).

---

## Part 2: Auto-Acknowledge on Browser Notification Click

### Existing Context

- Browser notification click: `useBrowserNotifications.ts` — `fireNotification()` accepts an `onClick` callback (line 107); the callback navigates to the task detail page
- Acknowledge mutation: `useAttentionEvents.ts` (lines 71-78) — `acknowledgeMutation.mutate(eventId)` calls `PUT /api/v1/attention-events/{id}` with `{ acknowledge: true }`
- The panel currently only acknowledges when user explicitly interacts with a notification item in the panel

### Design

When firing a browser notification for an attention event, pass an enriched `onClick` callback that:
1. Navigates to the target page (existing behavior)
2. Calls `acknowledgeMutation.mutate(eventId)` for each event ID included in that browser notification

The acknowledge mutation is already available in `GlobalNotifications.tsx` where browser notifications are fired (via `useAttentionEvents`). No new API endpoints are needed.

**Grouping consideration**: Browser notifications can group multiple events (e.g. `specs_pushed` + `agent_interaction_completed` for same task). The onClick should acknowledge all event IDs in the group, not just the first.

---

## Key Files to Change

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `AttentionEventPROpened` constant |
| `api/pkg/services/attention_service.go` | Add title/description for `pr_opened` |
| `api/pkg/server/spec_task_workflow_handlers.go` | Emit `pr_opened` after PR creation |
| `frontend/src/hooks/useAttentionEvents.ts` | Add `'pr_opened'` to type union |
| `frontend/src/components/system/GlobalNotifications.tsx` | Icon, color, click handler, browser notification onClick |
