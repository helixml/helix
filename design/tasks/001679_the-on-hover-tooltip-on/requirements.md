# Requirements: Fix Notifications Panel Tooltip to Show Full Prompt

## User Story

As a user hovering over a notification in the notifications panel, I want to see the **task's latest prompt in full**, untruncated, so I can understand what the task is currently about without having to navigate away.

"Latest" means: if the user has edited the prompt since the task was created, the edited text should appear — not any earlier version.

## Current Behavior

The tooltip on `AttentionEventItem` (in `GlobalNotifications.tsx`) shows:
- `event.spec_task_name` — the task's short name (already short, not the prompt)
- `event.title` — the event type label (e.g. "Spec ready", "Agent finished")

This is the same text as what's already visible in the notification card, making the tooltip redundant.

## Expected Behavior

The tooltip should show the task's current prompt text in full. In the backend, this is the field that gets rewritten whenever the user edits a task's prompt (`SpecTask.Description`). See design.md for the precise field choice and why other prompt-related fields on `SpecTask` are unsuitable.

## Acceptance Criteria

- [ ] Hovering a notification item shows the task's current prompt in the tooltip, in full, with no truncation
- [ ] If the user edits the task prompt and a new notification is emitted afterwards, the tooltip reflects the edited text — never a stale earlier version
- [ ] If the prompt field is somehow empty, fall back to `spec_task_name` (the short name)
- [ ] The event title (e.g. "Spec ready") may remain as a secondary line in the tooltip
- [ ] Long prompts wrap rather than getting cut off (tooltip already uses `whiteSpace: 'pre-wrap'`)
- [ ] No layout or visual regressions in the notifications panel
