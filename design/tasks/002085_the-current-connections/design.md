# Design: Hover-to-Delete Cross Icon on Org Chart Connections

## Summary

Replace the default React Flow edge rendering for org-chart connections with a custom edge component that renders the same line plus a hover-only × button at the edge midpoint. Clicking the × dispatches to the existing `onEdgesDelete` pipeline, so no backend or service-layer changes are needed.

## Scope

Single file (plus one small new sibling): `/home/retro/work/helix/frontend/src/pages/HelixOrgChart.tsx`. Everything else — backend handlers, React Query hooks (`useUnsubscribeWorkerAtChart`, the parent-removal hook), DTOs — stays as is.

## Relevant Existing Code

- `frontend/src/pages/HelixOrgChart.tsx`
  - Edges are pushed at lines ~631 (reporting) and ~752 (subscription) with `type: 'default'` and a `data` payload of either `{ kind: 'report', childWorkerId, parentWorkerId }` or `{ kind: 'sub', workerId, streamId }`.
  - `onEdgesDelete` (line ~998) already routes by `data.kind` to `onUnsubscribeWorker` / `onRemoveParent`.
  - `<ReactFlow … deleteKeyCode={['Backspace', 'Delete']} />` (line ~1035).
- Chart library: `@xyflow/react ^12.11.0` (v12 API — `BaseEdge`, `EdgeLabelRenderer`, `getBezierPath`/`getStraightPath`, and `useReactFlow().deleteElements()` are all available).
- Backend `DELETE` routes are wired and used today — verified in `api/pkg/org/interfaces/server/api/api.go`.

## Approach

### 1. Add a custom edge component

Create a new component `DeletableEdge` (kept inside `HelixOrgChart.tsx` for proximity to the rest of the chart code, or as a sibling file `HelixOrgChartEdges.tsx` if the parent file is already large — author's call). It uses React Flow v12's standard custom-edge recipe:

```tsx
import { BaseEdge, EdgeLabelRenderer, getStraightPath, type EdgeProps } from '@xyflow/react'

const DeletableEdge: React.FC<EdgeProps> = (props) => {
  const { id, sourceX, sourceY, targetX, targetY, style, data, markerEnd } = props
  const [hover, setHover] = useState(false)
  const reactFlow = useReactFlow()

  const [edgePath, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY })

  const ariaLabel = data?.kind === 'sub' ? 'Remove subscription' : 'Remove reporting line'

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        style={style}
        markerEnd={markerEnd}
        interactionWidth={20} // larger invisible hit area for hover
      />
      {/* a transparent overlay path widens the hover/click target without changing the visual */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
      />
      {hover && (
        <EdgeLabelRenderer>
          <button
            type="button"
            aria-label={ariaLabel}
            onMouseEnter={() => setHover(true)}
            onMouseLeave={() => setHover(false)}
            onClick={() => reactFlow.deleteElements({ edges: [{ id }] })}
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: 'all',
              /* sizing, border, background omitted — match existing chip styles */
            }}
          >
            ×
          </button>
        </EdgeLabelRenderer>
      )}
    </>
  )
}
```

Key points:

- Calling `reactFlow.deleteElements({ edges: [{ id }] })` goes through the same path that `Backspace` does, so React Flow fires `onEdgesDelete` and the existing dispatch in `HelixOrgChart.tsx` runs unchanged. **No duplication of delete logic.**
- The transparent stroke-20 overlay path is the standard React Flow trick for a hoverable edge — without it, only the 1.25–1.5 px visible line is a hover target, which feels broken.
- Keeping `hover` state local to the edge avoids re-rendering the whole chart on mouse moves.

### 2. Register the edge type and tag existing edges

Inside `HelixOrgChart.tsx`:

```tsx
const edgeTypes = useMemo(() => ({ deletable: DeletableEdge }), [])

// at edges.push(...) for BOTH kinds:
edges.push({
  …existing fields…,
  type: 'deletable',
})

// in <ReactFlow …>:
<ReactFlow … edgeTypes={edgeTypes} … />
```

The `data.kind` payload already carries everything `DeletableEdge` needs to choose the right `aria-label`; no other plumbing is required because the existing `onEdgesDelete` already reads `data.kind`.

### 3. Path helper choice

Use `getStraightPath` to match the current `type: 'default'` straight-line appearance for both edge kinds. If a future change wants curves, swapping in `getBezierPath` is a one-liner. Do **not** switch to bezier as part of this task — keep visual parity.

### 4. Styling

- Button: 18 px square, circular, neutral background (`var(--mui-palette-background-paper)` or the same `paper`-tinted background used by hovered nodes), subtle 1 px border, × glyph centered.
- On hover/focus of the button itself: bump to a slightly higher-contrast background (e.g. error-tinted) to telegraph the destructive action.
- Use Material UI's `IconButton` + `CloseIcon` if it slots in cleanly with the rest of the file; otherwise a plain styled `<button>` is fine — pick whichever matches the existing chart's conventions.
- Respect light/dark theme by reusing the same `isLight` flag the surrounding code already consults.

## Failure Handling

Already covered by today's hooks: `useSubscribeWorkerAtChart` / `useUnsubscribeWorkerAtChart` and the parent-edit equivalents already invalidate caches on success and surface mutation errors via the existing snackbar/error path. The × click flows through `onEdgesDelete` → those same hooks, so nothing new is needed.

## Accessibility

- The × is a real `<button>` with `aria-label`.
- It is visible on hover **and** on keyboard focus of the edge (React Flow makes selected edges focusable; we render the × when the edge is selected too, by reading `selected` from `EdgeProps`). This means tabbing across the chart can reach edges and their deletes.
- Keyboard delete via `Backspace`/`Delete` continues to work, so users who prefer keyboard never lose anything.

## Alternatives Considered

- **A context menu / right-click menu.** More discoverable for some users, much heavier to build (positioning, dismiss-on-outside-click, theming) and inconsistent with the rest of the chart which has no other context menus.
- **A persistent × always shown on edges.** Visually noisy on a chart that can have dozens of edges; defeats the purpose of the chart being a clean overview.
- **A delete button in a side-panel inspector that opens when an edge is selected.** More plumbing for the same result; hover-X is the lightest path to the same outcome and matches conventions in other diagram tools.

The hover-X approach wins on (a) zero backend work, (b) minimal new code, (c) discoverability without visual noise, (d) keyboard parity preserved.

## Risks / Gotchas

- The dashed subscription edges have `strokeDasharray`; the transparent overlay path must not inherit it (set `strokeDasharray: 'none'` on the overlay) or hover targeting becomes spotty between dashes.
- `EdgeLabelRenderer` portals into a DOM layer above the SVG; CSS `pointer-events: all` on the button is required, otherwise pan/zoom on the canvas swallows the click.
- React Flow v12 deprecated some v11 helpers — make sure imports come from `@xyflow/react`, not the legacy `reactflow` package.

## Notes for Future Agents Cloning This Spec

- This codebase uses **`@xyflow/react` v12** (not legacy `reactflow`). Imports and the `BaseEdge`/`EdgeLabelRenderer` APIs differ between v11 and v12 — check the installed version before pasting recipes from blog posts.
- Edge delete is already centralised in `onEdgesDelete` keyed off `data.kind`. When adding new edge interactions, prefer to route through `reactFlow.deleteElements()` (or equivalent v12 helpers) rather than calling mutation hooks directly from a child component — keeps a single dispatch site.
- The chart distinguishes "subscription" vs "reporting" edges entirely by `data.kind`; styling and handle wiring are separate. Any new interaction that needs to behave differently for the two kinds should likewise branch on `data.kind` and not on visual style.
