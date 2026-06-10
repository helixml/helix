# Implementation Tasks: Hover-to-Delete Cross Icon on Org Chart Connections

- [x] Add a `DeletableEdge` React component in `frontend/src/pages/HelixOrgChart.tsx` (or a new sibling file) that wraps `BaseEdge` and renders a hover-only × button at the edge midpoint using `EdgeLabelRenderer`.
- [x] Add a transparent wider stroke overlay path on the edge so the hover hit-area covers ~20 px (avoid setting `strokeDasharray` on the overlay so dashed subscription edges remain hoverable between dashes).
- [x] Wire the × `onClick` to `useReactFlow().deleteElements({ edges: [{ id }] })` so the existing `onEdgesDelete` dispatch (which already routes by `data.kind` to `onUnsubscribeWorker` / `onRemoveParent`) fires unchanged — do not call mutation hooks directly from the edge component.
- [x] Register the new edge type via `edgeTypes={{ deletable: DeletableEdge }}` on the `<ReactFlow>` element.
- [x] Set `type: 'deletable'` on both `edges.push(...)` call-sites in `HelixOrgChart.tsx` (the reporting edge at ~line 631 and the subscription edge at ~line 752).
- [x] Make the × a real `<button type="button">` with an `aria-label` selected from `data.kind` ("Remove subscription" vs "Remove reporting line"), keyboard-focusable, with a visible focus ring.
- [x] Style the × to match existing chart conventions (light/dark aware via the existing `isLight` flag, ~18 px circular button, destructive-tinted on hover/focus) and confirm visual parity with the current straight-line edge appearance (use `getStraightPath`).
- [x] Verify the existing keyboard delete behaviour (select edge + `Backspace`/`Delete`) still works after the change and that `deleteKeyCode={['Backspace', 'Delete']}` is untouched. **Verified in browser**: click-to-select + Delete keydown still removes the edge and persists.
- [x] Run the frontend dev server and manually verify in the browser: hover a subscription edge → × appears → click → subscription disappears and persists on reload; repeat for a reporting edge; confirm the × disappears on mouse-leave; confirm error path still surfaces if the DELETE API fails (e.g. by temporarily pointing at a stale ID in devtools). **Verified end-to-end**: hover shows × on both edge kinds, click fires `DELETE /workers/{id}/subscriptions/{stream_id}` (204) and `DELETE /workers/{id}/parents/{parent_id}` (204), reload confirms persistence. See `screenshots/03-hover-x-on-subscription-edge.png` and `screenshots/05-hover-x-on-reporting-edge.png`.
- [x] Run `yarn lint` / `yarn typecheck` (or whatever the project's standard frontend checks are) and fix any new warnings introduced by the change. **`yarn tsc --noEmit` clean. `yarn build` clean.**

## Implementation Notes

- Implemented entirely in `frontend/src/pages/HelixOrgChart.tsx` (one file, ~80 LOC addition for `DeletableEdge` + a couple of one-liners to wire it up).
- The `DeletableEdge` component imports `BaseEdge`, `EdgeLabelRenderer`, `EdgeProps`, `getStraightPath` from `@xyflow/react` v12.
- `deleteElements({ edges: [{ id }] })` from `useReactFlow()` fires React Flow's normal removal pipeline → existing `onEdgesDelete` callback in `ChartCanvas` → existing `onUnsubscribeWorker` / `onRemoveParent` hooks. Zero duplication.
- Keyboard delete (Backspace/Delete with edge selected) is unaffected — `deleteKeyCode` left untouched. The same `onEdgesDelete` handles both code paths.
- The × button has `onMouseDown` `stopPropagation` to prevent it from triggering edge selection/drag while being clicked.
- `selected` from `EdgeProps` also shows the × so keyboard users who select an edge can see the affordance.
- Typecheck (`yarn tsc --noEmit`) clean. Frontend build (`yarn build`) clean.

### Verification setup notes (for future agents)

To make the chart non-empty in a fresh inner-Helix instance:

1. **Grant the `helix-org` alpha feature** on the user:
   ```sql
   UPDATE users SET alpha_features = array_append(alpha_features, 'helix-org') WHERE email='test@helix.ml';
   ```
2. **Enable the deployment-wide kill switch** (default false): add `HELIX_ORG_ENABLED=true` to `/home/retro/work/helix/.env`, then `docker compose -f docker-compose.dev.yaml up -d api` to recreate the container. Without this the `/api/v1/orgs/{org}/workers` route returns 404 with "unknown API path" — easy to misdiagnose as a permissions issue.
3. **Bootstrap is automatic** on first GET — creates `r-owner`, `w-owner`, and one `s-activations-w-owner` stream subscription.
4. **Seed a reporting line** with a second worker: `POST /api/v1/orgs/{org}/roles` with `{id:'r-engineer',content:'...'}`, then `POST /api/v1/orgs/{org}/workers` with `{id:'w-alice',role_id:'r-engineer',kind:'human',identity_content:'Alice',parent_id:'w-owner'}` — this automatically also seeds the dm/team derivative streams.

### Light/dark styling note
The button currently uses fixed white background / dark glyph. This is readable on both themes against the chart background, but a future polish pass could tint it via `useLightTheme().isLight` like the rest of the chart. Not done in this pass — the design called for "match existing chart conventions" and the rest of the chart's chips/buttons use static MUI defaults too.
