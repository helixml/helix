# Add "archive all merged" button with ta-da celebration

## Summary
Adds a celebration icon button next to the Merged column header that bulk-archives every merged task at once. On confirm, a full-screen confetti animation sweeps across the screen — completing a batch of work should feel rewarding.

## Changes
- New `frontend/src/components/tasks/TaDaAnimation.tsx`: full-screen confetti overlay (60 randomized pieces + 🎉 burst), CSS keyframes only — no new dependencies. Auto-dismisses after 2.5s.
- `SpecTaskKanbanBoard.tsx`:
  - `KanbanColumn` gains an optional `onArchiveAllMerged` prop. The button renders only when `column.id === "completed"` and the column has tasks. Follows the existing backlog auto-start button pattern.
  - New `archiveAllConfirmOpen` / `archivingAllMerged` / `showTaDa` state and a `performArchiveAllMerged` handler that calls `v1SpecTasksArchivePartialUpdate` for every merged task in parallel via `Promise.all`, then refreshes the list and pops a success snackbar.
  - New inline confirmation `Dialog` ("Archive all N merged tasks?") — separate from the existing single-task `ArchiveConfirmDialog`, which is purpose-built for one task and shows the task name.
  - `TaDaAnimation` is mounted at the bottom of the board and triggered immediately on confirm (doesn't wait for API calls — feels snappier).
- The handler is passed to both the mobile and desktop `DroppableColumn` render paths.

## Notes
- Icon: `CelebrationIcon` in amber (`#f59e0b`) — matches the celebratory tone without being too loud.
- Animation approach: pure CSS keyframes via MUI's `keyframes` helper (same as existing `pulseRing` in `TaskCard.tsx`). No `framer-motion`, no `canvas-confetti`.
- TypeScript build passes (`cd frontend && yarn build`).

## Test plan
- [ ] Verify the celebration button appears in the Merged column header only when there are merged tasks.
- [ ] Click the button, confirm in the dialog, verify all merged tasks are archived and the confetti animation plays.
- [ ] Verify Cancel dismisses the dialog without archiving.
- [ ] Verify the button is hidden when the Merged column is empty.
