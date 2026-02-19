# Implementation Tasks

- [ ] In `frontend/src/components/spec-tasks/InlineCommentBubble.tsx`, add `trimEnd()` to the `displayResponse` before splitting into lines in the `useMemo` hook (~line 38)
- [ ] Verify the fix works by testing with an agent response that ends with multiple blank lines
- [ ] Confirm expanded view still shows the full unmodified response
- [ ] Confirm streaming responses display correctly during typing