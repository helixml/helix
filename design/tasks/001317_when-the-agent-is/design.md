# Design: Fix Empty Truncated Response in Comment Bubbles

## Overview

Simple bug fix in `InlineCommentBubble.tsx` to filter empty lines before truncating for the collapsed view.

## Current Behavior

```typescript
const lines = displayResponse.split('\n')
const lastLines = lines.slice(-COLLAPSED_LINES).join('\n')
```

If response ends with `\nSome text\n\n\n`, the last 4 lines are mostly empty.

## Solution

Trim trailing whitespace from the response before splitting into lines:

```typescript
const trimmedResponse = displayResponse.trimEnd()
const lines = trimmedResponse.split('\n')
```

This is the minimal change that fixes the problem. The response often has trailing newlines from markdown formatting, and `trimEnd()` removes them before we count/slice lines.

## Alternative Considered

Filter to only non-empty lines: `lines.filter(l => l.trim())`. Rejected because this would alter the visual structure of code blocks and formatted content that may intentionally include blank lines within the content.

## Files Changed

- `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` - Add `trimEnd()` before splitting lines

## Implementation Notes

- The fix is on line 50: `displayResponse.trimEnd().split("\n")` 
- This trims trailing whitespace/newlines before counting and slicing lines
- The expanded view still shows the full `displayResponse` (unmodified), only the truncated view uses the trimmed version for line counting
- Streaming responses work correctly because `trimEnd()` is applied fresh on each render via `useMemo`
- Build verified with `yarn build` - no errors