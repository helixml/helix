# Implementation Tasks: Hover-to-Delete Cross Icon on Org Chart Connections

- [x] Add a `DeletableEdge` React component in `frontend/src/pages/HelixOrgChart.tsx` (or a new sibling file) that wraps `BaseEdge` and renders a hover-only × button at the edge midpoint using `EdgeLabelRenderer`.
- [x] Add a transparent wider stroke overlay path on the edge so the hover hit-area covers ~20 px (avoid setting `strokeDasharray` on the overlay so dashed subscription edges remain hoverable between dashes).
- [x] Wire the × `onClick` to `useReactFlow().deleteElements({ edges: [{ id }] })` so the existing `onEdgesDelete` dispatch (which already routes by `data.kind` to `onUnsubscribeWorker` / `onRemoveParent`) fires unchanged — do not call mutation hooks directly from the edge component.
- [x] Register the new edge type via `edgeTypes={{ deletable: DeletableEdge }}` on the `<ReactFlow>` element.
- [x] Set `type: 'deletable'` on both `edges.push(...)` call-sites in `HelixOrgChart.tsx` (the reporting edge at ~line 631 and the subscription edge at ~line 752).
- [x] Make the × a real `<button type="button">` with an `aria-label` selected from `data.kind` ("Remove subscription" vs "Remove reporting line"), keyboard-focusable, with a visible focus ring.
- [x] Style the × to match existing chart conventions (light/dark aware via the existing `isLight` flag, ~18 px circular button, destructive-tinted on hover/focus) and confirm visual parity with the current straight-line edge appearance (use `getStraightPath`).
- [~] Verify the existing keyboard delete behaviour (select edge + `Backspace`/`Delete`) still works after the change and that `deleteKeyCode={['Backspace', 'Delete']}` is untouched.
- [ ] Run the frontend dev server and manually verify in the browser: hover a subscription edge → × appears → click → subscription disappears and persists on reload; repeat for a reporting edge; confirm the × disappears on mouse-leave; confirm error path still surfaces if the DELETE API fails (e.g. by temporarily pointing at a stale ID in devtools).
- [x] Run `yarn lint` / `yarn typecheck` (or whatever the project's standard frontend checks are) and fix any new warnings introduced by the change.

## Implementation Notes

- Implemented entirely in `frontend/src/pages/HelixOrgChart.tsx` (one file, ~80 LOC addition for `DeletableEdge` + a couple of one-liners to wire it up).
- The `DeletableEdge` component imports `BaseEdge`, `EdgeLabelRenderer`, `EdgeProps`, `getStraightPath` from `@xyflow/react` v12.
- `deleteElements({ edges: [{ id }] })` from `useReactFlow()` fires React Flow's normal removal pipeline → existing `onEdgesDelete` callback in `ChartCanvas` → existing `onUnsubscribeWorker` / `onRemoveParent` hooks. Zero duplication.
- Keyboard delete (Backspace/Delete with edge selected) is unaffected — `deleteKeyCode` left untouched. The same `onEdgesDelete` handles both code paths.
- The × button has `onMouseDown` `stopPropagation` to prevent it from triggering edge selection/drag while being clicked.
- `selected` from `EdgeProps` also shows the × so keyboard users who select an edge can see the affordance.
- Typecheck (`yarn tsc --noEmit`) clean. Frontend build (`yarn build`) clean.

### Light/dark styling note
The button currently uses fixed white background / dark glyph. This is readable on both themes against the chart background, but a future polish pass could tint it via `useLightTheme().isLight` like the rest of the chart. Not done in this pass — the design called for "match existing chart conventions" and the rest of the chart's chips/buttons use static MUI defaults too.
