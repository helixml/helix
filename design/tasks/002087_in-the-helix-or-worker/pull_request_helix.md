# feat(frontend): open worker right-rail links in new tab

## Summary

On the Helix-OR Worker Detail page (`HelixOrgWorkerDetail.tsx`), the
Role / Project / Agent links in the right-hand rail used to navigate
the user away in the same tab. Operators almost always want to peek at
those without losing their place on the worker view, so this PR rewrites
them as real anchors that open in a new browser tab, with the
conventional "open in new" icon (square + arrow) next to each value.

## Changes

- Replace the three `<Button onClick={router.navigate}>` blocks (Role,
  Project, Agent) with MUI `<Link href target="_blank"
  rel="noopener noreferrer">` elements whose `href` is generated via
  `router.buildPath(routeName, params)` — so the route table stays the
  single source of truth.
- Append `<OpenInNewIcon sx={{ fontSize: 14 }} />` inside each link to
  signal the new-tab behaviour before the click.
- Preserve the existing per-link sizing (Role at body2, Project/Agent
  at `0.7rem` monospace with `wordBreak: 'break-all'`).
- Fall back to plain `<Typography>` when `orgSlug` is missing — the
  prior behaviour was a button with a no-op onClick, which was worse
  UX.
- Import the default router as `router5` to avoid shadowing the
  existing `const router = useRouter()` local in the component.

Because we're using a native `<a>` (via MUI `<Link>`), middle-click /
Cmd-click / right-click → "Open link in new tab" work natively without
any JS interception.

## Files changed

- `frontend/src/pages/HelixOrgWorkerDetail.tsx`

No backend, route, or API changes.
