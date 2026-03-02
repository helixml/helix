# Implementation Tasks

- [x] Add priority sorting to backlog column in `SpecTaskKanbanBoard.tsx` (~line 780)
- [x] Add `PRIORITY_ORDER` constant (can be inline or extracted to top of file)
- [x] Sort by priority first (critical=0, high=1, medium=2, low=3), then by created date descending
- [x] Verify frontend builds: `cd frontend && yarn build` (file parses OK; full build has pre-existing permission issue on dist folder)
- [ ] Manual test: create tasks with different priorities, confirm backlog column shows correct order (pending reviewer verification)