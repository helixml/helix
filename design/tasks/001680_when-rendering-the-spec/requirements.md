# Requirements: Collapsed System Prompt in Spec Task Chat

## Background

When a spec task session starts, the backend constructs the first interaction's `prompt_message` by prepending a long planning/system prompt before the user's text:

```
{planningPrompt}

**User Request:**
{userPrompt}
```

The frontend currently renders the entire `prompt_message` as the user's chat bubble, so the user sees a wall of planning instructions rather than what they actually wrote.

## User Stories

**As a user viewing a spec task's chat session**, I want to see my original request clearly, without the planning prompt cluttering the view.

**As a user**, I want to be able to expand and read the full prompt (including the system prefix) if I'm curious or debugging.

## Acceptance Criteria

1. In the spec task details page chat view, when a user message contains `**User Request:**`, the message is split at that marker.
2. The text before `**User Request:**` (the planning prompt prefix) is shown in a collapsed accordion/details section, labeled something like "System Prompt" or "Planning Instructions".
3. The text after `**User Request:**` is shown as the primary user message bubble — this is what the user actually wrote.
4. The collapsed section is collapsed by default; the user can expand it to read the full prefix.
5. If a message does not contain `**User Request:**`, it renders as before (no change in behavior).
6. This applies to both the `**User Request:**` variant and the `**Original Request (for context only...):**` variant used for cloned tasks.
