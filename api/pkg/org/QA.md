# Helix Org — QA test plan

End-to-end UI test for helix-org. Run before merging any change to
`frontend/src/pages/HelixOrg*.tsx`, `frontend/src/components/orgs/`,
`api/pkg/org/`, or `api/pkg/server/helix_org*.go`.

Every feature is tested in exactly one place; sections reference
each other instead of repeating steps. Skip nothing without reading
the why.

## Mental model

- **Role** — the job description. Carries the markdown a Worker
  reads at activation plus the tool list that becomes the Worker's
  live MCP surface. There is no separate per-Worker grants table —
  capability is the Role's responsibility.
- **Worker** — a human or AI agent. Holds a single `role_id` (the
  capability binding). Who reports to whom is a separate many-to-many
  relation (see Reporting line), not a field on the Worker.
- **Reporting line** — an `org_reporting_lines` `(org, manager,
  report)` row meaning *report* reports to *manager*. A Worker may
  report to several managers; the owner Worker `w-owner` has none.
  Worker deletion drops every line that references it via
  `ON DELETE CASCADE` foreign keys. The graph is a cycle-guarded DAG.
- **Subscription** — a `(org, worker, stream)` row. Worker-anchored:
  firing a Worker drops the row, and a new hire into the same Role
  does NOT automatically inherit. The hiring playbook re-subscribes
  new hires explicitly (see `bootstrap/templates/owner_role.md`).
  This lets two Workers in the same Role consume different streams
  (specialisation) or only the on-call subset of a Role wake up on
  an event (load patterns).

The Chart tab is a ReactFlow canvas. Roles are group frames that
contain their Workers (a Role can hold many Workers). Worker → Worker
edges are reporting lines (`org_reporting_lines` rows); drag from a
manager's bottom handle to a subordinate to add one, delete an edge to
remove that line (a Worker can have several). Streams hang off the
right; drag from a Worker's right handle to a Stream to subscribe.
Click a Role header to edit it, a Worker to open its detail page.

## Setup

Acting user has the `helix-org` alpha flag and is a member of the
test org. Sign in at `/login`, click **Org** in the primary
sidebar. Tests run against `…/orgs/<org>/helix-org/*`.

## §1. Bootstrap + sidebar

1. Land on `…/helix-org/chart`. Middle sidebar shows highlighted
   **Chart** plus **Roles / Workers / Streams / Settings**.
2. Chart shows one Role frame: `r-owner` containing one Worker node
   `w-owner`. No other roles, no other workers.
3. Network tab: `/workers /roles /streams` requests all
   2xx in parallel.
4. Confirm DB:
   ```sql
   SELECT id, role_id FROM org_workers WHERE id = 'w-owner';
   ```
   One row: `(w-owner, r-owner)`. There is no `parent_id` column —
   reporting lives in `org_reporting_lines`; confirm it's empty:
   `SELECT * FROM org_reporting_lines WHERE org_id = '<org>'` returns
   zero rows. No `org_positions` table exists
   (`SELECT to_regclass('org_positions')` → NULL).

## §2. Roles list + tool editor

`Role.Tools` is the live MCP surface for every Worker holding the
Role. Editing a Role's Tools changes capability for every Worker
in that Role on their next MCP request.

1. **Roles** in the middle sidebar. Columns: ID / Content / Tools /
   Streams / Updated.
2. `r-owner` has its bootstrap tool set populated (non-empty). The
   removed position tools (`create_position`, `list_positions`,
   `get_position`, `list_position_children`) are NOT present — pin
   so re-adding them is a deliberate, visible change.
3. `r-owner` vertical-dot menu offers **Open** and a **Delete**
   disabled with `Owner — protected`.
4. **+ New Role** → `r-test-dm`, content `# DM`. Detail page opens,
   Tools field empty.
5. Click the Tools dropdown. The available tools render. Tick `dm` —
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

1. **+ New Worker** (the Workers tab primary action button; the Chart
   also hires via the per-Role hire icon). Form: `id`, `kind`,
   `role_id` (dropdown), `parent_id` (optional — the new hire's initial
   manager; creates one reporting line), `identity_content`.
2. Submit kind `ai`, id `w-ai-1`, role `r-test-dm`,
   parent `w-owner`. Row appears in the Workers table — Role
   column shows `r-test-dm`, Reports to shows `w-owner`.
3. Click the `w-ai-1` row → URL becomes
   `…/helix-org/workers/w-ai-1`. The detail page must NOT crash
   the API on first load.
4. Try **Fire worker** on `w-owner`. Friendly snackbar surfaces
   the 409 `cannot fire the owner worker`.
5. Hire `w-carol` into `r-test-dm`, fire from her detail page →
   confirm dialog. Worker gone from list.
6. Delete `r-test-dm` from the Roles tab — dialog enumerates
   "fires every Worker holding this Role". Confirm; both the
   role and `w-ai-1` go.
7. `r-owner` Delete is hidden / API refuses with 409.

## §4. Cross-org isolation, persistence, theme

1. Switch to a second org via the top-left selector. The Chart
   shows the fresh `r-owner / w-owner` baseline — no leakage from
   the first org. Hire something in the second org and switch
   back: first org unchanged.
2. Restart the API container. Everything persists — no `org_*`
   data is dropped on boot.
3. Toggle the top-right sun/moon. Both modes render the
   Chart canvas (role frames, worker nodes, stream nodes) cleanly.

## §5. Workers list

`…/helix-org/workers` table — columns ID / Kind / Role / Reports
to / Identity / Tools. Vertical-dot menu offers **Open** and
**Fire**; `w-owner`'s Fire shows `Owner — protected`. Filter by
Role using the column header search (roles can repeat across
workers, so the list must be filterable, not grouped).

## §6. Streams list, detail, live tail

Every **AI** Worker has an auto-created `s-activations-<workerID>`
stream (humans don't need spawner activation, so `w-owner` is the
only human with one — seeded at bootstrap so chat lands
somewhere). Both kinds of hierarchy stream are derived from the
reporting graph by the topology reconciler (`application/topology`):
the activation stream's subscribers are the Worker's **managers**, and
any Worker with ≥1 direct report also gets an `s-team-<managerID>`
broadcast stream (members = manager + direct reports). The Streams
surface lives at `…/helix-org/streams`.

1. **Streams list** — columns ID / Name / Transport / Subscribers
   / Created. Every AI worker has a matching
   `s-activations-<workerID>` row, plus `s-activations-w-owner`. Any
   Worker that has at least one direct report also shows an
   `s-team-<managerID>` row.
2. **Subscribers column** shows worker ids (not position ids).
   For a freshly-hired `w-ai-1` (parent `w-owner`),
   `s-activations-w-ai-1`'s subscriber list is `[w-owner]` — its
   **manager** is subscribed, because activation-stream observers are
   derived from the reporting line, not from whoever clicked hire — and
   explicitly NOT `[w-ai-1]` (a worker subscribed to its own activation
   stream would loop dispatch). `s-team-w-owner` exists with subscribers
   `[w-owner, w-ai-1]`.
3. **Detail page**: click any stream id. URL becomes
   `…/helix-org/streams/<id>`. Header shows id (monospace) +
   transport kind chip + description + `created by … · ts` +
   subscribers chip-list. Messages section lists EventCard rows
   newest-first.
4. **Live SSE tail**: publish a new event. The new event appears
   at the top within ~1.5s without reload.

## §7. GitHub streams — one-click setup

Pre-conditions: GitHub OAuth connected with `repo,
admin:repo_hook, read:org`. `SERVER_URL` is a public host
(loopback refused).

Create → pick repo → submit → webhook installed end-to-end.
Detail page exposes **Edit on GitHub →** and **Re-install**.

## §8. Worker-anchored subscriptions

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
   does NOT activate. The hiring playbook re-subscribes
   explicitly to opt in.
4. **Specialisation check**: hire two AI Workers `w-secrev`
   and `w-perfrev` into one shared role `r-code-reviewer`.
   Subscribe `w-secrev` → `s-security-prs` and `w-perfrev` →
   `s-perf-prs`. Publish to `s-security-prs`: only `w-secrev`
   activates.

## §9. Stream delete

Firing a worker removes both kinds of hierarchy stream it owns —
its `s-activations-<workerID>` stream and, if it was a manager, its
`s-team-<workerID>` team stream. Topology owns the teardown (Fire
reconciles after the row is gone; there is no inline stream delete).

1. Hire a fresh AI `w-cleanup`. Its activation stream row + an
   entry in `s-activations-w-cleanup`'s subscriber list appear.
2. Fire `w-cleanup`. `s-activations-w-cleanup` row disappears
   from the Streams list within ~1s (`lifecycle.Fire` →
   `topology.Reconcile`). Events on that stream survive in
   `org_events` as an audit trail.
3. **Team stream teardown.** Hire AI `w-cleanup-mgr`, then hire AI
   `w-cleanup-rep` with parent `w-cleanup-mgr`. Confirm
   `s-team-w-cleanup-mgr` now exists (subscribers
   `[w-cleanup-mgr, w-cleanup-rep]`). Fire `w-cleanup-mgr` (the
   confirm dialog notes its report loses its manager). Both
   `s-activations-w-cleanup-mgr` **and** `s-team-w-cleanup-mgr`
   disappear from the Streams list:
   `SELECT id FROM org_streams WHERE id IN
   ('s-activations-w-cleanup-mgr','s-team-w-cleanup-mgr')` returns
   zero rows. `w-cleanup-rep` survives, keeping its own
   `s-activations-w-cleanup-rep`.

## §10. Chat: inline transcript + Human Desktop

The worker detail page renders the worker's conversation inline using
the same transcript view the spec-task page uses (`EmbeddedSessionView`
+ `RobustPromptInput`), reading the per-Worker project's long-lived
"Human Desktop" exploratory session. The operator should NOT have to
click out to the external desktop tab just to see what the worker is
doing. The desktop launch stays available for the full Zed GUI / video.

1. **Inline transcript auto-loads.** Open a worker that has already
   been chatted with (`…/helix-org/workers/<id>` with a `project_id`).
   The Chat panel shows the conversation inline — user turns, the
   agent's responses, and its MCP tool calls (collapsible) — without
   any click. The resolve is GET-only: opening the page must NOT spin
   up a desktop container (Network tab: one
   `GET …/projects/<pid>/exploratory-session`, no create/resume POST).
2. **No-session empty state.** A freshly-hired worker that has never
   been chatted with (no `project_id`, or project with no exploratory
   session — the GET returns 204) shows "No conversation yet — launch
   the Human Desktop to start one", not a crash or a spinner.
3. **Send a message and verify the worker responds.** This is the
   load-bearing test — the worker chat is useless if messages don't
   actually reach the agent. With the transcript shown, type a prompt
   the agent must visibly act on (e.g. "Reply with the single word
   PONG and nothing else") and send.
   - **The message must dispatch, not park.** The composer must NOT get
     stuck showing a queue header reading **"Message queue (saved
     locally)"** with the message sitting under it forever. That header
     means the prompt was written to local storage but never sent — the
     symptom of the 53b336e01 regression where the client-side queue
     pump was deleted. For a worker/Human-Desktop session (no
     `spec_task_id`) the composer pumps its own queue via
     `onSend → streaming.NewInference`; the queued item should clear
     within ~1s, not persist.
   - **Network:** sending fires `POST …/sessions/chat` for the worker's
     session id (NOT `POST …/prompt-history/sync`, which is the
     spec-task-only path and 400s without a `spec_task_id`). It must
     return 2xx, NOT **401** — the worker's "Human Desktop" session is
     owned by whoever bootstrapped the org, not necessarily the operator
     driving the worker, so the chat endpoint authorizes via org/project
     RBAC (`authorizeUserToSession`, `ActionUpdate`), not strict
     owner-equality. An operator who can see the transcript (read) but
     lacks write access on the project is correctly refused; a read-only
     org member should not be able to drive the agent.
   - **The user turn appears** inline immediately.
   - **The agent replies** — a new assistant interaction streams into
     the transcript live (the WebSocket is subscribed to the session),
     ending with the expected output (e.g. `PONG`). If the desktop was
     paused, the first send wakes it (spinner, then the reply). No
     navigation required.
   - **DB cross-check** (optional): the new interactions land against
     the worker's session —
     `SELECT state, prompt_message FROM interactions WHERE session_id =
     '<sid>' ORDER BY created DESC LIMIT 2;` shows the user prompt and a
     `complete` assistant turn.
4. **Open Human Desktop** still lands on
   `…/projects/<project_id>/desktop/<session_id>` in a new tab — the
   full GUI surface, NOT the bare composer at `/agent/<id>`. After
   launch, the inline transcript on the worker page reflects the same
   session.

## §11. Worker sandbox: Zed launch, per-Worker tools, stale-session recovery

Open a fresh AI Worker's sandbox: Zed launches, the per-Worker `gh`
startupScript installs cleanly, and `gh auth status` is green. The
Worker's Role tools are present in the sandbox MCP surface.

## §12. Chart canvas: reporting + subscription drag

The Chart is a ReactFlow canvas keyed off the `org_reporting_lines`
join table (many-to-many: a Worker may report to several managers)
and worker-anchored subscriptions — there are no Position rows. This
pins the drag interactions and the `POST /workers/{id}/parents` /
`DELETE /workers/{id}/parents/{parent_id}` endpoints — both of which
now reconcile the activation/team streams the edge implies (see 3a).

1. On `…/helix-org/chart`, hire three AI workers into a new role
   `r-eng` via the role frame's hire icon: `w-alice`, `w-bob`,
   `w-carol`. All appear as Worker nodes inside the `r-eng` frame with
   no reporting edges (top-level orphans).
2. Drag from `w-owner`'s **bottom** handle to `w-alice`'s **top**
   handle. A solid reporting edge appears; snackbar `w-alice now
   reports to w-owner`. DB: `SELECT manager_id FROM org_reporting_lines
   WHERE report_id='w-alice' AND org_id='<org>'` → `{w-owner}`. The
   `r-owner` frame now sits above `r-eng` (dagre lays the role tree out
   from the cross-role edge).
   - **Topology side-effects** (the new manager edge wires the comms
     channels). `s-activations-w-alice` now has `w-owner` as a
     subscriber (the manager observes the report's transcript):
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-activations-w-alice'` → `{w-owner}`. And the manager's
     team stream now exists with both of them:
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-team-w-owner'` → `{w-owner, w-alice}`.
3. Drag from `w-alice`'s bottom handle to `w-bob`'s top handle →
   `w-bob` reports to `w-alice` (intra-role edge; both stay in
   `r-eng`).
3a. **Multi-manager + reparent-desync fix (highest-priority
   regression).** Drag from `w-carol`'s bottom handle to `w-alice`'s
   top handle. A second reporting edge appears; snackbar `w-alice now
   reports to w-carol`. `GET /workers/w-alice → .parent_ids` returns
   `[w-owner, w-carol]` (order may vary). DB:
   `SELECT manager_id FROM org_reporting_lines WHERE
   report_id='w-alice'` → two rows.
   - **Both managers now observe the transcript.**
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-activations-w-alice'` → `{w-owner, w-carol}`. And
     `w-carol`'s team stream now exists:
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-team-w-carol'` → `{w-carol, w-alice}`.

   Then select **only** the `w-carol → w-alice` edge and press
   **Delete**: snackbar `w-alice no longer reports to w-carol`; the
   `w-owner → w-alice` edge survives; `parent_ids` is back to
   `[w-owner]`.
   - **The ex-manager is unsubscribed — this is the bug this PR fixes.**
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-activations-w-alice'` → `{w-owner}` only (NOT
     `{w-owner, w-carol}` — the old bug left `w-carol` subscribed after
     the edge was removed). `s-team-w-carol` is gone (w-carol has no
     other reports):
     `SELECT id FROM org_streams WHERE id='s-team-w-carol'` → zero rows.
     `w-owner`'s observership and `s-team-w-owner` are untouched.
4. **Cycle guard**: drag from `w-bob`'s bottom handle to `w-alice`'s
   top handle (would make alice→bob→alice). API returns 409; snackbar
   surfaces the cycle error; no edge added. DB unchanged.
5. Select the `w-owner → w-alice` edge, press **Delete** (and retest
   with **Backspace**, re-adding the edge between the two). Edge gone;
   snackbar `w-alice no longer reports to w-owner`; the
   `org_reporting_lines` row for `(w-owner, w-alice)` is gone (no row
   where `report_id='w-alice'`).
6. Create a stream `s-test` (Streams tab). It appears as a dashed
   node to the right of the tree. Drag from `w-alice`'s **right**
   (amber) handle to `s-test` → dashed subscription edge; snackbar
   `w-alice now consumes s-test`. `GET /streams` shows `w-alice` in
   subscribers.
7. Delete the subscription edge → `w-alice` drops from the stream's
   subscriber list (worker-anchored unsubscribe).
8. Fire `w-alice` from her node's trash icon → confirm dialog lists
   that her one direct report (`w-bob`) loses its manager. Confirm;
   node gone, `w-bob`'s edge to her removed.

## §13. Reporting-line comms: `managers` / `reports` tools

The two lazy read tools resolve a Worker's reporting lines live, so the
fixed worker policy can refer to "your managers" / "your reports"
abstractly (escalate up via `managers`+`dm`; brief down via
`reports`+`publish` to the team stream). Both are MCP tools on each
Worker's surface — call them via `tools/call` at
`/api/v1/mcp/helix-org/<org>/workers/<id>/mcp` (the same endpoint §2.8
uses for `tools/list`). The owner Role and every Role drafted via
`/role` carry `managers` + `reports`.

Setup: in a fresh role that grants `managers`, `reports`, hire AI
`w-mgr` (parent `w-owner`), AI `w-rep` (parent `w-mgr`), and AI `w-sub`
(parent `w-rep`) — so `w-rep` is both a report (of `w-mgr`) and a
manager (of `w-sub`).

1. **`managers` from a report.** `tools/call managers` on `w-rep` (no
   args) → `{"managers":[{"id":"w-mgr","role":"<roleId>",
   "dmStreamId":"s-dm-w-mgr-w-rep"}]}`. The `dmStreamId` is the
   deterministic sorted pair, so `dm`-ing `w-mgr` lands on it. Call
   `managers` on `w-owner` → `{"managers":[]}` — an **empty array, not
   null** (the owner reports to no one).
2. **`reports` from a manager.** `tools/call reports` on `w-mgr` →
   `teamStreamId` is `"s-team-w-mgr"` (non-null), and the `reports`
   array contains `w-rep` with `dmStreamId":"s-dm-w-mgr-w-rep"` and
   `manages: true` + `teamStreamId":"s-team-w-rep"` (because `w-rep`
   leads its own sub-team via `w-sub`). Publishing to the top-level
   `s-team-w-mgr` reaches `w-rep`; you delegate `w-rep`'s workstream to
   it rather than posting into `s-team-w-rep` yourself.
3. **`reports` from a leaf.** `tools/call reports` on `w-sub` →
   `teamStreamId": null` and `reports": []` (empty array — no one
   reports to `w-sub`).

## Pass criteria

- §1 — bootstrap creates one Worker (`w-owner` with `role_id =
  r-owner`); `org_reporting_lines` is empty; no `org_positions` table.
- §2 — `r-owner` has a non-empty tool set (position tools absent);
  multi-select adds/removes a tool; refresh persists; an edit
  propagates to every Worker in the role on the next MCP
  `tools/list`.
- §3 — AI worker creation doesn't crash the API; owner refuses
  fire (409); role delete dialog enumerates the affected workers
  before confirm.
- §4 — cross-org isolation holds; restart persists; both themes
  render.
- §6 — activation streams' subscribers list contains the Worker's
  manager id (derived from the reporting line, not whoever clicked
  hire); a manager with reports also has an `s-team-<id>` stream; live
  SSE replaces, doesn't append.
- §8 — subscriptions are worker-keyed; fire drops them; new
  hires do NOT inherit; two workers in the same role can hold
  disjoint subscription sets.
- §9 — fire removes the worker's activation stream (no orphans), and
  if the worker was a manager, its `s-team-<id>` stream is torn down
  too (topology owns the teardown).
- §12.3a — adding a second manager subscribes that manager to the
  report's activation stream and creates its team stream; **removing
  the edge unsubscribes the ex-manager** (the reparent-desync
  regression this PR fixes) and tears down the now-empty team stream;
  the surviving manager is untouched.
- §13 — `managers` returns each manager's id/role/`dmStreamId` (empty
  array, not null, for the owner); `reports` returns a non-null
  `s-team-<id>` teamStreamId + each report's `dmStreamId`, flags a
  report that manages its own sub-team (`manages: true` +
  `teamStreamId`), and returns `null` teamStreamId + empty `reports`
  for a leaf.
- §10 — the worker page shows the conversation inline (transcript +
  tool calls + composer) when a session exists, GET-only on load (no
  container spin-up); the empty state shows otherwise. Sending a
  message dispatches via `POST …/sessions/chat` (the composer does NOT
  get stuck on "Message queue (saved locally)") and the worker's agent
  replies live in the transcript. "Open Human Desktop" lands on
  `…/projects/<pid>/desktop/<sid>`, never on `…/agent/<id>`.
- §11 — fresh sandbox: Zed launches; per-Worker `gh`
  startupScript installs cleanly; `gh auth status` green.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- A Worker holds at most one Role.
- A Worker's reporting lines are many-to-many (one `org_reporting_lines`
  row per manager–report pair). A Worker may report to several managers
  simultaneously; the graph is a cycle-guarded DAG, not a tree.
- `w-owner` / `r-owner` are protected at the API; UI hides the
  trash/fire affordance and surfaces a friendly 409.
- Adding a reporting line is cycle-guarded server-side: dragging a
  manager edge that would close a reporting loop is rejected with a 409.
