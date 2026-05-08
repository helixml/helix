# Implementation Tasks

- [~] In `api/pkg/types/attention_event.go`, add a `SpecTaskDescription string` field with `json:"spec_task_description" gorm:"-"`
- [ ] In `api/pkg/services/attention_service.go` `EmitEvent()`, set `event.SpecTaskDescription = task.Description` alongside the existing `SpecTaskName` assignment
- [ ] In `frontend/src/hooks/useAttentionEvents.ts`, add `spec_task_description?: string` to the `AttentionEvent` interface
- [ ] In `frontend/src/components/system/GlobalNotifications.tsx`, update the `AttentionEventItem` tooltip `title` to render `spec_task_description || spec_task_name || spec_task_id` (do NOT add `original_prompt` to the chain — stale text is worse than the short name)
- [ ] Test: create a task, edit its prompt to a long new value, trigger a notification event, hover the notification and verify the tooltip shows the **edited** prompt in full
