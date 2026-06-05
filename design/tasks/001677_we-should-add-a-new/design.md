# Design: PR Open Notification & Browser Notification Auto-Read

## Overview

Three improvements to the notification system:
1. PR-opened notification: emit `pr_ready` *immediately* when Helix creates a PR (instead of waiting for the orchestrator polling loop to detect it), and add a small `ExternalLink` button to the panel item that opens the external PR in a new tab. Body click still navigates to the in-app task page so users can inspect the task without leaving Helix.
2. Auto-acknowledging notifications when a user clicks a browser/desktop notification to navigate within Helix.
3. Fix missing `specs_pushed` notifications on the orchestrator-driven SpecReview transition (the git-push path was already correct).

### Why no new `pr_opened` event type

We initially added a separate `pr_opened` type, but it duplicated `pr_ready` — same metadata, near-identical title/description, same trigger semantics ("a PR exists for this task"). When Helix creates a PR, both would fire (the orchestrator picks it up later regardless), giving the user two notifications for one event. Reverted to a single `pr_ready` and just made it fire earlier + link externally. Idempotency on PR ID means the orchestrator's later emission collapses with the workflow handler's immediate one.

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

---

## Part 3: Fix Missing `specs_pushed` Notifications on SpecReview Transition (Orchestrator Path)

### Problem (after deeper investigation)

There are two code paths that transition a task to `SpecReview`:

1. **`api/pkg/services/spec_task_orchestrator.go` `handleSpecGeneration` (lines 545-572)** — polling loop sets status to `SpecReview` when all three spec doc fields are populated. **NO notification emitted.** This is the bug.
2. **`api/pkg/services/git_http_server.go` lines 1521-1597** — when design docs are pushed via git directly. **Already emits `specs_pushed`** via `attentionService.EmitEvent()` at line 1581.

So the original report ("neither sends a notification") was inaccurate — the git push path is fine. Only the orchestrator path is missing the emission. The dead `HandleSpecGenerationComplete` in `spec_driven_task_service.go` is a separate concern.

### Fix

Emit `AttentionEventSpecsPushed` in the orchestrator's `handleSpecGeneration` immediately after the status transition. The orchestrator already holds an `attentionService` reference (used by the `pr_ready` flow). Use the task ID as the idempotency qualifier — the orchestrator polls every loop, so we need idempotency to prevent duplicates if the polling fires multiple times before the event is observed.

### Files to Change

| File | Change |
|------|--------|
| `api/pkg/services/spec_task_orchestrator.go` | Emit `specs_pushed` after transitioning to SpecReview (`handleSpecGeneration`, ~line 568) |

---

## Key Files to Change

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `AttentionEventPROpened` constant |
| `api/pkg/services/attention_service.go` | Add title/description for `pr_opened` |
| `api/pkg/server/spec_task_workflow_handlers.go` | Emit `pr_opened` after PR creation |
| `frontend/src/hooks/useAttentionEvents.ts` | Add `'pr_opened'` to type union |
| `frontend/src/components/system/GlobalNotifications.tsx` | Icon, color, click handler, browser notification onClick |
| `api/pkg/services/spec_task_orchestrator.go` | Emit `specs_pushed` in `handleSpecGeneration` after SpecReview transition (~line 568) |
