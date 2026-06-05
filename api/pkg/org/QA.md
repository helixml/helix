# Helix Org — QA test plan

End-to-end UI test for helix-org. Run before merging any change to
`frontend/src/pages/HelixOrg*.tsx`, `frontend/src/components/orgs/`,
`api/pkg/org/`, or `api/pkg/server/helix_org*.go`.

Each section is a regression pin — the bug it guards is in the
heading. Skip nothing without reading the why. Every feature is
tested in exactly one place; sections reference each other instead
of repeating steps.

## Mental model

- **Role** — the job description. Carries the markdown a Worker
  reads at activation plus the tool list that becomes the Worker's
  live MCP surface. There is no separate per-Worker grants table —
  capability is the Role's responsibility.
- **Worker** — a human or AI agent. Holds a single `role_id` (the
  capability binding) and an optional `parent_id` (the Worker it
  reports to). The owner Worker `w-owner` has no parent.
- **Subscription** — a `(org, worker, stream)` row. Worker-anchored:
  firing a Worker drops the row, and a new hire into the same Role
  does NOT automatically inherit. The hiring playbook re-subscribes
  new hires explicitly (see `bootstrap/templates/owner_role.md`).
  This lets two Workers in the same Role consume different streams
  (specialisation) or only the on-call subset of a Role wake up on
  an event (load patterns).

The chart UI is gone. The Overview tab groups Workers by Role
(role cards, click a Role to edit, click a Worker to open detail).

## Setup

Acting user has the `helix-org` alpha flag and is a member of the
test org. Sign in at `/login`, click **Org** in the primary
sidebar. Tests run against `…/orgs/<org>/helix-org/*`.

## §1. Bootstrap + sidebar

1. Land on `…/helix-org/overview`. Middle sidebar shows highlighted
   **Overview** plus **Roles / Workers / Streams / Settings**.
2. Overview shows one Role card: `r-owner` with one Worker badge
   `w-owner`. No other roles, no other workers.
3. Network tab: `/overview /workers /roles /streams` requests all
   2xx in parallel (bootstrap-race regression — see §1 in prior
   plan: first request used to win, the rest 500-ed with
   `create owner role: already exists`).
4. Confirm DB:
   ```sql
   SELECT id, role_id, parent_id FROM org_workers;
   ```
   One row: `(w-owner, r-owner, NULL)`. No `org_positions` table
   exists (`SELECT to_regclass('org_positions')` → NULL).

## §2. Roles list + tool editor (regression: 25-tool bootstrap)

`Role.Tools` is the live MCP surface for every Worker holding the
Role. Editing a Role's Tools changes capability for every Worker
in that Role on their next MCP request.

1. **Roles** in the middle sidebar. Columns: ID / Content / Tools /
   Streams / Updated.
2. `r-owner`'s **Tools count is 21** — the bootstrap seed. Drop
   from 25 reflects removed position tools (create_position,
   list_positions, get_position, list_position_children) — pin so
   re-adding them is a deliberate, visible change.
3. `r-owner` vertical-dot menu offers **Open** and a **Delete**
   disabled with `Owner — protected`.
4. **+ New Role** → `r-test-dm`, content `# DM`. Detail page opens,
   Tools field empty.
5. Click the Tools dropdown. ~21 options render. Tick `dm` —
   popper stays open (`disableCloseOnSelect`). Press Escape.
6. **Save** → snackbar `role r-test-dm saved` → button disables.
7. Hard refresh — `dm` chip persists.
8. **Live propagation.** Hire an AI Worker into `r-test-dm`
   (§3.2). Add `publish` via the dropdown + Save. Hit the
   Worker's MCP endpoint
   (`/api/v1/mcp/helix-org/<org>/workers/<id>/mcp` → `tools/list`):
   `publish` is now in the list without any `hire_worker`,
   `grant_tool`, or session restart. Remove + Save: next
   `tools/list` no longer includes it.

## §3. Hire workers + cascade semantics

Pins the AI-hire path, the owner protections, and the cascade
dialogs.

1. **+ New Worker** (the Workers tab grew a primary action button
   now that the chart canvas is gone). Form: `id`, `kind`,
   `role_id` (dropdown), `parent_id` (optional, defaults to
   `w-owner`), `identity_content`.
2. Submit kind `ai`, id `w-ai-1`, role `r-test-dm`,
   parent `w-owner`. Row appears in the Workers table — Role
   column shows `r-test-dm`, Reports to shows `w-owner`.
3. Click the `w-ai-1` row → URL becomes
   `…/helix-org/workers/w-ai-1`. The detail page must NOT crash
   the API on first load (regression: nil-deref in
   `WorkerProject.Ensure` when the worker had no role/position
   resolution).
4. Try **Fire worker** on `w-owner`. Friendly snackbar surfaces
   the 409 `cannot fire the owner worker`.
5. Hire `w-carol` into `r-test-dm`, fire from her detail page →
   confirm dialog. Worker gone from list.
6. Delete `r-test-dm` from the Roles tab — dialog enumerates
   "fires every Worker holding this Role". Confirm; both the
   role and `w-ai-1` go.
7. `r-owner` Delete is hidden / API refuses with 409.

## §4. Cross-org isolation, persistence, theme

1. Switch to a second org via the top-left selector. Overview
   shows the fresh `r-owner / w-owner` baseline — no leakage from
   the first org. Hire something in the second org and switch
   back: first org unchanged.
2. Restart the API container. Everything persists (regression:
   `ResetSchema=true` on production wiring used to drop every
   `org_*` table on boot).
3. Toggle the top-right sun/moon. Both modes render the
   overview cards cleanly.

## §5. Workers list

`…/helix-org/workers` table — columns ID / Kind / Role / Reports
to / Identity / Tools. Vertical-dot menu offers **Open** and
**Fire**; `w-owner`'s Fire shows `Owner — protected`. Filter by
Role using the column header search (regression: roles can repeat
across workers, so the list must be filterable, not grouped).

## §6. Streams list, detail, live tail

Every **AI** Worker has an auto-created `s-activations-<workerID>`
stream (humans don't need spawner activation, so `w-owner` is the
only human with one — seeded at bootstrap so chat lands
somewhere). The Streams surface lives at `…/helix-org/streams`.

1. **Streams list** — columns ID / Name / Transport / Subscribers
   / Created. Every AI worker has a matching
   `s-activations-<workerID>` row, plus `s-activations-w-owner`.
2. **Subscribers column** shows worker ids (not position ids).
   For a freshly-hired `w-ai-1`, `s-activations-w-ai-1`'s
   subscriber list is `[w-owner]` (the hiring caller is auto-
   subscribed) and explicitly NOT `[w-ai-1]` (a worker
   subscribed to its own activation stream would loop dispatch).
3. **Detail page**: click any stream id. URL becomes
   `…/helix-org/streams/<id>`. Header shows id (monospace) +
   transport kind chip + description + `created by … · ts` +
   subscribers chip-list. Messages section lists EventCard rows
   newest-first.
4. **Live SSE tail**: publish a new event. The new event appears
   at the top within ~1.5s without reload.

## §7. GitHub streams — one-click setup

(Unchanged from prior revision; full procedure in the previous
QA.md history. Position-removal did not touch the github
transport.) Pre-conditions: GitHub OAuth connected with `repo,
admin:repo_hook, read:org`. `SERVER_URL` is a public host
(loopback refused).

Create → pick repo → submit → webhook installed end-to-end.
Detail page exposes **Edit on GitHub →** and **Re-install**.

## §8. Worker-anchored subscriptions (regression: position survival)

Subscriptions are keyed on `(org, worker, stream)`. Firing a
Worker drops their subscription rows; a new hire into the same
Role does NOT inherit.

1. **Worker detail Subscriptions panel** (`…/workers/<id>`, below
   the Role's Tools): N-count reflects the worker's subscription
   set. Multi-select dropdown shows every stream with description
   + checkbox state, with `disableCloseOnSelect` (same shape as
   the role tool editor in §2). Toggling updates this worker's
   set. Caption: "Subscriptions are per-Worker — they die when
   this Worker is fired. A new hire into the same Role won't
   inherit them."
2. **Dies on fire**: hire AI `w-cycle` into a fresh Role,
   subscribe `w-cycle` → a test stream, fire `w-cycle`. Inspect
   `org_subscriptions` — no row references `w-cycle`. Publish a
   message to that stream and verify no activation fires (no
   recipient).
3. **No automatic inheritance on rehire**: hire `w-cycle-2`
   into the same Role. Publish to the test stream. `w-cycle-2`
   does NOT activate (regression in the other direction: prior
   position-anchored model used to inherit; that flexibility was
   the bug we removed). The hiring playbook re-subscribes
   explicitly to opt in.
4. **Specialisation check**: hire two AI Workers `w-secrev`
   and `w-perfrev` into one shared role `r-code-reviewer`.
   Subscribe `w-secrev` → `s-security-prs` and `w-perfrev` →
   `s-perf-prs`. Publish to `s-security-prs`: only `w-secrev`
   activates. Position-anchored subs could not express this.

## §9. Stream delete (regression: orphan activation streams)

Firing a worker used to leave its `s-activations-<workerID>`
stream behind.

1. Hire a fresh AI `w-cleanup`. Its activation stream row + an
   entry in `s-activations-w-cleanup`'s subscriber list appear.
2. Fire `w-cleanup`. `s-activations-w-cleanup` row disappears
   from the Streams list within ~1s (`lifecycle.Fire` cascade).
   Events on that stream survive in `org_events` as an audit
   trail.

## §10. Chat → Human Desktop (regression: bare /agent route)

Chat MUST happen inside the per-Worker project's Human Desktop
session — same surface a normal project uses — not the legacy
bare composer at `/agent/<id>`.

(Procedure unchanged from prior revision; position-removal did
not touch the desktop pipeline.) Click **Open Human Desktop** on
the worker detail page → lands on
`…/projects/<project_id>/desktop/<session_id>` in a new tab.

## §11. Worker sandbox: Zed launch, per-Worker tools, stale-session recovery

(Unchanged. Position-removal did not touch the sandbox /
spawner pipeline. Procedures in the prior QA.md history.)

## Pass criteria

- §1 — bootstrap creates one Worker (`w-owner` with `role_id =
  r-owner`, `parent_id = NULL`); no `org_positions` table.
- §2 — `r-owner.tools.length == 21`; multi-select adds/removes a
  tool; refresh persists; an edit propagates to every Worker in
  the role on the next MCP `tools/list`.
- §3 — AI worker creation doesn't crash the API; owner refuses
  fire (409); role delete dialog enumerates the affected workers
  before confirm.
- §4 — cross-org isolation holds; restart persists; both themes
  render.
- §6 — activation streams' subscribers list contains the hiring
  caller's worker id (not their position id); live SSE replaces,
  doesn't append.
- §8 — subscriptions are worker-keyed; fire drops them; new
  hires do NOT inherit; two workers in the same role can hold
  disjoint subscription sets.
- §9 — fire removes the worker's activation stream (no orphans).
- §10 — chat button lands on `…/projects/<pid>/desktop/<sid>`,
  never on `…/agent/<id>`.
- §11 — fresh sandbox: Zed launches; per-Worker `gh`
  startupScript installs cleanly; `gh auth status` green.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- A Worker holds at most one Role.
- A Worker's `parent_id` points at at most one other Worker.
  Hierarchy is a tree (no co-managers in the current model).
- `w-owner` / `r-owner` are protected at the API; UI hides the
  trash affordance and surfaces a friendly 409.
- The Overview tab does not currently visualise the reporting
  tree (parent_id chains). It groups by Role. Future iteration
  can layer a tree view on the same data.
