# Implementation Tasks

- [ ] In `api/pkg/store/store_attention_events.go` `UpdateAttentionEvent()`, when `update.Dismiss` is true: first fetch the `spec_task_id` and `user_id` for the given event `id`, then set `dismissed_at` on all events matching that `spec_task_id` and `user_id` instead of `WHERE id = ?`
- [ ] Manually test: create two events for the same task (e.g. two spec pushes), dismiss the notification, confirm it does not reappear after the cache invalidates
