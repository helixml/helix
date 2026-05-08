# Requirements: Fix Notifications Panel Tooltip to Show Full Prompt

## User Story

As a user hovering over a notification in the notifications panel, I want to see the **latest, untruncated prompt** for the task so I can understand what the task is currently about without having to navigate away.

## Current Behavior

The tooltip on `AttentionEventItem` (in `GlobalNotifications.tsx`) shows:
- `event.spec_task_name` — the task's short name (already short, not the prompt)
- `event.title` — the event type label (e.g. "Spec ready", "Agent finished")

This is the same text as what's already visible in the notification card, making the tooltip redundant.

## Expected Behavior

The tooltip should show the **task's current prompt in full** — the latest version of the prompt text, reflecting any edits the user has made. In the backend this is `SpecTask.Description` (mutable; updated by the task edit handler whenever the user modifies the prompt).

Important: this is **not** `OriginalPrompt`. `OriginalPrompt` is the immutable first-ever prompt the task was created with and would be stale after any edit. We deliberately do not use it.

## Acceptance Criteria

- [ ] Hovering a notification item shows the task's current prompt (`SpecTask.Description`) in the tooltip, in full, with no truncation
- [ ] If the user edits the task prompt and a new notification is emitted, the tooltip reflects the edited text (not a stale earlier version)
- [ ] If `description` is somehow empty, fall back to the existing `spec_task_name` (do **not** fall back to `original_prompt` — stale data is worse than the short name)
- [ ] The event title (e.g. "Spec ready") may remain as a secondary line in the tooltip
- [ ] Long prompts wrap rather than getting cut off (tooltip already uses `whiteSpace: 'pre-wrap'`)
- [ ] No layout or visual regressions in the notifications panel
