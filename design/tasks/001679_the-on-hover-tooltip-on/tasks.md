# Implementation Tasks

- [ ] In `api/pkg/types/attention_event.go`, add `SpecTaskDescription string` and `SpecTaskOriginalPrompt string` fields, both with `gorm:"-"` and the corresponding `json` tags
- [ ] In `api/pkg/services/attention_service.go` `EmitEvent()`, set `event.SpecTaskDescription = task.Description` and `event.SpecTaskOriginalPrompt = task.OriginalPrompt` alongside the existing `SpecTaskName` assignment
- [ ] In `frontend/src/hooks/useAttentionEvents.ts`, add `spec_task_description?: string` and `spec_task_original_prompt?: string` to the `AttentionEvent` interface
- [ ] In `frontend/src/components/system/GlobalNotifications.tsx`, update the `AttentionEventItem` tooltip `title` to render `spec_task_description || spec_task_original_prompt || spec_task_name || spec_task_id`
- [ ] Test: create a task, edit its prompt to a long new value, trigger a notification event, hover the notification and verify the tooltip shows the **edited** prompt (not the original)
