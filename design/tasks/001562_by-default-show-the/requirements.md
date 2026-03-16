# Requirements: Show User Prompt + Spec as Primary Content

## Problem Statement

When a user opens an issue (clicks on its title in the task list), the left panel currently shows the full session chat thread. The first message in that thread is the raw system prompt — a long, technical blob of agent instructions — with the user's actual request appended at the end. This is confusing and buries the most valuable content.

The user wants:
1. **Their original request** to be immediately visible and prominent
2. **The spec** (requirements.md, design.md, tasks.md) to be the hero content — not buried behind a chat thread
3. The system prompt to be **collapsed** by default, accessible only if the user is curious

## User Stories

**US1 – Immediate clarity on what the task is about**
> As a user opening an issue, I want to immediately see what I asked for (my original prompt) and what the agent planned, so I can quickly understand the task without scrolling past a wall of system instructions.

**US2 – Spec is the star**
> As a user reviewing a task, I want the spec (requirements, design, tasks checklist) to be front and center when I open it, since the spec is the most important artifact produced during planning.

**US3 – System prompt is accessible but not intrusive**
> As a user who is curious about the agent's instructions, I want to be able to expand a collapsed section to see the system prompt, but I don't want it taking up prime real estate by default.

**US4 – Consistent behavior across task states**
> As a user, the improved layout should work whether the task is in backlog, planning, spec_review, implementation, or done states.

## Acceptance Criteria

- [ ] When opening a task detail view, the **left/main content area** shows:
  1. The user's original prompt (from `task.original_prompt` or `task.description`) at the top, prominently styled
  2. The spec documents (requirements.md, design.md, tasks.md) rendered as markdown below the prompt
- [ ] The first chat interaction's system prompt content is **collapsed** by default in the session view
- [ ] A clearly labelled collapsible section ("System Prompt" or "Agent Instructions") allows expanding to view the full system prompt
- [ ] The user's actual prompt_message (the non-system-prompt part of the first interaction) remains visible in the chat thread as before
- [ ] The spec content is rendered from the existing `v1SpecTasksDocumentsDetail` API endpoint
- [ ] The layout defaults to showing the spec view on the right panel when a spec is available (instead of desktop view) for tasks that have completed planning
- [ ] Existing functionality (desktop view, file diff, details, chat) is preserved; this is an enhancement to the default state
