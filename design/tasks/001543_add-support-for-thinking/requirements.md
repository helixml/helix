# Requirements: Add Support for Thinking Tags in SpecTask Chat

## Background

When a Claude Code agent runs as a SpecTask agent, it produces "thinking" output — internal reasoning that Claude generates before its final response. Currently this thinking content is either displayed as raw XML-like text or dropped entirely in the SpecTask detail page chat, cluttering the UI and making it unreadable.

The existing regular session chat already has full support for `<think>...</think>` tags via `ThinkingWidget` (collapsible, with streaming timer). SpecTask needs the same treatment.

## User Stories

**As a user watching a SpecTask run**, I want thinking output from the Claude Code agent to be collapsed into a "💡 Thoughts" widget, so I can focus on the actual task output without wading through internal reasoning text.

**As a user reviewing completed tasks**, I want to be able to expand the thinking widget to see the agent's reasoning, so I can understand why the agent made certain decisions.

## Acceptance Criteria

1. When a Claude Code agent emits thinking content during a SpecTask run, it appears as a collapsible `ThinkingWidget` ("💡 Thoughts") in the chat panel — not as raw text.
2. During active streaming, the widget shows a running timer (e.g., "Thinking 0:42") and a glow effect.
3. After streaming completes, the widget is collapsed by default and expandable on click.
4. Non-thinking content (regular tool output, text) is unaffected.
5. Historical messages (already stored) that contain the thinking tag format are rendered correctly.

## Out of Scope

- Changes to how thinking is displayed in non-SpecTask sessions (already working).
- Support for thinking in interactive user-to-agent chat replies (only agent output).
