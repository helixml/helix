# Requirements: No Search on Mobile — Spec Task View

## User Story

As a mobile user on the spec task (kanban) view, I should not see a search bar, because it takes up space on small screens and the mobile layout already handles navigation differently.

## Context

The spec task view (`SpecTasksPage`) shows a kanban board of agent tasks. On desktop, the kanban board header includes a "Search tasks..." text field. On mobile, this header is already hidden (`display: { xs: "none", md: "flex" }`).

However, the **Backlog expanded view** (`BacklogTableView` + `BacklogFilterBar`) is accessible on mobile (tap the backlog column header → expands to full-screen backlog table), and it renders a visible search field with no mobile guard.

## Acceptance Criteria

1. On mobile screens (< `md` breakpoint), no search input is visible in the spec task view.
2. The "Search prompts..." text field inside `BacklogFilterBar` is hidden on mobile.
3. The "Search tasks..." text field in the kanban board header remains working on desktop (no regression).
4. Filtering by priority in `BacklogFilterBar` may optionally remain on mobile, or be hidden too for consistency — keeping it simple, hide the whole filter bar on mobile.
5. The backlog table itself still loads and displays tasks on mobile when expanded (only the filter bar is hidden).
