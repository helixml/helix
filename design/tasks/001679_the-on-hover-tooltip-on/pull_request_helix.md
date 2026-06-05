# Show task prompt in notifications panel tooltip

## Summary

The on-hover tooltip on a notifications-panel item showed only `spec_task_name + event.title` — the same text already visible in the notification card. Hovering provided no extra context.

This PR carries the task's current `Description` (the live, user-editable prompt — updated whenever the user edits the prompt) through to the `AttentionEvent` record and renders it in the tooltip, so hovering surfaces the full prompt for the task.

## Changes

- `api/pkg/types/attention_event.go` — add `SpecTaskDescription string` (`gorm:"type:text"`, persisted; mirrors the existing denormalization pattern used by `SpecTaskName` / `ProjectName`)
- `api/pkg/services/attention_service.go` — populate `event.SpecTaskDescription = task.Description` in `EmitEvent()` so the value is snapshotted into each new event row
- `frontend/src/hooks/useAttentionEvents.ts` — add `spec_task_description?: string` to the `AttentionEvent` interface
- `frontend/src/components/system/GlobalNotifications.tsx` — tooltip title now renders `spec_task_description || spec_task_name || spec_task_id`

## Notes

- Deliberately uses `Description` rather than `OriginalPrompt`. `Description` is mutated by the task-edit handler on every prompt change; `OriginalPrompt` is the immutable first-ever prompt and would silently surface stale text after any user edit.
- New column added via GORM auto-migrate; no separate migration required.
- Snapshot-at-emit-time semantics: each notification carries the prompt as it was when the event fired. Subsequent edits affect future notifications, not historical ones — which matches user intuition for a notification feed.

## Testing

- `go build ./api/pkg/types ./api/pkg/services` — clean
- Frontend `tsc --noEmit` — clean
- WARNING: in-browser hover test NOT performed. The inner-Helix `helix-api-1` container is currently failing Air rebuilds on pre-existing `kodit_service.go` errors (vendored-kodit version mismatch in the container, present on `main` before this branch). Reviewer should hover a notification after deploy to confirm the full prompt now appears.
