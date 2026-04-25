# Design: Auto-clear notifications when a task is finished

## Background — what's actually in the codebase

This codebase has a fully built attention-event notification system; the user request maps cleanly onto it.

- **Model:** `api/pkg/types/attention_event.go` — `AttentionEvent` has `SpecTaskID`, `UserID`, `OrganizationID`, `EventType`, `AcknowledgedAt`, `DismissedAt`, `SnoozedUntil`.
- **Store:** `api/pkg/store/store_attention_events.go` — already has `Create`, `List`, `Update`, `BulkDismissAttentionEvents(userID, orgID)`, `CleanupExpiredAttentionEvents`. Note the `UpdateAttentionEvent(id, dismiss=true)` path already cascades a dismissal to *all* events for the same `spec_task_id` (lines 139-160) — a precedent we mirror.
- **Service:** `api/pkg/services/attention_service.go` — emits events into the store and forwards to Slack.
- **API:** `api/pkg/server/attention_event_handlers.go` — REST handlers under `/api/v1/attention-events`.
- **Frontend:** `frontend/src/hooks/useAttentionEvents.ts` polls every 10s; `frontend/src/components/tasks/TaskCard.tsx:600,723` renders the per-task red dot from `attentionEvents.length > 0`.

**Done transitions** all live in two files (grep for `TaskStatusDone` / `MergedToMain = true`):
- `api/pkg/services/git_http_server.go:1035-1049` — branch merged to main detected during a push.
- `api/pkg/services/spec_task_orchestrator.go:778-799` — all PRs across all repos merged.
- `api/pkg/services/spec_task_orchestrator.go:848-856` — branch-merge fallback poll.
- `api/pkg/services/spec_task_orchestrator.go:1080,1123` — additional `handleDone` / state-machine paths.

There is no existing per-task bulk dismissal — we add one.

## Solution

### 1. New store method
Add to `store.Store` interface (`api/pkg/store/store.go`) and implement in `store_attention_events.go`:

```go
// DismissAttentionEventsForTask marks every active (not already dismissed)
// attention event for the given spec task as dismissed. Idempotent.
// Returns the number of rows updated.
DismissAttentionEventsForTask(ctx context.Context, specTaskID string) (int64, error)
```

Implementation mirrors `BulkDismissAttentionEvents`, scoped by `spec_task_id` instead of `user_id`:

```go
result := s.gdb.WithContext(ctx).
    Model(&types.AttentionEvent{}).
    Where("spec_task_id = ? AND dismissed_at IS NULL", specTaskID).
    Update("dismissed_at", &now)
```

Also regenerate the gomock in `store_mocks.go` (`go generate ./pkg/store/...` or update by hand to match other methods).

### 2. Hook into the Done transition
We add **one helper** on `*GitHTTPServer` and `*SpecTaskOrchestrator` (or a small free function in a shared services file) that's called immediately after each `s.store.UpdateSpecTask(ctx, task)` that wrote `TaskStatusDone`:

```go
func dismissTaskNotifications(ctx context.Context, s store.Store, taskID string) {
    n, err := s.DismissAttentionEventsForTask(ctx, taskID)
    if err != nil {
        log.Warn().Err(err).Str("task_id", taskID).
            Msg("Failed to clear attention events for finished task (non-fatal)")
        return
    }
    if n > 0 {
        log.Info().Int64("dismissed", n).Str("task_id", taskID).
            Msg("Cleared attention events for finished task")
    }
}
```

Call it from each Done transition site listed above. Five call sites in two files — no other layer needs changes.

### 3. No frontend changes
`useAttentionEvents` already polls every 10 seconds and the `ListAttentionEvents` query filters out `dismissed_at IS NOT NULL`. The next poll after the Done transition naturally drops the events.

## Key decisions / tradeoffs

- **Server-side dismissal, not client-side acknowledgement.** Acknowledge sets `acknowledged_at` (clears the "new" state but keeps it visible); dismiss removes it from the active list. We want the events gone, not just marked seen — dismiss is correct.
- **No new API endpoint.** This is purely an internal side-effect of the Done transition; exposing `DismissAttentionEventsForTask` over HTTP would invite drift. Bell-panel "Dismiss all" already exists for manual cases.
- **No event hub / pubsub.** With only ~5 call sites, calling a helper directly is simpler and easier to grep than a new event channel. Don't over-engineer.
- **Best-effort, not transactional.** A DB failure on the dismiss call must not roll back the Done transition — log and move on. The next manual click or "Dismiss all" still works.
- **Scope to `Done` + `MergedToMain`, not failures.** Failed tasks (`spec_failed`, `implementation_failed`) emit a `*_failed` AttentionEvent that *is* the signal — clearing it would defeat the purpose. Done = success = nothing left to attend to.

## Notes for future agents working in this area

- **The `dismiss=true` path on `UpdateAttentionEvent` already cascades to all events for the same task** (`store_attention_events.go:139-160`). When the user clicks the red dot to dismiss, it dismisses every event for that task — useful precedent. Our new method generalises that behaviour to "any task ID" without needing to look up an event first.
- **Notifications are deduplicated by `idempotency_key`** (= `taskID|eventType|qualifier`), so re-emitting the same event after we dismiss it would *not* resurrect the dismissed row — it returns the existing dismissed row. That means once we dismiss, it stays dismissed. Good.
- **The `ListAttentionEvents` query uses `DISTINCT ON (spec_task_id)`** — only one event per task is shown in the panel, but multiple may exist in the DB. Bulk-dismissing by `spec_task_id` is the right granularity.
- **`CleanupExpiredAttentionEvents` already deletes dismissed rows** older than a configured TTL — our new dismissals will eventually be garbage-collected; no extra cleanup needed.
- **TaskCard `attentionEvents` prop** comes from the parent (Projects.tsx Kanban) which filters by `spec_task_id`. No prop wiring changes.
