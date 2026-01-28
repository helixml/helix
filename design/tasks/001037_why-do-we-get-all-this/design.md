# Design: Markdown Placeholder Syntax Collision Fix

## Architecture

The fix targets `frontend/src/components/session/Markdown.tsx`, specifically the `MessageProcessor` class and placeholder patterns used throughout.

## Problem Analysis

Current placeholder patterns use double underscores:
- `__INLINE_CODE_N__`
- `__CODE_BLOCK_N__`
- `__CITATION_DATA__...__CITATION_DATA__`
- `__THINKING_WIDGET__...__THINKING_WIDGET__`

**Failure mode**: In Markdown, `__text__` is bold syntax. When placeholders leak into content passed to react-markdown (with remarkGfm), the `__` markers are consumed as formatting, corrupting the output:
- `__CITATION_DATA__` → `CITATION_DATA` (underscores eaten by bold parsing)
- `__INLINE_CODE_43__` → mangled text

**Why placeholders leak:**
1. **CITATION_DATA**: Added after `sanitizeHtml()`, extracted in `processContent()`. Timing issues or extraction failures cause leakage.
2. **INLINE_CODE/CODE_BLOCK**: Restore loop may fail if content is modified during processing or during partial streaming chunks.

## Solution: Use Non-Markdown Placeholder Syntax

Replace double underscores with a pattern that has no meaning in Markdown:

```typescript
// Before (collides with markdown bold syntax)
`__INLINE_CODE_${index}__`
`__CODE_BLOCK_${index}__`
`__CITATION_DATA__${json}__CITATION_DATA__`
`__THINKING_WIDGET__${content}__THINKING_WIDGET__`

// After (no markdown interpretation)
`<<<INLINE_CODE_${index}>>>`
`<<<CODE_BLOCK_${index}>>>`
`<<<CITATION_DATA>>>${json}<<</CITATION_DATA>>>`
`<<<THINKING_WIDGET>>>${content}<<</THINKING_WIDGET>>>`
```

The `<<<` and `>>>` pattern:
- Has no meaning in Markdown syntax
- Visually distinct and unlikely to appear in LLM output
- Easy to search/replace with regex

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Placeholder format | `<<<TYPE>>>` | No markdown interpretation, visually distinct |
| Backward compatibility | None needed | Placeholders are internal, never persisted |
| Scope | All placeholder patterns | Consistent fix across all marker types |

## Files to Change

- `frontend/src/components/session/Markdown.tsx`:
  - `sanitizeHtml()`: Update `__CODE_BLOCK_N__` and `__INLINE_CODE_N__` patterns
  - `addCitationData()`: Update `__CITATION_DATA__` pattern
  - `processThinkingTags()`: Update `__THINKING_WIDGET__` pattern
  - `processContent()` in `InteractionMarkdown`: Update extraction patterns

## Risks

- **Low**: Simple string replacement, no logic changes
- **Testing**: Existing tests in `MessageProcessor.test.tsx` will need pattern updates