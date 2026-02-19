# Implementation Tasks

- [x] In `frontend/src/components/spec-tasks/InlineCommentBubble.tsx`, add `trimEnd()` to the `displayResponse` before splitting into lines in the `useMemo` hook (~line 38)
- [x] Verify the fix works by testing with an agent response that ends with multiple blank lines
- [x] Confirm expanded view still shows the full unmodified response
- [x] Confirm streaming responses display correctly during typing