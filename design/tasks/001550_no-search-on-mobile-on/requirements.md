# Requirements: Search on Mobile — Spec Task View

## User Story

As a mobile user on the spec task view (kanban board), I need to be able to search/filter tasks by name or description. Without search, it's impossible to find a specific task among many.

## Context

The spec task view (`SpecTasksPage`) shows a kanban board. On desktop, the kanban board header includes a "Search tasks..." text field. On mobile, the entire kanban header is hidden (`display: { xs: "none", md: "flex" }`), so there is currently no way to filter tasks on mobile.

## Acceptance Criteria

1. On mobile, a search input is accessible to filter tasks in the kanban view.
2. The search filters tasks across all visible kanban columns (same logic as desktop).
3. Clearing the search returns all tasks.
4. The search input fits within the compact mobile layout without obscuring the kanban column content.
5. Desktop behavior is unchanged (no regression).
