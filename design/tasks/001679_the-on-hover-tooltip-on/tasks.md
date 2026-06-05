# Implementation Tasks

- [x] In `api/pkg/types/attention_event.go`, add a `SpecTaskDescription string` field with `json:"spec_task_description,omitempty" gorm:"type:text"` (persisted, mirrors existing `Description` column)
- [x] In `api/pkg/services/attention_service.go` `EmitEvent()`, set `event.SpecTaskDescription = task.Description` alongside the existing `SpecTaskName` assignment
- [x] In `frontend/src/hooks/useAttentionEvents.ts`, add `spec_task_description?: string` to the `AttentionEvent` interface
- [x] In `frontend/src/components/system/GlobalNotifications.tsx`, update the `AttentionEventItem` tooltip `title` to render `spec_task_description || spec_task_name || spec_task_id` (do NOT add `original_prompt` to the chain — stale text is worse than the short name)
- [x] Test: create a task, edit its prompt, trigger a notification event, hover the notification and verify the tooltip shows the **edited** prompt in full
  - WARNING: NOT tested in browser — `helix-api-1` Air rebuild blocked by pre-existing `kodit_service.go` errors on main (vendored-kodit module mismatch in the container; not introduced by this branch). Code-level verification done: Go services package and frontend `tsc --noEmit` both pass. Reviewer should hover a notification post-deploy to confirm.
