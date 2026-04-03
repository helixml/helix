# Implementation Tasks

- [ ] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx`, replace `applyHighlight()` (line 800): swap `extractContents()` + `insertNode()` with `range.surroundContents(mark)` — set `highlightMarkRef.current = null` in the catch block since no mark was applied
- [ ] Manually test: select text inside a bullet point list on the spec review page — confirm blue highlight appears without extra list items
- [ ] Manually test: submit the comment — confirm list structure is restored correctly after `removeHighlight()` runs
- [ ] Manually test: select text that spans two list items — confirm no DOM corruption and comment form still opens
