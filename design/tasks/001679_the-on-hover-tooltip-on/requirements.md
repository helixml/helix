# Requirements: Fix Notifications Panel Tooltip to Show Full Prompt

## User Story

As a user hovering over a notification in the notifications panel, I want to see the **current full prompt** for the task so I can understand what the task is about without having to navigate away.

## Current Behavior

The tooltip on `AttentionEventItem` (in `GlobalNotifications.tsx`) shows:
- `event.spec_task_name` — the task's short name (already truncated/short, not the prompt)
- `event.title` — the event type label (e.g. "Spec ready", "Agent finished")

This is the same text as what's already visible in the notification card, making the tooltip redundant.

## Expected Behavior

The tooltip should show the **task's current full prompt** — i.e. the latest, possibly user-edited prompt text. This is stored as `SpecTask.Description` in the backend (mutable; updated whenever the user edits the prompt). It is NOT `OriginalPrompt`, which is the immutable first-ever prompt and may be stale.

## Acceptance Criteria

- [ ] Hovering a notification item shows the task's current full prompt (`SpecTask.Description`) in the tooltip
- [ ] If the user edits the task prompt, the next notification's tooltip reflects the edited text
- [ ] If `description` is empty, fall back to `original_prompt`, then `spec_task_name`, then `spec_task_id` (matches existing frontend fallback chain)
- [ ] The event title (e.g. "Spec ready") may remain as a secondary line in the tooltip
- [ ] No layout or visual regressions in the notifications panel
