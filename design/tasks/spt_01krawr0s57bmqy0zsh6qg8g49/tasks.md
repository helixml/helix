# Implementation Tasks: Fix Mobile Overflow on Spec Task Chat & Padding on Mobile Search Bar

- [ ] Add `minWidth: 0` to the `<Box sx={{ flex: 1 }}>` wrapping `RobustPromptInput` in the mobile chat view at `frontend/src/components/tasks/SpecTaskDetailContent.tsx:2741`
- [ ] Add `width: '100%', minWidth: 0` to the outer `<Box>` of `RobustPromptInput` at `frontend/src/components/common/RobustPromptInput.tsx:1142–1146`
- [ ] Add `flexWrap: 'wrap'` to the action-buttons row in `RobustPromptInput` at `frontend/src/components/common/RobustPromptInput.tsx:1424–1430`
- [ ] Tighten the keyboard-hint cue row spacing in `RobustPromptInput` at `frontend/src/components/common/RobustPromptInput.tsx:1625–1634` — change `gap: 2` to `gap: { xs: 1, sm: 2 }` and add `rowGap: { xs: 0.5, sm: 1 }`
- [ ] Add `pt: 2` and change `px: 1` → `px: 2` on the mobile search bar container at `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx:1654–1661`
- [ ] Run `cd frontend && yarn build` to confirm no TypeScript / build errors
- [ ] Verify in the inner Helix at `http://localhost:8080`: open the spec tasks list page on a narrow viewport (DevTools → device toolbar → ~360px wide), confirm the mobile search bar is no longer flush against the top and has visible side margins
- [ ] In the inner Helix, open a spec task detail page in the same narrow viewport, switch to the "Chat" tab, and confirm the prompt input box and the keyboard-hint cue row sit fully within the viewport with no horizontal clipping
- [ ] Test the desktop view (≥ 900px wide) to confirm no visual regressions on the split-view layout, the chat panel, or the kanban header search input
- [ ] Capture before/after screenshots if helpful and attach to the PR
