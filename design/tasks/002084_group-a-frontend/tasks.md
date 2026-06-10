# Implementation Tasks: Shared Hire-Worker / New-Role UI Across Workers, Roles, and Chart Pages

## Extract shared components

- [~] Create `frontend/src/components/helix-org/HireWorkerDrawer.tsx` by extracting `HireDrawer` from `HelixOrgChart.tsx:855-926`. Wrap the existing `<Box>` body in its own MUI `<Drawer anchor="right">` so the component owns its open/close chrome.
- [ ] Add `presetRoleId?: string`, `open: boolean`, and `onClose: () => void` props to `<HireWorkerDrawer>`. When `presetRoleId` is set, render the role as read-only text (current behaviour); otherwise render a required `<TextField select>` populated from `useListHelixOrgRoles()`.
- [ ] Add a `Reports to (optional)` `<TextField select>` to `<HireWorkerDrawer>` populated from `useListHelixOrgWorkers()`. Include a `(none)` option. When set, include `parent_id` in the `HireWorkerRequest` body.
- [ ] Reset internal form state (`id`, `kind`, `identity`, `roleId`, `parentId`) on the `open` false → true transition via `useEffect`.
- [ ] Disable the `Hire` button when `identity.trim() === ''` OR `!roleId`. Update the success toast to read `hired ${id} reporting to ${parent}` when `parent_id` was supplied; keep the existing "drag an edge…" hint otherwise.
- [ ] Create `frontend/src/components/helix-org/NewRoleDialog.tsx` by extracting `CreateRoleDialog` from `HelixOrgChart.tsx:775-829` verbatim with `{ open, onClose }` props.

## Wire shared components into HelixOrgChart.tsx

- [ ] Delete the inline `HireDrawer` and `CreateRoleDialog` definitions from `HelixOrgChart.tsx`.
- [ ] Replace the `<CreateRoleDialog>` usage with `<NewRoleDialog open={roleDialogOpen} onClose={() => setRoleDialogOpen(false)} />`.
- [ ] Replace the `<Drawer>` + inline `<HireDrawer>` block (`:1327-1339`) with `<HireWorkerDrawer open={selection.kind === 'hire'} onClose={() => setSelection({ kind: 'none' })} presetRoleId={selection.kind === 'hire' ? selection.roleId : undefined} />`.
- [ ] Verify the per-role node `PersonAddOutlinedIcon` button and the floating top-right `+ New role` button still behave identically (placement, click handlers, snackbar messages).

## HelixOrgWorkers.tsx — hire button + role filter

- [ ] Add `useState<boolean>` for hire drawer open state; add a primary-action `+ New Worker` button (with `<PersonAddOutlinedIcon>`) above the table, right-aligned in a header `<Stack direction="row" justifyContent="space-between">`.
- [ ] Add `roleFilter` state seeded from `router.params.role` (or URL `?role=` param). On change, update both state and URL via `router.setParams({ role: value || undefined })`. Render a `<TextField select size="small" label="Role">` on the left side of the header row populated from `useListHelixOrgRoles()`, with `All roles` as the default option.
- [ ] Apply the filter to the worker list before building `tableData`: `const filteredWorkers = roleFilter ? workers.filter(w => w.role_id === roleFilter) : workers`. Use `filteredWorkers` in the existing `useMemo` mapping.
- [ ] Update the empty-state copy: when `workers.length === 0`, show "No workers yet." with a primary `+ New Worker` button below. When `filteredWorkers.length === 0` but `workers.length > 0`, show "No workers match this role."
- [ ] Render `<HireWorkerDrawer open={hireOpen} onClose={() => setHireOpen(false)} />` at the bottom of the page (no `presetRoleId`).

## HelixOrgRoles.tsx — new role button

- [ ] Add `useState<boolean>` for the new-role dialog open state; add a `+ New Role` button (top right of the page, mirroring the Workers tab pattern) inside a header `<Stack direction="row" justifyContent="space-between">`.
- [ ] Render `<NewRoleDialog open={newRoleOpen} onClose={() => setNewRoleOpen(false)} />` at the bottom of the page.
- [ ] Update the empty-state copy to point at the new button instead of the Chart (`"Create roles from the Chart…"` → `"Click + New Role to create one."`).

## Verification

- [ ] `cd frontend && yarn build` — TypeScript must compile.
- [ ] Inner-Helix browser test: register / log in, create an org, navigate to Workers tab → click `+ New Worker` → verify drawer opens with Role + Reports-to selectors → hire a worker with both set → confirm the row shows the right `Role` and `Reports to`.
- [ ] Inner-Helix browser test: Workers tab → set Role filter → confirm table narrows and URL gains `?role=…`. Refresh the page → confirm filter survives. Clear to `All roles` → URL `?role=` is removed.
- [ ] Inner-Helix browser test: Roles tab → click `+ New Role` → create a role → confirm it appears in the table.
- [ ] Inner-Helix browser test: Chart → click per-role hire icon → confirm role is preselected and read-only, Reports-to selector is present → hire → confirm worker appears with correct reporting line in the chart.
- [ ] Inner-Helix browser test: Chart → click floating `+ New role` → confirm the same dialog opens and works.

## Out of scope (do NOT include)

- Backend changes — `parent_id` is already accepted by the hire endpoint.
- Generated API client regeneration — no swagger changes.
- An in-canvas `+ New Role` affordance on the Chart (deferred, see design doc §6).
- Multi-parent selection in the hire form — set extra parents post-hire via the Chart's drag flow as today.
- Bulk hire / CSV import.
