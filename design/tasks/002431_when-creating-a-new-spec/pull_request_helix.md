# Navigate to the task detail page after creating a spec task

## Summary

Creating a spec task from the "New SpecTask" side panel used to leave you where
you were: on the kanban board the panel closed and the new card was somewhere in
Backlog for you to hunt down, and on a task detail page nothing visible happened
at all. Now creation takes you straight to the new task.

The workspace view already opened the new task as a tab, so that path is
deliberately unchanged — and creating from the workspace side panel now opens the
task as a tab too, rather than throwing away your panel layout.

## Changes

- `SpecTasksPage.handleTaskCreated` navigates to `project-task-detail` for the new
  task (kanban/audit views); in workspace mode it stays on the board route and
  passes `tab=workspace&openTask=<id>` so the task opens as a tab.
- `SpecTaskDetailPage.handleTaskCreated` navigates to the newly created task, so
  creating a follow-up task from a detail page visibly switches to it.
- Navigation is a normal history push — Back returns to the board.
- Removed the now-dead "focus the Start Planning button on the new card" plumbing
  (`focusTaskId` → `focusStartPlanning` → `startPlanningButtonRef`) across
  `SpecTasksPage`, `SpecTaskKanbanBoard`, `TaskCard`, and `SpecTaskActionButtons`.
  Nothing set it any more once we navigate away.

No API or backend changes. `NewSpecTaskForm` stays navigation-free — the
destination remains the caller's decision, which is what keeps the workspace
tab behaviour intact.

## Testing

Verified end-to-end in a dev Helix stack (browser, not just unit tests):

- Kanban → create → lands on `/orgs/…/tasks/<id>`; Back returns to the board.
- Task detail page → create → URL's `taskId` swaps to the new task, content follows.
- Workspace side panel → create → stays on `…/specs?tab=workspace&openTask=<id>`,
  task opens as a tab.
- Workspace create-tab → create → unchanged (create tab replaced by task tab).
- Create with an attachment → attachment is present on the destination page, i.e.
  navigation does not cut the upload short (`onTaskCreated` fires after the upload).
- No console errors. `yarn test`: 259 passed / 1 skipped (37 files). `yarn tsc` clean.

## Screenshots

![Create from kanban lands on the task detail page](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002431_when-creating-a-new-spec/screenshots/01-kanban-create-lands-on-detail.png)

![Create from a detail page swaps to the new task](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002431_when-creating-a-new-spec/screenshots/02-detail-page-create-swaps-to-new-task.png)

![Workspace create opens the task as a tab](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002431_when-creating-a-new-spec/screenshots/03-workspace-create-opens-tab.png)

![Attachment survives the navigation](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002431_when-creating-a-new-spec/screenshots/04-attachment-survives-navigation.png)
