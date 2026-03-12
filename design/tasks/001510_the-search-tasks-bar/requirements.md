# Requirements: Task Search History

## Summary

Add search history to the task search bars so users can quickly recall previous searches. History entries can be dismissed individually with an X button.

## User Stories

1. **As a user**, when I focus the task search bar, I want to see my recent searches so I can quickly re-run a previous query without retyping it.
2. **As a user**, I want to click a history entry to populate and execute that search immediately.
3. **As a user**, I want to dismiss individual history entries with an X button so I can clean up searches I no longer need.
4. **As a user**, I want my search history to persist across page reloads so I don't lose it when I navigate away.
5. **As a user**, I want search history scoped per project so different projects don't pollute each other's history.

## Acceptance Criteria

- [ ] Focusing the search bar (when empty) shows a dropdown of up to 10 recent searches, most recent first
- [ ] Submitting a search (pressing Enter or after a debounce pause) adds it to history
- [ ] Duplicate searches move to the top rather than creating a second entry
- [ ] Each history entry has a visible X button that removes it from history
- [ ] Clicking a history entry fills the search bar and filters tasks immediately
- [ ] History persists in `localStorage`, scoped by project ID
- [ ] History works in both the **Kanban board** search bar and the **Backlog table** filter bar
- [ ] The Tab picker menu search (small dropdown in TabsView) does NOT need history — it's ephemeral
- [ ] Typing in the search bar filters the history dropdown to matching entries (autocomplete behavior)
- [ ] The dropdown closes when the user clicks outside or presses Escape

## Out of Scope

- Backend storage / cross-device sync for search history
- Global unified search bar history (the `UnifiedSearchBar` component — currently commented out)
- Full-text search or fuzzy matching within history