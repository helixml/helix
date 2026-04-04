# Requirements: Long Session Performance in SpecTask Chat

## Problem

The SpecTask chat panel (`EmbeddedSessionView`) renders all interactions in a single DOM list. Long-running agentic sessions accumulate dozens (sometimes hundreds) of interactions, making the UI unusably slow. Additionally, scroll-to-bottom after an agent response is unreliable in some scenarios.

There are currently **two separate chat renderers** in the codebase:
- `EmbeddedSessionView` (used by SpecTask sidebar chat): no pagination, and buggy scroll-to-bottom that doesn't reliably keep the user scrolled to the bottom when the agent responds
- `Session.tsx` (used by main Helix chat + Optimus / Project Manager "New Chat" via `PreviewPanel`): has a block-based partial-render concept (`INTERACTIONS_PER_BLOCK = 20`) but still fetches all interactions from the API; scroll-to-bottom works fine

## User Stories

1. **As a user with a long spec task session**, I want the chat panel to remain responsive and fast, so I don't have to wait for a slow render every time I check progress.
2. **As a user**, I want to see the most recent messages immediately, so I don't have to scroll through history.
3. **As a user**, I want to load older messages by scrolling up or clicking a button, so I can review earlier context if needed.
4. **As a user**, I want the chat to stay scrolled to the bottom when the agent sends a new message, so I always see the latest response without manual scrolling.

## Acceptance Criteria

- Opening a SpecTask session with 50+ interactions does not cause noticeable lag or jank.
- By default, only the most recent ~20 interactions are shown (configurable constant).
- A "Load older messages" affordance at the top of the chat loads the previous page of interactions.
- Auto-scroll to bottom works reliably when a new message arrives (including during streaming).
- If the user has scrolled up to read history, auto-scroll does NOT hijack their position.
- Scroll-back-to-bottom resumes auto-scroll when user reaches the bottom.
- The fix applies consistently across SpecTask chat and any other embedded usage of the same component.
