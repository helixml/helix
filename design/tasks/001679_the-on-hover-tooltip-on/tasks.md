# Implementation Tasks

- [ ] In `api/pkg/types/attention_event.go`, add `OriginalPrompt string` field with `json:"original_prompt" gorm:"-"`
- [ ] In `api/pkg/services/attention_service.go` `EmitEvent()`, set `event.OriginalPrompt = task.OriginalPrompt` alongside the existing `SpecTaskName` assignment
- [ ] In `frontend/src/hooks/useAttentionEvents.ts`, add `original_prompt?: string` to the `AttentionEvent` interface
- [ ] In `frontend/src/components/system/GlobalNotifications.tsx`, update the `AttentionEventItem` tooltip `title` to show `event.original_prompt` (falling back to `spec_task_name` then `spec_task_id`)
- [ ] Test: create a task with a long prompt, trigger a notification event, hover the notification and verify the full prompt appears in the tooltip
