# Implementation Tasks

- [~] Add priority sorting to backlog column in `SpecTaskKanbanBoard.tsx` (~line 780)
- [~] Add `PRIORITY_ORDER` constant (can be inline or extracted to top of file)
- [~] Sort by priority first (critical=0, high=1, medium=2, low=3), then by created date descending
- [ ] Verify frontend builds: `cd frontend && yarn build`
- [ ] Manual test: create tasks with different priorities, confirm backlog column shows correct order