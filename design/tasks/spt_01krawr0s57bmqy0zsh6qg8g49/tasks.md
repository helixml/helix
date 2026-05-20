# Implementation Tasks: Fix Mobile Overflow on Spec Task Chat & Padding on Mobile Search Bar

- [x] Add `minWidth: 0` to the `<Box sx={{ flex: 1 }}>` wrapping `RobustPromptInput` in the mobile chat view at `frontend/src/components/tasks/SpecTaskDetailContent.tsx:2741`
- [~] Add `width: '100%', minWidth: 0` to the outer `<Box>` of `RobustPromptInput` at `frontend/src/components/common/RobustPromptInput.tsx:1142–1146`
- [ ] Add `flexWrap: 'wrap'` to the action-buttons row in `RobustPromptInput` at `frontend/src/components/common/RobustPromptInput.tsx:1424–1430`
- [ ] Add `pt: 2` and change `px: 1` → `px: 2` on the mobile search bar container at `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx:1654–1661`
- [ ] Run `cd frontend && yarn build` to confirm no TypeScript / build errors
- [ ] Verify in the inner Helix at `http://localhost:8080`: open the spec tasks list page on a narrow viewport (DevTools → device toolbar → ~360px wide), confirm the mobile search bar is no longer flush against the top and has visible side margins
- [ ] In the inner Helix, open a spec task detail page in the same narrow viewport, switch to the "Chat" tab, queue at least one message (so the queue panel above the input is visible), and confirm the queue panel and the prompt input box both sit fully within the viewport with no horizontal clipping
- [ ] Test the desktop view (≥ 900px wide) to confirm no visual regressions on the split-view layout, the chat panel, or the kanban header search input
- [ ] Capture before/after screenshots if helpful and attach to the PR
