# Requirements: Project Board "Skip Planning" UI Polish

## Problem

The project board's backlog task cards have a UX issue: a "Skip planning" toggle switch sits directly under a "Start Planning" CTA button. This is contradictory — the user sees "Start Planning" immediately above a control that says "Skip planning." It's confusing and undermines the intended default flow (planning first).

Additionally, when a task is already in the `spec_generation` phase (i.e., planning is actively running), a "Skip Planning" button still appears. This is disruptive — if planning is already underway, we should let it complete rather than prominently offering an escape hatch.

## User Stories

### US-1: Clearer backlog task action
**As a** project user viewing a task in the backlog column,
**I want** a single, clear CTA that defaults to starting planning,
**So that** I'm not confused by contradictory labels ("Start Planning" + "Skip planning").

**Acceptance Criteria:**
- The backlog task card shows a single primary CTA: **"Start Planning"** (yellow/warning color).
- There is a secondary, less prominent option to go directly to implementation — but it does NOT use a toggle/switch that modifies the meaning of the primary button.
- Planning remains the default and visually dominant action.
- The `just_do_it_mode` field is still set correctly on the backend when the user chooses to skip planning.

### US-2: Remove "Skip Planning" from active planning state
**As a** project user viewing a task that is currently in spec generation,
**I want** the "Skip Planning" button to be removed,
**So that** I don't accidentally interrupt planning that is already in progress.

**Acceptance Criteria:**
- When `task.status === "spec_generation"`, no "Skip Planning" button is rendered.
- The task card in the planning column shows no action buttons (planning is in progress — let it run).

## Out of Scope

- Changes to the backend `just_do_it_mode` field or API behavior.
- Changes to the planning column WIP limits or auto-start toggle.
- Any changes to the task detail/drawer view (only the kanban card actions are in scope).
