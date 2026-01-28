# Requirements: Markdown Placeholder Syntax Collision Bug

## Problem Statement

During streaming responses, users see corrupted placeholder text in rendered markdown:
- `__INLINE_CODE_43__ensure_go()__INLINE_CODE_44__setup_dev_networking` instead of properly rendered inline code
- `CITATION_DATA{"excerpts":[],"isStreaming":false}CITATION_DATA` appearing as visible text

## User Stories

1. **As a user**, I want streaming markdown to render inline code correctly, so that I can read responses without seeing internal placeholder tokens.

2. **As a user**, I want citation metadata to be invisible, not displayed as raw text in the response.

## Root Cause Analysis

The `MessageProcessor` class in `Markdown.tsx` uses placeholder tokens with double underscores:
- `__INLINE_CODE_N__` for protecting inline code during sanitization
- `__CITATION_DATA__...__CITATION_DATA__` for passing citation metadata to the React component
- `__CODE_BLOCK_N__` for protecting code blocks
- `__THINKING_WIDGET__...__THINKING_WIDGET__` for thinking content

**The bug**: In Markdown syntax, `__text__` renders as **bold** (`<strong>text</strong>`). When these placeholders leak into content passed to react-markdown (with remarkGfm), the double underscores are consumed as bold formatting syntax instead of being treated as literal text.

This explains:
- `__CITATION_DATA__` becomes `CITATION_DATA` - underscores consumed as bold markers
- `__INLINE_CODE_43__` gets mangled - markdown tries to parse `__` as formatting

**Why placeholders leak:**
1. **CITATION_DATA**: Added AFTER `sanitizeHtml()`, then extracted in `processContent()`. If extraction fails or has timing issues, markers pass through to react-markdown.
2. **INLINE_CODE**: Restore loop in `sanitizeHtml()` may fail if DOMPurify modifies placeholders or during partial streaming content.

## Acceptance Criteria

- [ ] Placeholder tokens never appear in rendered output (even when leaking occurs)
- [ ] Inline code renders correctly during streaming and final state
- [ ] Citation data markers are invisible to users
- [ ] Solution uses placeholder syntax that cannot be interpreted as markdown formatting
- [ ] No regression in existing markdown rendering functionality