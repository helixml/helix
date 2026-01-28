# Requirements: Inline Code Placeholder Rendering Bug

## Problem Statement

During streaming responses, users see raw placeholder text like `__INLINE_CODE_43__ensure_go()__INLINE_CODE_44__setup_dev_networking` instead of properly rendered inline code.

## User Stories

1. **As a user**, I want streaming markdown to render inline code correctly, so that I can read responses without seeing internal placeholder tokens.

2. **As a user**, I want consistent markdown rendering between streaming and final states, so that the content doesn't change appearance unexpectedly.

## Root Cause Analysis

The `sanitizeHtml()` method in `Markdown.tsx` uses a protect-and-restore pattern:
1. Replace inline code `` `code` `` with placeholders like `__INLINE_CODE_0__`
2. Run DOMPurify sanitization
3. Restore placeholders back to original inline code

**The bug**: If an LLM literally outputs text containing `__INLINE_CODE_N__` patterns (e.g., because it saw similar patterns in system prompts or documentation), the restore logic fails because those placeholders were never added to the local `inlineCode` array.

Additionally, high placeholder indices (43, 44) suggest messages with many inline code blocks, increasing collision risk.

## Acceptance Criteria

- [ ] Placeholder tokens (`__INLINE_CODE_N__`, `__CODE_BLOCK_N__`) never appear in rendered output
- [ ] Inline code renders correctly during streaming
- [ ] Inline code renders correctly in final state
- [ ] Solution handles edge case where LLM outputs text resembling placeholders
- [ ] No regression in existing markdown rendering functionality