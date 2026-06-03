# Helix Org — QA test plan

End-to-end manual test for the helix-org chart UI. Lives in
`api/pkg/org/QA.md` because it's specific to the org-mode alpha; run
it before merging any change that touches `frontend/src/pages/HelixOrgChart.tsx`,
`frontend/src/components/orgs/HelixOrgSidebar.tsx`, `api/pkg/org/`, or
`api/pkg/server/helix_org*.go`.

The chart visualises **two levels** of grouping:

```
Role (group)
├── Position (slot, holds 0 or 1 worker)
├── Position
└── Position
```

Multiple positions can share a Role (e.g. several engineers all hold
`r-engineer`). Each Position holds at most one Worker. The UI must
let an operator add a Role, add a Position under that Role, hire a
Worker into an empty Position, fire the Worker, delete a Position,
and delete a Role. The last two cascade in the backend — deleting a
Position fires its Worker; deleting a Role removes every Position
under it and fires every Worker in those Positions.

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
2. Expect the URL to become `http://localhost:8080/orgs/<org>/helix-org/chart`.
3. Expect the page chrome:
   - breadcrumbs read `<org> / Chart`
   - title block reads **Chart** with the explanatory subtitle
   - middle sidebar shows a single **Chart** entry (highlighted)
   - canvas is bounded, theme-matched (light/dark), chart fully visible without scrolling
4. Expect the initial chart to show **one Role group (`r-owner`)** containing **one Position (`p-root`)** with **one Worker chip (`w-owner`)**.

## 2. Add a new Role

1. Click **+ Role** in the canvas toolbar.
2. Enter ID `r-engineer` and any content (e.g. `# Engineer\nBuilds things.`).
3. Submit.
4. Expect a new empty group labelled `r-engineer` to appear on the canvas. No positions inside yet.

## 3. Add a Position under that Role

1. Inside the `r-engineer` group, click **+ Position**.
2. Enter Position ID `p-eng-1`.
3. Submit.
4. Expect an empty Position card to appear inside the `r-engineer` group, with a **Hire** affordance and no Worker chip.

## 4. Hire a Worker into the empty Position

1. Click the empty `p-eng-1` Position card.
2. The right-side drawer opens. Fill:
   - Handle: `w-alice`
   - Kind: `human`
   - Identity content: `# Alice — software engineer.`
3. Click **Hire**.
4. Expect the drawer to close and the `p-eng-1` Position card to now show a `w-alice` Worker chip.
5. Refresh the page — the Worker should still be there (it's persisted).

## 5. Add a second Position under the same Role

1. Click **+ Position** inside `r-engineer` again.
2. Enter ID `p-eng-2`. Submit.
3. Hire another Worker (`w-bob`) into `p-eng-2` the same way as step 4.
4. Expect the `r-engineer` group now contains two Position cards — `p-eng-1 → w-alice` and `p-eng-2 → w-bob`.

## 6. Fire a Worker

1. Click the `w-bob` chip in `p-eng-2`.
2. The Worker drawer opens with metadata and a **Fire** button.
3. Click **Fire**, confirm.
4. Expect the chip to disappear; the `p-eng-2` Position is now empty (with the Hire affordance back).
5. The owner Worker (`w-owner`) cannot be fired — clicking Fire returns 409 and surfaces a friendly error.

## 7. Delete a Position (cascades — fires its Worker if any)

1. Hire someone into `p-eng-2` again (e.g. `w-carol`).
2. Click the `p-eng-2` Position to select it.
3. Click **Delete Position** (in the drawer or a hover affordance).
4. Confirm the destructive action.
5. Expect:
   - `p-eng-2` disappears from the chart
   - `w-carol` is also gone (the backend fired the Worker as part of position-delete)
6. Verify in the DB:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT id FROM org_workers WHERE org_id='<orgID>' AND id='w-carol';"
   ```
   should return 0 rows.

## 8. Delete a Role (cascades — deletes every Position under it and fires every Worker)

1. Make sure `r-engineer` still has at least one Position with a Worker (`p-eng-1 → w-alice`).
2. Add another quick Position + Worker so it's clear the cascade works for multiple.
3. Click the `r-engineer` group header.
4. Click **Delete Role**.
5. Confirm the destructive action — the modal explicitly enumerates what will be deleted (e.g. "this will delete 2 Positions and fire 2 Workers: w-alice, w-newbie").
6. Expect:
   - The `r-engineer` group disappears entirely
   - All Positions that were inside it are gone
   - All Workers that filled those Positions are fired
7. Verify in the DB:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT id FROM org_roles WHERE org_id='<orgID>' AND id='r-engineer';
      SELECT id FROM org_positions WHERE org_id='<orgID>' AND role_id='r-engineer';
      SELECT id FROM org_workers WHERE org_id='<orgID>' AND id IN ('w-alice','w-newbie');"
   ```
   all three should return 0 rows.
8. The owner Role (`r-owner`) cannot be deleted — the delete affordance is either disabled or the action returns 409.

## 9. Cross-org isolation

1. Switch to a second org via the org switcher in the top-left.
2. Open the chart for that org. Expect a fresh `r-owner / p-root / w-owner` baseline — none of the Roles, Positions, or Workers from the first org appear.
3. Hire `w-alice` into `p-root` of the second org. Confirm it appears in the second org's chart only — the first org's chart still shows whatever you left there.

## 10. Light + dark theme

1. Toggle the theme via the top-right sun/moon icon.
2. Expect both modes to render the chart cleanly:
   - light: white canvas, dark text, subtle grey grid, light Role group fills
   - dark: dim grey canvas, light text, darker grid, dim Role group fills
3. The mini-map, controls, edges, and Position card borders all swap with the theme.

## 11. Re-load / refresh

1. Hard-refresh the page (Ctrl-Shift-R).
2. Expect everything you've done to persist — the chart is server-authoritative, not client-state.

## Pass criteria

- All 11 sections complete without error.
- No console errors in the browser dev tools (the three Vite WS errors at startup are expected — those come from the dev-server proxy, not the app).
- DB-level checks in steps 7 and 8 return 0 rows where expected.

## Known limitations (today)

- The Role group can be empty (zero Positions) — that's allowed and the UI must handle it.
- The Position card holds at most one Worker. Hiring into an already-filled Position is rejected with an error in the drawer.
- The owner Worker / Role / Position (`w-owner` / `r-owner` / `p-root`) are protected from deletion at the API layer; the UI must surface that as a friendly error, not a stack trace.
