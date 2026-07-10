# Helix Org — QA test plan

End-to-end UI test for helix-org. Run before merging any change to
`frontend/src/pages/HelixOrg*.tsx`, `frontend/src/components/orgs/`,
`api/pkg/org/`, or `api/pkg/server/helix_org*.go`.

Every feature is tested in exactly one place; sections reference
each other instead of repeating steps. Skip nothing without reading
the why.

## Mental model

- **Bot** — the one and only org-graph entity (the merge of the former
  Role and Worker). A Bot has an `id` (convention `b-<kebab-case>`, e.g.
  `b-root`, `b-eng`), markdown `content` (its prompt — read on every
  activation; this content IS its only identity, there is no separate
  identity record), a `tools` list (its live MCP surface), a `topics`
  manifest, and `parent_ids` (the bots it reports to). Every Bot is a
  uniform agent — there is no `kind`, no human/ai split, no Role→Worker
  binding. A Bot is singular: one row, one prompt, one tool list.
- **Reporting line** — an `org_reporting_lines` `(org, manager,
  report)` row meaning *report* reports to *manager*. A Bot may
  report to several managers; a top-level (root) Bot has none.
  Bot deletion drops every line that references it via
  `ON DELETE CASCADE` foreign keys. The graph is a cycle-guarded DAG.
- **Subscription** — an `(org, bot, topic)` row (`org_subscriptions`,
  column `bot_id`). Bot-anchored: deleting a Bot drops its rows. A
  topic subscription is opt-in from the Bot's prompt — code never
  auto-subscribes a Bot. Different bots therefore consume different
  topics (specialisation) and only the on-call subset of a team wakes
  on an event (load patterns).

A fresh org starts **empty**; the human operator builds it from the
chart — **New bot** (one step). The convention these tests follow is to
create a root Bot `b-root` (no parent) first. It is an ordinary row with
no special status — `b-root` can be deleted like any other.

The Chart tab is a ReactFlow canvas. Bots are plain nodes — there are no
role group-frames. Bot → Bot edges are reporting lines
(`org_reporting_lines` rows); drag from a manager's bottom handle to a
subordinate to add one, delete an edge to remove that line (a Bot can
have several). Topics hang off the right; drag from a Bot's right handle
to a Topic to subscribe. Click a Bot to open its detail page.

## Setup

Acting user has the `helix-org` alpha flag and is a member of the
test org. Sign in at `/login`, click **Org** in the primary
sidebar. Tests run against `…/orgs/<org>/helix-org/*`.

## §1. Empty org + seed the root

1. Land on `…/helix-org/chart`. Middle sidebar shows highlighted
   **Chart** plus **Bots / Topics / Settings**.
2. A fresh org is **empty**: the chart shows the empty state
   *"No bots yet. Click **New bot** to get started."* — no bot nodes.
   Confirm DB:
   `SELECT count(*) FROM org_bots WHERE organization_id = '<org>'`
   → 0; `SELECT * FROM org_reporting_lines WHERE org_id = '<org>'`
   → zero rows. No `org_roles` / `org_workers` / `org_positions` /
   `org_environments` tables exist
   (`SELECT to_regclass('org_workers')` → NULL).
3. Network tab: `/bots /topics` requests all 2xx in
   parallel (each returns an empty list).
4. **Seed the root.** Click **New bot** → `b-root`, content
   `# Root`, no parent. It appears on the chart with an enabled
   **Delete** (no "protected" lock — the root is an ordinary row).
   Later sections assume this root exists.
   Confirm: `SELECT id FROM org_bots WHERE id = 'b-root'`
   → `(b-root)`.

## §2. Bots list + create + tool editor

`Bot.Tools` is the Bot's live MCP surface. Editing a Bot's Tools
changes its capability on its next MCP request. Creating a Bot is one
step (no separate "New Role" then "Add Worker") — the Bots-tab **New
bot** button, the chart **New bot**, and the MCP `create_bot` tool all
take `id` + `content` + optional `parentId`.

1. **Bots** in the middle sidebar. Columns: ID / Content / Tools /
   Topics / Reports to / Updated.
2. `b-root` (seeded in §1) shows the baseline read tools the New-bot
   flow injects (`managers`, `reports`, `read_events`, … — non-empty).
   The removed position tools (`create_position`, `list_positions`,
   `get_position`, `list_position_children`) are NOT present — pin
   so re-adding them is a deliberate, visible change.
3. `b-root` vertical-dot menu offers **Open** and an enabled
   **Delete** — no bot is protected.
4. **+ New bot** → `b-test-dm`, content `# DM`, no parent. Row
   appears; detail page opens, Tools field empty.
5. On the detail page click the Tools dropdown. The available tools
   render. Tick `dm` — popper stays open (`disableCloseOnSelect`).
   Press Escape.
6. **Save** → snackbar `bot b-test-dm saved` → button disables.
7. Hard refresh — `dm` chip persists.
8. **Live tool surface.** On `b-test-dm`, add `publish` via the
   dropdown + Save. Hit its MCP endpoint
   (`/api/v1/mcp/helix-org/<org>/workers/b-test-dm/mcp` — note the URL
   path segment is still `workers`, kept to avoid rippling outside the
   package; the entity is a Bot) → `tools/list`: `publish` is now in
   the list without any extra create call, tool-assignment step, or
   session restart. Remove + Save: next `tools/list` no longer
   includes it.

## §3. Create + delete bots, cascade semantics

Pins the create path and the delete dialog. No bot is protected.

1. **+ New bot** (the Bots tab primary action button; the Chart also
   creates via its **New bot** affordance). Form: `id`, `content`,
   `parent_id` (optional — the new bot's initial manager; creates one
   reporting line).
2. Submit id `b-ai-1`, content `# AI 1`, parent `b-root`. Row appears
   in the Bots table — Reports to shows `b-root`.
3. Click the `b-ai-1` row → URL becomes
   `…/helix-org/bots/b-ai-1`. The detail page must NOT crash
   the API on first load.
4. **Delete bot** on `b-ai-1` → confirm dialog → bot gone. No bot is
   protected: deleting any bot, including the root `b-root`, succeeds
   (REST `DELETE /bots/{id}` returns 204, never a 409 lock).
   (Re-create `b-ai-1` to continue.)
5. Create `b-carol` (parent `b-root`), delete from her detail page →
   confirm dialog. Bot gone from list.
6. Delete `b-test-dm` from the Bots tab — the dialog notes any direct
   reports lose this manager. Confirm; the bot goes and its reporting
   lines drop.
7. `b-root` is deletable too — its **Delete** is enabled and cascades
   like any other bot (it has no special status). There is NO MCP
   delete tool — a bot's agent must not delete bots from chat; delete
   is REST-only (`DELETE /bots/{id}`).

## §4. Cross-org isolation, persistence, theme

1. **Cross-org isolation** is its own section now — see §16. The
   shallow smoke test that lived here (switch org, confirm a fresh
   org starts empty) is subsumed by §16's two-level, colliding-ID
   gate. Do §16, not a re-run here.
2. Restart the API container. Everything persists — no `org_*`
   data is dropped on boot.
3. Toggle the top-right sun/moon. Both modes render the
   Chart canvas (bot nodes, topic nodes, reporting edges) cleanly.

## §5. Bots list (filtering)

`…/helix-org/bots` table — columns ID / Content / Tools / Topics /
Reports to / Updated. Vertical-dot menu offers **Open** and
**Delete** (enabled for every bot — nothing is protected).
Filter by reporting line / content using the column header search
(bots can share managers, so the list must be filterable, not
grouped).

## §6. Topics list, detail, live tail

**Every** Bot has an auto-created `s-transcript-<botID>` topic (its
append-only, observable transcript) — including the root `b-root`
seeded in §1. The hierarchy topics are derived from the reporting graph
by the reconciler (`application/reconcile`): a transcript's subscribers
are the Bot's **managers** (a manager-less root has none — its
transcript is unobserved, never self-subscribed), and any Bot with ≥1
direct report also gets an `s-team-<managerID>` broadcast topic
(members = manager + direct reports). The Topics surface lives at
`…/helix-org/topics`.

1. **Topics list** — columns ID / Name / Transport / Subscribers
   / Created. Every bot has a matching `s-transcript-<botID>` row,
   including `s-transcript-b-root` (the manager-less root). Any Bot
   that has at least one direct report also shows an
   `s-team-<managerID>` row.
2. **Subscribers column** shows bot ids. For a freshly-created
   `b-ai-1` (parent `b-root`), `s-transcript-b-ai-1`'s subscriber list
   is `[b-root]` — its **manager** is subscribed, because transcript
   observers are derived from the reporting line, not from whoever
   created the bot — and explicitly NOT `[b-ai-1]` (a bot subscribed to
   its own transcript would loop dispatch). `s-team-b-root` exists with
   subscribers `[b-root, b-ai-1]`.
3. **Detail page**: click any topic id. URL becomes
   `…/helix-org/topics/<id>`. Header shows id (monospace) +
   transport kind chip + description + `created by … · ts` +
   subscribers chip-list. Messages section lists EventCard rows
   newest-first.
4. **Live SSE tail**: publish a new event. The new event appears
   at the top within ~1.5s without reload.

## §7. GitHub topics — one-click setup

Pre-conditions: the Helix GitHub App installed for the org (the repo
picker lists its installable repos). `SERVER_URL` is a public host
(loopback refused).

Create → pick repo → submit → webhook installed end-to-end.
Detail page exposes **Edit on GitHub →** and **Re-install**.

- **Shared repo picker.** Both the New Topic dialog and the per-topic
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
  events list). Confirm a save round-trips: `org_topics` config still
  has the original `events` and `webhook_id` after editing only the
  repo.
- **Settings.** The `transport.github` row is no longer shown on the
  Settings page — the GitHub App provisions the webhook secret itself,
  so there is nothing to paste. (The registry key still exists; it's
  just not operator-facing.)

## §8. Bot-anchored subscriptions

Subscriptions are keyed on `(org, bot, topic)` (`org_subscriptions`,
column `bot_id`). Deleting a Bot drops its subscription rows; nothing
inherits them — subscription is opt-in from a bot's prompt.

1. **Bot detail Subscriptions panel** (`…/bots/<id>`, below the Tools
   editor): N-count reflects the bot's subscription set. Multi-select
   dropdown shows every topic with description + checkbox state, with
   `disableCloseOnSelect` (same shape as the tool editor in §2).
   Toggling updates this bot's set. Caption: "Subscriptions are
   per-Bot — they die when this Bot is deleted."
2. **Dies on delete**: create `b-cycle` (parent `b-root`), subscribe
   `b-cycle` → a test topic, delete `b-cycle`. Inspect
   `org_subscriptions` — no row references `b-cycle`. Publish a
   message to that topic and verify no activation fires (no recipient).
3. **No automatic inheritance**: create `b-cycle-2` (same content,
   parent `b-root`). Publish to the test topic. `b-cycle-2` does NOT
   activate — code never subscribes a bot; the bot opts in explicitly
   from its prompt.
4. **Specialisation check**: create two bots `b-secrev` and `b-perfrev`
   (both parent `b-root`). Subscribe `b-secrev` → `s-security-prs` and
   `b-perfrev` → `s-perf-prs`. Publish to `s-security-prs`: only
   `b-secrev` activates.

## §9. Topic delete

Deleting a bot removes both kinds of hierarchy topic it owns — its
`s-transcript-<botID>` topic and, if it was a manager, its
`s-team-<botID>` team topic. Topology owns the teardown (Delete
reconciles after the row is gone; there is no inline topic delete).

1. Create a fresh bot `b-cleanup` (parent `b-root`). Its transcript
   row + an entry in `s-transcript-b-cleanup`'s subscriber list appear.
2. Delete `b-cleanup`. `s-transcript-b-cleanup` row disappears
   from the Topics list within ~1s (`lifecycle.Delete` →
   `topology.Reconcile`). Events on that topic survive in
   `org_events` as an audit trail.
3. **Team topic teardown.** Create `b-cleanup-mgr` (parent `b-root`),
   then create `b-cleanup-rep` with parent `b-cleanup-mgr`. Confirm
   `s-team-b-cleanup-mgr` now exists (subscribers
   `[b-cleanup-mgr, b-cleanup-rep]`). Delete `b-cleanup-mgr` (the
   confirm dialog notes its report loses its manager). Both
   `s-transcript-b-cleanup-mgr` **and** `s-team-b-cleanup-mgr`
   disappear from the Topics list:
   `SELECT id FROM org_topics WHERE id IN
   ('s-transcript-b-cleanup-mgr','s-team-b-cleanup-mgr')` returns
   zero rows. `b-cleanup-rep` survives, keeping its own
   `s-transcript-b-cleanup-rep`.

## §10. Chat: inline transcript

The bot detail page renders the bot's conversation inline using
the same transcript view the spec-task page uses (`EmbeddedSessionView`
+ `RobustPromptInput`), reading the per-Bot project's long-lived
"Human Desktop" exploratory session. The inline transcript is the chat
surface — there are no "Open Human Desktop" / "Restart Desktop" buttons
on the page (removed); session (re)provisioning is handled by the
bot's activation flow and the Advanced → Restart agent session
action (§15). The embedded transcript defaults to auto-scroll ON
(`autoScrollOnMount`), so live output follows to the bottom without the
operator un-pausing it.

1. **Inline transcript auto-loads.** Open a bot that has already
   been chatted with (`…/helix-org/bots/<id>` with a `project_id`).
   The Chat panel shows the conversation inline — user turns, the
   agent's responses, and its MCP tool calls (collapsible) — without
   any click. The resolve is GET-only: opening the page must NOT spin
   up a desktop container (Network tab: one
   `GET …/projects/<pid>/exploratory-session`, no create/resume POST).
2. **No-session empty state.** A freshly-created bot that has never
   been chatted with (no `project_id`, or project with no exploratory
   session — the GET returns 204) shows "No conversation yet for this
   bot", not a crash or a spinner.
3. **Send a message and verify the bot responds.** This is the
   load-bearing test — the bot chat is useless if messages don't
   actually reach the agent. With the transcript shown, type a prompt
   the agent must visibly act on (e.g. "Reply with the single word
   PONG and nothing else") and send.
   - **The message must dispatch, not park.** The composer must NOT get
     stuck showing a queue header reading **"Message queue (saved
     locally)"** with the message sitting under it forever. That header
     means the prompt was written to local storage but never sent — the
     symptom of the 53b336e01 regression where the client-side queue
     pump was deleted. For a bot/Human-Desktop session (no
     `spec_task_id`) the composer pumps its own queue via
     `onSend → streaming.NewInference`; the queued item should clear
     within ~1s, not persist.
   - **Network:** sending fires `POST …/sessions/chat` for the bot's
     session id (NOT `POST …/prompt-history/sync`, which is the
     spec-task-only path and 400s without a `spec_task_id`). It must
     return 2xx, NOT **401** — the bot's "Human Desktop" session is
     owned by an org member, not necessarily the operator currently
     driving the bot, so the chat endpoint authorizes via org/project
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
     the bot's session —
     `SELECT state, prompt_message FROM interactions WHERE session_id =
     '<sid>' ORDER BY created DESC LIMIT 2;` shows the user prompt and a
     `complete` assistant turn.
4. **No desktop-launch buttons.** The page must NOT render "Open Human
   Desktop", "Restart Desktop", or a topbar "Start new chat" button —
   all removed. Chat happens inline; the only restart affordance is the
   Advanced accordion (§15).
5. **The live desktop stream, not a Start-Desktop placeholder.** The inline
   transcript is the *text* half; this is the *visual* half — chatting with
   the bot drives a real GNOME/Zed desktop and the operator must be able to
   **watch** it. From the bot detail page click the **Project** id in the
   right rail (§15.2) → lands on the project's spec board
   (`…/projects/<pid>/specs`). Because the bot you just chatted with (§10.3)
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
   both the bot-chat transcript and this desktop view.

## §11. Bot sandbox: Zed launch, per-Bot tools, stale-session recovery

Open a fresh Bot's sandbox: Zed launches, the per-Bot `gh`
startupScript installs cleanly, and `gh auth status` is green. The
Bot's tools are present in the sandbox MCP surface.

## §12. Chart canvas: reporting + subscription drag

The Chart is a ReactFlow canvas keyed off the `org_reporting_lines`
join table (many-to-many: a Bot may report to several managers)
and bot-anchored subscriptions — there are no role frames and no
Position rows. This pins the drag interactions and the
`POST /bots/{id}/parents` / `DELETE /bots/{id}/parents/{parent_id}`
endpoints — both of which now reconcile the activation/team topics the
edge implies (see 3a).

1. On `…/helix-org/chart`, create three bots via **New bot**:
   `b-alice`, `b-bob`, `b-carol` (all parent-less). All appear as Bot
   nodes with no reporting edges (top-level orphans).
2. Drag from `b-root`'s **bottom** handle to `b-alice`'s **top**
   handle. A solid reporting edge appears; snackbar `b-alice now
   reports to b-root`. DB: `SELECT manager_id FROM org_reporting_lines
   WHERE report_id='b-alice' AND org_id='<org>'` → `{b-root}`. Dagre
   lays the tree out from the new edge (`b-root` above `b-alice`).
   - **Topology side-effects** (the new manager edge wires the comms
     channels — and they exist ONLY because the edge was wired; the
     orphan bots from step 1 had no team/DM topics, only their own
     `s-transcript-<id>`). `s-transcript-b-alice` now has `b-root` as
     a subscriber (the manager observes the report's transcript):
     `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-transcript-b-alice'` → `{b-root}`. The manager's team
     topic now exists with both of them:
     `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-team-b-root'` → `{b-root, b-alice}`. And the 1:1 DM
     channel for the edge now exists too — DM channels are scoped to the
     reporting graph, provisioned here, NOT created on demand by the `dm`
     tool: `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-dm-b-alice-b-root'` → `{b-alice, b-root}` (id is the
     sorted pair).
3. Drag from `b-alice`'s bottom handle to `b-bob`'s top handle →
   `b-bob` reports to `b-alice`.
3a. **Multi-manager + reparent-desync fix (highest-priority
   regression).** Drag from `b-carol`'s bottom handle to `b-alice`'s
   top handle. A second reporting edge appears; snackbar `b-alice now
   reports to b-carol`. `GET /bots/b-alice → .parent_ids` returns
   `[b-root, b-carol]` (order may vary). DB:
   `SELECT manager_id FROM org_reporting_lines WHERE
   report_id='b-alice'` → two rows.
   - **Both managers now observe the transcript.**
     `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-transcript-b-alice'` → `{b-root, b-carol}`. And
     `b-carol`'s team topic now exists:
     `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-team-b-carol'` → `{b-carol, b-alice}`.

   Then select **only** the `b-carol → b-alice` edge and press
   **Delete**: snackbar `b-alice no longer reports to b-carol`; the
   `b-root → b-alice` edge survives; `parent_ids` is back to
   `[b-root]`.
   - **The ex-manager is unsubscribed — this is the bug this PR fixes.**
     `SELECT bot_id FROM org_subscriptions WHERE
     topic_id='s-transcript-b-alice'` → `{b-root}` only (NOT
     `{b-root, b-carol}` — the old bug left `b-carol` subscribed after
     the edge was removed). `s-team-b-carol` is gone (b-carol has no
     other reports), and so is the DM channel for the dropped edge:
     `SELECT id FROM org_topics WHERE id IN
     ('s-team-b-carol','s-dm-b-alice-b-carol')` → zero rows.
     `b-root`'s observership, `s-team-b-root`, and the
     `s-dm-b-alice-b-root` channel are untouched.
4. **Cycle guard**: drag from `b-bob`'s bottom handle to `b-alice`'s
   top handle (would make alice→bob→alice). API returns 409; snackbar
   surfaces the cycle error; no edge added. DB unchanged.
5. Select the `b-root → b-alice` edge, press **Delete** (and retest
   with **Backspace**, re-adding the edge between the two). Edge gone;
   snackbar `b-alice no longer reports to b-root`; the
   `org_reporting_lines` row for `(b-root, b-alice)` is gone (no row
   where `report_id='b-alice'`).
6. Create a topic `s-test` (Topics tab). It appears as a dashed
   node to the right of the tree. Drag from `b-alice`'s **right**
   (amber) handle to `s-test` → dashed subscription edge; snackbar
   `b-alice now consumes s-test`. `GET /topics` shows `b-alice` in
   subscribers.
7. Delete the subscription edge → `b-alice` drops from the topic's
   subscriber list (bot-anchored unsubscribe).
8. Delete `b-alice` from her node's trash icon → confirm dialog lists
   that her one direct report (`b-bob`) loses its manager. Confirm;
   node gone, `b-bob`'s edge to her removed.

## §13. Reporting-line comms: `managers` / `reports` tools

The two lazy read tools resolve a Bot's reporting lines live, so the
fixed bot policy can refer to "your managers" / "your reports"
abstractly (escalate up via `managers`+`dm`; brief down via
`reports`+`publish` to the team topic). Both are MCP tools on each
Bot's surface — call them via `tools/call` at
`/api/v1/mcp/helix-org/<org>/workers/<id>/mcp` (the same endpoint §2.8
uses for `tools/list`; the URL path segment is still `workers`). The
seeded `b-root` and every bot drafted via `create_bot` carry
`managers` + `reports` (baseline reads are injected on create).

Setup: create `b-mgr` (parent `b-root`), `b-rep` (parent `b-mgr`), and
`b-sub` (parent `b-rep`) — so `b-rep` is both a report (of `b-mgr`) and
a manager (of `b-sub`).

1. **`managers` from a report.** `tools/call managers` on `b-rep` (no
   args) → `{"managers":[{"id":"b-mgr",
   "dmStreamId":"s-dm-b-mgr-b-rep"}]}`. The `dmStreamId` is the
   deterministic sorted pair, so `dm`-ing `b-mgr` lands on it. Call
   `managers` on `b-root` → `{"managers":[]}` — an **empty array, not
   null** (the root reports to no one).
2. **`reports` from a manager.** `tools/call reports` on `b-mgr` →
   `teamStreamId` is `"s-team-b-mgr"` (non-null), and the `reports`
   array contains `b-rep` with `dmStreamId":"s-dm-b-mgr-b-rep"` and
   `manages: true` + `teamStreamId":"s-team-b-rep"` (because `b-rep`
   leads its own sub-team via `b-sub`). Publishing to the top-level
   `s-team-b-mgr` reaches `b-rep`; you delegate `b-rep`'s workstream to
   it rather than posting into `s-team-b-rep` yourself.
3. **`reports` from a leaf.** `tools/call reports` on `b-sub` →
   `teamStreamId": null` and `reports": []` (empty array — no one
   reports to `b-sub`).
4. **`dm` is reporting-scoped.** `tools/call dm` from `b-rep` to its
   manager `b-mgr` (a reporting pair) succeeds — the channel was
   provisioned when the edge was wired. `tools/call dm` from `b-mgr` to
   `b-sub` (a **skip-level** bot, no direct reporting edge) is
   **refused** with an error naming `managers`/`reports` — there is no
   implicit DM channel to an arbitrary or skip-level bot, and the
   `dm` tool does NOT mint one. (Confirm the refusal wrote nothing:
   `SELECT id FROM org_topics WHERE id='s-dm-b-mgr-b-sub'` → zero rows.)

## §14. Breadcrumbs (all helix-org pages)

Every helix-org page builds its breadcrumb trail from the shared
`useHelixOrgBreadcrumbs(section?)` hook (`components/helix-org/`), so the
trail is consistent and the leading org-name crumb is always a link to
the chart. No page uses the old `orgBreadcrumbs={true}` plain-text org
crumb, and the detail pages have NO standalone "back" button — the
breadcrumb is the back-nav.

1. **List pages.** On `…/helix-org/bots`, `…/topics`
   the trail is `<org name> / <Section>` (e.g. `test / Topics`). The
   org-name crumb is a clickable link → navigates to
   `…/helix-org/chart`. The section word is the current (leaf) crumb,
   not a link.
2. **Detail pages.** On a bot/topic detail page the trail is
   `<org name> / <Section> / <leaf id>` (e.g. `test / Bots / b-ai-1`).
   Both `<org name>` (→ chart) and `<Section>` (→ the list page) are
   links. There is no separate "Bots" / "Topics" back button anywhere.
3. **Settings.** `…/helix-org/settings` shows `test / Settings`; the
   org crumb links to the chart.
4. The org link works from **every** page above, not just settings —
   this is the regression that motivated the shared hook (the list
   pages previously rendered the org name as plain text).

## §15. Bot detail surfaces: content, links, Advanced

The bot detail page is the merged role+worker detail. Beyond the tool
editor (§2), the subscriptions panel (§8), and the inline chat (§10),
it exposes an editable content/prompt, deep links into Helix, and a
guarded restart.

1. **Editable content.** The Content section is a Monaco markdown
   editor (not a read-only block) with a **Save** button — this is the
   bot's prompt and its only identity. Edit the text → Save is enabled
   → click (or Cmd/Ctrl+S) → snackbar `bot <id> saved`;
   `update_bot` (REST `PUT …/bots/<id>`) returns 2xx. The Spawner
   projects the new content into the bot's prompt on the next
   activation.
2. **Right-rail links.** In the right panel, the **Project** id is a
   link → `…/projects/<project_id>/specs` (the project's board), and an
   **Agent** row links to `…/agent/<agent_app_id>`. Both render only
   when the bot has been provisioned (has a project / agent app).
3. **Advanced → Restart agent session.** A collapsed **Advanced**
   accordion at the bottom of the page expands to a **Restart agent
   session** button with a caption warning that in-progress work is
   lost. Clicking re-activates the bot (`POST …/bots/<id>/activate`)
   → snackbar confirming the restart was queued.

## §16. Multi-tenancy: read + write isolation across two orgs

This is the load-bearing tenancy gate. It exists because of the
cross-tenant leak fixed in
`https://github.com/helixml/helix/pull/2570` (root cause in
`design/2026-06-09-org-multitenancy-spawner-leak.md`): a new org's
operator asked to "create a bot" and the bot landed in a **different**
org.

**Two distinct threats — test both.** "Multi-tenancy" here means two
things, and they need different detectors:

1. **Read leakage** (the more dangerous): org-b's operator or agent can
   *see* org-a's bots / topics / events / transcripts. The right
   detector is **distinct sentinel data** — plant a uniquely-named,
   secret-tagged record in each org and assert the *other* org's reads
   never surface it, and that cross-org id-guessing returns not-found.
   Colliding ids are the WRONG detector for read leakage: a leak can hide
   behind the matching id (you can't tell whose row you got back).
2. **Write leakage** (the literal #2570 symptom): org-b's `create_bot`
   lands in org-a. The right detector is **colliding ids**
   in two orgs at once — because the store is tenant-safe by composite
   `(id, org_id)` PK (confirm: `\d org_bots` shows
   `PRIMARY KEY (id, org_id)`), so every write bug we have shipped lived
   in a process-wide layer *above* the store keyed by an id unique only
   *within* an org — and **every org's root bot is conventionally
   `b-root`**, ids like `b-engineer` repeat across orgs. A test
   with *different* ids per org can pass while the singleton leaks; the
   same id in both orgs is what bites.

So §16 does both: a **read-isolation** pass with distinct sentinels, then
**write-isolation** passes (Levels 1–2) with colliding ids.

**Ordering matters.** The spawner bug froze the **first** org to ever
activate a bot and replayed its identity for everyone else. So always
activate/act in **org-a first**, then org-b, and assert org-b's writes
landed in org-b. If you only ever exercise one org, or org-b first, the
frozen-identity leak hides.

**Setup.** Acting user owns (or is a global admin of — `authorizeOrgMember`
grants admins a temporary owner membership for any org) two helix-org
orgs, both already seeded with a `b-root` root (per §1). On
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
  "http://localhost:8080/api/v1/mcp/helix-org/$1/workers/b-root/mcp" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" -d "$2" \
  | sed -n 's/^data: //p'; }

# plant DISTINCT sentinels with secret content:
mcp org-a '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_bot","arguments":{"id":"b-secret-aaa","content":"# SECRET-A-DATA do-not-leak"}}}'
mcp org-b '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_bot","arguments":{"id":"b-secret-bbb","content":"# SECRET-B-DATA do-not-leak"}}}'
```

1. **Cross-org `list_bots` shows only the acting org.** `list_bots` on
   org-b must return `b-secret-bbb` and **never** `b-secret-aaa` /
   `SECRET-A-DATA`; `list_bots` on org-a is the mirror image. Same for
   `list_topics` — org-a's unique topic ids
   (e.g. `s-dummy-*`, `s-transcript-<an org-a-only bot>`) must be
   absent from org-b's list. The only ids that may appear in both lists
   are generic per-org rows (`s-transcript-b-root`, `s-team-b-root`) —
   those are each org's *own* row that happens to share an id, not a leak;
   confirm the row's `org_id` and content are the reader's, not the
   other's.
2. **Cross-org id-guess returns not-found, not the other org's row.**
   `get_bot {id:"b-secret-aaa"}` issued against **org-b**'s endpoint
   must come back `record not found` (`isError:true`) — NOT org-a's
   data. This is the `loadAuthorized…` / composite-PK guard; a
   regression that dropped the `org_id` predicate would return the row.
3. **UI/REST surface, same assertion.** `GET /api/v1/orgs/<org-b>/bots`
   (what the Bots list page renders) returns only org-b's bots —
   none of org-a's distinct bot ids. Repeat for `/topics`.
4. **Events / transcripts don't cross.** Publish a secret-tagged event to
   an org-a topic; `read_events` / `list_topic_events` on every org-b
   topic must never return it. (The #2570 streamhub fix namespaced wake
   topics by org so one org's publish can't even wake another's
   subscribers; this asserts the read path stays scoped too.)

### Level 1 — UI/UX (operator switches orgs and acts via the chart)

1. In **org-a**'s `…/helix-org/chart`, create a bot `b-mt` (parent
   `b-root`) via **New bot** (§3/§12).
2. Switch to **org-b** via the top-left org selector. The chart renders
   org-b's *own* baseline — `b-mt` from org-a is **absent**.
   This is the read-isolation half: a colliding id in another org must
   not bleed into this canvas.
3. In org-b, **New bot** → enter an id that already exists in org-a
   (e.g. `b-engineer`, which org-a already holds) → Create. It succeeds
   (no "already exists" — the PK is composite). Create `b-mt` in org-b
   too.
4. **DB — both rows exist, each scoped to its own org, content distinct:**
   ```sql
   SELECT org_id, id, left(content,30) FROM org_bots
     WHERE id IN ('b-engineer','b-mt') ORDER BY id, org_id;
   ```
   Each id returns one row per org; org-a's content is org-a's, org-b's
   is org-b's. The UI create landed in the org the operator had switched
   to — never the other.
5. Switch back to org-a: its `b-mt` / `b-engineer` are unchanged
   (org-b's writes did not mutate them).

### Level 2 — Helix tool level (the root bot's MCP surface)

This is the actually-exploited surface: the org-graph MCP tools a
Bot's agent calls (here the root `b-root`). Endpoint:
`POST /api/v1/mcp/helix-org/<org>/workers/b-root/mcp` (URL path segment
still `workers`) — auth is a Bearer api_key for an **alpha-flagged**
member (`hasAlphaFeature`, `authorizeOrgMember`); the org is resolved
from the **URL path**. It speaks JSON-RPC; no separate `initialize` is
needed, but the request must send `Accept: application/json,
text/event-stream` and the reply comes back as an SSE `data:` line.

1. **Tools are org-scoped by the URL, even under identical ids.** Drive
   the *same* ids into both orgs and confirm they split:
   ```bash
   KEY=<user api key>
   mcp(){ curl -s -X POST \
     "http://localhost:8080/api/v1/mcp/helix-org/$1/workers/b-root/mcp" \
     -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
     -H "Accept: application/json, text/event-stream" -d "$2" \
     | sed -n 's/^data: //p'; }
   # create_bot b-mt in BOTH orgs
   mcp org-a '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_bot","arguments":{"id":"b-mt","content":"# MT (a)","parentId":"b-root"}}}'
   mcp org-b '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_bot","arguments":{"id":"b-mt","content":"# MT (b)","parentId":"b-root"}}}'
   ```
   Each call returns `{"id":"b-mt", "activation_id":…}`.
   The DB query from Level 1.4 must show one row per org with the right
   per-org content. Critically: a `create_bot` against org-b's URL must
   write **zero** rows into org-a (`SELECT count(*) FROM org_bots WHERE
   id='b-mt' AND org_id='<org-a>'` stays at exactly 1).
2. **The root agent's stamped MCP URL is its own org — the real leak.**
   The gateway above is *always* org-correct (it reads `org` from the
   path). The #2570 bug was upstream of it: the **spawner** stamped the
   wrong org's MCP URL onto the Bot's agent app, so org-b's root
   desktop booted pointing at org-a, and the agent's `create_bot` then
   hit org-a's gateway. Catch it by inspecting what the spawner wrote —
   **after activating org-a first, then org-b:**
   ```sql
   -- agent app id for each org's b-mt (provisioned on create/activate):
   SELECT org_id, value FROM org_bot_runtime_state
     WHERE bot_id='b-mt' AND key='agent_app_id';
   -- the helix-org MCP entry on that app must embed the bot's OWN org:
   SELECT config FROM apps WHERE id='<that app id>';  -- grep the url:
   --   …/api/v1/mcp/helix-org/<this bot's org_id>/workers/b-mt/mcp
   ```
   org-a's app URL embeds org-a's id; org-b's app URL embeds **org-b's**
   id — *not* org-a's, even though org-a activated first. Under the old
   bug org-b's app carried org-a's id. (Re-activate with
   `POST /api/v1/orgs/<org>/bots/<id>/activate` if a desktop needs a
   nudge.)
3. **Gold standard — the literal bug report, end-to-end.** With org-a's
   root activated first, open **org-b**'s `b-root` detail page and chat
   (§10): "Create a bot `b-chat`." Watch the agent's `create_bot` tool
   call in the transcript, then assert `SELECT org_id FROM org_bots
   WHERE id='b-chat'` → **org-b only**, never org-a. This reproduces the
   exact reported symptom against the fixed code.

The unit-level counterpart is the colliding-IDs integration gate added
in #2570 (drives Dispatcher → Queue → Spawner with two orgs holding
identical ids; asserts an org-a event activates only org-a's `b-root`,
under org-a, and that both orgs' `b-root` activate concurrently on
independent lanes). §16 is the live, end-to-end version of that gate.

### Known sharp edge — per-Bot repo id is NOT org-scoped

Surfaced while running §16.2: creating the **same** bot id into two
orgs *within the same wall-clock second* makes the second org's
activation fail. The per-Bot git repo id is
`code-<botID>-<unix-seconds>` and the `git_repositories` table PK is
**global**, not `(id, org_id)` — so the two creates mint the identical
repo id and the second collides:
```
activation.fail bot=b-mt … create per-Bot repo: 500 …
  duplicate key value violates unique constraint "git_repositories_pkey"
```
The org-graph rows (`org_bots`) still land correctly in each org — only
the second org's *desktop provisioning* aborts. Spacing the creates by
>1s sidesteps it (the timestamp differs), but the latent defect is
real: it is the same id-collision class as #2570, one layer down in the
git-repo service, which #2570 did not cover. Either scope the repo id by
org or make it collision-proof (ULID, not second-granularity).

> **Terminology note (Stream → Topic rename).** The wire is a
> **Topic** everywhere: table `org_topics` (was `org_streams`), REST
> `/topics…`, MCP `create_topic`/`list_topics`/`get_topic`, frontend
> `TopicNode`. The `s-` id prefix is unchanged. Any older artefacts that
> still say "stream"/`org_streams` should be read as "topic"/`org_topics`.

## §17. Processors (transform / filter / router)

A **Processor** sits on the edge between Topics and reshapes or routes the
Messages crossing it. It reads **one input Topic** and writes **one or
more output Topics**; everything still flows through Topics (a branch's
output Topic is a real, auto-provisioned `org_topics` row, just collapsed
into the node visually). Kinds (`org_processors.kind`):

- **template** — rewrites `Message.body` via a Go `text/template` against
  the `{{ .Message.* }}` context. 1 input → 1 output, always passes.
- **truncate** — caps the body to `max_bytes` (rune-safe). 1 → 1.
- **filter / router** — N outputs, each a branch with a predicate; a
  Message goes to every branch whose predicate renders truthy; an empty
  predicate is the default/catch-all. **A router is just a filter with >1
  output** — no separate kind.

Execution is a late-bound fan-out arm of the dispatcher: publishing to a
processor's input Topic runs `Process` and re-publishes each result onto
the output Topic, which then dispatches to ITS subscribers (so chains
recurse; a hop guard + create-time cycle check bound it).

**Ports & the data-flow vs org-structure rule (the node design).** The
node has an **IN** port (left, orange) and one labelled **OUT** port per
branch (right, purple). Data-flow edges attach to the **SIDES** of nodes;
the Bot's **top/bottom** handles are reserved for **org structure**
(reporting lines). A processor-output → Bot edge therefore lands on the
Bot's **right** data port (`id="topic"`, the same port used to
subscribe to a Topic), never the top.

Setup: seed `b-root` (§1), and create two local Topics
`s-in` and `s-feed` (Topics tab). Bots won't run a turn this session
(no model budget), so verify the data path via DB + the REST publish
endpoint, not a live activation. Publish:
`POST /api/v1/orgs/<org>/topics/<id>/publish` with `{"body":…,"subject":…,"as":…}`.

1. **Create a template processor (drawer).** Chart → **Processor** →
   name `fmt`, kind `template`, input `s-in`, template
   `From {{ .Message.from }}: {{ .Message.subject }}`. Create. A
   `processor` node appears (header `fmt`, `IN` on the left, `OUT default`
   on the right). DB: `org_processors` has one row, kind `template`,
   `input_topic_id='s-in'`, `outputs` JSON has one `Owned:true` topic;
   that output `org_topics` row exists.
2. **Raw-JSON preview + syntax help.** Publish a message to `s-in`
   (`{"body":"hi","subject":"Order #7","as":"a@vip.com"}`). Reopen `fmt`
   (click its header). The **"Latest message … raw `{{ .Message }}` JSON"**
   box shows the canonical envelope pretty-printed
   (`{"from":"a@vip.com","subject":"Order #7","body":"hi"}`) — this is how
   the operator discovers the available keys. Click **Show syntax help**:
   it lists the `.Message.*` fields, the function set
   (`upper/lower/trunc/default/contains/hasPrefix/hasSuffix`) with the
   arg-order note (*test string first, field last* — `hasSuffix "@vip.com"
   .Message.from`), and (filter) the truthy-match rules.
3. **Execution (data path).** Publish to `s-in`; the output Topic
   (`outputs[0].topic_id`) gains an event whose decoded body is the
   rendered string. `org_events` for that topic shows
   `body="From a@vip.com: Order #7"`. (With a live agent, a Bot
   subscribed to the output Topic activates with that body.)
4. **Wire Topic → IN (drag-to-wire input).** On the chart, drag from
   `s-feed`'s right (amber) handle into `fmt`'s **IN** port. Snackbar
   `fmt now reads s-feed`; `org_processors.input_topic_id='s-feed'`. The
   IN edge re-routes from `s-feed`.
5. **Wire OUT → Bot (drop on the bot's SIDE).** Drag from `fmt`'s
   **OUT** port and drop **anywhere on `b-root`** (notably its right
   side — NOT the top). A dashed data edge connects to the Bot's right
   port; snackbar `b-root now consumes <out-topic>`;
   `org_subscriptions` has `(b-root, <out-topic>)`. Confirm the edge
   endpoint is on the Bot's **side**, not the top reporting handle.
6. **Wire OUT → another processor (chain).** Create a second processor
   `cap` (truncate, `max_bytes` 20, input `s-in`). Drag `fmt`'s OUT →
   `cap`'s IN: `cap.input_topic_id` becomes `fmt`'s output topic; a chain
   edge renders upstream-branch → IN.
7. **Disconnect the input (delete the IN edge).** Hover the IN edge of
   `cap`, click the **× / "Disconnect input"**. Snackbar `cap disconnected
   from its input`; `org_processors.input_topic_id=''` (empty); the edge
   is gone AND **stays gone after refresh** (it is NOT a no-op that
   reappears). `cap` is now inert until re-wired.
8. **Delete a subscription edge.** Delete the `fmt → b-root` data edge →
   `b-root` drops from that output Topic's subscribers
   (`org_subscriptions` row gone), persisted across refresh.
9. **filter / router.** Create `route` (filter, input `s-in`) with two
   branches: `vip` predicate `{{ hasSuffix "@vip.com" .Message.from }}`,
   and `default` (empty predicate). Two `OUT` ports render. Publish a VIP
   (`as:"x@vip.com"`) and a plain (`as:"x@y.com"`) message: the **vip**
   output Topic gets only the VIP event; the **default** output Topic gets
   both. Wire each branch port to a different Bot.
10. **Delete a processor (cascade).** Delete `route` (node trash icon →
   confirm). The processor row is gone and its **auto-provisioned output
   Topics are cascaded** (and their subscriptions), but any *explicit*
   shared output Topic survives.

### §17 regression checks (UX teething bugs — keep these working)

- **Duplicate name → 409, clean message** (not a raw `SQLSTATE 23505` /
  doubled `create processor: create processor:` leak). Create two
  processors with the same name in one org → second returns 409
  `a processor named "X" in this org already exists`.
- **A processor's owned output Topic is not independently deletable.**
  Its delete on the Topic list / detail returns **409**
  (`… is an output of processor …; delete the processor instead`) — never
  leaving the processor with a dangling output. Deleting the processor is
  the way to remove it (idempotent if the topic is already gone).
- **The input edge deletes for real.** Deleting it **disconnects** the
  input (clears `input_topic_id`) and stays gone — it must NOT be a no-op
  that vanishes then reappears on refresh.
- **OUT → Bot works dropping anywhere on the Bot.** Loose
  connection mode + `connectionRadius` mean you don't have to hit the tiny
  top target handle; dropping on the Bot's body/side connects.
- **Data edges attach to node SIDES; top/bottom stay for reporting.** A
  proc-output → Bot edge ends on the Bot's right data port (≈0px
  off it), far from the top handle.
- **Nodes are clickable, not occluded.** Processor nodes sit at their
  input Topic's Y (clear of the page header); clicking the header opens
  the edit drawer (it must not be covered by the header description or the
  MiniMap — the MiniMap lives bottom-left, Controls top-left).
- **Validation surfaces as 400 with a helpful message, drawer stays
  open:** empty/malformed template, `max_bytes ≤ 0`, malformed/unknown-func
  predicate, >1 output on a transform, unknown kind. A genuinely-unknown
  template field (`{{ .Message.nope }}`) is accepted (renders empty at
  runtime) — that's correct, not a bug.
- **Cycle guard on wiring.** Re-pointing a processor's input (or a chain)
  such that a branch's output leads back to its own input is rejected 409;
  no edge persists.

## §18. Slack transport (critical flows)

Three tenancy layers (see `design/2026-06-23-helix-org-slack-serviceconnection.md`):
the deployment-global `slack_app` (admin), the per-org `slack_workspace`
install, and the workspace-scoped Slack Topic.

**No global Slack app may be configured on this deployment — that is
expected, not a failure.** The global `slack_app` is a helix-admin
ServiceConnection set up outside org mode; a QA run can't create one from
the org UI. When none exists, the correct behaviour is graceful
degradation (below), not an error. Only run the install/inbound steps if
an app is present: `SELECT count(*) FROM service_connections WHERE type =
'slack_app'` → ≥1, and at least one has the credentials for its
`slack_ingress_mode` (`rest` → client id/secret + signing secret;
`socket` → app token + bot token).

- **Graceful with no app (always runnable).** With zero `slack_app` rows:
  the org Settings → Slack panel shows the connect surface but "Install to
  Slack" is unavailable (no `slack_client_id`); a real `POST
  /api/v1/slack/events` delivery returns **503** (inert, no signing
  secret), while a `url_verification` handshake still echoes its challenge
  (**200**) so a manifest-set Request URL can verify before the app is
  configured; the New Topic dialog still offers `kind=slack` but a
  workspace must be picked. No console errors, no 500s.
- **Workspace install (needs an app).** REST app with client creds:
  Settings → Slack → **Install to Slack** → approve in Slack → redirected
  back with `?slack_installed=1`; a `slack_workspace` row lands
  (org-scoped, `slack_team_id` set, bot token encrypted) and an
  auto-managed Topic `s-slack-ws-<connID>` appears named *"Slack:
  <workspace> (<app>)"*. Socket/on-prem app: paste the `xoxb-` bot token
  instead — backend `auth.test`s it, derives the team, stores the same row
  shape. Re-installing the same workspace refreshes the token (no
  duplicate row).
- **Inbound → routing → reply.** Post in a channel the bot is in. The
  message publishes onto the workspace Topic (`org_events` row, `extra`
  carries `slack_channel`/`slack_team_id`); the bot's own posts are
  dropped (no echo loop). A subscribed Bot (or a processor filter)
  activates; its prompt carries the `how_to_reply` hint. The Bot mints
  a token (`mint_credential provider=slack resource=<team_id>`) and posts
  back via `chat.postMessage` under its own name. Unknown team / no bound
  Topic → 200 + silently dropped.
- **Isolation + cascade.** `GET /api/v1/orgs/<org>/slack/workspaces`
  returns only that org's installs (cross-org id-guess → 404). Deleting
  the global `slack_app` cascade-removes every workspace install it
  produced **and** their `s-slack-ws-*` Topics across all orgs; a
  socket-mode app's live connection is torn down without a restart.

## §19. Human nodes (people in the org graph)

Design: `design/2026-07-07-humans-in-the-org.md`. A human node is a Bot
with `kind=human` — a placeholder for a real person, **never
spawned/activated**. Humans are **never free-created**: a human node is
always the projection of an existing org member (`helix_user_id` is the
anchor). Membership drives the nodes; there is no `create_human` tool and
no "New human" button.

1. **Org create → human node + Chief of Staff (peers, no edge).** Create a
   new org. The chart at `…/helix-org/chart` shows **two unconnected**
   nodes: your human node (id `h-<yourUserID>`, display = your name,
   rendered with a person icon / blue border / **Human** label) and a
   **Chief of Staff** bot. There is **no reporting line** between them —
   humans stay out of the reporting graph. Confirm DB:
   `SELECT id, kind, helix_user_id FROM org_bots WHERE org_id='<org>'`
   → an `h-<userID>` row `kind='human'` + a `chief-of-staff` row
   `kind=''`; `SELECT * FROM org_reporting_lines WHERE org_id='<org>'`
   → **zero rows** referencing the human node.
2. **No tools on the human** (`SELECT tools FROM org_bots WHERE
   id='h-<userID>'` → null/empty — a human never makes an MCP request).
3. **Add a member → their node appears.** Add a second member (invite +
   accept, or add-member). A `h-<theirUserID>` human node appears on the
   chart. Remove them → the node disappears
   (`org_bots` row gone).
4. **Human is never activated.** Subscribe a human node to any topic and
   publish an event to it: no agent run is spawned for the human
   (`org_activations` has no `worker_id='h-<userID>'` row) — the
   dispatcher skips human subscribers (`b.IsHuman()` guard).
5. **`who owns X` (no new code).** A bot with `list_bots` reads a human
   node's content and can name them as an owner — prompt-driven over the
   existing read surface.

## Pass criteria

- §1 — a fresh org is **empty** (no bots); seeding via New bot creates
  `b-root`; `org_reporting_lines` is empty; no `org_roles` /
  `org_workers` / `org_positions` / `org_environments` table.
- §2 — `b-root` has a non-empty tool set (position tools absent);
  multi-select adds/removes a tool; refresh persists; an edit
  changes the bot's surface on the next MCP `tools/list`.
- §3 — bot creation doesn't crash the API; deleting any bot
  (including the root) succeeds with no protection (204, never a 409
  lock); the delete dialog notes reports that lose a manager before
  confirm; there is no MCP delete tool (REST-only).
- §4 — restart persists; both themes render. (Cross-org isolation is
  §16.)
- §16 — **read isolation**: with distinct sentinels planted per org,
  neither org's `list_bots` / REST list / `read_events` surfaces the
  other's records, and a cross-org `get_bot` id-guess returns
  `record not found` (never the other org's row). **Write isolation**:
  under colliding ids, a UI create after switching orgs and an MCP
  `create_bot` against an org's URL each appear in exactly one org's
  `org_bots` (composite-PK rows, one per org, distinct content) and
  write zero rows into the other.
  Activating org-a's root first must NOT taint org-b: org-b's `b-mt`
  agent app embeds **org-b's** id in its helix-org MCP URL, and an org-b
  root-chat "create a bot" lands in org-b. Watch the per-Bot repo-id
  collision (same bot id + same second across orgs fails the second
  activation on `git_repositories_pkey`).
- §6 — every bot has an `s-transcript-<id>` topic; a transcript's
  subscribers list contains the Bot's manager id (derived from the
  reporting line, not whoever created it); a manager with reports also
  has an `s-team-<id>` topic; live SSE replaces, doesn't append.
- §8 — subscriptions are bot-keyed; delete drops them; nothing
  inherits; two bots can hold disjoint subscription sets.
- §9 — delete removes the bot's transcript (no orphans), and
  if the bot was a manager, its `s-team-<id>` topic is torn down
  too (topology owns the teardown).
- §12.3a — adding a second manager subscribes that manager to the
  report's transcript and creates its team topic; **removing
  the edge unsubscribes the ex-manager** (the reparent-desync
  regression this PR fixes) and tears down the now-empty team topic;
  the surviving manager is untouched.
- §13 — `managers` returns each manager's id/`dmStreamId` (empty
  array, not null, for the root); `reports` returns a non-null
  `s-team-<id>` teamStreamId + each report's `dmStreamId`, flags a
  report that manages its own sub-team (`manages: true` +
  `teamStreamId`), and returns `null` teamStreamId + empty `reports`
  for a leaf. `dm` works only between reporting pairs (channel
  provisioned by topology on edge-wiring); a `dm` to a skip-level /
  non-reporting bot is refused and mints nothing.
- §10 — the bot page shows the conversation inline (transcript +
  tool calls + composer) when a session exists, GET-only on load (no
  container spin-up), auto-scroll defaulting ON; the empty state shows
  otherwise. Sending a message dispatches via `POST …/sessions/chat`
  (the composer does NOT get stuck on "Message queue (saved locally)")
  and the bot's agent replies live in the transcript. No
  desktop-launch / "Start new chat" buttons remain on the page. Following
  the right-rail **Project** link → spec board → **View Human Desktop**
  (the running-session label, not Open/Resume) opens `TeamDesktopPage` with
  a **live** streamed desktop — never the "Desktop Paused" / **Start
  Desktop** overlay.
- §14 — every helix-org page's breadcrumb comes from the shared hook;
  the org-name crumb links to the chart from every page (list, detail,
  settings), and detail pages carry an org / Section / leaf trail with
  no standalone back button.
- §15 — bot content is an editable Monaco field saved via
  `update_bot` (`PUT …/bots/<id>`); the right-rail Project and Agent ids
  link into Helix; the Advanced accordion's "Restart agent session"
  re-activates the bot with a data-loss warning.
- §11 — fresh sandbox: Zed launches; per-Bot `gh`
  startupScript installs cleanly; `gh auth status` green.
- §17 — a processor of each kind creates with an auto-provisioned output
  Topic; the drawer's preview shows the raw `{{ .Message }}` JSON and the
  syntax help lists fields/functions/match rules; publishing to the input
  Topic transforms/routes the message onto the right output Topic(s).
  Wiring works in all three directions — Topic → IN, OUT → Bot (drop
  on the Bot's **side**, not top), OUT → processor (chain) — and data
  edges land on node **sides** while top/bottom stay for reporting.
  **Every edge deletes for real and persists:** the input edge
  disconnects the input; subscription edges unsubscribe. Duplicate name →
  409 clean (no raw driver error); an owned output Topic can't be deleted
  independently (409); deleting the processor cascades its owned outputs.
  Validation errors are 400 with a helpful message and the drawer stays
  open.
- §18 — with no `slack_app` configured the Slack surfaces degrade
  gracefully (install unavailable, events endpoint 503, no errors). With
  an app: a workspace install creates an org-scoped `slack_workspace` +
  its `s-slack-ws-*` Topic; an inbound message routes onto that Topic
  (bot self-echo dropped) and a Bot replies via a `slack`-minted token;
  workspace lists are org-isolated; deleting the global app cascades away
  its installs and their Topics.
- No console errors beyond the three Vite WS errors at startup.

## Known limitations

- A Bot is singular: one row, one prompt (its `content`), one tool list.
  There is no separate role/worker, no `kind`, and no separate identity
  record.
- A Bot's reporting lines are many-to-many (one `org_reporting_lines`
  row per manager–report pair). A Bot may report to several managers
  simultaneously; the graph is a cycle-guarded DAG, not a tree.
- Adding a reporting line is cycle-guarded server-side: dragging a
  manager edge that would close a reporting loop is rejected with a 409.
- Org isolation is enforced by composite `(id, org_id)` PKs in the store;
  every tenancy bug to date lived in a process-wide layer above it keyed
  by an id unique only within an org (`b-root`). §16 is the
  colliding-ID gate that exercises that layer. One layer it does NOT yet
  cover is safe: the per-Bot git repo id (`code-<botID>-<second>`)
  has a **global** PK, so two orgs creating the same bot id in the same
  second collide and the second org's desktop fails to provision (the
  org-graph rows still land correctly). See §16's "Known sharp edge".
