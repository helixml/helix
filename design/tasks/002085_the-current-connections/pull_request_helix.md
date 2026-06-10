# feat(frontend): hover-to-delete × on org chart connections

## Summary

Adds a hover-only × button at the midpoint of every connection in the org
chart (`HelixOrgChart`). Today the only way to remove a subscription or a
reporting line from the chart view is to click the edge to select it and
press `Backspace`/`Delete` — undiscoverable for users who don't know that
keyboard shortcut. This change surfaces a visible affordance without
adding visual noise (the × appears on hover, disappears on mouse-leave).

## Changes

- `frontend/src/pages/HelixOrgChart.tsx`:
  - New `DeletableEdge` React-Flow custom edge using `BaseEdge` + `EdgeLabelRenderer` from `@xyflow/react` v12; renders a transparent stroke-20 overlay path for a usable hover hit-area (the visible line is only 1.25–1.5px) and a small × button at the edge midpoint while hovered or selected.
  - The × `onClick` routes through `useReactFlow().deleteElements({ edges: [{ id }] })` so the existing `onEdgesDelete` dispatch (subscription → `DELETE /workers/{id}/subscriptions/{stream_id}`, reporting → `DELETE /workers/{id}/parents/{parent_id}`) fires unchanged. No new backend endpoints, no service-layer changes, no duplicated mutation logic.
  - Both `edges.push(...)` call-sites now set `type: 'deletable'`; new `edgeTypes={{ deletable: DeletableEdge }}` prop on `<ReactFlow>`.
  - `aria-label` differentiates the two edge kinds ("Remove subscription" / "Remove reporting line"); button is a real `<button type="button">` with a visible focus ring.
  - Existing keyboard delete (`Backspace`/`Delete` on selected edge) is preserved — same `onEdgesDelete` handles both paths.

## Verification

Tested end-to-end in the inner Helix at `http://localhost:8080`:

- Hovering a subscription edge shows the ×; click fires `DELETE /workers/w-owner/subscriptions/s-activations-w-owner → 204`; edge disappears; persists on page reload.
- Hovering a reporting edge shows the ×; click fires `DELETE /workers/w-alice/parents/w-owner → 204`; edge disappears.
- Keyboard delete (click edge to select, press `Delete`) still removes the edge.
- `yarn tsc --noEmit` clean. `yarn build` clean.

## Screenshots

![Chart with both edge kinds](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002085_the-current-connections/screenshots/04-chart-with-both-edge-kinds.png)

Hovering a **subscription** edge shows "Remove subscription":

![Hover × on subscription edge](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002085_the-current-connections/screenshots/03-hover-x-on-subscription-edge.png)

Hovering a **reporting** edge shows "Remove reporting line":

![Hover × on reporting edge](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002085_the-current-connections/screenshots/05-hover-x-on-reporting-edge.png)
