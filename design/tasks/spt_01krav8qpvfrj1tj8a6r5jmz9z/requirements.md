# Requirements: Restore Kanban Board Scroll Position on Return Navigation

## Background

The Kanban board lives at `org_project-specs` (`/orgs/:org_id/projects/:id/specs`,
rendered by `frontend/src/pages/SpecTasksPage.tsx` → `SpecTaskKanbanBoard.tsx`).
It has two independent scroll surfaces:

- **Horizontal**: the outer column strip (`SpecTaskKanbanBoard.tsx:1711-1735`,
  `overflowX: "auto"`).
- **Vertical**: each column body (`DroppableColumn` inner box at
  `SpecTaskKanbanBoard.tsx:567-591`, `overflowY: "auto"`).

When a user clicks a task, the board unmounts because navigation goes to a
sibling route (`org_project-task-detail`). When the user returns
(browser back, breadcrumb, or close button), the board re-mounts from scratch:

1. Scroll positions reset to `(0, 0)`.
2. React Query has cached `useSpecTasks` data, but on first render `loading` is
   true and the columns render empty (or with stale-then-fresh content) for a
   tick before the DOM has scrollable content.

This loses the user's place and is jarring when the board has many tasks.

## User Stories

### US-1 — Restore horizontal scroll position

**As a** user with many Kanban columns wider than my viewport,
**when I** click a task to see its detail and then navigate back to the board,
**I want** the column strip to be scrolled to exactly the same horizontal
position I left it at,
**so that** I don't have to re-scroll to find where I was working.

**Acceptance criteria**

- Given I have scrolled the board horizontally to a non-zero position
  and I click a task card, when the task-detail page opens and I navigate back
  (browser back, breadcrumb, close button, or any `orgNavigate` back to
  `org_project-specs`), then the outer column strip's `scrollLeft` matches the
  value at the moment I navigated away (±1 px tolerance).
- The restored position is applied **after** task data has loaded and the DOM is
  wide enough to accept it; we never try to scroll a not-yet-rendered container.
- If the restored `scrollLeft` exceeds the new `scrollWidth` (e.g. because
  columns have been deleted), it clamps to the maximum without throwing.

### US-2 — Restore per-column vertical scroll position

**As a** user reading down a long column on the Kanban board,
**when I** open a task and return,
**I want** each column's vertical scroll position preserved independently,
**so that** the column I was reading stays where I left it.

**Acceptance criteria**

- Vertical `scrollTop` is stored **per column id**, not as a single global
  value (columns scroll independently; see `SpecTaskKanbanBoard.tsx:1786-1817`).
- On return, each column with a stored `scrollTop` is restored to that value
  once its DOM content has rendered.
- Columns that did not have a stored position (e.g. newly created columns, or
  columns the user never interacted with) start at `scrollTop = 0`.
- Restoration applies on both desktop multi-column view and mobile single-column
  view (`SpecTaskKanbanBoard.tsx:1744-1779`).

### US-3 — Don't restore when the user came from elsewhere

**As a** user who hasn't been on this Kanban board recently,
**when I** open the board fresh (e.g. from the project list, a deep link, or
after switching projects),
**I want** the board to start at scroll position `(0, 0)` as today,
**so that** the restoration feature never disorients me by jumping somewhere I
have no memory of.

**Acceptance criteria**

- Stored scroll state is **scoped per `projectId`**. Returning to a *different*
  project's Kanban does not apply scroll values from another project.
- The state is in-memory only (lost on full page reload / new tab). We are
  preserving "the last place you were within this SPA session", not a permanent
  preference.

### US-4 — Don't fight the user

**As a** user already interacting with the board,
**when** restoration would cause a visible scroll jump after I have started
scrolling or clicking,
**I want** the restoration to back off,
**so that** I'm never yanked away from what I'm doing.

**Acceptance criteria**

- Restoration runs **once per mount**, immediately after the columns first
  render with data; if the user has already manually scrolled before that, the
  restoration is skipped.
- No flicker: the columns should not visibly render at `scrollTop = 0` and then
  jump. Either the scroll is applied in the same paint cycle as the content
  becoming scrollable, or the columns remain visually anchored during the
  transition (acceptable: a brief loading spinner is shown while data loads
  exactly as today, then content appears already scrolled).

## Out of Scope

- Persisting scroll position across full page reloads or new browser tabs
  (no `sessionStorage` / `localStorage`).
- Restoring scroll for the workspace or audit tab views — only the kanban view
  (`viewMode === "kanban"` in `SpecTasksPage.tsx:1172-1198`).
- Restoring scroll inside the expanded backlog table
  (`BacklogTableView`); only the standard column view is in scope.
- Restoring focus or selection of a specific card. Just the scroll containers.

## Constraints

- React Query already caches task data via `useSpecTasks` with a 3.1 s refetch
  interval (`SpecTaskKanbanBoard.tsx:1031-1076`). The cache is shared across
  mounts, so the *first render after return* may have either stale data (cache
  hit) or be in `loading` (cache miss). The solution must handle both.
- Per repo convention (`helix/CLAUDE.md`), dependency arrays must contain only
  primitives that change — do not put context values, refs, or hook objects in
  effect deps.
- No new dependencies.
