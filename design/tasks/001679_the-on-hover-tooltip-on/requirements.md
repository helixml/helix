# Requirements: Fix Notifications Panel Tooltip to Show Full Prompt

## User Story

As a user hovering over a notification in the notifications panel, I want to see the full original prompt for the task so I can understand what the task is about without having to navigate away.

## Current Behavior

The tooltip on `AttentionEventItem` (in `GlobalNotifications.tsx`) shows:
- `event.spec_task_name` — the task's short name (already truncated/short, not the original prompt)
- `event.title` — the event type label (e.g. "Spec ready", "Agent finished")

This is the same text as what's already visible in the notification card, making the tooltip redundant and unhelpful.

## Expected Behavior

The tooltip should show the **full original prompt** that the user submitted when creating the task (`SpecTask.OriginalPrompt`), so hovering gives the complete context.

## Acceptance Criteria

- [ ] Hovering a notification item shows the task's full original prompt in the tooltip
- [ ] If `original_prompt` is empty, fall back to the existing `spec_task_name` behavior
- [ ] The event title (e.g. "Spec ready") may remain as a secondary line in the tooltip
- [ ] No layout or visual regressions in the notifications panel
