# Design: Inline Code Placeholder Rendering Fix

## Architecture

The fix targets `frontend/src/components/session/Markdown.tsx`, specifically the `sanitizeHtml()` method in the `MessageProcessor` class.

## Problem Analysis

Current flow:
1. `sanitizeHtml()` replaces `` `code` `` → `__INLINE_CODE_0__`
2. DOMPurify runs
3. Restore `__INLINE_CODE_0__` → `` `code` ``

**Failure mode**: If LLM output contains literal `__INLINE_CODE_N__` text, step 3 can't restore it (not in array), so placeholder leaks into rendered output.

## Solution: Use UUID-Based Placeholders

Replace sequential indices with random UUIDs to eliminate collision risk:

```typescript
// Before (collision-prone)
return `__INLINE_CODE_${inlineCode.length - 1}__`;

// After (collision-resistant)  
const uuid = crypto.randomUUID();
inlineCodeMap.set(uuid, match);
return `__INLINE_CODE_${uuid}__`;
```

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Placeholder format | Keep `__INLINE_CODE_*__` pattern | Minimal change, existing restore logic works |
| ID generation | `crypto.randomUUID()` | Browser-native, no dependencies, zero collision risk |
| Data structure | `Map<string, string>` instead of array | Direct lookup by UUID key |

## Scope

- **In scope**: `sanitizeHtml()` placeholder generation for inline code and code blocks
- **Out of scope**: Slack bot (`slack_bot.go`) - different context, lower risk

## Risks

- **Low**: `crypto.randomUUID()` is widely supported (all modern browsers, Node 19+)
- **Mitigation**: If older browser support needed, fallback to `Math.random().toString(36).slice(2)`
