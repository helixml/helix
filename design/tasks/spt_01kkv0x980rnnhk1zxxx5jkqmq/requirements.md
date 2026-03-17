# Requirements

## Problem

When a user opens a task (by clicking its title), the left panel shows `EmbeddedSessionView` — which renders all chat interactions starting with the first one. That first interaction contains the full system prompt concatenated with the user's original request, making it a wall of mostly irrelevant text. The spec (requirements, design, tasks) is buried under a "Details" tab in the right panel.

The spec is the single most important artefact — it is what the user cares about.

## User Stories

**US-1**: As a user opening a task, I want to immediately see what I asked for (my original prompt) and what the agent planned (the spec), so I can quickly understand the task at a glance without scrolling past a system prompt.

**US-2**: As a user, I want the system prompt in the chat to be collapsed by default, so it doesn't dominate the view — but I can expand it if I'm curious.

**US-3**: As a user, I want the chat, desktop, file changes, and task details to remain accessible via tabs/panel, just not as the primary focus.

## Acceptance Criteria

- WHEN I open a task, the left panel shows: my original prompt prominently at the top, followed by the spec documents (requirements, design, tasks) rendered as markdown tabs.
- WHEN the spec has not yet been generated, the left panel shows only the original prompt with a status note.
- WHEN I view the chat, the first interaction (system prompt) is collapsed/hidden by default, showing only a one-line "System prompt (collapsed)" toggle.
- WHEN I click "Show system prompt", the full first interaction expands.
- WHEN a session is active on desktop, the right panel contains tabs for: Chat | Desktop | Changes | Details.
- WHEN there is no active session, the layout falls back to a single panel (current mobile/no-session behaviour).
