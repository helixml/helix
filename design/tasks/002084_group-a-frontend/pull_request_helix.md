# feat(frontend): shared hire-worker + new-role components across helix-org pages

## Summary

Bundles three QA regressions on the helix-org UI that all live in the
same two files (`HelixOrgWorkers.tsx`, `HelixOrgChart.tsx`) and have
overlapping mechanics. Extracts the hire form and new-role dialog into
two shared components under `frontend/src/components/helix-org/`, so
Workers, Roles and the Chart canvas can all open the same drawer/dialog.

Closes
- https://github.com/helixml/helix/issues/2537
- https://github.com/helixml/helix/issues/2538
- https://github.com/helixml/helix/issues/2539

## Changes

- **`+ New Worker` button on the Workers tab** (#2537) — primary action
  in the page header and a second one in the empty state. Opens the new
  `<HireWorkerDrawer>`.
- **`Reports to` (parent_id) selector on the hire form** (#2538) —
  populated from `useListHelixOrgWorkers()`, sent as `parent_id` in the
  hire request body (`HireWorkerRequest.parent_id` was already on the
  type; only the input + body wiring were missing). Success toast
  becomes `hired X reporting to Y` when set.
- **Role filter on the Workers list** (#2539) — `<TextField select>`
  above the table, options come from `useListHelixOrgRoles()`. Selection
  persists in the URL (`?role=…`) via `router.mergeParams` /
  `router.removeParams`, so a filtered view is shareable and
  refresh-safe. Client-side filter.
- **`+ New Role` button on the Roles tab** — parity with Workers; opens
  the shared `<NewRoleDialog>`. The Chart's existing floating "New role"
  button now uses the same component.
- **Role selector in the drawer when no preset role is given** — needed
  so the Workers tab's `+ New Worker` flow can pick a role. When the
  drawer is opened from the Chart's per-role hire icon, `presetRoleId`
  is set and the field is rendered as read-only text (mirrors the prior
  Chart drawer behaviour).
- **Two new files:**
  - `frontend/src/components/helix-org/HireWorkerDrawer.tsx`
  - `frontend/src/components/helix-org/NewRoleDialog.tsx`
- **`HelixOrgChart.tsx`** drops its inline `HireDrawer` + `CreateRoleDialog`
  (and the now-unused `Drawer` / `TextField` / `MenuItem` / `Divider` /
  `CloseIcon` imports); both call sites switch to the shared components.

No backend changes, no generated API client regen.

## Test plan

- [x] `cd frontend && yarn build` — clean TypeScript build.
- [x] Inner-Helix browser test: register, create org, navigate to
      Workers → `+ New Worker` → drawer shows Role + Reports-to
      selectors → hire with both set → table row shows
      `Role=r-engineer, Reports to=w-owner` (not `—`); toast reads
      `hired w-alice reporting to w-owner`.
- [x] Inner-Helix browser test: Workers → Role filter narrows table,
      URL gains `?role=r-engineer`, filter survives a full page reload.
- [x] Inner-Helix browser test: Roles → `+ New Role` → dialog opens,
      creates role, row appears.
- [x] Inner-Helix browser test: Chart → per-role hire icon → drawer
      opens with Role rendered as read-only `r-engineer` text and
      Reports-to selector present.
- [x] Inner-Helix browser test: Chart → floating `+ New role` → same
      shared dialog opens.

## Screenshots

![Workers tab — empty with NEW WORKER button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/01-workers-tab-empty.png)

![Hire drawer from Workers tab — Role + Reports-to selectors](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/02-hire-drawer-from-workers-tab.png)

![Roles tab — NEW ROLE button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/03-roles-tab-with-new-role-button.png)

![NEW ROLE created — r-engineer row](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/04-role-created.png)

![Workers tab with role+filter row](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/05-workers-tab-with-owner.png)

![Hire drawer filled out](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/06-hire-drawer-filled.png)

![After hire — Reports-to column populated](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/07-hire-success-with-parent.png)

![Role filter applied — only r-engineer rows visible](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/08-role-filter-applied.png)

![Chart after hire — w-alice under w-owner](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/09-chart-after-hire.png)

![Chart's per-role hire icon — preset role read-only](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002084_group-a-frontend/screenshots/10-chart-hire-drawer-preset-role.png)
