# Design: Shared Hire-Worker / New-Role UI Across Workers, Roles, and Chart Pages

## Files Touched

- `frontend/src/pages/HelixOrgWorkers.tsx` — add header action button,
  role filter, wire shared drawer.
- `frontend/src/pages/HelixOrgRoles.tsx` — add `+ New Role` header
  button, wire shared dialog.
- `frontend/src/pages/HelixOrgChart.tsx` — replace inline `HireDrawer`
  and `CreateRoleDialog` with imports from the shared component.
- `frontend/src/components/helix-org/HireWorkerDrawer.tsx` — **new**
  shared component (extracted from `HelixOrgChart.tsx`).
- `frontend/src/components/helix-org/NewRoleDialog.tsx` — **new** shared
  component (extracted from `HelixOrgChart.tsx`).

`frontend/src/components/helix-org/` already exists and is where
helix-org-specific shared widgets live (see `GitHubAppPanel.tsx`,
`GitHubRepoPicker.tsx`). That's the natural home for these two.

## Component: `<HireWorkerDrawer>`

Lifts the existing `HireDrawer` from `HelixOrgChart.tsx:855-926` into a
shared component and grows two new fields.

### Props
```ts
type HireWorkerDrawerProps = {
  open: boolean
  onClose: () => void
  // When provided (Chart use): role is locked and shown as read-only.
  // When omitted (Workers tab use): a Role selector is rendered and is
  // required before submit is enabled.
  presetRoleId?: string
}
```

A single `open + onClose` prop pair, MUI `<Drawer anchor="right">`
rendered inside the component (not by the parent), keeps the API
ergonomic. Parents own one `useState<boolean>` and pass `open`.

### Internal state
```ts
const [id, setId] = useState('')
const [kind, setKind] = useState<'human' | 'ai'>('human')
const [identity, setIdentity] = useState('')
const [roleId, setRoleId] = useState(presetRoleId ?? '')
const [parentId, setParentId] = useState<string>('')
```

Resets on `open` transitioning from false → true (so reopening shows a
clean form, not stale state).

### Data sources
- `useListHelixOrgRoles()` — for the Role selector when `presetRoleId`
  is absent. Already exists in `helixOrgService.ts:208`.
- `useListHelixOrgWorkers()` — for the Reports-to selector. Already
  exists. Options rendered as `{w.id} — {w.role_id ?? '(no role)'}`.
- `useHireHelixOrgWorker()` — mutation; unchanged.

### Form layout (top → bottom)
1. `Role` — if `presetRoleId`: caption + monospace text, read-only
   (matches current Chart drawer). Otherwise: `<TextField select>` with
   roles list, required.
2. `Kind` — `<TextField select>` Human / AI. Unchanged.
3. `Handle (optional)` — text input. Unchanged.
4. **`Reports to (optional)`** — `<TextField select>` populated from
   workers; first option `(none)`. When chosen, sent as `parent_id`.
5. `Identity content` — multiline. Unchanged.
6. `Hire` / `Cancel` buttons.

### Submit
```ts
const body: HireWorkerRequest = {
  role_id: roleId,
  kind,
  identity_content: identity,
}
if (id.trim()) body.id = id.trim()
if (parentId) body.parent_id = parentId
await hire.mutateAsync(body)
```

`parent_id` is already on `HireWorkerRequest` (see
`helixOrgService.ts:59`), so no service-layer change is needed.

Submit is disabled when:
- `identity.trim() === ''` (current rule), OR
- `!roleId` (new rule when no preset role).

Toast text drops the "drag an edge from a manager…" hint when
`parent_id` was supplied (it's no longer accurate); otherwise unchanged.

## Component: `<NewRoleDialog>`

Lifts `CreateRoleDialog` from `HelixOrgChart.tsx:775-829` essentially
verbatim. Props:
```ts
type NewRoleDialogProps = { open: boolean; onClose: () => void }
```
No behavioural changes — it's a straight move so all three callers
share the same code path.

## Changes to `HelixOrgWorkers.tsx`

### Header row (above the table)
```tsx
<Stack direction="row" justifyContent="space-between" alignItems="center">
  <TextField
    select
    size="small"
    label="Role"
    value={roleFilter}
    onChange={(e) => setRoleFilter(e.target.value)}
    sx={{ minWidth: 200 }}
  >
    <MenuItem value="">All roles</MenuItem>
    {roles.map(r => <MenuItem key={r.id} value={r.id}>{r.id}</MenuItem>)}
  </TextField>
  <Button
    variant="contained"
    startIcon={<PersonAddOutlinedIcon />}
    onClick={() => setHireOpen(true)}
  >
    New Worker
  </Button>
</Stack>
```

Filter state lives in a small URL-sync hook (mirrors the pattern in
`useViewMode`): seed `roleFilter` from `router.params.role` (or
`URLSearchParams`), and on change call `router.setParams({ role: value
|| undefined })`. localStorage is not needed here — Workers/Roles
filters are session-y, and the URL alone gives the shareability win.

Existing empty state ("No workers yet. Hire one from the chart.")
becomes "No workers yet." with a primary `+ New Worker` button below
it. A new no-match state ("No workers match this role.") shows when
`roleFilter` is set but `filteredWorkers.length === 0`.

### Filtering
Apply once in the `useMemo` that builds `tableData`:
```ts
const filteredWorkers = useMemo(
  () => roleFilter ? workers.filter(w => w.role_id === roleFilter) : workers,
  [workers, roleFilter],
)
```
Client-side; no API change. The backend's `GET /workers` returns all
workers in the org and the list is small (~ tens to low hundreds in
realistic orgs), so a client filter is the right altitude.

### Drawer
```tsx
<HireWorkerDrawer open={hireOpen} onClose={() => setHireOpen(false)} />
```
No `presetRoleId` from this page — the user picks the role inside the
form. (If `roleFilter` is set, we could optionally seed `presetRoleId`
with it; deferred — opening "New Worker" with a different role
preselected than the current filter is more surprising than helpful.)

## Changes to `HelixOrgRoles.tsx`

Same shape: add a top-right `+ New Role` button that toggles a
`useState<boolean>` for the dialog. No filter on this page — there's
nothing to filter Roles by yet.

## Changes to `HelixOrgChart.tsx`

- Delete the inline `HireDrawer` function (`:855-926`) and
  `CreateRoleDialog` (`:775-829`).
- Replace usage at `:1313` with `<NewRoleDialog open={…} onClose={…} />`.
- Replace usage at `:1327-1339` with `<HireWorkerDrawer
  open={selection.kind === 'hire'} onClose={…} presetRoleId={selection.roleId} />`.
  The `Selection` discriminated union can either stay as-is (the
  shared component reads `presetRoleId` only when the drawer is open)
  or simplify to `useState<string | null>(null)` for the active role —
  preference is to keep `Selection` for symmetry with future
  drawer kinds (e.g. when the chart eventually grows other in-canvas
  panels).
- Per-role node hire icon and floating "New role" button: unchanged
  in placement; both now open the shared component.

## Decisions & Trade-offs

1. **Shared component vs prop-driven composition.** Two small
   components (one drawer, one dialog) with three call sites is the
   simplest reuse. A single mega-component with a `mode` prop was
   considered and rejected — the dialog and the drawer have different
   chrome (MUI Dialog vs MUI Drawer) and different content; sharing
   them would force conditional rendering with no shared logic.

2. **Role selector populated from `useListHelixOrgRoles` vs a free-text
   field.** Selector — hire is a structured action and we already know
   the live role set. A free-text field would let an operator type a
   role that doesn't exist, and the backend would reject it with a
   confusing error.

3. **Parent-id selector as `<TextField select>` vs MUI Autocomplete.**
   Plain `<TextField select>` mirrors the existing `Kind` field's
   pattern and is enough for the realistic worker counts (tens). If
   orgs grow into hundreds of workers we'd revisit with `Autocomplete`,
   which supports type-ahead. Deferred to keep the diff small.

4. **Owner worker selectability as parent.** The owner *can* be a
   manager (and usually is — the default `s-team-w-owner` stream is
   exactly the "team that reports to the owner"). Don't filter it out.

5. **Role filter persistence: URL only, not localStorage.** Sandboxes
   uses URL+localStorage for the table↔card view toggle (which is a
   per-user preference). A role filter is per-task context — saving it
   would surprise the user on the next visit. URL only.

6. **Chart `+ New Role` in-canvas affordance.** Discussed in the task
   brief; deferred. The floating top-right button is fine for now and
   there's no natural anchor on the canvas itself. If we add a
   no-roles-yet hint panel later, that's a candidate spot.

7. **Toast wording.** When `parent_id` is set at hire, the current
   "drag an edge from a manager to set who they report to" hint becomes
   misleading. New text: `hired ${id} reporting to ${parent}` when
   `parent_id` was set, else the existing message.

## Patterns Used (for future agents)

- **`SimpleTable`** is the standard for org list pages — not raw MUI
  Table. See `frontend/src/components/widgets/SimpleTable.tsx` and the
  rules in `helix/CLAUDE.md` (UI Styles section).
- **Filter / action row** sits above the table inside the page `Stack`,
  not in `topbarContent`. Mirrors Sandboxes pattern.
- **URL-synced UI state** uses `router.params` (react-router5 via
  `useRouter()`) — not React Router's `useSearchParams` (we're on v5,
  not v6).
- **Mutations** already invalidate the right query keys — no extra
  invalidation needed in this PR.
- **`HireWorkerRequest`** lives at `frontend/src/services/helixOrgService.ts:57-62`
  and `parent_id` is already on the type — no service-layer or generated
  client change needed.

## Implementation Notes

- **URL params API.** This codebase's `useRouter()` is react-router5 + a custom
  wrapper (see `frontend/src/contexts/router.tsx`). For URL-synced filter
  state use `router.mergeParams({ key: value })` (replace-mode merge into
  current params) and `router.removeParams(['key'])`. **Do not** use
  `router.setParams(...)` — its first arg is the full param set, and
  passing a partial wipes the route's required params (`org_id`, …) on
  the next navigate. `mergeParams` is the right hammer for query-style
  filter persistence.
- **HireWorkerRequest.parent_id already existed.** No service-layer or
  generated client regen was needed for #2538. The frontend Service
  type (`helixOrgService.ts:57-62`) already declared it; the hire
  endpoint and `Reconciler` already understood it. The bug was purely
  in the form's missing input.
- **Inner-Helix bootstrapping for testing.** The `/api/v1/orgs/{org}/…`
  routes are gated by two things: `HELIX_ORG_ENABLED=true` in `.env` AND
  the requesting user having `helix-org` in their `users.alpha_features`.
  Without both, the request 404s on the URL prefix (no handler
  registered) — there's no "feature off" message, it just falls through
  to "unknown API path". Bootstrap the test env once with:
  - `echo HELIX_ORG_ENABLED=true >> .env && docker compose -f docker-compose.dev.yaml up -d api`
  - `docker exec helix-postgres-1 psql -U postgres -d postgres -c "UPDATE users SET alpha_features='{helix-org}' WHERE email='test@helix.ml'"`
- **Docker bind-mount flakiness.** After a Docker daemon restart the
  `/app/src` bind mount inside `helix-frontend-1` was empty. A simple
  `docker compose -f docker-compose.dev.yaml restart frontend` fixed it
  and Vite served changes immediately afterwards.
- **Shared component placement.** `frontend/src/components/helix-org/`
  was the natural home; this directory already housed `GitHubAppPanel`,
  `GitHubRepoPicker`, etc.

## Risks

- ReactFlow's drawer-vs-dialog handling on the Chart page is unchanged
  (the shared component renders its own `<Drawer>`), so no regression
  expected on chart interactions.
- Resetting form state on `open` transitions should be explicit
  (`useEffect(() => { if (open) reset() }, [open])`) — otherwise stale
  state from a previous open survives. Tested in QA.
- Role filter on URL: must use a key that doesn't conflict with
  existing org-route params (`org_id` is the only one). `?role=…` is
  safe.
