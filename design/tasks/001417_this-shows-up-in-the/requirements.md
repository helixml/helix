# Requirements: Fix Blinker HTML Tag Visibility During Streaming

## Problem Statement

When rendering streaming markdown content, the blinker cursor HTML (`<span class="blinker-class">┃</span>`) is sometimes displayed as literal text in monospace font instead of rendering as a styled blinking cursor. This primarily happens when the blinker appears inside or adjacent to code blocks during streaming.

## User Stories

### US-1: Clean Streaming Experience
As a user watching an AI response stream, I should see a clean blinking cursor at the end of content, never raw HTML tags like `<span class="blinker-class">┃</span>`.

## Acceptance Criteria

- [ ] Blinker renders as a styled cursor, not literal HTML text, during all streaming scenarios
- [ ] Blinker adjacent to code blocks does not get rendered inside `<pre><code>` context
- [ ] Partial/unclosed HTML tags in streaming content don't cause blinker to render as text
- [ ] Existing blinker behavior (appears during streaming, disappears when done) is preserved
- [ ] No regression in thinking tag handling

## Root Cause Analysis

The `addBlinker()` method in `MessageProcessor` appends the blinker span to raw markdown:
```
return message + '<span class="blinker-class">┃</span>';
```

The code already skips adding the blinker when *inside* an incomplete code block (odd count of triple backticks). However, the blinker is still appearing in monospace font, indicating it's being rendered inside `<pre><code>` context.

**Primary cause**: When `react-markdown` with `rehype-raw` processes the content, the blinker span appended after a code block may be incorrectly interpreted as part of the code block's HTML output. The `rehype-raw` plugin processes raw HTML embedded in markdown, and the interaction between markdown code fences and the raw HTML blinker span appears to cause the span to end up inside the `<pre><code>` tags.

**Secondary cause**: Unclosed HTML tags (e.g., `<div` without `>`) could also cause the blinker to be misinterpreted.

## Scope

- **In Scope**: `Markdown.tsx` MessageProcessor class, specifically the `addBlinker()` method
- **Out of Scope**: Backend streaming, WebSocket handling, other markdown rendering issues