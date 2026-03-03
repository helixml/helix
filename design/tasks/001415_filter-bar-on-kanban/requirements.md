# Requirements: Filter Bar Task Number Search

## Overview
The kanban board filter should support searching by task number without requiring leading zeros.

## User Stories

### US-1: Search by Partial Task Number
**As a** user viewing the kanban board  
**I want to** type a task number like "1412" in the search box  
**So that** I can quickly find task #001412 without typing the full zero-padded format

## Acceptance Criteria

### AC-1: Numeric Search Matches Task Numbers
- Given the filter contains only digits (e.g., "1412")
- When the filter is applied
- Then tasks whose `task_number` matches that number should appear in results
- And existing text matching (name, description, implementation_plan) should continue to work

### AC-2: Zero-Padded Format Also Works
- Given the filter contains "001412" (full zero-padded format)
- When the filter is applied
- Then task #001412 should appear in results

### AC-3: Text Search Still Works
- Given a task name or description contains "1412" as text (e.g., "Fix bug 1412")
- When the user searches "1412"
- Then that task should appear in results (via text matching)
- And task #001412 should also appear (via numeric matching)

## Out of Scope
- Changing how task numbers are displayed
- Adding a separate "task number" search field
- Regex or advanced search syntax