# Requirements

## User Request
> The "search tasks" should be part of the URL so that when clicking back, you go back to the view with your search terms filled in and don't have to type them again. Also, when hovering over the title of the spec task, show the creation date before the prompt.

## User Story 1 — Search filter persisted in URL

**As a** user filtering the spec-task kanban board with the "Search tasks..." input
**I want** my search query to live in the URL
**So that** browser Back, page refresh, and shared links restore the same filtered view without me retyping.

### Acceptance criteria
- Typing in the kanban "Search tasks..." input updates a `search` (or similarly named) query parameter on the current URL.
- Reloading the page with `?search=foo` in the URL pre-fills the search input with `foo` and applies the filter immediately.
- Clicking Browser Back from a navigation that happened *after* I typed a search term returns me to the kanban with the term still in the input (not an empty input).
- Clearing the search input (the "x" adornment) removes the query parameter from the URL.
- The behaviour does not interfere with existing URL parameters (`tab`, `openTask`, `openDesktop`, `openReview`, `view`, `invite`, `new`).
- URL writes go through router5's `mergeParams` (the project's existing pattern) — not raw `window.history.replaceState`, which corrupts the back-stack (documented in `useUrlTab.ts`).

## User Story 2 — Tooltip shows creation date before prompt

**As a** user hovering over a task card's title on the kanban board
**I want** to see when the task was created in addition to its prompt/description
**So that** I have temporal context without opening the task detail.

### Acceptance criteria
- Hovering the title of a task card on the kanban board shows a tooltip whose first line is the task's creation date, followed by the existing prompt/description text.
- The date format is human-readable (e.g. `Created 6 May 2026, 14:32` or similar — exact format is a design choice, must be a single short line).
- If `created_at` is missing/invalid, the tooltip falls back to the current behaviour (description / name only, no broken date line).
- The 500ms enter delay and existing tooltip styling (pre-wrap, top placement, arrow) are preserved.

## Out of scope
- Persisting the *label filter* or *assignee filter* in the URL (those already live in localStorage per project — separate concern).
- Changing the search semantics (still uses `matchesAllTokens` on name/description/implementation_plan).
- Adding the date to the visible card body (only to the hover tooltip).
- The `AgentKanbanBoard` (different surface — only `SpecTaskKanbanBoard` + `TaskCard` are in scope).
