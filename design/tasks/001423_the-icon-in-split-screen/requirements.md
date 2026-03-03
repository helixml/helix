# Requirements: Search/Filter Bar in Split Screen '+' Menu

## Overview

Add a search/filter bar to the '+' icon menu in split screen mode, similar to Chrome's "search tabs" feature (the down arrow in the top right). The search bar should be auto-focused when the menu opens.

## User Stories

### US-1: Filter Tasks by Search
**As a** user with many tasks in a project  
**I want to** quickly filter the task list in the '+' menu by typing  
**So that** I can find and open specific tasks without scrolling

### US-2: Auto-Focused Search
**As a** user opening the '+' menu  
**I want** the search bar to be highlighted/focused by default  
**So that** I can immediately start typing to filter

## Acceptance Criteria

1. **Search bar placement**: A search/filter text field appears at the top of the '+' menu, above "Create New Task"
2. **Auto-focus**: When menu opens, search field is automatically focused
3. **Filter behavior**: Typing filters the task list in real-time (case-insensitive)
4. **Filter scope**: Filters by task title (short_title, user_short_title, or name)
5. **No results state**: Shows "No matching tasks" when filter has no matches
6. **Clear on close**: Search query resets when menu closes
7. **Human Desktop visibility**: Human Desktop option remains visible regardless of filter (it's not a task)

## Out of Scope

- Filtering by task status, date, or other metadata
- Persisting search query across menu opens
- Keyboard navigation through filtered results (Tab/Arrow keys)
- Fuzzy matching or advanced search operators