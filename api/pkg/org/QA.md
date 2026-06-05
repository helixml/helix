# Helix Org — QA test plan

End-to-end UI test for helix-org. Run before merging any change to
`frontend/src/pages/HelixOrg*.tsx`, `frontend/src/components/orgs/`,
`api/pkg/org/`, or `api/pkg/server/helix_org*.go`.

Each section is a regression pin — the bug it guards is in the
heading. Skip nothing without reading the why. Every feature is
tested in exactly one place; sections reference each other instead
of repeating steps.

## Mental model

- **Role** — flat container, groups positions sharing a role id.
- **Position** — slot inside one role, holds at most one worker.
  Has top (target), bottom (manager→subordinate source), and right
  (drag-to-subscribe source) handles.
- **Reporting edge** — position bottom → position top.
- **Subscription edge** — position right → stream pseudo-node. The
  position consumes events from the stream. POSITION-anchored —
  hiring/firing a worker doesn't change which streams the slot
  receives.

Layout is dagre + bezier edges. Stream pseudo-nodes live in a
dedicated column to the right of the org tree so they never overlap
the role grid.

## Setup

Acting user has the `helix-org` alpha flag and is a member of the
test org. Sign in at `/login`, click **Org** in the primary
sidebar. Tests run against `…/orgs/<org>/helix-org/*`.

## §1. Bootstrap + sidebar (regression: 500 on first chart load)

1. Land on `…/helix-org/chart`. Middle sidebar shows highlighted
   **Chart** plus **Roles / Workers / Streams / Settings**.
2. Canvas shows `r-owner` containing `p-root` holding `w-owner`,
   with a dashed amber edge to `s-activations-w-owner`.
3. **Every** helix-org request on first chart load is 2xx (DevTools
   network tab). The page fires `/chart /workers /roles /streams`
   in parallel; before the bootstrap-race fix, only the first
   succeeded and the rest 500-ed with
   `create owner role: already exists`.

## §2. Build the chart

Covers role / position creation, parent edges, parallel-edge
geometry, and edge severing — the load-bearing pieces of the
chart UX.

1. **+ New Role** → ID `r-engineer`, content `# Engineer`. Frame
   appears next to `r-owner`.
2. `r-engineer` header **+** → Position `p-eng-1`. Snackbar tells
   you to draw an edge to a manager.
3. Repeat to create `p-eng-2`, `p-eng-3`, `p-eng-4`, plus orphan
   positions `p-ceo`, `p-cto` under `r-owner`.
4. Drag `p-root` bottom → `p-eng-1` top. Bezier appears; snackbar
   `p-eng-1 now reports to p-root`; dagre reflows.
5. Wire `p-ceo → p-eng-2`, `p-cto → p-eng-3`, `p-root → p-eng-4`,
   then **re-wire** `p-ceo → p-eng-3`. Last edge wins (positions
   have a single parent); previous `p-cto → p-eng-3` silently
   drops. **No trunk collapse** — every reporting line is its own
   bezier (regression: smoothstep used to merge them).
6. Click a bezier to select it; press **Delete** or **Backspace**.
   Edge disappears; snackbar confirms. Both keys must work
   (xyflow defaults to Backspace only — Linux/Windows users
   couldn't sever otherwise).
7. Hard refresh — everything persists.

## §3. Hire workers + cascade semantics

Pins the AI-hire path (regression: API nil-deref), the owner
protections (regression: deletable owner), and the cascade dialogs.

1. **Hire worker** on `p-eng-1` → kind `human`, handle `w-alice`.
   Chip appears in the slot.
2. **Hire worker** on `p-eng-2` → kind **AI**, handle `w-ai-1`.
   AI chip renders with the robot icon.
3. Click the `w-ai-1` chip → URL becomes
   `…/helix-org/workers/w-ai-1`. The chart MUST NOT crash the API
   on this click (regression: nil-deref in `WorkerProject.Ensure`).
4. Try **Fire worker** on `w-owner`. Friendly snackbar surfaces
   the 409 `cannot fire the owner worker`; chip stays.
5. Hire `w-carol` into `p-eng-2`, click its position-card trash →
   confirm. Dialog body enumerates the cascade:
   `Deleting position p-eng-2 will cascade: • fire worker w-carol`.
   Position + worker gone.
6. Hire `w-newbie` into `p-eng-3`, then trash `r-engineer`'s
   header. Dialog enumerates `3 positions … 2 workers (w-alice,
   w-newbie)`. Confirm — frame, positions, workers, all edges go.
7. `r-owner` trash icon is hidden; the API also refuses with 409
   if hit directly.

## §4. Cross-org isolation, persistence, theme

1. Switch to a second org via the top-left selector. Chart shows a
   fresh `r-owner / p-root / w-owner` baseline — no leakage from
   the first org. Hire something in the second org and switch
   back: first org unchanged.
2. Restart the API container (or wait for Air's next rebuild) and
   re-open the chart. Everything persists (regression:
   `ResetSchema=true` on production wiring used to drop every
   `org_*` table on boot).
3. Toggle the top-right sun/moon. Both modes render the chart,
   minimap, controls, edge strokes, handle dots, and card borders
   cleanly.

## §5. Roles list + tool editor (regression: 29-tool bootstrap)

1. Click **Roles** in the middle sidebar. Columns: ID / Content /
   Tools / Streams / Updated.
2. `r-owner`'s **Tools count is 29** — the bootstrap seed shares
   one slice with the owner Worker's grants; a regression would
   drift the two apart.
3. `r-owner` vertical-dot menu offers **Open** and a **Delete**
   disabled with `Owner — protected`.
4. **+ New Role** → `r-test-dm`, any content. Detail page opens,
   Tools field empty.
5. Click the Tools dropdown. 29 options render (checkbox +
   monospace tool name + one-line description). Tick `dm` — popper
   stays open (`disableCloseOnSelect`). Press Escape.
6. **Save** (was disabled) is enabled. Click → snackbar
   `role r-test-dm saved` → button disables again.
7. Hard refresh — the `dm` chip persists. Re-open the dropdown,
   untick `dm`, **Save**. Tools back to `[]`. Delete `r-test-dm`
   via the right-rail Delete (cleanup).

## §6. Workers list

`…/helix-org/workers` table — columns ID / Kind / Position /
Identity / Tools. **No +New Worker button** (hires come from chart
position cards so role+position is explicit at hire time). The
vertical-dot menu offers **Open** and **Fire**; `w-owner`'s Fire
shows `Owner — protected`.

## §7. Streams list, detail, live tail

Every **AI** Worker has an auto-created `s-activations-<workerID>`
stream (humans don't need spawner activation, so `w-owner` is the
only human with one — seeded at bootstrap so chat lands somewhere).
The Streams surface lives at `…/helix-org/streams`.

1. **Streams list** — columns ID / Name / Transport / Subscribers /
   Created. Every AI worker on the chart has a matching
   `s-activations-<workerID>` row, plus `s-activations-w-owner`.
2. **Chart anchoring** (regression: every stream dangled off
   w-owner). With AI workers hired, dashed amber edges run from
   each subscribing position to the stream pseudo-node:
   - `p-root → s-activations-w-owner` (owner watches its own).
   - `p-root → s-activations-<hire>` (hiring caller's position
     subscribes automatically).
   - **NO** edge from `p-eng-N` to its own activation stream —
     a worker's own slot is not subscribed (would loop dispatch).
3. Stream nodes sit in the right-side column, not in line with
   the role grid; reporting edges run vertical between role
   frames, subscription edges run horizontal to the stream
   column. No overlap regardless of layout.
4. **Detail page**: click any stream id in the table (or the
   pseudo-node on the chart). URL becomes
   `…/helix-org/streams/<id>`. Header shows id (monospace) +
   transport kind chip + description + `created by … · ts` +
   subscribers chip-list. Messages section lists EventCard rows
   newest-first: `<from> [→ <to>]` left, ISO timestamp right,
   subject (if any), then either canonical message body or raw
   body, finally event id.
5. **Live SSE tail**: publish a new event to the open stream
   (chart → Streams → top-right **+ New Stream** to make a
   `local` test stream, then publish via the chat composer or
   `POST /streams/<id>/publish`). The new event appears at the
   top within ~1.5s without reload; total count does NOT double
   (each SSE frame replaces — flicker is normal).

## §8. GitHub streams — one-click setup

Operator picks ONLY which repo; helix calls GitHub's REST API to
register the webhook (URL, content-type, secret, `events: ["*"]`)
on their behalf using their connected OAuth.

**Pre-conditions:** GitHub OAuth connection on Connected Services
(else `Repository` dropdown 412s). `SERVER_URL` is a public host;
the install endpoint refuses 412 on loopback URLs (GitHub itself
won't register a hook pointing at localhost).

**Create:**
1. **+ New Stream** → Transport = `github`.
2. **Repository** is a searchable Autocomplete populated from
   `GET /github/repos`. If the dropdown is empty or shows
   "Connect a GitHub account on Connected Services first" but
   you've already connected, the issue is missing scopes — click
   **Reconnect with stream permissions →** for the popup flow
   that requests `repo, admin:repo_hook, read:org` (the broader
   Connect button in Connected Services requests no scopes by
   default).
3. **Create**. Stream row inserts + install-webhook call fires in
   sequence. Success toast: `Stream created · webhook installed
   on GitHub (id <hookID>)`. If install fails (e.g. no admin),
   the stream is still created and the toast tells the operator
   to retry from the detail page.

**Detail page — Connect to GitHub panel** has three states:
   - **Installed**: "Helix has registered a webhook on
     `owner/name` (id N)". Buttons **Edit on GitHub →** (opens
     `https://github.com/<owner>/<name>/settings/hooks/<id>`) +
     **Re-install** (idempotent — adopts the existing hook
     rather than creating a duplicate).
   - **Not installed**: "Install webhook on GitHub" button.
   - **Loopback warning** (red banner): SERVER_URL is localhost
     — points at the helix-org Settings page where
     `streams.public_url` overrides SERVER_URL without a
     container restart.

**End-to-end** (real cloudflared / ngrok):
4. Set `SERVER_URL` (or `streams.public_url` on Settings) to a
   public host. Create a github stream for a repo you own.
5. Edit on GitHub → confirm Payload URL matches the per-stream
   URL, Content type `application/json`, "Send me everything"
   selected.
6. Open an issue (or comment) on the repo. Within seconds the
   stream's detail page shows the delivery (SSE).

## §9. Position-anchored subscriptions

Subscriptions are keyed on `(org, position, stream)`, NOT
`(org, worker, stream)`. Hires/fires don't touch them; only
DeletePosition cascades.

1. **Drag to subscribe**: hover any position card → small amber
   dot on the right (the dedicated stream-source handle). Drag
   onto a stream pseudo-node → snackbar `<position> now consumes
   <stream>`. Dashed amber edge renders on next refetch
   (≤1.5s). Re-drag the same pair is idempotent.
2. **Survives fire**: hire `w-cycle` (AI) into `p-cycle`,
   subscribe `p-cycle` to a stream, then fire `w-cycle`. The
   dashed edge stays — `lifecycle.Fire` is forbidden from
   touching subscriptions. Hire `w-cycle-2` into `p-cycle` and
   publish to that stream: the new hire activates without any
   explicit subscribe call (it inherits).
3. **Worker detail Subscriptions panel** (`…/workers/<id>`,
   below Tool grants): N-count reflects the worker's POSITION's
   subscription set. Multi-select dropdown shows every stream
   with description + checkbox state, with
   `disableCloseOnSelect` (same shape as the role tool editor in
   §5). Toggling updates the position's set; caption beneath
   reads "Subscriptions are position-anchored — they outlive the
   worker. Whoever fills `<position>` next inherits this set."
   Unassigned workers (no position) render the panel as a
   read-only stub.
4. **DeletePosition cascades**: deleting `p-cycle` removes its
   subscription rows.

## §10. Stream delete (regression: orphan activation streams)

Firing a worker used to leave its `s-activations-<workerID>`
stream behind, and the chart had no in-canvas affordance to
delete a stream.

1. Hire a fresh AI `w-cleanup`. Its activation stream
   pseudo-node + dashed edge appear (per §7.2).
2. Fire `w-cleanup`. Position returns to **Hire worker**; the
   `s-activations-w-cleanup` pseudo-node disappears within ~1s
   (`lifecycle.Fire` cascade). Events on that stream survive in
   `org_events` as an audit trail (not keyed on Streams).
3. Hover any stream pseudo-node → trash icon top-right. Click →
   `ConfirmDeleteDialog` enumerates the cascade (stream + N
   subscriptions, events retained). Confirm; pseudo-node
   vanishes; Streams list page reflects the deletion without a
   reload (shared cache invalidation).

## §11. Chat → Human Desktop (regression: bare /agent route)

Chat MUST happen inside the per-Worker project's Human Desktop
session — same surface a normal project uses — not the legacy
bare composer at `/agent/<id>`.

1. `…/helix-org/workers/<id>`. Button label is **Open Human
   Desktop** (project provisioned) or **Provision + open Human
   Desktop** (right-rail "Project" empty).
2. Click. Label flips to `Provisioning agent app…` while
   `ensureWorkerChat` runs.
3. **Lands on `…/projects/<project_id>/desktop/<session_id>`**
   in a new tab. Landing on `…/agent/<id>` is a regression — the
   button must never reach the legacy bare composer.
4. The desktop viewer renders with a `Send message to agent…`
   composer. Round-trip pinned in §12.
5. Refresh the worker detail page. Right-rail **Project** field
   shows the project id. Re-click the button to navigate
   straight to the desktop (Ensure fast-paths).

## §12. Worker sandbox: Zed launch, per-Worker tools, stale-session recovery

Pins the chain `desktop click → fresh container → Zed launches
→ WebSocket connects → Claude responds → per-Worker startup
script runs → tooling works`. The base desktop image stays
lean; anything a particular Worker needs (e.g. `gh`) is
operator-configurable per project via `configure_worker_project`
(§12b).

**§12a. Zed launches in a fresh container.** Hire a fresh AI
worker; once the container appears, the desktop viewer renders
the GNOME shell + Zed pane. API logs show `External agent added
message … role=assistant` as Claude processes the hire-time
activation. No `no external agent WebSocket connection` warnings
for this session. `/home/retro/.local/` is `retro`-owned (any
build-time tool that touches `${HOME}` as root will poison it
and stop Zed from creating `~/.local/share/zed/extensions` —
the desktop image must not invoke such tools during build).

**§12b. `gh` is available + GH_TOKEN auto-injected.** Tools
beyond the base image are a **per-Worker concern**, set on the
Worker's helix project via the `configure_worker_project` MCP
tool (`startupScript` field) — NOT baked into the desktop image
or wired into the image-level startup scripts. `GH_TOKEN` is
plumbed by the helix-org spawner on every activation from the
org's connected GitHub OAuth (`SpawnSecretInjector` →
`PutProjectSecret("GH_TOKEN")` → env var on next container
boot); no operator step is needed for the token itself.

Pre-conditions:
- At least one org member has connected GitHub on Connected
  Services with `repo, admin:repo_hook, read:org` scopes. The
  `Reconnect with stream permissions →` flow in §8 grants them.
  Without a connected member the resolver returns "" and
  `GH_TOKEN` stays unset (soft skip — not an error).
- The Worker's project `startupScript` installs `gh`. From the
  hiring manager prompt (or an operator's MCP call):
  ```
  configure_worker_project(
    workerId: "w-…",
    startupScript: """
      #!/bin/bash
      set -e
      type -p gh >/dev/null || {
        curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
          | sudo dd of=/etc/apt/keyrings/githubcli-archive-keyring.gpg
        sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
          | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null
        sudo apt-get update && sudo apt-get install -y gh
      }
    """
  )
  ```
  Re-running is a no-op (the `type -p gh` guard short-circuits
  once gh is present). The script runs as `retro` on every
  fresh container start.

Open a terminal inside the desktop and verify:
- `gh --version` resolves (installed by the startupScript).
- `gh auth status` reports `✓ Logged in to github.com account
  <login> (GH_TOKEN)` with scopes `repo, admin:repo_hook,
  read:org`. (If `read:org` is missing, re-auth via §8's
  `Reconnect with stream permissions →`.)
- `gh issue comment <number> --repo <owner>/<repo> --body "from
  helix-org worker"` succeeds. Comment lands on the issue under
  the operator's GitHub identity (the org's OAuth token is what
  signs the call).

Verifying the env var: open a terminal in the desktop and run
`env | grep GH_TOKEN` (NOT `su - retro -c …` — a login shell
strips the inherited env and will lie). The token is injected
on docker create, so any non-login shell inside the container
sees it.

Server-side cross-checks if `gh auth status` still says no
token:
- `SELECT id, name, project_id FROM secrets WHERE name='GH_TOKEN'
  AND project_id=<worker's project_id>;` — secret row present
  means the SpawnSecretInjector ran. Absent means the resolver
  returned "" (no member with GitHub OAuth — fix the
  pre-condition).
- `grep "Injected project secrets into desktop env" api-logs |
  grep <session_id>` — present means the env actually reached
  `agent.Env` on container create. If this line is missing for
  a session whose secret row exists, the projection path is
  broken (runtime regression).
- WARN `put project secret failed … already exists for this
  project` is **expected** on every activation after the first
  for now (PutProjectSecret is not idempotent against existing
  secrets). The first-activation token sticks; OAuth rotation
  does NOT propagate — re-hire the worker, or delete + recreate
  the secret manually, to refresh.

**§12c. Stale-session recovery after `./stack build-ubuntu`.**
Image rebuilds leave every pre-existing exploratory session row
pointing at a now-dead container. The API's "running" status
flag is stale; clicking **Open Human Desktop** silently reuses
the dead pointer and the viewer hangs on
`Failed to connect to sandbox via RevDial`.

DO NOT use `POST /sessions/<id>/resume` — it returns 200 but
doesn't actually respawn the runner. Instead (dev-host only;
production has hydra-driven lifecycle):

```bash
# 1. kill containers from the old image tag
docker compose -f docker-compose.dev.yaml exec sandbox-nvidia \
  docker ps --format "{{.Names}}\t{{.Image}}" | grep ubuntu-external
docker compose -f docker-compose.dev.yaml exec sandbox-nvidia \
  docker rm -f <ubuntu-external-…>

# 2. wipe session rows + runtime-state pointers
docker exec helix-postgres-1 psql -U postgres -d postgres -c "
  DELETE FROM sessions WHERE id IN ('ses_<stale-1>','ses_<stale-2>');
  DELETE FROM org_worker_runtime_state
    WHERE key='session_id'
      AND value IN ('ses_<stale-1>','ses_<stale-2>');"
```

Next click on **Open Human Desktop** sees no exploratory session,
calls `v1ProjectsExploratorySessionCreate`, spawns a fresh
container from the current image, and chat works.

## Pass criteria

- §1 — first chart load is 2xx end-to-end (no bootstrap-race
  500).
- §2 — parallel reporting edges render as distinct beziers;
  Delete *and* Backspace both sever; refresh persists.
- §3 — AI chip click doesn't crash the API; owner refuses fire
  (409); cascade dialogs enumerate collateral before confirm.
- §4 — cross-org isolation holds; restart persists; both themes
  render.
- §5 — `r-owner.tools.length == 29`; multi-select adds/removes a
  tool; refresh persists.
- §7 — activation streams anchor to the SUBJECT worker's
  position (never universally to p-root); stream column right
  of the org tree; live SSE replaces, doesn't append.
- §8 — github stream Create installs the webhook end-to-end with
  no manual URL copy; loopback `SERVER_URL` refused with a clear
  message; **Edit on GitHub →** opens the right hook on
  github.com.
- §9 — position subscriptions survive fire; new hires inherit;
  DeletePosition cascades.
- §10 — fire removes the worker's activation stream from chart
  and from the streams list (no orphans).
- §11 — chat button lands on `…/projects/<pid>/desktop/<sid>`,
  never on `…/agent/<id>`.
- §12 — fresh sandbox: Zed launches; with a `configure_worker_project`
  startupScript that installs `gh`, `gh auth status` green;
  `gh issue comment` round-trips. Stale-session recovery
  procedure unblocks all pre-existing workers after a desktop
  rebuild.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- Position has at most one parent — a second incoming reporting
  edge replaces the first.
- A role frame can be empty (zero positions); the canvas shows
  "No positions yet — click + to add one".
- A position holds at most one worker — hiring into a filled
  position is rejected.
- `w-owner` / `r-owner` / `p-root` are protected at the API; UI
  hides the trash affordance and surfaces a friendly 409.
