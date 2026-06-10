# Implementation Tasks: Hover-to-Delete Cross Icon on Org Chart Connections

- [~] Add a `DeletableEdge` React component in `frontend/src/pages/HelixOrgChart.tsx` (or a new sibling file) that wraps `BaseEdge` and renders a hover-only × button at the edge midpoint using `EdgeLabelRenderer`.
- [ ] Add a transparent wider stroke overlay path on the edge so the hover hit-area covers ~20 px (avoid setting `strokeDasharray` on the overlay so dashed subscription edges remain hoverable between dashes).
- [ ] Wire the × `onClick` to `useReactFlow().deleteElements({ edges: [{ id }] })` so the existing `onEdgesDelete` dispatch (which already routes by `data.kind` to `onUnsubscribeWorker` / `onRemoveParent`) fires unchanged — do not call mutation hooks directly from the edge component.
- [ ] Register the new edge type via `edgeTypes={{ deletable: DeletableEdge }}` on the `<ReactFlow>` element.
- [ ] Set `type: 'deletable'` on both `edges.push(...)` call-sites in `HelixOrgChart.tsx` (the reporting edge at ~line 631 and the subscription edge at ~line 752).
- [ ] Make the × a real `<button type="button">` with an `aria-label` selected from `data.kind` ("Remove subscription" vs "Remove reporting line"), keyboard-focusable, with a visible focus ring.
- [ ] Style the × to match existing chart conventions (light/dark aware via the existing `isLight` flag, ~18 px circular button, destructive-tinted on hover/focus) and confirm visual parity with the current straight-line edge appearance (use `getStraightPath`).
- [ ] Verify the existing keyboard delete behaviour (select edge + `Backspace`/`Delete`) still works after the change and that `deleteKeyCode={['Backspace', 'Delete']}` is untouched.
- [ ] Run the frontend dev server and manually verify in the browser: hover a subscription edge → × appears → click → subscription disappears and persists on reload; repeat for a reporting edge; confirm the × disappears on mouse-leave; confirm error path still surfaces if the DELETE API fails (e.g. by temporarily pointing at a stale ID in devtools).
- [ ] Run `yarn lint` / `yarn typecheck` (or whatever the project's standard frontend checks are) and fix any new warnings introduced by the change.
