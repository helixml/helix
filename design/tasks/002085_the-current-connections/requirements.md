# Requirements: Hover-to-Delete Cross Icon on Org Chart Connections

## Background

The org chart (`HelixOrgChart.tsx`) renders two kinds of connections between nodes:

- **Subscription edges** — a worker subscribes to a stream (dashed orange line)
- **Reporting edges** — one worker reports to / is delegated by another (solid grey line)

Today, the only way to remove either connection from the chart view is to click the edge to select it and then press `Backspace`/`Delete`. This is undiscoverable: users see the line but have no visible affordance telling them they can remove it. The backend already supports deletion via `DELETE /workers/{id}/subscriptions/{stream_id}` and `DELETE /workers/{id}/parents/{parent_id}`, and the frontend already wires both into `onEdgesDelete`. The gap is purely UX.

## User Story

> As a user looking at the org chart, when I hover over a connection between a worker and a stream (subscription) or between two workers (reporting line), I want to see a small cross (×) icon appear on the line so I can click it to remove that subscription or reporting line without first having to know that I can select-and-delete with the keyboard.

## Acceptance Criteria

1. Hovering over any edge in the chart (both subscription and reporting kinds) displays a small × button positioned at the midpoint of the edge.
2. The × button only appears while the pointer is over the edge or over the × itself; it disappears when the pointer leaves both.
3. Clicking the × removes the connection:
   - For subscription edges → calls the existing unsubscribe path (`DELETE /workers/{id}/subscriptions/{stream_id}`).
   - For reporting edges → calls the existing remove-parent path (`DELETE /workers/{id}/parents/{parent_id}`).
4. The existing keyboard delete behaviour (select edge + `Backspace`/`Delete`) continues to work unchanged.
5. The × is keyboard-accessible: it is a real `<button>` with an `aria-label` such as "Remove subscription" or "Remove reporting line" and is focusable via Tab once the edge is hovered (or always, where reasonable).
6. The × button has a visible hover/focus state and is large enough to click comfortably (~16-20 px tap target) without overpowering the edge visually.
7. After a successful click, the edge disappears from the chart and the underlying React Query cache is invalidated, so the change persists on reload — matching what happens today when the edge is deleted via keyboard.
8. If the delete API call fails, the edge remains and an error surface is shown consistent with how other org-chart mutation failures are handled today.

## Out of Scope

- No new backend endpoints, DTOs, or schema changes.
- No confirmation dialog ("Are you sure?") — deletion is immediate, matching current keyboard-delete behaviour.
- No bulk-delete UI or multi-select gestures.
- No change to how edges are *created* (drag-to-connect).
- No change to other chart visualisations elsewhere in the app — this is scoped to `HelixOrgChart.tsx`.
