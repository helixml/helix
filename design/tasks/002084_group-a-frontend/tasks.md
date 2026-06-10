# Implementation Tasks: Shared Hire-Worker / New-Role UI Across Workers, Roles, and Chart Pages

## Extract shared components

- [x] Create `frontend/src/components/helix-org/HireWorkerDrawer.tsx` by extracting `HireDrawer` from `HelixOrgChart.tsx:855-926`. Wrap the existing `<Box>` body in its own MUI `<Drawer anchor="right">` so the component owns its open/close chrome.
- [x] Add `presetRoleId?: string`, `open: boolean`, and `onClose: () => void` props to `<HireWorkerDrawer>`. When `presetRoleId` is set, render the role as read-only text (current behaviour); otherwise render a required `<TextField select>` populated from `useListHelixOrgRoles()`.
- [x] Add a `Reports to (optional)` `<TextField select>` to `<HireWorkerDrawer>` populated from `useListHelixOrgWorkers()`. Include a `(none)` option. When set, include `parent_id` in the `HireWorkerRequest` body.
- [x] Reset internal form state (`id`, `kind`, `identity`, `roleId`, `parentId`) on the `open` false → true transition via `useEffect`.
- [x] Disable the `Hire` button when `identity.trim() === ''` OR `!roleId`. Update the success toast to read `hired ${id} reporting to ${parent}` when `parent_id` was supplied; keep the existing "drag an edge…" hint otherwise.
- [x] Create `frontend/src/components/helix-org/NewRoleDialog.tsx` by extracting `CreateRoleDialog` from `HelixOrgChart.tsx:775-829` verbatim with `{ open, onClose }` props.

## Wire shared components into HelixOrgChart.tsx

- [x] Delete the inline `HireDrawer` and `CreateRoleDialog` definitions from `HelixOrgChart.tsx`.
- [x] Replace the `<CreateRoleDialog>` usage with `<NewRoleDialog open={roleDialogOpen} onClose={() => setRoleDialogOpen(false)} />`.
- [x] Replace the `<Drawer>` + inline `<HireDrawer>` block (`:1327-1339`) with `<HireWorkerDrawer open={selection.kind === 'hire'} onClose={() => setSelection({ kind: 'none' })} presetRoleId={selection.kind === 'hire' ? selection.roleId : undefined} />`.
- [x] Verify the per-role node `PersonAddOutlinedIcon` button and the floating top-right `+ New role` button still behave identically (placement, click handlers, snackbar messages).

## HelixOrgWorkers.tsx — hire button + role filter

- [x] Add `useState<boolean>` for hire drawer open state; add a primary-action `+ New Worker` button (with `<PersonAddOutlinedIcon>`) above the table, right-aligned in a header `<Stack direction="row" justifyContent="space-between">`.
- [x] Add `roleFilter` state seeded from `router.params.role`. On change, persist via `router.mergeParams({ role })` / `router.removeParams(['role'])`. Render a `<TextField select size="small" label="Role">` populated from `useListHelixOrgRoles()`, with `All roles` as the default option.
- [x] Apply the filter to the worker list before building `tableData`: `const filteredWorkers = roleFilter ? workers.filter(w => w.role_id === roleFilter) : workers`. Use `filteredWorkers` in the existing `useMemo` mapping.
- [x] Update the empty-state copy: when `workers.length === 0`, show "No workers yet." with a primary `+ New Worker` button below. When `filteredWorkers.length === 0` but `workers.length > 0`, show "No workers match this role."
- [x] Render `<HireWorkerDrawer open={hireOpen} onClose={() => setHireOpen(false)} />` at the bottom of the page (no `presetRoleId`).

## HelixOrgRoles.tsx — new role button

- [x] Add `useState<boolean>` for the new-role dialog open state; add a `+ New Role` button (top right of the page, mirroring the Workers tab pattern) inside a header `<Stack direction="row" justifyContent="space-between">`.
- [x] Render `<NewRoleDialog open={newRoleOpen} onClose={() => setNewRoleOpen(false)} />` at the bottom of the page.
- [x] Update the empty-state copy to point at the new button instead of the Chart (`"Create roles from the Chart…"` → `+ New Role` button).

## Verification

- [x] `cd frontend && yarn build` — TypeScript must compile.
- [x] Inner-Helix browser test: register / log in, create an org, navigate to Workers tab → click `+ New Worker` → verify drawer opens with Role + Reports-to selectors → hire a worker with both set → confirm the row shows the right `Role` and `Reports to`. (`hired w-alice reporting to w-owner` toast; row shows `r-engineer | w-owner`. See `screenshots/07-hire-success-with-parent.png`.)
- [x] Inner-Helix browser test: Workers tab → set Role filter → confirm table narrows and URL gains `?role=…`. Refresh the page → confirm filter survives. (URL `?role=r-engineer` persists across reload; only `w-alice` visible. `screenshots/08-role-filter-applied.png`.)
- [x] Inner-Helix browser test: Roles tab → click `+ New Role` → create a role → confirm it appears in the table. (`r-engineer` created, table row added. `screenshots/04-role-created.png`.)
- [x] Inner-Helix browser test: Chart → click per-role hire icon → confirm role is preselected and read-only, Reports-to selector is present. (Role shown as monospace static text, Reports-to dropdown present. `screenshots/10-chart-hire-drawer-preset-role.png`.)
- [x] Inner-Helix browser test: Chart → click floating `+ New role` → confirm the same dialog opens and works.

## Out of scope (do NOT include)

- Backend changes — `parent_id` is already accepted by the hire endpoint.
- Generated API client regeneration — no swagger changes.
- An in-canvas `+ New Role` affordance on the Chart (deferred, see design doc §6).
- Multi-parent selection in the hire form — set extra parents post-hire via the Chart's drag flow as today.
- Bulk hire / CSV import.
