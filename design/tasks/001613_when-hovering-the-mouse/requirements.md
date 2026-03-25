# Requirements: Tooltip for Truncated Task Titles

## User Story

As a user, when I hover over a spec task title that may be truncated in the UI, I want to see a tooltip showing the full text so I can read the complete title or prompt without opening the task.

## Locations in Scope

Three distinct UI surfaces display task names/titles with truncation and no tooltip:

### 1. Kanban card title (TaskCard.tsx)
`task.name` is shown with `wordBreak: "break-word"` — no ellipsis, but in a narrow column it can be long. No tooltip. Should show the full `description || original_prompt || name`.

### 2. Split-screen tab heading (TabsView.tsx)
Tab labels are capped at `maxWidth: 280px` with CSS ellipsis. A MUI Tooltip already wraps each tab, but it shows "Topic Evolution" (planning session title history) or just the task `name`. When no session history is available, the tooltip should fall back to `description || original_prompt || name`.

### 3. Notifications panel (GlobalNotifications.tsx)
Two lines are truncated with CSS ellipsis and have no tooltip:
- **Event title** (`event.title`): e.g. "Agent finished" — displayed in bold, truncated
- **Task name subtitle** (`event.spec_task_name || event.spec_task_id`): displayed in a caption line below the title, also truncated

Both need a tooltip on hover showing their full text.

## Acceptance Criteria

1. Hovering the Kanban card title shows a tooltip with `description || original_prompt || name`.
2. Hovering a tab label (when no session title history exists) shows `description || original_prompt || name` in the existing tooltip.
3. Hovering a notification row's title line shows the full `event.title`.
4. Hovering a notification row's task-name subtitle shows the full `event.spec_task_name`.
5. Multi-line prompts (containing `\n`) are rendered with each line on its own line in the tooltip — use `whiteSpace: "pre-wrap"` on the tooltip text node.
6. Tooltip enter delay is ~500ms, consistent with existing tooltips in the app.
7. No changes to places where the full text is already visible (e.g. the Description section in SpecTaskDetailContent Details tab).
