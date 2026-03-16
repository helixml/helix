# Requirements: Conversation Topic Visualization

## Background

The backend already generates per-interaction summaries (`interactions.summary`) and tracks session title evolution (`config.title_history` — a `[]TitleHistoryEntry` with `{title, changed_at, turn, interaction_id}`). The TOC API (`GET /api/v1/sessions/{id}/toc`) returns an ordered list of interaction summaries. This data is largely unused in the UI.

## User Stories

**US1 — Chapter dividers in chat**
As a user reading a long chat, I want to see visual "chapter" dividers between messages when the topic shifted, so I understand where one phase of conversation ended and another began.

**US2 — Conversation outline panel**
As a user in a long chat session, I want to open an outline/TOC sidebar that lists all the topics/chapters in this conversation, so I can quickly jump to any part.

**US3 — Topic preview in session list**
As a user looking at my session history, I want to see a topic evolution summary when I hover over or expand a session in the sidebar, so I can quickly understand what a session covers without opening it.

**US4 — Current chapter indicator**
As a user actively chatting, I want to see the current "chapter" label near the chat input or session header, so I always know what topic I'm currently in.

## Acceptance Criteria

- AC1: Topic chapter dividers appear in chat view between interactions where `title_history` records a title change. Each divider shows the new topic name and is visually distinct.
- AC2: An outline panel can be opened (toggle button, or persistent sidebar) showing numbered chapters with summaries; clicking any entry scrolls to that interaction.
- AC3: Session sidebar items show a topic chip list (up to 3–4 topics) on hover or in expanded state, sourced from `title_history`.
- AC4: The current active chapter label appears in the chat toolbar/header area, updating as the user scrolls.
- AC5: All data is fetched from existing APIs (`/toc`, session metadata); no new backend data generation is required.
