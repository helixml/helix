# Implementation Tasks

- [ ] Create `TaDaAnimation.tsx` component in `frontend/src/components/tasks/` — full-screen fixed overlay with CSS keyframe confetti (colored Box elements, random positions/delays, auto-dismisses after 2.5s)
- [ ] Add `onArchiveAllMerged?: () => void` prop to `KanbanColumn` and render the celebration/archive icon button in the header when `column.id === "completed"` and `column.tasks.length > 0`, following the backlog auto-start button pattern
- [ ] Add `archiveAllConfirmOpen` state + `handleArchiveAllMerged` + `onConfirmArchiveAll` to `SpecTaskKanbanBoard`: open dialog on button click, archive all merged tasks via `Promise.all(v1SpecTasksArchivePartialUpdate)` on confirm, refresh task list
- [ ] Add the bulk-archive confirmation `Dialog` (inline MUI Dialog) to `SpecTaskKanbanBoard`'s render output, showing count of tasks to be archived
- [ ] Wire `TaDaAnimation` into `SpecTaskKanbanBoard` — show on confirm, auto-hide after 2.5s
- [ ] Pass `onArchiveAllMerged` callback down to the `KanbanColumn` rendered for the `completed` column
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors
