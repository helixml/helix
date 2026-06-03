# Helix Org — QA test plan

End-to-end manual test for the helix-org chart UI. Lives in
`api/pkg/org/QA.md` because it's specific to the org-mode alpha; run
it before merging any change that touches
`frontend/src/pages/HelixOrgChart.tsx`,
`frontend/src/components/orgs/HelixOrgSidebar.tsx`, `api/pkg/org/`, or
`api/pkg/server/helix_org*.go`.

## Mental model

The chart has **two kinds of node** and one kind of edge:

- **Role** — a flat, top-level container. Roles have no parent. The
  role frame visually groups the positions that share the same role
  id (`r-engineer`, `r-pm`, …). Add positions to it, delete it, move
  on. There is no "role below" concept — role-to-role hierarchy is
  not modelled.
- **Position** — a slot inside one role that holds at most one worker.
  Positions are what the org graph hangs off — they have two
  ReactFlow handles (a target dot on the top, a source dot on the
  bottom).
- **Edge** — a directed line from a *manager position*'s source
  handle to a *subordinate position*'s target handle. The edge means
  "the worker in the subordinate position reports to the worker in
  the manager position." This is the entire hierarchy.

```
[r-owner]                       ← flat role, top-level container
  ┌──────────────────────────┐
  │ [p-ceo]      [p-cto]     │  ← parallel positions, both top-level
  └────│────────────│────────┘
       ↓            ↓             ← edge: manager → subordinate
[r-engineer]                    ← another flat role, lower only because
  ┌──────────────────────────┐     dagre routes nodes pointed at by
  │ [p-eng-a]    [p-eng-b]   │     incoming edges below their parents
  └──────────────────────────┘
```

Layout is dagre-driven: nodes with no incoming edges sit at the top
rank, and roles with no edges in or out become orphan roles parked at
the side. Edges are bezier curves so parallel reporting lines never
collapse onto each other.

## Setup

1. Local stack is up: `docker compose -f docker-compose.dev.yaml ps` shows `helix-api-1` and `helix-frontend-1` Up.
2. The acting user is granted the `helix-org` alpha feature:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "UPDATE users SET alpha_features = ARRAY['helix-org']::text[] WHERE email='you@example.com';"
   ```
3. The user is a member of the test organisation (`organization_memberships` row).
4. Sign in at `http://localhost:8080/login` and confirm the **Org** icon is the second item in the left rail (under **Projects**).

## 1. Open the chart

1. Click **Org** in the primary sidebar.
2. URL becomes `http://localhost:8080/orgs/<org>/helix-org/chart`.
3. Page chrome:
   - breadcrumbs read `<org> / Chart`
   - title **Chart** with the explanatory subtitle
   - middle sidebar shows a single **Chart** entry (highlighted)
   - canvas is bounded, theme-matched, the chart fits without scrolling
4. Initial chart shows **one role (`r-owner`)** containing **one position (`p-root`)** with **one worker chip (`w-owner`)**. No edges yet.

## 2. Add a new role

1. Click **+ New Role** in the top-right of the page.
2. Enter ID `r-engineer` and any content (e.g. `# Engineer\nBuilds things.`).
3. Submit.
4. Expect a new empty role frame labelled `r-engineer` to appear on the canvas next to `r-owner`.

## 3. Add a position to a role

Each role frame's header has three icon buttons on the right:

| Icon              | Tooltip                                          | Action                          |
| ----------------- | ------------------------------------------------ | ------------------------------- |
| `+` (box outline) | Add a position under this role                   | Opens **New position** dialog   |
| trash             | Delete role (cascade: positions + workers)       | Opens delete-confirm dialog     |

(`r-owner` only has the `+` icon — the trash is hidden because the
owner role is server-side protected.)

1. Inside the `r-engineer` frame's header, click the **+** icon.
2. Enter Position ID `p-eng-1`. Submit.
3. Expect an empty position card to appear inside the `r-engineer`
   frame, with:
   - `p-eng-1` label (top-left)
   - trash icon (top-right) — delete position
   - **Hire worker** button (bottom)
   - small target-handle dot on the top edge and source-handle dot on
     the bottom edge of the card
4. **The new position is an orphan** — no incoming edges. The success
   snackbar reads "position p-eng-1 created — draw an edge to a
   manager to set who they report to."

## 4. Wire reporting (manager → subordinate)

This is how the hierarchy gets built. There is no automatic parenting.

1. Hover the **bottom dot** of `p-root` (the source handle on the
   manager position) — the cursor turns into a crosshair.
2. Press, drag, and release on the **top dot** of `p-eng-1` (the
   target handle on the subordinate position).
3. Expect:
   - A bezier curve to appear connecting `p-root` (top) → `p-eng-1`
     (bottom).
   - Snackbar reads "p-eng-1 now reports to p-root".
   - The chart re-renders with `r-engineer` below `r-owner` in dagre
     rank.
4. Refresh the page — the edge persists.

If the user drops the wire outside any target handle, no edge is
created (ReactFlow's standard behaviour) and the chart is unchanged.

## 5. Hire a worker into a position

1. Click the **Hire worker** button on `p-eng-1`.
2. The right-side drawer opens. Fill:
   - Handle: `w-alice`
   - Kind: `human`
   - Identity content: `# Alice — software engineer.`
3. Click **Hire**.
4. Drawer closes; `p-eng-1` now shows a `w-alice` worker chip in place
   of the Hire button.
5. Refresh — the worker persists.

## 5a. Hire an AI worker and click the chip (regression: nil-deref)

This is the path that nil-derefed the API on every click before
[`feat(api): wire ProjectService through helix-org spawner`]
(`api/pkg/server/helix_org.go` builder + `lazyHelixOrgSpawner`). Run
it on every backend change touching `api/pkg/server/helix_org*.go` or
`api/pkg/org/infrastructure/runtime/helix/`.

1. Click the **Hire worker** button on `p-eng-2` (create the position
   first if needed).
2. In the drawer, choose **Kind: AI**, set Handle `w-ai-1`, and any
   identity content (`# w-ai-1 — automation drone.`).
3. Click **Hire**. The drawer closes and `p-eng-2` shows a `w-ai-1`
   chip with the robot icon (instead of the human silhouette).
4. **Click the `w-ai-1` chip** — this opens the worker drawer AND, on
   the API side, triggers the activation queue (the lazy spawner's
   first call for this worker).
5. Expect:
   - Worker drawer opens with the metadata (handle, kind=AI, identity).
   - **No API crash.** Check `docker compose logs --tail 50 api`: no
     `panic:` and no `runtime error: invalid memory address`.
   - The activation may still surface a runtime error (e.g. missing
     helix.api_key in the registry, no claude_code subscription, etc.)
     — that's fine; we're only asserting the spawner reaches the
     activation path without nil-derefing inside `WorkerProject.Ensure`.
6. The Go-side guard for this lives at
   `api/pkg/server/helix_org_spawner_test.go`:
   - `TestBuildHelixOrgSpawnerConfig_WiresProjectService` pins the
     builder.
   - `TestBuildHelixOrgSpawnerConfig_RejectsNilProjectService` pins
     the builder's nil-check.
   - `TestWorkerProjectEnsure_NilService_ReturnsError` pins the
     defensive guard inside the applier.
   Run `go test ./pkg/server/ -run TestBuildHelixOrgSpawnerConfig`
   before merging anything that touches this wiring.

## 6. Parallel positions and parallel edges

Two managers in the same role each having multiple subordinates is
the case that broke smoothstep edges. Verify it renders cleanly.

1. Add two more positions under `r-owner`:
   - From the `r-owner` header's **+** icon, create `p-ceo` and
     `p-cto`. Both are orphans for now (no parent_id).
2. Add more positions under `r-engineer`:
   - Create `p-eng-2`, `p-eng-3`, `p-eng-4` (orphans, same flow).
3. Wire a mix of reporting lines:
   - `p-ceo → p-eng-2` (drag bottom-of-p-ceo to top-of-p-eng-2)
   - `p-cto → p-eng-3`
   - `p-root → p-eng-4`
   - `p-ceo → p-eng-3` (one subordinate, multiple managers IS allowed
     — but in practice the last edge wins, since position.parent_id
     is single-valued. The previous `p-cto → p-eng-3` edge silently
     drops on the next chart refetch.)
4. Expect every reporting line to render as its own bezier curve. The
   trunk-style overlap that smoothstep produces with parallel
   managers must not appear.

## 7. Re-wire reporting

To change who a position reports to, just draw a new edge to it from
a different manager.

1. Wire `p-cto → p-eng-1` (manager source → subordinate target).
2. Expect the snackbar "p-eng-1 now reports to p-cto" and the chart
   re-renders with `p-eng-1` now hanging off `p-cto` instead of
   `p-root`. The old `p-root → p-eng-1` edge is gone (replaced, not
   added — a position has only one parent).

## 8. Sever reporting (delete an edge)

1. Click on the bezier curve from `p-cto` to `p-eng-1` to select it.
   Expect the line to thicken/colour-shift to indicate selection.
2. Press **Delete** (or **Backspace** on macOS).
3. Expect:
   - The edge disappears.
   - Snackbar reads "p-eng-1 no longer reports to anyone".
   - `p-eng-1`'s role frame may reflow as an orphan role if it has no
     other incoming edges.

## 9. Fire a worker

1. Click the `w-alice` chip in `p-eng-1`.
2. The worker drawer opens with metadata and a **Fire** button.
3. Click **Fire**, confirm.
4. Expect the chip to disappear; `p-eng-1` shows the **Hire worker**
   button again.
5. The owner worker (`w-owner`) cannot be fired — clicking Fire
   returns 409 and the snackbar surfaces a friendly error. The
   position card still shows `w-owner`.

## 10. Delete a position (cascades — fires its worker)

1. Hire someone into `p-eng-2` again (e.g. `w-carol`).
2. Click the **trash** icon in the top-right of the `p-eng-2` card.
3. Confirm the destructive action in the modal.
4. Expect:
   - `p-eng-2` disappears from the chart.
   - `w-carol` is also gone (the backend fired the worker as part of
     position-delete).
   - Any incoming edge to `p-eng-2` disappears with it.
5. Verify in the DB:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT id FROM org_workers WHERE org_id='<orgID>' AND id='w-carol';"
   ```
   should return 0 rows.
6. `p-root` cannot be deleted — the trash icon on `p-root` is hidden
   (the protection check sets `isRoot` and suppresses the affordance).

## 11. Delete a role (cascades — deletes every position under it and fires every worker)

1. Make sure `r-engineer` still has at least two positions, each with a
   worker.
2. Click the **trash** icon in the `r-engineer` role header.
3. Confirm in the modal — the body explicitly enumerates what will be
   deleted (e.g. "this will delete 3 positions and fire 2 workers:
   w-alice, w-newbie").
4. Expect:
   - The `r-engineer` frame disappears entirely.
   - All positions that were inside it are gone.
   - All workers that filled those positions are fired.
   - All edges to / from those positions disappear with them.
5. Verify in the DB:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT id FROM org_roles WHERE org_id='<orgID>' AND id='r-engineer';
      SELECT id FROM org_positions WHERE org_id='<orgID>' AND role_id='r-engineer';
      SELECT id FROM org_workers WHERE org_id='<orgID>' AND id IN ('w-alice','w-newbie');"
   ```
   all three return 0 rows.
6. The owner role (`r-owner`) has no trash icon. Attempting to delete
   it via the API returns 409.

## 12. Cross-org isolation

1. Switch to a second org via the org switcher in the top-left.
2. Open the chart for that org. Expect a fresh `r-owner / p-root /
   w-owner` baseline — none of the roles, positions, workers, or edges
   from the first org appear.
3. Hire `w-alice` into `p-root` of the second org. Confirm it appears
   in the second org's chart only — the first org's chart is unchanged.

## 13. Light + dark theme

1. Toggle the theme via the top-right sun/moon icon.
2. Expect both modes to render the chart cleanly:
   - light: white canvas, dark text, subtle grey grid, light role
     frame fills.
   - dark: dim grey canvas, light text, darker grid, dim role frame
     fills.
3. The minimap, controls, edge strokes, handle dots, and position card
   borders all swap with the theme.

## 14. Re-load / refresh

1. Hard-refresh the page (Ctrl-Shift-R).
2. Expect everything you've done — roles, positions, workers, and
   reporting edges — to persist. The chart is server-authoritative.

## Pass criteria

- All 14 sections complete without error.
- No console errors in the browser dev tools beyond the three Vite WS
  errors at startup (those come from the dev-server proxy, not the app).
- DB-level checks in sections 10 and 11 return 0 rows where expected.
- Parallel reporting lines (section 6) render as distinct bezier
  curves with no visible overlap.

## Known limitations (today)

- A position has at most one parent. Drawing a second incoming edge
  to the same position replaces the previous one — by design, since
  `position.parent_id` is single-valued.
- A role frame can be empty (zero positions); the UI handles that by
  showing the role with a "No positions yet — click + to add one"
  hint.
- A position card holds at most one worker. Hiring into an already-
  filled position is rejected with an error in the drawer.
- The owner worker / role / position (`w-owner` / `r-owner` /
  `p-root`) are protected from deletion at the API layer; the UI
  hides the relevant trash affordance and surfaces a friendly error
  if the API is hit directly.
