# Requirements: Fix Blinker HTML Tag Visibility During Streaming

## Problem Statement

When rendering streaming markdown content, the blinker cursor HTML (`<span class="blinker-class">┃</span>`) is sometimes displayed as literal text instead of rendering as a styled blinking cursor. This happens when the streaming content has unclosed/partial HTML tags.

## User Stories

### US-1: Clean Streaming Experience
As a user watching an AI response stream, I should see a clean blinking cursor at the end of content, never raw HTML tags like `<span class="blinker-class">┃</span>`.

## Acceptance Criteria

- [ ] Blinker renders as a styled cursor, not literal HTML text, during all streaming scenarios
- [ ] Partial/unclosed HTML tags in streaming content don't cause blinker to render as text
- [ ] Existing blinker behavior (appears during streaming, disappears when done) is preserved
- [ ] No regression in code block handling (blinker already skipped inside incomplete code blocks)
- [ ] No regression in thinking tag handling

## Root Cause Analysis

The `addBlinker()` method in `MessageProcessor` appends the blinker span after content:
```
return message + '<span class="blinker-class">┃</span>';
```

When streaming content has an unclosed HTML tag (e.g., `<div` without `>`), the blinker span gets interpreted as part of that partial tag, causing it to render as escaped text rather than HTML.

## Scope

- **In Scope**: `Markdown.tsx` MessageProcessor class, specifically the `addBlinker()` method
- **Out of Scope**: Backend streaming, WebSocket handling, other markdown rendering issues