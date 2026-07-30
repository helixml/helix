# Requirements: Navigate to Task Detail Page After Creating a Spec Task

## Background

Today, creating a spec task from the "New SpecTask" side panel leaves you exactly
where you were. On the kanban board (`SpecTasksPage`, `viewMode === "kanban"`) the
panel closes, the board refreshes, and the new card is *somewhere* in the Backlog
column — you have to hunt for it. On the task detail page (`SpecTaskDetailPage`)
the panel just closes and nothing visible happens at all, which reads as a failed
creation.

The workspace view already does the right thing: `TabsView` replaces the "create"
tab with a tab for the new task. This task brings the other two surfaces in line.

## User Stories

### US-1: Land on the new task after creating it from the board
**As a** user creating a spec task from the kanban board
**I want** the app to open the new task's detail page immediately
**So that** I can watch planning start without scanning the Backlog column for a card.

**Acceptance Criteria:**
- WHEN a task is created successfully from the New SpecTask panel on the kanban board
  THEN the app navigates to `org_project-task-detail` for the new task id.
- WHEN navigation happens THEN it is a normal history push, so the browser Back
  button returns to the board.
- WHEN the task is created with attachments THEN navigation happens *after* the
  attachment upload attempt finishes (success or handled failure), so the upload
  is not cancelled by the unmount.
- WHEN task creation fails THEN no navigation occurs and the existing error
  snackbar is shown with the form contents intact.

### US-2: Land on the new task after creating it from a task detail page
**As a** user creating a follow-up task from an existing task's detail page
**I want** to be taken to the new task's detail page
**So that** creating a task has a visible result instead of silently closing the panel.

**Acceptance Criteria:**
- WHEN a task is created from the New SpecTask panel on `SpecTaskDetailPage`
  THEN the app navigates to the detail page of the newly created task.
- WHEN the new detail page loads THEN the panel is closed and the page shows the
  new task (not the previous one).

### US-3: Workspace view keeps its tab behaviour
**As a** user working in the split-screen workspace view
**I want** a new task to open as a tab, not to throw me out of the workspace
**So that** my panel layout survives task creation.

**Acceptance Criteria:**
- WHEN a task is created from the create-tab inside `TabsView`
  THEN the existing behaviour is unchanged (create tab is replaced by a task tab).
- WHEN a task is created from the side panel while `viewMode === "workspace"`
  THEN the new task is opened as a workspace tab rather than navigating away
  (via the existing `openTask` route param).

### US-4: The detail page is usable for a brand-new task
**As a** user landing on a task that was created seconds ago
**I want** the page to render sensibly before the planning agent has named the task
**So that** the destination does not look broken.

**Acceptance Criteria:**
- WHEN the detail page opens for a task with no `name` yet
  THEN the breadcrumb falls back to "Task" and the content renders without error.
- WHEN the planning agent fills in the name/specs
  THEN the page reflects it via the existing refetch/streaming path (no new polling).

## Out of Scope

- Onboarding (`Onboarding.tsx`) — already navigates to the created task.
- `CloneTaskDialog` and the ProjectSettings "fix startup script" task — these
  create tasks by other paths and keep their current destinations.
- Any change to the task-creation API.

## Open Questions

1. **Workspace side panel** — I assumed creating from the side panel while in
   workspace mode should open a workspace tab (`project-specs?tab=workspace&openTask=<id>`)
   rather than leaving the workspace for the standalone detail page. Confirm, or
   should it navigate to the detail page like kanban does?
2. **Rapid multi-task entry** — navigating away ends the "type one task, then the
   next" flow some people use. Is that acceptable, or do you want an opt-out
   (e.g. a "stay here" checkbox / only navigate when a single task was created)?
3. **`focusTaskId` cleanup** — once we navigate away, the "auto-focus the Start
   Planning button on the new card" plumbing (`SpecTasksPage` → `SpecTaskKanbanBoard`
   → `TaskCard.focusStartPlanning`) has no remaining caller. I assumed we delete it
   per the repo's "clean up dead code" rule. Confirm it isn't wanted for something else.
4. **Just-do-it tasks** — for `just_do_it_mode` / `auto_start` tasks the detail page
   is still the destination (rather than, say, the desktop/session view). Confirm.
