# Design: Worker Detail Right-Rail Links Open in New Tab

## Current Implementation

File: `frontend/src/pages/HelixOrgWorkerDetail.tsx`, lines 341–420.

Each of the three links is currently a MUI `<Button variant="text">` with
an `onClick` that calls `router.navigate(routeName, params)` — the helix
router5 hook from `frontend/src/contexts/router.tsx`. Example (line 366):

```tsx
<Button
  size="small"
  variant="text"
  onClick={() => orgSlug && router.navigate('helix_org_role_detail', {
    org_id: orgSlug, role_id: data.role!.id
  })}
  sx={{ fontFamily: 'monospace', textTransform: 'none', ... }}
>
  {data.role.id}
</Button>
```

A `<Button onClick>` cannot be opened in a new tab natively — there's no
href for the browser to act on. That's why we need to swap to an anchor.

## Approach

Replace each of the three `<Button>` elements with an `<a target="_blank">`
(MUI `<Link>`) whose `href` is built from the helix router. Append a
small `OpenInNewIcon` inside the link.

### Building the href

`router5` (which helix wraps) exposes `router.buildPath(name, params)` —
it returns the path string for a named route (e.g.
`/orgs/acme/roles/role_xyz`). The default helix router instance is
exported from `frontend/src/router.tsx` (line 650). We import it
directly:

```tsx
import router from '../router'
const roleHref = router.buildPath('helix_org_role_detail', {
  org_id: orgSlug, role_id: data.role.id,
})
```

This avoids hard-coding URL strings and stays in sync with the route
table.

### The link element

Use MUI's `<Link>` with `component="a"`, `href`, `target="_blank"`,
`rel="noopener noreferrer"` (security best-practice for `_blank` targets
— prevents `window.opener` reverse-tabnabbing). Inline-flex layout puts
the `OpenInNewIcon` immediately to the right of the ID text. Pattern
mirrors the existing usage in
`frontend/src/components/dashboard/CreateProviderEndpointDialog.tsx`
line ~280–290:

```tsx
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import Link from '@mui/material/Link'

<Link
  href={roleHref}
  target="_blank"
  rel="noopener noreferrer"
  underline="hover"
  sx={{
    fontFamily: 'monospace',
    fontSize: '0.7rem',          // match Project/Agent — Role currently has no fontSize set,
                                  // we'll keep Role at its current 'body2' size (0.875rem) and
                                  // apply 0.7rem only to Project/Agent to preserve existing visuals.
    display: 'inline-flex',
    alignItems: 'center',
    gap: 0.5,
    wordBreak: 'break-all',
  }}
>
  {data.role.id}
  <OpenInNewIcon sx={{ fontSize: 14 }} />
</Link>
```

The `OpenInNewIcon` is already imported and used in several files in the
codebase — no new dependency needed. `@mui/icons-material` is already a
project dependency.

### Why MUI `<Link>` over plain `<a>`

Consistency with the rest of the codebase — MUI `<Link>` ships the
`underline` prop and themed colour out of the box, and the `Link`
component is the established choice across helix when an anchor needs to
respect MUI styling. Plain `<a>` would work but would need extra sx for
colour/underline.

### Why not keep `<Button>` and just add the icon?

A `<Button onClick={...navigate}>` is not a real anchor. The browser
cannot middle-click / Cmd-click / right-click → "open in new tab" on it.
Adding `target="_blank"` is meaningless on a `<button>`. We must use an
anchor with an `href` to get the native browser tab-opening behaviour
the user is asking for.

### Why not just add `target="_blank"` to an existing component?

There isn't one — these are buttons, not links. Hence the rewrite to MUI
`<Link>`.

## Key Decisions

1. **MUI `<Link>` + `target="_blank"` over `window.open(...)` in onClick.**
   Native anchor behaviour gives middle-click / Cmd-click / right-click
   menu for free. `window.open` from a click handler is blocked by
   stricter popup-blockers and breaks browser UX expectations.

2. **`router.buildPath()` to generate hrefs.** Keeps the spec docs and
   the route table as the single source of truth — if anyone renames a
   route, this code follows along.

3. **`rel="noopener noreferrer"`** on every `target="_blank"`. Standard
   security hygiene; prevents the opened page from accessing
   `window.opener`.

4. **Preserve current per-link sizing.** Role currently renders at body2
   size; Project/Agent at 0.7rem. We keep that asymmetry as-is to avoid
   gratuitous visual churn — the user asked for tab behaviour and an
   icon, not a re-design.

## Affected Files

| File | Change |
|---|---|
| `frontend/src/pages/HelixOrgWorkerDetail.tsx` | Replace three `<Button onClick>` blocks (lines 366–373, 379–386, 392–399) with MUI `<Link target="_blank">` + `OpenInNewIcon`. Add imports for `OpenInNewIcon` and `Link` from `@mui/material`, and the default `router` from `../router`. |

No backend / API / route changes. Frontend-only, single file.

## Out of Scope / Non-goals

- No new routes, no route renames.
- No backend changes.
- No change to the "Reports to" parent IDs (still plain text).
- No change to the back-arrow, fire-worker action, or accordion.
- No changes outside `HelixOrgWorkerDetail.tsx`.

## Testing Notes

- **Manual (inner Helix at `http://localhost:8080`)**: Navigate to a
  worker detail page (Helix-OR Workers list → click a worker), confirm
  each of Role / Project / Agent shows the new-tab icon and opens in a
  new tab without disturbing the worker view. Confirm middle-click
  also opens in a new tab.
- **No new unit tests required.** This is a presentation-layer swap
  with no logic branches; visual verification in the browser is the
  appropriate test.
- **Build check**: TypeScript via `npx tsc --noEmit -p tsconfig.json`
  must pass. Note `yarn build` itself fails in this dev environment
  because `frontend/dist/` is a root-owned bind mount (production
  frontend mode) — vite transforms all modules successfully, then
  EACCES on writing the output. The transform step is the real type/JSX
  check, so TS clean = build clean.

### Testing Limitation in This Environment

The inner Helix at `http://localhost:8080` in this dev environment does
**not** have the Helix-OR (helix-org) API endpoints wired in
(`/api/v1/orgs/{slug}/roles` returns "unknown API path"; there is no
`helix_org_workers` table in Postgres). That means a full click-through
of the rebuilt right-rail links cannot be performed here.

Verification done instead:
1. TypeScript: `npx tsc --noEmit` → 0 errors.
2. Vite transform: all 21,647 modules transformed without JSX/TS errors.
3. Route names confirmed present in `frontend/src/router.tsx`:
   `helix_org_role_detail` (line 567), `org_project-specs` (line 219),
   `org_agent` (line 291).
4. `router.buildPath(name, params)` is the standard router5 API and is
   used by the helix router context internally — it returns the path
   string for an anchor href. The `useRouter` hook does **not** expose
   `buildPath`, so importing the default router singleton from
   `../router` (aliased here as `router5` to avoid shadowing the local
   `const router = useRouter()`) is required.

The full click-through belongs in CI / on a deployment where the
helix-org API is wired in. The change is mechanical (JSX swap, no logic
branches) and is safe to ship pending that browser-level confirmation.

## Implementation Notes

- **Import alias**: imported the default router as `router5` (not
  `router`) because `const router = useRouter()` already exists at line
  74 of the component and shadowing it would change the meaning of
  every other `router.navigate(...)` call in the file (e.g. line 185
  for the back button). Aliasing the singleton avoids touching any
  other code.
- **Fallback when `orgSlug` is missing**: the original `<Button>`
  rendered always (with a no-op onClick when slug was missing). The new
  `<Link>` cannot have a no-op href without producing a broken link, so
  the fallback path renders plain `<Typography>` with matching font
  styling. This is *better* UX than the old behaviour, which was a
  button that did nothing on click.
- **`<Link>` `component` prop**: not needed — MUI `<Link>` defaults to
  an `<a>` element when given an `href`, which is exactly what we want.
