# Helix Org — QA test plan

End-to-end manual test for the helix-org chart UI. Run before merging
any change to `frontend/src/pages/HelixOrgChart.tsx`,
`frontend/src/components/orgs/HelixOrgSidebar.tsx`, `api/pkg/org/`, or
`api/pkg/server/helix_org*.go`.

Each section is a regression pin — the bug it guards is in the
heading. Skip nothing without reading the why.

## Mental model

- **Role** — flat container, no parent. Groups positions sharing the
  same role id.
- **Position** — slot inside one role, holds at most one worker. Two
  ReactFlow handles: target dot on top, source dot on bottom.
- **Edge** — manager-position bottom → subordinate-position top:
  "subordinate reports to manager". The only hierarchy.

Layout is dagre-driven; edges are bezier curves so parallel reporting
lines don't collapse onto one trunk.

## Setup

1. `docker compose -f docker-compose.dev.yaml ps` shows `helix-api-1`
   and `helix-frontend-1` Up.
2. Acting user has the `helix-org` alpha flag:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "UPDATE users SET alpha_features = ARRAY['helix-org']::text[] WHERE email='you@example.com';"
   ```
3. User is a member of the test org (`organization_memberships` row).
4. Sign in at `http://localhost:8080/login`.

Substitute `<org>` with the org slug (e.g. `test`) and `<orgID>` with
its `org_…` id in the SQL blocks below.

## 1. Baseline — bootstrap + sidebar

1. Click **Org** in the primary sidebar.
2. URL: `…/orgs/<org>/helix-org/chart`. Middle sidebar shows a
   highlighted **Chart** entry plus Roles / Workers / Streams /
   Settings entries.
3. Canvas shows **one role (`r-owner`)** containing **one position
   (`p-root`)** holding **one worker chip (`w-owner`)**, plus a
   dashed amber subscription edge to the auto-created
   `s-activations-w-owner` stream pseudo-node.
4. The chart endpoint must serve a single client-side load without a
   500. The HelixOrgChart page fires `/chart` `/workers` `/roles`
   `/streams` in parallel; before
   [`fix(api/server): serialize per-org helix-org bootstrap`]
   only the first request through the per-org bootstrap mutex
   succeeded and the rest 500-ed with
   `create owner role: already exists`. Watch the DevTools network
   tab on a hard refresh: **every helix-org request must be 2xx**.

## 2. Roles, positions, wiring (build the chart)

1. **+ New Role** on the chart toolbar → ID `r-engineer`, content
   `# Engineer`. Submit. Frame `r-engineer` appears next to `r-owner`.
2. Inside `r-engineer`'s header, click **+** → Position ID `p-eng-1`.
   Submit. Snackbar: `position p-eng-1 created — draw an edge to a
   manager to set who they report to`.
3. Drag from the **bottom dot** of `p-root` to the **top dot** of
   `p-eng-1`. Bezier curve appears; snackbar `p-eng-1 now reports to
   p-root`; `r-engineer` reflows below `r-owner` in dagre rank.
4. Hard-refresh — role, position and edge persist.

## 3. Hire human + AI workers (regression: nil-deref on chip click)

Bootstrap doesn't run the spawner for the owner, but it MUST run
without crashing for AI hires. The bug this section guards (nil-deref
inside `WorkerProject.Ensure`) crashed the API on every AI chip
click before
[`feat(api): wire ProjectService through helix-org spawner`]
landed.

1. Click **Hire worker** on `p-eng-1` → Kind `human`, Handle `w-alice`,
   any identity → **Hire**. Drawer closes; `w-alice` chip in
   `p-eng-1`.
2. Create `p-eng-2` (same flow as §2). Click **Hire worker** on it
   → Kind **AI**, Handle `w-ai-1`, any identity → **Hire**. AI chip
   renders with the robot icon.
3. **Click the `w-ai-1` chip**. URL becomes
   `…/helix-org/workers/w-ai-1`.
4. Check `docker compose logs --tail 50 api`: **no `panic:`, no
   `runtime error: invalid memory address`**. Activation may surface
   a non-fatal runtime error (missing claude_code subscription, etc.)
   — that's OK; we're only guarding the nil-deref.
5. Go-side guard for this lives at
   `api/pkg/server/helix_org_spawner_test.go` (`TestBuildHelixOrgSpawnerConfig_*`,
   `TestWorkerProjectEnsure_NilService_ReturnsError`). Run
   `go test ./pkg/server/ -run TestBuildHelixOrgSpawnerConfig` before
   merging anything touching the spawner wiring.

## 4. Parallel edges don't collapse (regression: smoothstep trunk)

Two managers with multiple subordinates each used to collapse into one
trunk under smoothstep edges. Verify bezier still keeps them apart.

1. Under `r-owner`, add positions `p-ceo` and `p-cto` (orphans).
2. Under `r-engineer`, add `p-eng-3` and `p-eng-4`.
3. Wire: `p-ceo → p-eng-2`, `p-cto → p-eng-3`, `p-root → p-eng-4`,
   then **re-wire** `p-ceo → p-eng-3` (last edge wins;
   `position.parent_id` is single-valued, so the previous
   `p-cto → p-eng-3` silently drops).
4. Every reporting line must render as its own bezier curve — no
   trunk overlap.

## 5. Re-wire + sever (regression: Delete key)

1. Drag `p-cto → p-eng-1`. Snackbar `p-eng-1 now reports to p-cto`;
   the previous `p-root → p-eng-1` edge disappears (replaced, since
   position has a single parent).
2. Click the `p-cto → p-eng-1` bezier to select it (it thickens /
   colour-shifts). Press **Delete** OR **Backspace**. Edge
   disappears; snackbar `p-eng-1 no longer reports to anyone`.
3. **Both keys must work** — @xyflow/react v12 defaults
   `deleteKeyCode` to Backspace only, which left Linux/Windows users
   unable to sever an edge. Fix in
   [`fix(frontend/helix-org): accept Delete key for edge severing`]
   set it to `['Backspace', 'Delete']`.

## 6. Owner protection + cascade semantics

Pin three rules at once: owner refuses delete (409), position-delete
fires its worker, role-delete fires every worker under it.

1. Try to fire `w-owner` (click chip → **Fire worker** → confirm).
   Expect a friendly snackbar surfacing the 409
   `cannot fire the owner worker`; chip stays.
2. Hire `w-carol` into `p-eng-2` again. Click the trash on the
   `p-eng-2` card → confirm. Dialog body must explicitly enumerate
   what cascades: `Deleting position p-eng-2 will cascade:
   • fire worker w-carol`. Confirm. Position vanishes, w-carol gone.
3. Hire `w-alice` into `p-eng-1` and `w-newbie` into `p-eng-3`.
   Click trash on the `r-engineer` role header. Dialog body must
   enumerate `3 positions (p-eng-1, p-eng-3, p-eng-4) … 2 workers
   (w-alice, w-newbie)`. Confirm. Frame, positions, workers, and
   all attached edges disappear together.
4. DB-level sanity:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c "
     SELECT count(*) FROM org_roles WHERE org_id='<orgID>' AND id='r-engineer';
     SELECT count(*) FROM org_positions WHERE org_id='<orgID>' AND role_id='r-engineer';
     SELECT count(*) FROM org_workers WHERE org_id='<orgID>' AND id IN ('w-alice','w-newbie');"
   ```
   all three return 0. The owner-role trash icon is hidden; hitting
   the API directly returns 409 `cannot delete the owner role`.

## 7. Cross-org isolation + persistence

1. Switch to a second org via the top-left selector. Chart shows a
   fresh `r-owner / p-root / w-owner` baseline (plus its activation
   stream) — none of the test-org content leaks across.
2. Hire `w-alice` into `p-root` of the second org. It appears only in
   the second org's chart. Switch back: first org unchanged.
3. Restart the API container (or wait for the next Air rebuild) and
   re-open the chart. Every row above persists — guards the
   regression where production wired the org store with
   `ResetSchema=true` and dropped every `org_*` table on each boot
   (fix in `fix(api/server): stop dropping org_* tables`).

## 8. Light + dark theme

Toggle the top-right sun/moon icon. Both modes must render the chart,
minimap, controls, edge strokes, handle dots, and position card
borders cleanly; ReactFlow's `data-color-mode` follows the toggle.

## 9. Roles list + tool manifest (regression: 29-tool bootstrap)

The owner role's tools manifest must list every built-in tool. The
chart and API answer the same way.

1. Click **Roles** in the helix-org sidebar. URL:
   `…/helix-org/roles`. Table columns:
   ID / Content / Tools / Streams / Updated.
2. The `r-owner` row's **Tools count must be 29** (same as
   `Registry.List()` returns from `GET /api/v1/orgs/<org>/tools`).
   API check:
   ```bash
   curl -sH "Cookie: $(grep helix_sess ~/.helix-creds.txt | cut -d= -f2-)" \
     "http://localhost:8080/api/v1/orgs/<org>/roles/r-owner" \
     | jq '.tools | length'
   ```
   returns `29`. The bootstrap seed in
   `api/pkg/org/application/bootstrap/bootstrap.go::Run` shares one
   slice with the owner Worker's ToolGrants, so the two stay in
   lockstep; this assertion guards against a future refactor dropping
   the manifest seed.
3. The row's vertical-dot menu shows **Open** + **Delete** (the
   Delete is disabled for `r-owner` with `Owner — protected`).

### 9b. Add/remove a tool via multi-select

Exercises the role editor's tool dropdown end-to-end.

1. From Roles, **+ New Role** → `r-test-dm` with any content. The
   detail page opens; Tools field empty.
2. Click the empty Tools dropdown. 29 options render, each with a
   checkbox, monospace tool name, and one-line description. Clicking
   **dm** must **NOT** close the popper (`disableCloseOnSelect` on).
3. A chip `dm` appears in the field. Press Escape to close.
4. **Save** (which was disabled until now) is enabled. Click it →
   snackbar `role r-test-dm saved` → button returns to disabled.
5. Hard refresh; chip persists.
   `curl …/api/v1/orgs/<org>/roles/r-test-dm | jq '.tools'` returns
   `["dm"]`.
6. Re-open the dropdown, click `dm` again — chip disappears. Save.
   Tools back to `[]`. Delete `r-test-dm` via the right-rail Delete
   button (cleanup).

## 10. Workers list page

`…/helix-org/workers` — third item in the middle nav. Table columns
ID / Kind / Position / Identity / Tools. **No "+ New Worker" button**:
hires come from chart position cards, so role+position context is
explicit at hire time. Vertical-dot menu offers **Open** and **Fire**
(disabled on `w-owner` with `Owner — protected`).

## 11. Chat → Human Desktop (regression: bare /agent route)

Critical worker flow. Chat MUST happen inside the per-Worker project's
Human Desktop session — same surface a normal project uses — not the
bare agent composer at `/agent/<id>`.

**Pre-condition**: `zed_external` runtime wired on this host;
otherwise the desktop session is created but never connects.

1. Navigate to `…/helix-org/workers/w-owner`. The chat button label
   is **Open Human Desktop** (project provisioned) or
   **Provision + open Human Desktop** (right-rail "Project" empty).
2. Click. While `ensureWorkerChat` POSTs and provisions the project
   and exploratory session, the label flips to
   `Provisioning agent app…`.
3. **The page MUST land on `/orgs/<org>/projects/<project_id>/desktop/<session_id>`.**
   Landing on `/orgs/<org>/agent/<id>` is a regression of
   [`feat(helix-org): chat button opens the Human Desktop`]
   — the agent route is the legacy bare composer and is no longer
   reachable from this button.
4. The desktop viewer renders with a `Send message to agent…`
   composer. Type `hello`, submit, user bubble appears in the
   transcript. The LLM reply may not arrive if `zed_external` isn't
   actually connected (`helixml/helix#2397` territory) — that's not
   a chart regression.
5. Refresh the worker detail page. The right-rail **Project** field
   now shows the project id. Clicking the button again navigates
   straight to the desktop route (Ensure fast-paths, exploratory-
   session GET returns the existing session).

## Pass criteria

- §1 — every helix-org request on first chart load is 2xx (no
  bootstrap race 500).
- §3 — no API panic on AI chip click.
- §4 — parallel reporting lines render as distinct beziers.
- §5 — Delete *and* Backspace both sever a selected edge.
- §6 — owner refuses delete (409), cascade dialogs enumerate
  collateral, DB counts drop to 0 after confirm.
- §7 — chart survives an API restart with all rows intact.
- §9 — `r-owner.tools | length == 29`.
- §11 — chat button lands on `…/projects/<pid>/desktop/<sid>`,
  never `…/agent/<id>`.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- A position has at most one parent — a second incoming edge replaces
  the first (`position.parent_id` is single-valued).
- A role frame can be empty (zero positions); the UI shows
  "No positions yet — click + to add one".
- A position holds at most one worker — hiring into a filled
  position is rejected.
- `w-owner` / `r-owner` / `p-root` are protected at the API; UI
  hides the trash affordance and surfaces a friendly 409.
