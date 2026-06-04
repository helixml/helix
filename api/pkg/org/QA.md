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
- **Position** — slot inside one role, holds at most one worker. Three
  ReactFlow handles: target dot on top, source dot on bottom (for
  manager→subordinate edges), and a small amber dot on the right for
  drag-to-subscribe edges to stream nodes.
- **Reporting edge** — manager-position bottom → subordinate-position
  top: "subordinate reports to manager". The hierarchy.
- **Subscription edge** — position right → stream pseudo-node:
  "events on this stream activate whoever fills this slot".
  POSITION-anchored, not worker-anchored: hiring or firing a worker
  doesn't change which streams the slot consumes.

Layout is dagre-driven; edges are bezier curves so parallel reporting
lines don't collapse onto one trunk. Stream pseudo-nodes live in a
dedicated column to the right of the org tree so they never collide
with the role grid; the right-side stream handle keeps subscription
edges off the bottom-handle reporting geometry.

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

## 11. Streams: list, anchoring, detail page, live tail

The Streams surface lives at `…/helix-org/streams`. Every Worker has
an auto-created activation Stream (`s-activations-<workerID>`) so a
non-empty org always has at least one row.

### 11a. Streams list

1. Click **Streams** in the helix-org sidebar. URL becomes
   `…/helix-org/streams`. Table columns: ID / Name / Transport /
   Subscribers / Created.
2. Every Worker on the chart must have a matching
   `s-activations-<workerID>` row. Fresh org with just w-owner
   shows exactly one row; hire `w-alice` and a second row appears
   after a brief refetch.
3. Top-right **+ New Stream** opens the create dialog. The
   **Transport** dropdown lists `local`, `webhook`, `github`,
   `postmark`. Picking `github` swaps the config fields for a
   single searchable **Repository** dropdown (populated from
   `GET /github/repos`); helix installs the webhook for the
   operator after Create. Full one-click flow is pinned in §11e.

### 11b. Chart anchoring (regression: every stream dangled off w-owner)

Open the chart with multiple workers hired. Pre-fix, every activation
stream's edge originated from `p-root` because the dashed edge
followed the SUBSCRIBER (always w-owner via the hire hook) instead
of the SUBJECT. The current model derives each dashed edge from
real `org_subscriptions` rows (position-anchored), so one stream can
show edges from multiple positions if multiple slots subscribed.

1. With `w-owner` in `p-root` and an AI worker (`w-alice`) hired
   into some `p-eng-N`, after hiring `w-alice` (which auto-
   subscribes the hiring caller's position via
   `EnsureActivationStream`) expect dashed edges:
   - `p-root → s-activations-w-owner` (owner watches own activations
     via the bootstrap auto-sub)
   - `p-root → s-activations-w-alice` (owner watches the new hire's
     activations — the hiring caller subscribes)
   - **NO** edge from `p-eng-N` to its own activation stream — the
     worker's own position is NOT subscribed (would loop dispatch).
2. Stream nodes live in a vertical column to the right of the org
   tree, not in line with the role grid. Adding another position to
   the role frame must not push or overlap a stream node.
3. Edges from the position's **right** handle (the dedicated
   `stream` source handle), not the bottom — so a stream sitting
   directly below a subordinate position never produces overlapping
   geometry. Visually inspect: solid reporting edges stay vertical
   between role frames; dashed stream edges run horizontally to the
   column on the right.

### 11c. Stream detail page (regression: only a list existed)

This is the "messages flowing through the system" view. Originally
there was no per-stream surface — clicking a stream just dumped you
back on the list. The detail page rebuilds the old htmx
`/ui/streams?id=…` shape as React, hydrated from `GET /streams/{id}`
then live-tailed via SSE.

1. **Entry point 1 — chart**: click any stream pseudo-node. URL
   becomes `…/helix-org/streams/<stream_id>`. Header shows the
   stream id (monospace) + transport kind chip + description +
   `created by <worker> · <timestamp>` + subscribers chip-list.
2. **Entry point 2 — list**: the ID column in the streams table is
   a link. Clicking it lands on the same detail URL.
3. Backend pin (`TestGetStream_IncludesRecentEvents`): the page
   depends on `GET /streams/{id}` carrying
   `recent_events: EventCard[]` in newest-first order.
4. Messages section shows EventCard rows: `<from> [→ <to>]` on the
   left, ISO timestamp on the right, subject (if any) on a second
   line, then either the canonical message body (when
   `has_message=true`) or the raw body, finally the event id in
   monospace.

### 11d. Live SSE tail

The detail page subscribes to
`GET /api/v1/orgs/<org>/streams/<stream_id>/events` and replaces the
list wholesale on every server push.

1. With the detail page open on a stream, publish two new events to
   that stream from another tab or curl:
   ```bash
   curl -sH "Cookie: $(grep helix_sess ~/.helix-creds.txt | cut -d= -f2-)" \
     -H "Content-Type: application/json" -X POST \
     -d '{"subject":"sse-test","body":"arrived live"}' \
     "http://localhost:8080/api/v1/orgs/<org>/streams/<stream_id>/publish"
   ```
2. **Without reloading**, the new event must appear at the top of
   the Messages list within ~1.5 seconds. The total count must not
   double — each SSE frame replaces, not appends. A flickering
   "everything re-renders" is normal and is the simpler-than-diff
   contract this page is built on.

### 11e. GitHub streams — one-click setup

Earlier iterations of the github-transport flow asked the operator
to copy a payload URL into GitHub's webhook UI, paste a webhook
secret, and pick events by hand. The current flow does ALL of that
on their behalf — the only thing the user picks is *which repo* to
subscribe to. Helix calls GitHub's REST API with the operator's
connected OAuth token, registers the webhook with the right URL,
content-type, secret, and `events: ["*"]` (send-me-everything)
default, and stores the resulting hook id back on the stream so
the detail page can deep-link to it.

#### Pre-conditions
- Operator has a connected GitHub OAuth (helix Connected Services
  page). That's what `GitHubTokenResolver` returns — without it
  `GET /github/repos` and the install endpoint both 412.
- `SERVER_URL` is a publicly reachable host (NOT `localhost:8080`
  / `127.0.0.1` / `0.0.0.0`). The install endpoint refuses with
  412 on loopback origins so operators don't end up with a hook
  pointed at a URL GitHub can't reach. Use cloudflared / ngrok /
  reverse proxy for local testing.

#### Create flow
1. **Streams → New Stream**. Transport = `github`.
2. **Repository** is a searchable Autocomplete populated by
   `GET /api/v1/orgs/<org>/github/repos`. Typing filters the
   list. (Empty / error state: "Could not load repos — connect a
   GitHub account on Connected Services first.")
3. No events picker on the create dialog — defaults to `["*"]`.
   The caption reads "Default: receive every event from this
   repo. You can narrow the events whitelist from the stream's
   detail page after it's created."
4. **Create**. Two things happen in sequence:
   - `POST /streams` creates the stream row.
   - `POST /streams/<id>/github/install-webhook` calls the GitHub
     API. The toast on success reads
     "Stream created · webhook installed on GitHub (id <hookID>)".
   - On install failure (e.g. repo without admin rights), the
     stream is still created and the toast tells the operator
     "Stream created but webhook install failed: <reason>. Open
     the stream detail page and click 'Re-install webhook'."

#### Detail page
5. Open the stream detail page. The **Connect to GitHub** panel
   shows one of three states:
   - **Installed** — "Helix has registered a webhook on
     `owner/name` (id <hookID>). Deliveries flow into this stream
     automatically — no manual setup." Buttons: **Edit on GitHub
     →** (opens
     `https://github.com/<owner>/<name>/settings/hooks/<id>` in
     a new tab) + **Re-install** (idempotent).
   - **Not installed** — "No webhook registered yet for `repo`.
     Helix can install it for you — one click, no copying URLs."
     Button: **Install webhook on GitHub**.
   - **Loopback warning** — if `SERVER_URL` is localhost, a red
     banner at the top of the panel says "GitHub's servers can't
     reach this URL, so deliveries won't arrive. Run helix behind
     a public hostname (cloudflared / ngrok / reverse proxy) and
     update SERVER_URL before relying on this stream." Sits ABOVE
     either of the other two states.

#### Backend regression pins
6. `GitHubConfig` (`api/pkg/org/domain/transport/github.go`) gained
   two optional fields: `webhook_id` (int64) and
   `webhook_html_url` (string). Populated by
   `installGitHubWebhook` after a successful GitHub API call;
   serialised into the stream's `transport_config` JSON column.
7. The events whitelist now accepts `"*"` as a wildcard meaning
   "deliver every event". Validator allows it (special case in
   `Validate()`); transport's `contains()` short-circuits on `*`.
   Mirrors GitHub's own `events: ["*"]` semantics so the API call
   we make matches what helix's transport accepts.
8. `transport.github.webhook_secret` is **auto-generated** on
   first install (`ensureGitHubWebhookSecret` in
   `api/pkg/org/interfaces/server/api/api.go`). 32-byte random
   hex, persisted via `Registry.Set` as the system owner. The
   operator never has to paste a secret manually. Existing
   secrets are preserved — if the operator set one before, it's
   reused.
9. `installGitHubWebhook` is idempotent — re-running the
   mutation adopts an existing hook on the repo whose URL
   matches helix's, rather than creating a duplicate. Pin in
   `github.Client.UpsertWebhook`.

#### End-to-end test (real cloudflared tunnel)
10. `cloudflared tunnel --url http://localhost:8080`, copy the
    public URL.
11. `export SERVER_URL=<public-url>` in `.env`, restart api
    container.
12. Create a github stream for a repo you own. Confirm:
    - `SELECT transport_config FROM org_streams WHERE id='<id>'`
      shows `{"repo":"owner/name","events":["*"],"webhook_id":<N>,"webhook_html_url":"https://github.com/owner/name/settings/hooks/<N>"}`.
    - Visit the html URL in a fresh tab — GitHub's webhook
      edit page loads, Payload URL matches the public stream
      URL, Content type is `application/json`, "Just the push
      event?" → "Send me everything" is selected.
13. Open an issue on the repo. Within seconds the stream
    detail page's Messages feed shows the delivery (SSE).

#### Edge cases
14. **No GitHub OAuth connection** — Repository dropdown is
    empty + helper text points to Connected Services. Create
    button stays clickable (operator can still pick `local`
    transport).
15. **Localhost SERVER_URL** — Create succeeds (no need to gate
    creation on the URL being public), but install returns 412
    with "SERVER_URL ... is a loopback address — GitHub can't
    reach it." Detail page shows the loopback warning + "not
    installed" state.
16. **Re-install** — clicking the button on a stream that
    already has a webhook hits the same endpoint; server adopts
    the existing hook (no duplicate). Toast: "Webhook installed
    on GitHub (id <same-id>)."

### 11f. Delete from chart + Fire cascade (regression: orphan activation streams)

Before this fix, firing a Worker left its `s-activations-<workerID>`
Stream lying around — the Streams page kept rendering a ghost row
and the chart kept a dashed pseudo-node for a Worker that no longer
existed. There was also no UI affordance to clean a Stream up from
the chart itself; the operator had to leave the canvas to delete
via the Streams list. Both fixed.

1. Hire a fresh AI worker (`w-cleanup` into a new position). The
   chart shows the worker chip + a new
   `s-activations-w-cleanup` dashed pseudo-node anchored to its
   position (per §11b).
2. Fire `w-cleanup` (chip → Fire worker → confirm). Expect:
   - The position card returns to **Hire worker** state.
   - **The `s-activations-w-cleanup` pseudo-node disappears from
     the chart** within ~1s (cascade from `lifecycle.Fire`).
   - DB-level sanity:
     ```bash
     docker exec helix-postgres-1 psql -U postgres -d postgres -c \
       "SELECT id FROM org_streams WHERE org_id='<orgID>' AND id='s-activations-w-cleanup';"
     ```
     returns 0 rows. Events on that stream survive in `org_events`
     as an audit trail (the events table isn't keyed on Streams).
3. Pin: `lifecycle_test.go::TestFire_RemovesWorkersActivationStream`
   covers the same contract at the Go layer.
4. Chart-side cleanup: hover any stream pseudo-node — a small trash
   icon appears in the top-right. Click it. The same
   ConfirmDeleteDialog used for role/position deletes opens with
   the body:
   ```
   Deleting stream <id>:
     • removes the Stream row
     • drops N subscription(s) (<position ids>)
     • events on this stream survive as an audit trail
   This is irreversible.
   ```
   The subscription chips are POSITION IDs (subscriptions are
   position-anchored — see §11g).
5. Confirm. The pseudo-node vanishes from the chart immediately;
   `GET /api/v1/orgs/<org>/streams/<id>` returns 404.
6. The Streams page (Vertical-dot Delete menu) must surface the
   same DELETE — both UIs hit the same endpoint and share the same
   cache-invalidation, so the deleted stream disappears from the
   list view too without a manual refresh.

### 11g. Position-anchored subscriptions (data-model pivot)

Subscriptions used to be keyed on `(org, worker, stream)`. Firing a
worker dropped every stream they consumed, and a new hire into the
same slot started fresh — wrong for the common case where the slot
("eng lead") owns the consumed channels. The current model keys on
`(org, position, stream)`: subscriptions outlive workers,
DeletePosition is the only cascade that drops them.

Three surfaces exercise this contract.

#### Survives-fire (dispatch model)

1. Hire `w-cycle` (AI) into a new `p-cycle` position. Subscribe
   `p-cycle` to `s-test-feed` (either via the chart drag in §11g.UI
   or via `POST /api/v1/orgs/<org>/positions/p-cycle/subscriptions
   {stream_id:"s-test-feed"}`).
2. Fire `w-cycle` via the chip's **Fire worker** flow. The
   subscription row MUST survive — `lifecycle.Fire` is forbidden
   from touching `org_subscriptions`. DB-level check:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT * FROM org_subscriptions WHERE org_id='<orgID>' \
      AND position_id='p-cycle' AND stream_id='s-test-feed';"
   ```
   returns 1 row.
3. Hire a fresh AI worker `w-cycle-2` into `p-cycle`. Publish an
   event to `s-test-feed`. The dispatcher MUST activate `w-cycle-2`
   even though `w-cycle-2` never explicitly subscribed — the
   subscription on `p-cycle` is inherited.
4. Now DeletePosition `p-cycle` (chart trash icon → confirm). The
   row from step 2 must be gone. Re-query the SELECT — 0 rows. The
   `lifecycle.DeletePosition` cascade is the only path that drops
   subscriptions; the role-delete cascade reaches them indirectly
   via DeletePosition.

#### Chart drag-to-subscribe

5. Open the chart. Hover any position card — a small amber dot
   appears on the **right** edge (the dedicated stream-source
   handle, distinct from the bottom reporting source).
6. Drag from that dot to the top edge of a stream pseudo-node and
   release. Snackbar: `<position id> now consumes <stream id>`. A
   dashed amber edge from the position to the stream renders on
   the next refetch (≤1.5s). DB row appears at
   `org_subscriptions(<orgID>, <position>, <stream>)`.
7. Re-drag the same pair → idempotent. The backend returns 200
   with the existing row and the chart doesn't duplicate the edge.

#### Worker detail Subscriptions panel

8. Navigate to `…/helix-org/workers/<workerId>`. Below "Tool grants"
   a **Subscriptions (N)** section renders. The number reflects
   the worker's position's current subscription set (not the
   worker's — the panel resolves worker → position before fetching).
9. Click the multi-select. The popper lists every Stream in the org
   with description, checkbox state for currently-subscribed
   entries, and `disableCloseOnSelect` so toggling several streams
   in one pass doesn't bounce closed (same UX shape as the role
   tools dropdown).
10. Tick a stream → subscribe; untick → unsubscribe. Snackbar
    confirms the delta count. Captioned beneath: "Subscriptions are
    position-anchored — they outlive the worker. Whoever fills
    `<position>` next inherits this set."
11. Unassigned workers (no position) render the panel as a
    read-only "This Worker is unassigned (no position) — there's
    nothing to subscribe." rather than failing.

## 12. Chat → Human Desktop (regression: bare /agent route)

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
   transcript. The full chat round-trip (LLM reply arrives,
   `gh` works inside the sandbox, etc.) is pinned separately in
   §13 — this step only confirms the routing + composer mount.
5. Refresh the worker detail page. The right-rail **Project** field
   now shows the project id. Clicking the button again navigates
   straight to the desktop route (Ensure fast-paths, exploratory-
   session GET returns the existing session).

## 13. Worker sandbox: Zed launch, `gh` auth, stale-session recovery

This section pins the chain from "click Open Human Desktop" to
"chat goes from React composer through API → WebSocket → Zed →
Claude → response". The current iteration ships `gh` inside the
desktop image with the org's GitHub OAuth token auto-injected as
`GH_TOKEN`, so a worker can comment on GitHub issues / PRs the
moment its desktop is up.

### 13a. Fresh sandbox container — Zed launches cleanly

Regression pin: an earlier iteration of the Dockerfile invoked
`gh --version` at the end of the install layer as a smoke test.
`gh` writes `${HOME}/.local/state/gh/device-id` on first
invocation as its analytics opt-in. During the docker build HOME
is root, so this layer left `/home/retro/.local/` owned by root —
and Zed, running as retro at session-start, crashed on:

  `Zed failed to launch: permission denied when creating
   directories ["/home/retro/.local/share/zed/extensions", …]`

Without Zed there's no agent WebSocket back to the API, so every
follow-up chat interaction failed with "no external agent
WebSocket connection" and auto-wake retries exhausted. The
end-user symptom was the chat composer queueing messages forever.
Fix: don't invoke `gh` during the build — `apt-get install -y gh`
already fails the build on installation error, no runtime smoke
test needed (replaced with `which gh`).

Verify on a fresh container spawned from the current desktop
image (`sandbox-images/helix-ubuntu.version`):

1. Hire a fresh AI worker via the chart → wait for the container
   to spawn.
2. `docker compose exec sandbox-nvidia docker exec <container> ls -la /home/retro/.local`
   must show ownership `retro retro` for `.`, `share/` and
   `state/` (NOT `root root`).
3. `docker compose exec sandbox-nvidia docker exec <container> pgrep -fa zed`
   shows `/zed-build/zed /home/retro/work/<workerID> /home/retro/work/helix-specs`
   running as retro.
4. `docker compose exec sandbox-nvidia docker logs <container> | grep WEBSOCKET`
   shows `✅ [WEBSOCKET] WebSocket connected!` followed by
   `Sent initial agent_ready (connection ready for messages)`.
5. API logs show `External agent added message … role=assistant`
   lines as Zed's Claude processes the hire-time activation.

### 13b. `gh` is installed and authenticated via injected `GH_TOKEN`

The desktop image ships `gh` from the official cli.github.com apt
repo. The helix-org spawner injects the org's GitHub OAuth token
(via the same `GitHubTokenResolver` the github-stream transport
uses) as the project secret `GH_TOKEN` on every activation; the
sandbox runtime exposes project secrets as env vars on container
start, so a fresh desktop has `gh` already authenticated.

1. Open a terminal inside the desktop sandbox (Ghostty already
   exists; otherwise `docker compose exec sandbox-nvidia docker exec -u retro <container> bash`).
2. `echo "$GH_TOKEN"` returns a 40-char `gho_…` token.
3. `gh --version` reports the installed version (no analytics
   opt-in prompt — the device-id file is written under retro's
   home now, not root's).
4. `gh auth status` reports `✓ Logged in to github.com account
   <login> (GH_TOKEN)`. Scopes shown: `repo, admin:repo_hook,
   read:org` (assuming the operator re-authed via the
   "Reconnect with stream permissions →" link in §11e after the
   read:org scope was added).
5. `gh issue comment <number> --repo <owner>/<repo> --body
   "from helix-org worker"` succeeds — confirms the token has
   real write authority. Comment lands on the issue under the
   operator's GitHub identity.

### 13c. Stale-session recovery after a desktop-image rebuild

Whenever `./stack build-ubuntu` ships a new image (because the
Dockerfile changed, or a new Zed binary landed), containers from
the old image are still pointed at by every pre-existing
exploratory session row. The API marks them "running" because
the underlying session record still says so, even after we kill
the container — so "Open Human Desktop" reuses the dead pointer
instead of spawning fresh. End-user symptom: clicking the button
lands on a black desktop viewer; FE polls the screenshot endpoint
and the API logs `Failed to connect to sandbox via RevDial for
stream WebSocket: "no connection"` forever.

Recovery (single-host dev only — production has hydra-driven
session lifecycle handled separately):

1. Identify stale sessions by image:
   ```bash
   docker compose -f docker-compose.dev.yaml exec sandbox-nvidia \
     docker ps --format "{{.Names}}\t{{.Image}}" | grep ubuntu-external
   ```
   Anything NOT on the current `sandbox-images/helix-ubuntu.version`
   tag is stale.

2. Remove the stale containers:
   ```bash
   docker compose -f docker-compose.dev.yaml exec sandbox-nvidia \
     docker rm -f <ubuntu-external-…>
   ```

3. Wipe their `sessions` rows + their pointers from
   `org_worker_runtime_state` so the FE can't reuse them:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c "
     DELETE FROM sessions WHERE id IN ('ses_<stale-1>','ses_<stale-2>');
     DELETE FROM org_worker_runtime_state
       WHERE key='session_id'
         AND value IN ('ses_<stale-1>','ses_<stale-2>');
   "
   ```

4. Click **Open Human Desktop** on each affected worker. The
   FE's `openChat` calls `v1ProjectsExploratorySessionDetail`,
   gets 204 (no session), calls `v1ProjectsExploratorySessionCreate`,
   the API spawns a fresh container from the current desktop
   image, Zed launches with proper `/home/retro/.local`
   ownership, and chat works.

5. **DO NOT** try to "resume" stale sessions in place. The
   `POST /sessions/<id>/resume` endpoint returns 200 but does
   not actually respawn the container — it just flips
   `config.external_agent_status` back to "resumed" while the
   underlying runner stays gone. Was a real gotcha during the
   rollout.

### 13d. Pinned end-to-end chat round-trip

Pinned in playwright once per release to confirm the entire chain
holds:

1. Hire a fresh AI worker (any position).
2. Click **Open Human Desktop** on that worker's detail page.
3. New tab opens to `…/projects/<project_id>/desktop/<session_id>`.
4. Within ~30s: desktop viewer renders the GNOME shell + Zed
   pane; API logs show `External agent added message …
   role=assistant message_id=5/6/…` as Claude works through
   the hire-time activation (pull helix-specs, read role.md
   / identity.md / agent.md, commit a checkpoint).
5. Open a terminal in the desktop, run `gh auth status`, and
   post an issue comment to a repo the OAuth token has admin
   on. Comment appears on the issue.

If any of steps 4-5 fails: check §13a (Zed launching), §13b (`gh`
auth), §13c (stale-session contamination) before chasing deeper
bugs. The chain is fragile in exactly those three places.

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
- §11b — activation streams anchor to their SUBJECT worker's
  position, never universally to `p-root`. Stream column lives to
  the right of the org tree; no geometric overlap with reporting
  edges.
- §11c — clicking a stream node (chart) OR a stream id (list)
  lands on `…/helix-org/streams/<id>` with the EventCard list
  rendered from `recent_events`.
- §11d — publishing while the detail page is open surfaces the new
  event in the list within ~1.5s without a reload, replacing not
  appending.
- §11e — creating a github stream + clicking Create installs the
  webhook on GitHub automatically (no manual URL copy / secret
  paste); the detail page shows the "Edit on GitHub →" link;
  loopback `SERVER_URL` is refused at install time with a clear
  message. Inbound deliveries to the per-stream URL with a valid
  HMAC return 204; bad HMAC returns 401.
- §11f — firing a Worker cascades-deletes its
  `s-activations-<workerID>` Stream (pseudo-node gone from chart,
  `org_streams` row gone from DB, events retained); chart stream
  trash icon → confirm dialog → DELETE /streams/{id} works
  identically to the Streams list page.
- §11g — subscriptions are POSITION-anchored:
  - firing a worker leaves the position's subscription rows alone
    (`org_subscriptions` survives `lifecycle.Fire`);
  - hiring into a position inherits its subscriptions — dispatch
    activates the new hire on the next event without any explicit
    subscribe call;
  - DeletePosition is the only cascade that drops them
    (`org_subscriptions` rows for the deleted position go to 0);
  - dragging a position's right-side `stream` handle onto a stream
    pseudo-node creates a row at
    `(org, position, stream)` (idempotent server-side);
  - the Worker detail Subscriptions panel resolves worker → position
    and edits the position's subscription set via the
    `/positions/{id}/subscriptions` endpoints.
- §12 — chat button lands on `…/projects/<pid>/desktop/<sid>`,
  never `…/agent/<id>`.
- §13a — fresh sandbox container's `/home/retro/.local` is
  owned by retro:retro and Zed launches cleanly; WS reaches
  `agent_ready`.
- §13b — `gh auth status` inside a fresh sandbox reports
  "Logged in to github.com" via `GH_TOKEN`; a real
  `gh issue comment` against a repo the OAuth token has admin
  on succeeds.
- §13c — after `./stack build-ubuntu` ships a new image,
  stale-session recovery procedure (kill containers, wipe
  sessions + runtime-state pointers) unblocks Open Human Desktop
  for all pre-existing workers.
- §13d — full chat round-trip (hire → desktop → Claude
  activates + posts an `External agent added message` → `gh`
  works from inside the sandbox) green at least once per
  release.
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
