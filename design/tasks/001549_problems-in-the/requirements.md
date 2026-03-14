# Requirements: Fix Streaming Responses in Comment Bubbles

## Problem

Two visual bugs in the design review comment system when agent responses stream in:

1. **Tool calls render as raw markdown** — the `**Tool Call: name**\nStatus: status` blocks appear as plain bold text instead of collapsible tool call widgets.
2. **Thinking block is oversized** — the `ThinkingWidget` uses fixed heights appropriate for the main chat area (minHeight 120px, inner height 120px), making it far too large inside compact comment bubbles (300px wide) and the comment log sidebar.

## User Stories

**As a reviewer**, when the agent responds to my comment with tool calls (e.g., reading files, writing specs), I want to see those as collapsible tool call cards — the same as in the main chat — so I can understand what the agent did without being distracted by raw markdown syntax.

**As a reviewer**, when the agent thinks through my comment, I want the thinking block to fit neatly inside the comment bubble without overwhelming it.

## Acceptance Criteria

1. In `InlineCommentBubble`, agent responses containing `**Tool Call: <name>**\nStatus: <status>` blocks render as `CollapsibleToolCall` widgets (collapsed by default).
2. In `CommentLogSidebar`, the same tool call blocks render as `CollapsibleToolCall` widgets.
3. The `ThinkingWidget`, when rendered inside a comment bubble or sidebar, uses reduced dimensions that fit the compact context (no tall empty boxes).
4. Plain markdown text in agent responses still renders correctly with no regressions.
