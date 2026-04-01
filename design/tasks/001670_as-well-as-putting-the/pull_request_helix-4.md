# Add red dot notification indicator on Kanban cards

## Summary
- Removes the broken orange/green "attention dot" (local-state based, never worked reliably)
- Replaces it with a small red dot (top-right of card) backed by the same `AttentionEvent` API as the notification panel
- Clicking the card acknowledges all unread `agent_interaction_completed` events for that task, dismissing both the card dot and the notification panel entry in sync

## Changes
- `TaskCard.tsx`: Remove `useAgentActivityCheck` hook and orange/green dot rendering; add `attentionEvents` prop, red dot badge, and acknowledge-on-click in `handleCardClick`; include `attentionEvents.length` in memo comparison
- `SpecTaskKanbanBoard.tsx`: Import `useAttentionEvents`, build a per-task event map filtered to unread `agent_interaction_completed` events, pass to each `TaskCard`
