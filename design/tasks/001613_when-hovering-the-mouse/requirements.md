# Requirements: Tooltip for Truncated Task Titles

## User Story

As a user, when I hover over a spec task title (which may be truncated in the UI), I want to see a tooltip showing the full original prompt so I can read the complete request without opening the task.

## Acceptance Criteria

1. **Kanban card title**: Hovering over the task name text on a Kanban card shows a tooltip with the full `description` or `original_prompt` (whichever is set). If neither is set, fall back to the full `name`.

2. **Split-screen tab heading**: Hovering over a tab label in the TabsView (where the task title is truncated by CSS ellipsis) shows the full original prompt in the tooltip. The tab already has a tooltip for "Topic Evolution" - when no session title history exists, it should show the original prompt instead.

3. **Multi-line support**: If the prompt contains newline characters, each line is shown on its own line in the tooltip (not collapsed to a single line).

4. **Delay**: Tooltip appears after a short hover delay (consistent with other tooltips in the app, ~500ms enter delay).

5. **No change to existing behavior** in places where the full text is already visible (e.g. the Description section in the Details tab of SpecTaskDetailContent).
