# Requirements: Standardise Review Spec & View PR Button Colours to Match Create Task

## Background

In the spec-task UI, the per-status action buttons ("Review Spec", "View Pull
Request") render in slightly different colours from the primary "Create Task"
button — in both light and dark mode. This looks inconsistent. They should all
use the same brand colour as the "Create Task" button.

The "Create Task" button (`NewSpecTaskForm.tsx`) uses MUI `color="secondary"`,
which is the Helix brand teal/cyan (`#0e7490` light, `#00d5ff` dark — see
`frontend/src/themes.tsx`). This is the canonical colour for the primary action.

Current divergences (all in
`frontend/src/components/tasks/SpecTaskActionButtons.tsx`):
- **Review Spec** uses `color="info"` (MUI default blue) — the obvious mismatch.
- **View Pull Request** uses a computed colour: `secondary` when the PR is open,
  but `success` (green) when merged and `inherit` (grey) when closed. The PR
  state is already shown separately by the adjacent `PRStateBadge`, so the
  state-based button colour is redundant.

## User Stories

### US-1: Consistent primary action colour
As a Helix user, I want the "Review Spec" and "View Pull Request" buttons to use
the same brand colour as the "Create Task" button, so the UI looks consistent in
both light and dark mode.

**Acceptance Criteria:**
- The "Review Spec" button uses `color="secondary"` (not `info`), in both the
  inline and stacked layouts.
- The "View Pull Request" button uses `color="secondary"` in all PR states
  (open, merged, closed), in both inline and stacked layouts.
- The colour matches the "Create Task" button in both light and dark mode.

### US-2: No loss of PR-state information
As a Helix user, I still want to see at a glance whether a PR is open, merged, or
closed.

**Acceptance Criteria:**
- PR state continues to be communicated via the existing `PRStateBadge` (and CI
  status icon) next to the button — no information is lost by removing the
  state-based button colour.

## Out of Scope
- The "Reopen" button (done phase) — keeps its current `info` outlined style.
- The legacy "Create Task" button in `AgentKanbanBoard.tsx` (defaults to
  `primary`) — not part of the spec-task action button set the user referenced.
- The pulse-glow animation behaviour on "Review Spec" stays (only its colour may
  be aligned — see design).
