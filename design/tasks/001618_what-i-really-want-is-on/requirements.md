# Requirements: Navigation History Dropdown on Kanban Board

## User Stories

**US-1:** As a user, I want a history dropdown button just above the Kanban board columns (right-hand side) so I can quickly jump to recently visited pages without using the browser's back button.

**US-2:** As a user, I want the history list de-duplicated by URL (most recent visit wins) so I don't see the same page listed multiple times.

**US-3:** As a user, I want the list sorted most-recent-first so the pages I visited most recently appear at the top.

**US-4:** As a user, the feature should work entirely client-side (no backend changes needed).

## Acceptance Criteria

- [ ] A small down-arrow icon button (similar in appearance to Chrome's download/history button) appears on the right side of the Kanban board header bar, just above the columns.
- [ ] Clicking the button opens a dropdown menu listing recently visited pages, most recent first.
- [ ] Each entry shows a readable page title (e.g., "Task: Add login page", "Design Review: auth-flow").
- [ ] Duplicate URLs are collapsed — only the most recent visit is shown.
- [ ] Clicking a history entry navigates to that page using the router.
- [ ] History is stored in `localStorage` so it persists across page refreshes (but is per-browser).
- [ ] History tracks at most 30 entries (after dedup).
- [ ] The feature works for at minimum: spec task detail pages and design review pages.
- [ ] The dropdown closes when a selection is made or when clicking outside.
