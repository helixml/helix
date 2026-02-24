# Requirements: Kanban Board Search Filter

## Overview

Add a search/filter input to the SpecTasksPage (kanban board) that filters tasks across title, description, and implementation tasks. This is a client-side filter since all task data is already loaded in the frontend.

## User Stories

1. **As a user**, I want to quickly find a specific task on the kanban board by typing part of its name, so I can navigate to it without scrolling through all columns.

2. **As a user**, I want to search task descriptions, so I can find tasks related to a specific feature or bug even if I don't remember the exact title.

3. **As a user**, I want to search implementation plan content (tasks checklist), so I can find which task contains a specific piece of work.

## Acceptance Criteria

- [ ] Search input appears in the SpecTasksPage topbar (visible in kanban view)
- [ ] Filter is applied client-side with no additional API calls
- [ ] Matches against: `name`, `description`, and `implementation_plan` fields
- [ ] Search is case-insensitive
- [ ] Empty search shows all tasks (current behavior)
- [ ] Filtered tasks remain in their respective columns (backlog, planning, review, etc.)
- [ ] Clear button (X) to reset the search
- [ ] Filter state persists across view mode changes (kanban ↔ workspace) within the same session
- [ ] Search input has appropriate placeholder text (e.g., "Search tasks...")
- [ ] Visual feedback when no tasks match the search (each column shows "No matching tasks" or similar)

## Out of Scope

- Server-side search/filtering
- Search history or saved searches
- Advanced query syntax (AND/OR operators)
- Highlighting matched text in results