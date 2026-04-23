# Requirements: Make Task Search Work on Mobile Kanban

## Problem

The kanban board's search/filter toolbar is completely hidden on mobile viewports (`display: { xs: "none", md: "flex" }`). Users on phones cannot search or filter tasks at all — they can only scroll through the single visible column.

The search input, label filter, and assignee filter all live in a toolbar bar that is desktop-only. The `searchFilter` state is managed locally inside `SpecTaskKanbanBoard` and passed to `DroppableColumn`, so the filtering logic works — but there's no mobile UI to trigger it.

## User Stories

### US-1: Search tasks on mobile
**As a** mobile user viewing the kanban board,
**I want to** search tasks by name, description, or content,
**so that** I can quickly find specific tasks without scrolling through every column.

**Acceptance Criteria:**
- A search icon is visible on mobile kanban view
- Tapping it reveals a search input field
- Typing filters tasks across the currently visible column (same `matchesAllTokens` logic as desktop)
- A clear button dismisses the search and resets the filter
- Search persists when switching columns via the `MobileColumnSidebar`

### US-2: Visual feedback for active search
**As a** mobile user,
**I want to** see that a search filter is active,
**so that** I don't wonder why some tasks are missing.

**Acceptance Criteria:**
- When a search filter is active, a visual indicator is shown (e.g., badge on search icon, colored bar, or chip)
- The "No matching tasks" empty state appears when search yields no results (already implemented in `DroppableColumn`)
- The task count in `MobileColumnSidebar` reflects filtered results, not total

### US-3: Filter by labels/assignee on mobile (stretch)
**As a** mobile user,
**I want to** filter tasks by labels and assignees,
**so that** I can narrow down what I'm looking for beyond text search.

**Acceptance Criteria:**
- A filter button near search opens a bottom sheet or dropdown with label and assignee filters
- Filters work the same as desktop (AND logic for labels, single-select for assignee)
- Active filters show a badge/indicator

## Out of Scope

- Global search (`UnifiedSearchBar` / `GlobalSearchDialog`) — those are separate features for searching across all resource types, not task-within-kanban search
- Drag-and-drop on mobile (already not supported)
- Changing the mobile column sidebar layout
