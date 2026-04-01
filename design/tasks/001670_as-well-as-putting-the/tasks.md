# Implementation Tasks

- [ ] In `SpecTaskKanbanBoard.tsx`, import `useAttentionEvents` and build a `taskAttentionEvents` map (keyed by `spec_task_id`) containing only unread `agent_interaction_completed` events
- [ ] Add `attentionEvents: AttentionEvent[]` prop to `TaskCard` (default `[]`) and import `useAttentionEvents` to get the `acknowledge` function
- [ ] In `TaskCard`, derive `hasUnreadNotification = attentionEvents.length > 0` and attach an `IntersectionObserver` (threshold 0.5) to the card root ref that calls `acknowledge` for each event on first intersection
- [ ] Render a small red dot (`Box`, 10px, `error.main`, absolute top-right) on the card when `hasUnreadNotification` is true
- [ ] Pass the mapped `attentionEvents` array to each `TaskCard` in `SpecTaskKanbanBoard`
- [ ] Verify the red dot disappears when the card is scrolled into view (React Query cache invalidation removes the event)
- [ ] Verify the notification dropdown and card red dot stay in sync (acknowledging in either place clears both)
