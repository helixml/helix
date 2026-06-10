# Requirements: Shared Hire-Worker / New-Role UI Across Workers, Roles, and Chart Pages

## Background

Three QA regressions filed against `cf93e1fd` cluster on the same two
frontend files (`HelixOrgWorkers.tsx`, `HelixOrgChart.tsx`) and have
overlapping mechanics. They are bundled into one PR because a single
extraction (the hire form → shared component) unlocks all three:

- [#2537](https://github.com/helixml/helix/issues/2537) — Workers tab is
  missing a `+ New Worker` primary action button. Hire is currently only
  reachable via the per-role icon on the Chart.
- [#2538](https://github.com/helixml/helix/issues/2538) — The hire form
  has no `Reports to` (parent_id) field. The backend already accepts
  `parent_id` on `POST /workers` (`HireWorkerRequest.parent_id` exists in
  `helixOrgService.ts:59`); only the form input + body wiring are missing.
- [#2539](https://github.com/helixml/helix/issues/2539) — The Workers
  list has no Role filter; rows must be scrolled.

The Roles tab also has no `+ New Role` button today — `New role` is only
available as a floating button on the Chart canvas. Roles is brought to
parity with Workers in the same PR so the two listing pages behave
symmetrically.

## User Stories

### US-1 — Hire a worker from the Workers tab (#2537)
**As an** org operator
**I want** a `+ New Worker` button on the Workers list page
**So that** I can hire without navigating to the Chart first.

**Acceptance criteria**
- A primary-action button labelled `+ New Worker` appears in the top
  right of the Workers tab, above the table (next to the page title,
  using the same pattern as Sandboxes / Tasks list pages).
- Clicking it opens the shared hire drawer (right-anchored MUI Drawer).
- After submit, the new worker appears in the table without a manual
  refresh (mutation already invalidates `QUERY_KEYS.workers`).
- An equivalent `+ New Worker` action is available from the empty-state
  panel ("No workers yet…") so first-time hire is one click from zero
  state.

### US-2 — Hire form includes Reports-to selector (#2538)
**As an** org operator
**I want** a `Reports to` selector in the hire form
**So that** the worker is wired into the reporting graph at hire time
without a follow-up drag in the Chart.

**Acceptance criteria**
- The hire form shows a `Reports to (manager)` autocomplete/select
  populated from the current worker list, with worker id + role shown.
- Field is optional and defaults to empty; the existing post-hire
  "drag an edge from a manager" workflow remains supported.
- When set, the field is sent as `parent_id` in the `HireWorkerRequest`
  body. The Workers table's `Reports to` column then shows the manager
  id (not `—`).
- The owner worker (`w-owner`) is selectable as a parent; the worker
  being hired is, of course, not in the list (the new worker doesn't
  exist yet at form-time).

### US-3 — Hire form includes Role selector when not launched from a role
**As an** org operator hiring from the Workers tab
**I want** a `Role` selector in the hire form (preset & locked when the
form is opened from a specific role node on the Chart)
**So that** I can pick the role without having to go to the Chart.

**Acceptance criteria**
- When the drawer is opened from a Chart role node, the Role is
  pre-selected and rendered as read-only text (current behaviour).
- When opened from the Workers tab (`+ New Worker`) without a preset
  role, the form shows a required `Role` selector populated from
  `useListHelixOrgRoles()`.
- Submit is disabled until a role is selected (covers the no-preset
  case).

### US-4 — Workers list has a Role filter (#2539)
**As an** org operator
**I want** to filter the Workers list by role
**So that** I can find workers in a specific role without scrolling.

**Acceptance criteria**
- A `Role` dropdown filter sits above the table (left-aligned, on the
  same row as / inline with the `+ New Worker` button cluster).
- Options come from the live `useListHelixOrgRoles()` list, plus an
  `All roles` default option.
- Selection filters `tableData` client-side (no API change). The empty
  table state when nothing matches shows `No workers match this role.`
- Selection persists in the URL query string (`?role=r-…`) so a filter
  view is shareable / refresh-safe. This follows the `useViewMode`
  pattern (URL + localStorage) used in Sandboxes/Tasks.

### US-5 — Create a role from the Roles tab
**As an** org operator
**I want** a `+ New Role` button on the Roles list page
**So that** Roles and Workers tabs behave symmetrically and I don't have
to bounce to the Chart to create a role.

**Acceptance criteria**
- A primary-action `+ New Role` button is in the top right of the Roles
  tab, mirroring the Workers tab placement.
- Clicking opens the shared `<NewRoleDialog>` (extracted from
  `HelixOrgChart.tsx`'s `CreateRoleDialog`).
- The existing floating `New role` button on the Chart canvas continues
  to work and uses the same dialog component.
- After submit, the role appears in the table without a manual refresh
  (mutation already invalidates `QUERY_KEYS.roles`).

### US-6 — Chart keeps its in-context affordances
**As a** chart user
**I want** the existing per-role hire `+` icon to remain on each role
node (current QA passes this).
**So that** quick in-place hire on the canvas is not regressed.

**Acceptance criteria**
- The `PersonAddOutlinedIcon` button on the role node header is
  unchanged in placement and behaviour; it now opens the **shared**
  `<HireWorkerDrawer>` with the role preselected.
- The floating `+ New role` button at top-right of the Chart canvas is
  unchanged in placement and behaviour; it now opens the **shared**
  `<NewRoleDialog>`.
- Stretch (deferred, not blocking this PR): an in-canvas affordance for
  creating a new role from the Chart surface itself. Discussion in the
  task brief: "I want some sort of symbol for the role button on the
  chart as well, but I don't quite know how to do that because there's
  no place to put that button naturally." Decision: leave the existing
  top-right floating button as-is for now; revisit if a natural anchor
  emerges (e.g. a blank-canvas hint pane).

## Non-Goals

- No backend changes. `parent_id` is already accepted by the hire
  endpoint and `useListHelixOrgRoles` already exists.
- No new icons in the chart canvas for "new role" — keep the floating
  button (see US-6 stretch).
- No multi-parent support in the hire form. The DB schema supports
  many-to-many (`parent_ids[]`) but the hire endpoint takes a single
  `parent_id`; setting additional parents post-hire is already handled
  by the chart's drag flow and the worker-detail page.
- No bulk hire / CSV import.
