# Requirements: Kanban Board Search Should Split Multi-Word Queries

## User Story

As a user searching tasks in the Kanban board, I want multi-word searches like "github public" to match tasks containing **all** of those words (anywhere in the text, in any order), so that I can find tasks without needing to guess the exact substring.

## Problem

The current `filterTasks` function in `SpecTaskKanbanBoard.tsx` (line 826) treats the entire search input as a single literal substring via `.includes()`. Searching "github public" only matches if those two words appear consecutively in that exact order. A task titled *"How come I can only access private repos in the GitHub repository browser? ... public repos too"* contains both "github" and "public" but not the exact substring "github public", so it doesn't match. Searching just "public" works because that single word is a valid substring.

The same bug exists in `BacklogTableView.tsx` (line 87) which uses an identical `.includes()` pattern on `original_prompt`.

## Acceptance Criteria

- [ ] Searching "github public" matches any task where **all** words appear (in any field: name, description, implementation_plan), regardless of order or proximity
- [ ] Search remains case-insensitive
- [ ] Single-word searches continue to work as before (substring match)
- [ ] Numeric task number search (`/^\d+$/`) still works as before
- [ ] Empty/whitespace-only search still returns all tasks
- [ ] The same fix is applied to `BacklogTableView.tsx` so both views behave consistently
- [ ] Extra whitespace between words is handled gracefully (e.g. "github  public" treated same as "github public")
