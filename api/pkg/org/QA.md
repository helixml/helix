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
  live MCP surface. There is no separate per-Worker tool record —
  the Role's tool list is the whole story.
- **Worker** — a human or AI agent. Holds a single `role_id` (the
  capability binding). Who reports to whom is a separate many-to-many
  relation (see Reporting line), not a field on the Worker.
- **Reporting line** — an `org_reporting_lines` `(org, manager,
  report)` row meaning *report* reports to *manager*. A Worker may
  report to several managers; a top-level (root) Worker has none.
  Worker deletion drops every line that references it via
  `ON DELETE CASCADE` foreign keys. The graph is a cycle-guarded DAG.
- **Subscription** — a `(org, worker, stream)` row. Worker-anchored:
  firing a Worker drops the row, and a new hire into the same Role
  does NOT automatically inherit. The hiring playbook re-subscribes
  new hires explicitly (it's in the Role's own prompt). This lets two
  Workers in the same Role consume different streams (specialisation)
  or only the on-call subset of a Role wake up on an event (load
  patterns).

A fresh org starts **empty**; the human operator builds it from the
chart — **New Role**, then **Add Worker**. The convention these tests
follow is to create a root Role `r-root` and a root Worker `w-root`
(no parent) first. These are ordinary rows with no special status —
`w-root`/`r-root` can be fired / deleted like any other.

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

## §1. Empty org + seed the root

1. Land on `…/helix-org/chart`. Middle sidebar shows highlighted
   **Chart** plus **Roles / Workers / Streams / Settings**.
2. A fresh org is **empty**: the chart shows the empty state
   *"No roles yet. Click **New role** to get started."* — no role
   frames, no worker nodes. Confirm DB:
   `SELECT count(*) FROM org_workers WHERE organization_id = '<org>'`
   → 0; `SELECT * FROM org_reporting_lines WHERE org_id = '<org>'`
   → zero rows. No `org_positions` / `org_environments` tables exist
   (`SELECT to_regclass('org_environments')` → NULL).
3. Network tab: `/workers /roles /streams` requests all 2xx in
   parallel (each returns an empty list).
4. **Seed the root.** Click **New role** → `r-root`, content
   `# Root`. It appears on the chart with a **Hire** icon and an
   enabled **Delete** (no "protected" lock — the root is an ordinary
   row). Then hire `w-root` into it (kind `human`, no parent) via
   the role's hire icon. Later sections assume this root exists.
   Confirm: `SELECT id, role_id FROM org_workers WHERE id = 'w-root'`
   → `(w-root, r-root)`.

## §2. Roles list + tool editor

`Role.Tools` is the live MCP surface for every Worker holding the
Role. Editing a Role's Tools changes capability for every Worker
in that Role on their next MCP request.

1. **Roles** in the middle sidebar. Columns: ID / Content / Tools /
   Streams / Updated.
2. `r-root` (seeded in §1) shows an **empty tools list** — the New-Role
   dialog no longer injects a baseline; operators add tools explicitly
   via the role detail page after creation.
   The removed position tools (`create_position`, `list_positions`,
   `get_position`, `list_position_children`) are NOT present — pin
   so re-adding them is a deliberate, visible change.
3. `r-root` vertical-dot menu offers **Open** and an enabled
   **Delete** — no role is protected.
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
   `publish` is now in the list without any `hire_worker` call,
   tool-assignment step, or session restart. Remove + Save: next
   `tools/list` no longer includes it.

## §3. Hire workers + cascade semantics

Pins the AI-hire path and the cascade dialogs. No worker or role is
protected.

1. **+ New Worker** (the Workers tab primary action button; the Chart
   also hires via the per-Role hire icon). Form: `id`, `kind`,
   `role_id` (dropdown), `parent_id` (optional — the new hire's initial
   manager; creates one reporting line), `identity_content`.
2. Submit kind `ai`, id `w-ai-1`, role `r-test-dm`,
   parent `w-root`. Row appears in the Workers table — Role
   column shows `r-test-dm`, Reports to shows `w-root`.
3. Click the `w-ai-1` row → URL becomes
   `…/helix-org/workers/w-ai-1`. The detail page must NOT crash
   the API on first load.
4. **Fire worker** on `w-ai-1` → confirm dialog → worker gone. No
   worker is protected: firing any worker, including the root `w-root`,
   succeeds. (Re-hire `w-ai-1` to continue.)
5. Hire `w-carol` into `r-test-dm`, fire from her detail page →
   confirm dialog. Worker gone from list.
6. Delete `r-test-dm` from the Roles tab — dialog enumerates
   "fires every Worker holding this Role". Confirm; both the
   role and `w-ai-1` go.
7. `r-root` is deletable too — its **Delete** is enabled and cascades
   like any other role (it has no special status).

## §4. Cross-org isolation, persistence, theme

1. **Cross-org isolation** is its own section now — see §16. The
   shallow smoke test that lived here (switch org, confirm a fresh
   org starts empty) is subsumed by §16's two-level, colliding-ID
   gate. Do §16, not a re-run here.
2. Restart the API container. Everything persists — no `org_*`
   data is dropped on boot.
3. Toggle the top-right sun/moon. Both modes render the
   Chart canvas (role frames, worker nodes, stream nodes) cleanly.

## §5. Workers list

`…/helix-org/workers` table — columns ID / Kind / Role / Reports
to / Identity / Tools. Vertical-dot menu offers **Open** and
**Fire** (enabled for every worker — nothing is protected).
Filter by Role using the column header search (roles can repeat
across workers, so the list must be filterable, not grouped).

## §6. Streams list, detail, live tail

Every **AI** Worker has an auto-created `s-transcript-<workerID>`
stream (its append-only, observable transcript). A manager-less **root**
Worker also gets one (so a top-level worker's chat turns have a home) —
including the human `w-root` seeded in §1. The hierarchy streams are
derived from the reporting graph by the reconciler
(`application/reconcile`): a transcript's subscribers are the Worker's
**managers** (a manager-less root has none — its transcript is
unobserved, never self-subscribed), and any Worker with ≥1 direct
report also gets an `s-team-<managerID>` broadcast stream (members =
manager + direct reports). The Streams surface lives at
`…/helix-org/streams`.

1. **Streams list** — columns ID / Name / Transport / Subscribers
   / Created. Every AI worker has a matching
   `s-transcript-<workerID>` row, plus `s-transcript-w-root` (the
   manager-less root). Any Worker that has at least one direct report
   also shows an `s-team-<managerID>` row.
2. **Subscribers column** shows worker ids (not position ids).
   For a freshly-hired `w-ai-1` (parent `w-root`),
   `s-transcript-w-ai-1`'s subscriber list is `[w-root]` — its
   **manager** is subscribed, because transcript observers are
   derived from the reporting line, not from whoever clicked hire — and
   explicitly NOT `[w-ai-1]` (a worker subscribed to its own transcript
   would loop dispatch). `s-team-w-root` exists with subscribers
   `[w-root, w-ai-1]`.
3. **Detail page**: click any stream id. URL becomes
   `…/helix-org/streams/<id>`. Header shows id (monospace) +
   transport kind chip + description + `created by … · ts` +
   subscribers chip-list. Messages section lists EventCard rows
   newest-first.
4. **Live SSE tail**: publish a new event. The new event appears
   at the top within ~1.5s without reload.

## §7. GitHub streams — one-click setup

Pre-conditions: the Helix GitHub App installed for the org (the repo
picker lists its installable repos). `SERVER_URL` is a public host
(loopback refused).

Create → pick repo → submit → webhook installed end-to-end.
Detail page exposes **Edit on GitHub →** and **Re-install**.

- **Shared repo picker.** Both the New Stream dialog and the per-stream
  Edit form use the same `GitHubRepoPicker` (autocomplete over
  installable repos + free-text fallback + refresh). The Edit form's
  Repository field is the picker, not a plain text input.
- **Events live on GitHub, not Helix.** The event-type whitelist is
  offered only at **creation** (Helix installs the webhook with those
  events). The post-creation Edit form does NOT show an events editor —
  a caption points the operator at the GitHub webhook UI. Editing the
  repo/branches and saving must PRESERVE the existing `events`,
  `webhook_id`, and `webhook_html_url` (the prior bug rebuilt the
  config from `{repo, events, branches}` only, wiping the managed
  fields and silently failing `GitHubConfig.Validate` on an empty
  events list). Confirm a save round-trips: `org_streams` config still
  has the original `events` and `webhook_id` after editing only the
  repo.
- **Settings.** The `transport.github` row is no longer shown on the
  Settings page — the GitHub App provisions the webhook secret itself,
  so there is nothing to paste. (The registry key still exists; it's
  just not operator-facing.)

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
its `s-transcript-<workerID>` stream and, if it was a manager, its
`s-team-<workerID>` team stream. Topology owns the teardown (Fire
reconciles after the row is gone; there is no inline stream delete).

1. Hire a fresh AI `w-cleanup`. Its transcript row + an
   entry in `s-transcript-w-cleanup`'s subscriber list appear.
2. Fire `w-cleanup`. `s-transcript-w-cleanup` row disappears
   from the Streams list within ~1s (`lifecycle.Fire` →
   `topology.Reconcile`). Events on that stream survive in
   `org_events` as an audit trail.
3. **Team stream teardown.** Hire AI `w-cleanup-mgr`, then hire AI
   `w-cleanup-rep` with parent `w-cleanup-mgr`. Confirm
   `s-team-w-cleanup-mgr` now exists (subscribers
   `[w-cleanup-mgr, w-cleanup-rep]`). Fire `w-cleanup-mgr` (the
   confirm dialog notes its report loses its manager). Both
   `s-transcript-w-cleanup-mgr` **and** `s-team-w-cleanup-mgr`
   disappear from the Streams list:
   `SELECT id FROM org_streams WHERE id IN
   ('s-transcript-w-cleanup-mgr','s-team-w-cleanup-mgr')` returns
   zero rows. `w-cleanup-rep` survives, keeping its own
   `s-transcript-w-cleanup-rep`.

## §10. Chat: inline transcript

The worker detail page renders the worker's conversation inline using
the same transcript view the spec-task page uses (`EmbeddedSessionView`
+ `RobustPromptInput`), reading the per-Worker project's long-lived
"Human Desktop" exploratory session. The inline transcript is the chat
surface — there are no "Open Human Desktop" / "Restart Desktop" buttons
on the page (removed); session (re)provisioning is handled by the
worker's activation flow and the Advanced → Restart agent session
action (§14). The embedded transcript defaults to auto-scroll ON
(`autoScrollOnMount`), so live output follows to the bottom without the
operator un-pausing it.

1. **Inline transcript auto-loads.** Open a worker that has already
   been chatted with (`…/helix-org/workers/<id>` with a `project_id`).
   The Chat panel shows the conversation inline — user turns, the
   agent's responses, and its MCP tool calls (collapsible) — without
   any click. The resolve is GET-only: opening the page must NOT spin
   up a desktop container (Network tab: one
   `GET …/projects/<pid>/exploratory-session`, no create/resume POST).
2. **No-session empty state.** A freshly-hired worker that has never
   been chatted with (no `project_id`, or project with no exploratory
   session — the GET returns 204) shows "No conversation yet for this
   worker", not a crash or a spinner.
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
     owned by an org member, not necessarily the operator currently
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
4. **No desktop-launch buttons.** The page must NOT render "Open Human
   Desktop", "Restart Desktop", or a topbar "Start new chat" button —
   all removed. Chat happens inline; the only restart affordance is the
   Advanced accordion (§14).
5. **The live desktop stream, not a Start-Desktop placeholder.** The inline
   transcript is the *text* half; this is the *visual* half — chatting with
   the worker drives a real GNOME/Zed desktop and the operator must be able to
   **watch** it. From the worker detail page click the **Project** id in the
   right rail (§15.2) → lands on the project's spec board
   (`…/projects/<pid>/specs`). Because the worker you just chatted with (§10.3)
   has a **running** exploratory session, the board's topbar shows **View
   Human Desktop** — NOT **Open Human Desktop** (no session) or **Resume Human
   Desktop** (session stopped). That label is itself the gate: if it reads
   Open/Resume the desktop isn't live and the chat above didn't actually wake
   it. Click **View Human Desktop** → routes to the `project-team-desktop`
   page (`TeamDesktopPage`). Its `ExternalAgentDesktopViewer` (`mode="stream"`)
   must render the **live streamed desktop** — the agent's actual screen,
   updating — and must NOT show the **"Desktop Paused"** overlay with its
   **Start Desktop** button (the paused/stopped state), nor stick on a
   "Starting Desktop…" / "Desktop may have failed to start" spinner. Type into
   this page's composer ("Send message to agent…") and the desktop visibly
   reacts (Zed/agent activity on screen) — confirming the same session backs
   both the worker-chat transcript and this desktop view.

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
2. Drag from `w-root`'s **bottom** handle to `w-alice`'s **top**
   handle. A solid reporting edge appears; snackbar `w-alice now
   reports to w-root`. DB: `SELECT manager_id FROM org_reporting_lines
   WHERE report_id='w-alice' AND org_id='<org>'` → `{w-root}`. The
   `r-root` frame now sits above `r-eng` (dagre lays the role tree out
   from the cross-role edge).
   - **Topology side-effects** (the new manager edge wires the comms
     channels — and they exist ONLY because the edge was wired; the
     orphan workers from step 1 had no team/DM streams, only their own
     `s-transcript-<id>`). `s-transcript-w-alice` now has `w-root` as
     a subscriber (the manager observes the report's transcript):
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-transcript-w-alice'` → `{w-root}`. The manager's team
     stream now exists with both of them:
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-team-w-root'` → `{w-root, w-alice}`. And the 1:1 DM
     channel for the edge now exists too — DM channels are scoped to the
     reporting graph, provisioned here, NOT created on demand by the `dm`
     tool: `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-dm-w-alice-w-root'` → `{w-alice, w-root}` (id is the
     sorted pair).
3. Drag from `w-alice`'s bottom handle to `w-bob`'s top handle →
   `w-bob` reports to `w-alice` (intra-role edge; both stay in
   `r-eng`).
3a. **Multi-manager + reparent-desync fix (highest-priority
   regression).** Drag from `w-carol`'s bottom handle to `w-alice`'s
   top handle. A second reporting edge appears; snackbar `w-alice now
   reports to w-carol`. `GET /workers/w-alice → .parent_ids` returns
   `[w-root, w-carol]` (order may vary). DB:
   `SELECT manager_id FROM org_reporting_lines WHERE
   report_id='w-alice'` → two rows.
   - **Both managers now observe the transcript.**
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-transcript-w-alice'` → `{w-root, w-carol}`. And
     `w-carol`'s team stream now exists:
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-team-w-carol'` → `{w-carol, w-alice}`.

   Then select **only** the `w-carol → w-alice` edge and press
   **Delete**: snackbar `w-alice no longer reports to w-carol`; the
   `w-root → w-alice` edge survives; `parent_ids` is back to
   `[w-root]`.
   - **The ex-manager is unsubscribed — this is the bug this PR fixes.**
     `SELECT worker_id FROM org_subscriptions WHERE
     stream_id='s-transcript-w-alice'` → `{w-root}` only (NOT
     `{w-root, w-carol}` — the old bug left `w-carol` subscribed after
     the edge was removed). `s-team-w-carol` is gone (w-carol has no
     other reports), and so is the DM channel for the dropped edge:
     `SELECT id FROM org_streams WHERE id IN
     ('s-team-w-carol','s-dm-w-alice-w-carol')` → zero rows.
     `w-root`'s observership, `s-team-w-root`, and the
     `s-dm-w-alice-w-root` channel are untouched.
4. **Cycle guard**: drag from `w-bob`'s bottom handle to `w-alice`'s
   top handle (would make alice→bob→alice). API returns 409; snackbar
   surfaces the cycle error; no edge added. DB unchanged.
5. Select the `w-root → w-alice` edge, press **Delete** (and retest
   with **Backspace**, re-adding the edge between the two). Edge gone;
   snackbar `w-alice no longer reports to w-root`; the
   `org_reporting_lines` row for `(w-root, w-alice)` is gone (no row
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
uses for `tools/list`). New Roles start with an empty tools list — operators must add
`managers` and `reports` (and any other reads) explicitly via the role
detail page after creating the role.

Setup: in a fresh role that lists `managers`, `reports`, hire AI
`w-mgr` (parent `w-root`), AI `w-rep` (parent `w-mgr`), and AI `w-sub`
(parent `w-rep`) — so `w-rep` is both a report (of `w-mgr`) and a
manager (of `w-sub`).

1. **`managers` from a report.** `tools/call managers` on `w-rep` (no
   args) → `{"managers":[{"id":"w-mgr","role":"<roleId>",
   "dmStreamId":"s-dm-w-mgr-w-rep"}]}`. The `dmStreamId` is the
   deterministic sorted pair, so `dm`-ing `w-mgr` lands on it. Call
   `managers` on `w-root` → `{"managers":[]}` — an **empty array, not
   null** (the root reports to no one).
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
4. **`dm` is reporting-scoped.** `tools/call dm` from `w-rep` to its
   manager `w-mgr` (a reporting pair) succeeds — the channel was
   provisioned when the edge was wired. `tools/call dm` from `w-mgr` to
   `w-sub` (a **skip-level** worker, no direct reporting edge) is
   **refused** with an error naming `managers`/`reports` — there is no
   implicit DM channel to an arbitrary or skip-level worker, and the
   `dm` tool does NOT mint one. (Confirm the refusal wrote nothing:
   `SELECT id FROM org_streams WHERE id='s-dm-w-mgr-w-sub'` → zero rows.)

## §14. Breadcrumbs (all helix-org pages)

Every helix-org page builds its breadcrumb trail from the shared
`useHelixOrgBreadcrumbs(section?)` hook (`components/helix-org/`), so the
trail is consistent and the leading org-name crumb is always a link to
the chart. No page uses the old `orgBreadcrumbs={true}` plain-text org
crumb, and the detail pages have NO standalone "back" button — the
breadcrumb is the back-nav.

1. **List pages.** On `…/helix-org/roles`, `…/workers`, `…/streams`
   the trail is `<org name> / <Section>` (e.g. `test / Streams`). The
   org-name crumb is a clickable link → navigates to
   `…/helix-org/chart`. The section word is the current (leaf) crumb,
   not a link.
2. **Detail pages.** On a role/worker/stream detail page the trail is
   `<org name> / <Section> / <leaf id>` (e.g. `test / Workers / w-ai-1`).
   Both `<org name>` (→ chart) and `<Section>` (→ the list page) are
   links. There is no separate "Roles" / "Streams" / "Workers" back
   button anywhere.
3. **Settings.** `…/helix-org/settings` shows `test / Settings`; the
   org crumb links to the chart.
4. The org link works from **every** page above, not just settings —
   this is the regression that motivated the shared hook (the list
   pages previously rendered the org name as plain text).

## §15. Worker detail surfaces: identity, links, Advanced

Beyond the inline chat (§10), the worker detail page exposes an
editable identity, deep links into Helix, and a guarded restart.

1. **Editable identity.** The Identity section is a Monaco markdown
   editor (not a read-only block) with a **Save** button. Edit the
   text → Save is enabled → click (or Cmd/Ctrl+S) → snackbar
   `identity saved`; `POST …/workers/<id>/identity` returns 2xx. The
   Spawner projects the new content into `identity.md` on the next
   activation.
2. **Right-rail links.** In the right panel, the **Project** id is a
   link → `…/projects/<project_id>/specs` (the project's board), and an
   **Agent** row links to `…/agent/<agent_app_id>`. Both render only
   when the worker has been provisioned (has a project / agent app).
3. **Advanced → Restart agent session.** A collapsed **Advanced**
   accordion at the bottom of the page expands to a **Restart agent
   session** button with a caption warning that in-progress work is
   lost. Clicking re-activates the worker (`POST …/workers/<id>/activate`)
   → snackbar confirming the restart was queued.

## §16. Multi-tenancy: read + write isolation across two orgs

This is the load-bearing tenancy gate. It exists because of the
cross-tenant leak fixed in
`https://github.com/helixml/helix/pull/2570` (root cause in
`design/2026-06-09-org-multitenancy-spawner-leak.md`): a new org's
operator asked to "create a role and hire a worker" and the role + worker
landed in a **different** org.

**Two distinct threats — test both.** "Multi-tenancy" here means two
things, and they need different detectors:

1. **Read leakage** (the more dangerous): org-b's operator or agent can
   *see* org-a's roles / workers / streams / events / transcripts. The
   right detector is **distinct sentinel data** — plant a uniquely-named,
   secret-tagged record in each org and assert the *other* org's reads
   never surface it, and that cross-org id-guessing returns not-found.
   Colliding ids are the WRONG detector for read leakage: a leak can hide
   behind the matching id (you can't tell whose row you got back).
2. **Write leakage** (the literal #2570 symptom): org-b's `create_role` /
   `hire_worker` lands in org-a. The right detector is **colliding ids**
   in two orgs at once — because the store is tenant-safe by composite
   `(id, org_id)` PK (confirm: `\d org_roles` shows
   `PRIMARY KEY (id, org_id)`), so every write bug we have shipped lived
   in a process-wide layer *above* the store keyed by an id unique only
   *within* an org — and **every org's root worker is conventionally
   `w-root`**, its role `r-root`, ids like `r-engineer` repeat across
   orgs. A test
   with *different* ids per org can pass while the singleton leaks; the
   same id in both orgs is what bites.

So §16 does both: a **read-isolation** pass with distinct sentinels, then
**write-isolation** passes (Levels 1–2) with colliding ids.

**Ordering matters.** The spawner bug froze the **first** org to ever
activate a worker and replayed its identity for everyone else. So always
activate/act in **org-a first**, then org-b, and assert org-b's writes
landed in org-b. If you only ever exercise one org, or org-b first, the
frozen-identity leak hides.

**Setup.** Acting user owns (or is a global admin of — `authorizeOrgMember`
grants admins a temporary owner membership for any org) two helix-org
orgs, both already seeded with a `w-root`/`r-root` root (per §1). On
localhost the pair is `test` (org-a) and `beta` (org-b); switch between
them with the top-left org selector.

### Read isolation — distinct sentinels, every read surface

This is the data-leakage half. Plant one **uniquely-named, secret-tagged**
record in each org (distinct ids — NOT colliding, so nothing can be
masked), then prove neither org can read the other's. Cover the MCP read
tools AND the REST endpoints the UI lists from — both resolve `org` from
the URL and query the composite-keyed store, but a regression in either
the gateway or a store query would leak here.

```bash
KEY=<user api key>
mcp(){ curl -s -X POST \
  "http://localhost:8080/api/v1/mcp/helix-org/$1/workers/w-root/mcp" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -d "$2" \
  | sed -n 's/^data: //p'; }

# plant DISTINCT sentinels with secret content:
mcp org-a '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_role","arguments":{"id":"r-secret-aaa","content":"# SECRET-A-DATA do-not-leak"}}}'
mcp org-b '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_role","arguments":{"id":"r-secret-bbb","content":"# SECRET-B-DATA do-not-leak"}}}'
```

1. **Cross-org `list_*` shows only the acting org.** `list_roles` on
   org-b must return `r-secret-bbb` and **never** `r-secret-aaa` /
   `SECRET-A-DATA`; `list_roles` on org-a is the mirror image. Same for
   `list_workers` and `list_streams` — org-a's unique stream ids
   (e.g. `s-dummy-*`, `s-transcript-<an org-a-only worker>`) must be
   absent from org-b's list. The only ids that may appear in both lists
   are generic per-org rows (`s-transcript-w-root`, `s-team-w-root`) —
   those are each org's *own* row that happens to share an id, not a leak;
   confirm the row's `org_id` and content are the reader's, not the
   other's.
2. **Cross-org id-guess returns not-found, not the other org's row.**
   `get_role {id:"r-secret-aaa"}` and `get_worker {id:"w-<org-a-only>"}`
   issued against **org-b**'s endpoint must come back
   `record not found` (`isError:true`) — NOT org-a's data. This is the
   `loadAuthorized…` / composite-PK guard; a regression that dropped the
   `org_id` predicate would return the row.
3. **UI/REST surface, same assertion.** `GET /api/v1/orgs/<org-b>/workers`
   (what the Workers list page renders) returns only org-b's workers —
   none of org-a's distinct worker ids. Repeat for `/roles`, `/streams`.
4. **Events / transcripts don't cross.** Publish a secret-tagged event to
   an org-a stream; `read_events` / `list_stream_events` on every org-b
   stream must never return it. (The #2570 streamhub fix namespaced wake
   topics by org so one org's publish can't even wake another's
   subscribers; this asserts the read path stays scoped too.)

### Level 1 — UI/UX (operator switches orgs and acts via the chart)

1. In **org-a**'s `…/helix-org/chart`, hire an AI Worker `w-mt` into a
   new role `r-mt` (use the role frame's hire icon, §3/§12).
2. Switch to **org-b** via the top-left org selector. The chart renders
   org-b's *own* baseline — `r-mt` / `w-mt` from org-a are **absent**.
   This is the read-isolation half: a colliding id in another org must
   not bleed into this canvas.
3. In org-b, **New role** → enter an id that already exists in org-a
   (e.g. `r-engineer`, which org-a already holds) → Create. It succeeds
   (no "already exists" — the PK is composite). Hire `w-mt` into `r-mt`
   in org-b too.
4. **DB — both rows exist, each scoped to its own org, content distinct:**
   ```sql
   SELECT org_id, id, left(content,30) FROM org_roles
     WHERE id IN ('r-engineer','r-mt') ORDER BY id, org_id;
   SELECT org_id, id, role_id FROM org_workers
     WHERE id='w-mt' ORDER BY org_id;
   ```
   Each id returns one row per org; org-a's content is org-a's, org-b's
   is org-b's. The UI create landed in the org the operator had switched
   to — never the other.
5. Switch back to org-a: its `r-mt` / `w-mt` / `r-engineer` are
   unchanged (org-b's writes did not mutate them).

### Level 2 — Helix tool level (the root worker's MCP surface)

This is the actually-exploited surface: the org-graph MCP tools a
Worker's agent calls (here the root `w-root`). Endpoint:
`POST /api/v1/mcp/helix-org/<org>/workers/w-root/mcp` — auth is a Bearer
api_key for an **alpha-flagged** member (`hasAlphaFeature`,
`authorizeOrgMember`); the org is resolved from the **URL path**. It
speaks JSON-RPC; no separate `initialize` is needed, but the request
must send `Accept: application/json, text/event-stream` and the reply
comes back as an SSE `data:` line.

1. **Tools are org-scoped by the URL, even under identical ids.** Drive
   the *same* ids into both orgs and confirm they split:
   ```bash
   KEY=<user api key>
   mcp(){ curl -s -X POST \
     "http://localhost:8080/api/v1/mcp/helix-org/$1/workers/w-root/mcp" \
     -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
     -H "Accept: application/json, text/event-stream" -d "$2" \
     | sed -n 's/^data: //p'; }
   # create_role r-mt in BOTH orgs, then hire_worker w-mt in BOTH
   mcp org-a '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_role","arguments":{"id":"r-mt","content":"# MT (a)"}}}'
   mcp org-b '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_role","arguments":{"id":"r-mt","content":"# MT (b)"}}}'
   mcp org-a '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"hire_worker","arguments":{"id":"w-mt","roleId":"r-mt","kind":"ai","identityContent":"# a","parentId":"w-root"}}}'
   mcp org-b '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"hire_worker","arguments":{"id":"w-mt","roleId":"r-mt","kind":"ai","identityContent":"# b","parentId":"w-root"}}}'
   ```
   Each call returns `{"id":"r-mt"}` / `{"id":"w-mt", "activation_id":…}`.
   The DB queries from Level 1.4 must show one row per org with the right
   per-org content. Critically: a `create_role` against org-b's URL must
   write **zero** rows into org-a (`SELECT count(*) FROM org_roles WHERE
   id='r-mt' AND org_id='<org-a>'` stays at exactly 1).
2. **The root agent's stamped MCP URL is its own org — the real leak.**
   The gateway above is *always* org-correct (it reads `org` from the
   path). The #2570 bug was upstream of it: the **spawner** stamped the
   wrong org's MCP URL onto the Worker's agent app, so org-b's root
   desktop booted pointing at org-a, and the agent's `create_role` /
   `hire_worker` then hit org-a's gateway. Catch it by inspecting what
   the spawner wrote — **after activating org-a first, then org-b:**
   ```sql
   -- agent app id for each org's w-mt (provisioned on hire/activate):
   SELECT org_id, value FROM org_worker_runtime_state
     WHERE worker_id='w-mt' AND key='agent_app_id';
   -- the helix-org MCP entry on that app must embed the worker's OWN org:
   SELECT config FROM apps WHERE id='<that app id>';  -- grep the url:
   --   …/api/v1/mcp/helix-org/<this worker's org_id>/workers/w-mt/mcp
   ```
   org-a's app URL embeds org-a's id; org-b's app URL embeds **org-b's**
   id — *not* org-a's, even though org-a activated first. Under the old
   bug org-b's app carried org-a's id. (Re-activate with
   `POST /api/v1/orgs/<org>/workers/<id>/activate` if a desktop needs a
   nudge.)
3. **Gold standard — the literal bug report, end-to-end.** With org-a's
   root activated first, open **org-b**'s `w-root` detail page and chat
   (§10): "Create a role `r-chat` and hire an AI worker `w-chat` into
   it." Watch the agent's `create_role` / `hire_worker` tool calls in the
   transcript, then assert `SELECT org_id FROM org_roles WHERE
   id='r-chat'` → **org-b only**, never org-a. This reproduces the exact
   reported symptom against the fixed code.

The unit-level counterpart is the colliding-IDs integration gate added
in #2570 (drives Dispatcher → Queue → Spawner with two orgs holding
identical ids; asserts an org-a event activates only org-a's `w-root`,
under org-a, and that both orgs' `w-root` activate concurrently on
independent lanes). §16 is the live, end-to-end version of that gate.

### Known sharp edge — per-Worker repo id is NOT org-scoped

Surfaced while running §16.2: hiring the **same** worker id into two
orgs *within the same wall-clock second* makes the second org's
activation fail. The per-Worker git repo id is
`code-<workerID>-<unix-seconds>` and the `git_repositories` table PK is
**global**, not `(id, org_id)` — so the two hires mint the identical
repo id and the second collides:
```
activation.fail worker=w-mt … create per-Worker repo: 500 …
  duplicate key value violates unique constraint "git_repositories_pkey"
```
The org-graph rows (`org_roles`, `org_workers`) still land correctly in
each org — only the second org's *desktop provisioning* aborts. Spacing
the hires by >1s sidesteps it (the timestamp differs), but the latent
defect is real: it is the same id-collision class as #2570, one layer
down in the git-repo service, which #2570 did not cover. Either scope the
repo id by org or make it collision-proof (ULID, not second-granularity).

## Pass criteria

- §1 — a fresh org is **empty** (no workers/roles); seeding via New
  Role → Add Worker creates `w-root` (`role_id = r-root`);
  `org_reporting_lines` is empty; no `org_positions` / `org_environments`
  table.
- §2 — `r-root` starts with an empty tool set (no baseline is injected
  on create); multi-select adds/removes a tool; refresh persists; an
  edit propagates to every Worker in the role on the next MCP
  `tools/list`.
- §3 — AI worker creation doesn't crash the API; firing any worker
  (including the root) succeeds with no protection (204, never a 409
  lock); role delete dialog enumerates the affected workers before
  confirm.
- §4 — restart persists; both themes render. (Cross-org isolation is
  §16.)
- §16 — **read isolation**: with distinct sentinels planted per org,
  neither org's `list_*` / REST list / `read_events` surfaces the other's
  records, and a cross-org `get_role` / `get_worker` id-guess returns
  `record not found` (never the other org's row). **Write isolation**:
  under colliding ids, a UI create after switching orgs and an MCP
  `create_role` / `hire_worker` against an org's URL each appear in
  exactly one org's `org_roles` / `org_workers` (composite-PK rows, one
  per org, distinct content) and write zero rows into the other.
  Activating org-a's root first must NOT taint org-b: org-b's `w-mt`
  agent app embeds **org-b's** id in its helix-org MCP URL, and an org-b
  root-chat "create a role / hire a worker" lands in org-b. Watch the
  per-Worker repo-id collision (same worker id + same second across orgs
  fails the second activation on `git_repositories_pkey`).
- §6 — transcripts' subscribers list contains the Worker's
  manager id (derived from the reporting line, not whoever clicked
  hire); a manager with reports also has an `s-team-<id>` stream; live
  SSE replaces, doesn't append.
- §8 — subscriptions are worker-keyed; fire drops them; new
  hires do NOT inherit; two workers in the same role can hold
  disjoint subscription sets.
- §9 — fire removes the worker's transcript (no orphans), and
  if the worker was a manager, its `s-team-<id>` stream is torn down
  too (topology owns the teardown).
- §12.3a — adding a second manager subscribes that manager to the
  report's transcript and creates its team stream; **removing
  the edge unsubscribes the ex-manager** (the reparent-desync
  regression this PR fixes) and tears down the now-empty team stream;
  the surviving manager is untouched.
- §13 — `managers` returns each manager's id/role/`dmStreamId` (empty
  array, not null, for the root); `reports` returns a non-null
  `s-team-<id>` teamStreamId + each report's `dmStreamId`, flags a
  report that manages its own sub-team (`manages: true` +
  `teamStreamId`), and returns `null` teamStreamId + empty `reports`
  for a leaf. `dm` works only between reporting pairs (channel
  provisioned by topology on edge-wiring); a `dm` to a skip-level /
  non-reporting worker is refused and mints nothing.
- §10 — the worker page shows the conversation inline (transcript +
  tool calls + composer) when a session exists, GET-only on load (no
  container spin-up), auto-scroll defaulting ON; the empty state shows
  otherwise. Sending a message dispatches via `POST …/sessions/chat`
  (the composer does NOT get stuck on "Message queue (saved locally)")
  and the worker's agent replies live in the transcript. No
  desktop-launch / "Start new chat" buttons remain on the page. Following
  the right-rail **Project** link → spec board → **View Human Desktop**
  (the running-session label, not Open/Resume) opens `TeamDesktopPage` with
  a **live** streamed desktop — never the "Desktop Paused" / **Start
  Desktop** overlay.
- §14 — every helix-org page's breadcrumb comes from the shared hook;
  the org-name crumb links to the chart from every page (list, detail,
  settings), and detail pages carry an org / Section / leaf trail with
  no standalone back button.
- §15 — worker identity is an editable Monaco field saved via
  `POST …/workers/<id>/identity`; the right-rail Project and Agent ids
  link into Helix; the Advanced accordion's "Restart agent session"
  re-activates the worker with a data-loss warning.
- §11 — fresh sandbox: Zed launches; per-Worker `gh`
  startupScript installs cleanly; `gh auth status` green.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- A Worker holds at most one Role.
- A Worker's reporting lines are many-to-many (one `org_reporting_lines`
  row per manager–report pair). A Worker may report to several managers
  simultaneously; the graph is a cycle-guarded DAG, not a tree.
- Adding a reporting line is cycle-guarded server-side: dragging a
  manager edge that would close a reporting loop is rejected with a 409.
- Org isolation is enforced by composite `(id, org_id)` PKs in the store;
  every tenancy bug to date lived in a process-wide layer above it keyed
  by an id unique only within an org (`w-root`, `r-root`). §16 is the
  colliding-ID gate that exercises that layer. One layer it does NOT yet
  cover is safe: the per-Worker git repo id (`code-<workerID>-<second>`)
  has a **global** PK, so two orgs hiring the same worker id in the same
  second collide and the second org's desktop fails to provision (the
  org-graph rows still land correctly). See §16's "Known sharp edge".
