# Org agents: migrate the PoC desktop runtime onto the spec-task sandbox model

**Date:** 2026-09-07
**Status:** Implemented on branch `feature/org-agent-sandbox-runtime` (2026-09-07); see §7 for what was verified live
**Scope:** `api/pkg/org/**`, `api/pkg/server/helix_org*.go`, `api/pkg/external-agent/hydra_executor.go`,
`api/pkg/sandbox/controller_session.go`, `frontend/src/components/helix-org/**`, `frontend/src/pages/App.tsx`

## Decisions (2026-09-07)

- Default stays **full desktop**. Headless is opt-in per bot (or per org default).
- `restart-agent` keeps its current semantics (stop, delete session, fresh session).
- **One PR.** No billing/usage work; no new lifecycle verbs. There are no org-agent
  users yet, so compatibility means: existing bots keep working unchanged, and at most
  a restart is needed to pick up a changed sandbox config.
- The bot UI gets the full spec-task workspace (desktop, chat, diff, files, terminal,
  reverse-tunnel browser, share links, controls) — everything except task-only panels.

## 1. Why

Org agents (helix-org Bots) run inside a per-bot Helix project's *exploratory session*
whose container is started by the same `HydraExecutor.StartDesktop` path spec tasks
use — but with none of the knobs spec tasks have. Every bot gets an uncapped, full
GNOME `helix-ubuntu` desktop, billed at the standard 12 vCPU desktop rate, that nobody
can size, make headless, or find in the Sandboxes list. Status is a two-state boolean
derived from session metadata.

Spec tasks already solved this (`design/2026-08-14-headless-spec-tasks.md`,
`design/2026-08-10-spec-task-desktop-billing.md`). This doc maps the current bot
runtime, contrasts it with spec tasks, and proposes converging bots onto the same
model: the owning entity (the Bot) is the source of truth for sandbox runtime and size
on every launch path, and the `sandboxes` row is named and linked.

## 2. Current implementation (as of `main` @ 53f68a160)

### 2.1 Entities

| Entity | Table / type | Owns |
|---|---|---|
| Bot (graph node) | `org_bots` → `orgchart.Node` (`api/pkg/org/domain/orgchart/node.go:56`) | id, name, content, tools, project allowlist, `PreserveContext`, `AgentID` (link to App), parents via `org_reporting_lines` |
| Canonical agent | `types.App` (one Assistant) | system prompt, `CodeAgentRuntime`, credential type, provider, model, reasoning effort, MCP servers |
| Runtime sidecar | `org_bot_runtime_state` KV (`api/pkg/org/infrastructure/runtime/helix/state.go`) | `project_id`, `agent_app_id`, `repo_id`, `session_id`, `hiring_user_id`, `restart_required_container` |
| Per-bot project | `projects` row named `<Bot> @ <Org>` + one git repo (`project.go:184 Ensure`) | startup script, secrets, `DefaultHelixAppID` = the App |
| Bot session | `sessions` row, `session_role=exploratory`, `agent_type=zed_external`, `Metadata.OrgWorkerID`, `RuntimeInstructions`, `AutoRestartOnCrash=true` (`helix_org_inproc.go:948`) | chat history, Zed thread, container pointers (`ContainerID`, `DevContainerID`, `SandboxID`, `ExternalAgentStatus`) |
| Sandbox meter row | `sandboxes` row, session-backed (`SessionID` set, `TimeoutSeconds=-1`), created by `beginSandboxMetering` (`hydra_executor.go:703`) | billing window, quota slot, host/container ids |
| Activation audit | `org_activations` | trigger kind, started/ended, outcome |

There is **no bot-level sandbox configuration anywhere**. The only runtime axis a bot
has is the agent harness on its App.

### 2.2 How a bot is woken (event → activation)

```
Trigger (GitHub/GitLab/Slack/Postmark webhook, cron via streamcron, manual publish)
  └─ publishing.Publish            append org_events, wakebus.Notify (NATS wake for live UI)
       └─ dispatch.Route           org_worker_attachments for that source → one Trigger per bot
            └─ agentdelivery.Queue NATS JetStream stream HELIX_ORG_AGENT_ACTIVATIONS,
                                   one durable pull consumer per (org, bot), 1 outstanding msg
                 └─ Spawner        blocks for the whole activation; Ack on success,
                                   NakWithDelay(1s…30m backoff) on error; InProgress every 10m
```

Other entry points into the same queue: `DispatchHire` (bot created), `DispatchManual`
(REST `/activate`, MCP `start_bot`). Processors fan out from `Route` and publish back
through `Publish`, so chains recurse through the same path.

Key properties: per-bot FIFO, restart-safe (consumers resume on API boot,
`queue.go:59`), no coalescing (one trigger per activation), global inflight cap of 8
(`spawner.go:34`), `CancelOutstanding` drains the consumer on stop/restart.

### 2.3 The activation loop (`spawner.go:117`)

1. Acquire global semaphore slot; create/adopt `org_activations` row.
2. Resolve the hiring user and put their bearer on the context (all Helix resources are
   owned by the human who hired the bot).
3. `ensureProject` → `WorkerProject.Ensure`: upsert project + App + repo, sync the
   project's `CodeAgentExecutionConfig` from the App, sync runtime secrets, attach the
   helix-org MCP.
4. `Mirror.Ensure` — transcript mirror subscribes to the session's pubsub topic and
   republishes settled entries onto `s-transcript-<bot>`.
5. Build the mandate from the App's assistant system prompt, render `AGENTS.md` /
   `CLAUDE.md` instructions, `SyncAgentProfile` onto the existing session.
6. `ensureSession`:
   - existing session and `!PreserveContext` → `ClearSession` (wipes interactions and
     the Zed thread; every activation starts on a fresh context window);
   - `EnsureAndSend`: existing session → `POST /sessions/{id}/messages` (prompt queue;
     if the desktop is down `autoStartDevContainerForSession` boots it); no session →
     `checkDesktopQuota` then `StartExternalAgentSession` → `StartDesktop`.
7. `pollUntilDone`: poll `GetOutput` with backoff until the *new* interaction reaches a
   terminal state. Bounded only by the 24h runaway guard. Session-level watchdogs own
   "is it stuck": `auto_wake_stuck_interactions.go`, `AutoRestartOnCrash`, the dead
   container reconciler, the orphan reaper.
8. Return → queue Ack/Nak → `org_activations.Complete`.

Between activations the desktop stays up until the global idle checker stops it
(`HELIX_DESKTOP_IDLE_TIMEOUT`, default 1h, `idle_checker.go:16`), which sets
`external_agent_status=terminated_idle`. The next trigger cold-starts it again
(≈3 min for a GNOME desktop).

### 2.4 What the container actually is

`StartExternalAgentSession` (`session_handlers.go:3091`) builds a `DesktopAgent` with
`OrganizationID`, `SessionID`, `UserID`, `ProjectPath`, repos — and nothing else. In
`StartDesktop`:

- `resolveSpecTaskLaunchConfig` (`hydra_executor.go:679`) is a no-op because
  `SpecTaskID == ""` → `DesktopType` stays `""` → `parseContainerType` → `"ubuntu"` →
  full GNOME + streaming, privileged isolation, display-capable host required.
- `VCPUs/MemoryMB` are 0 → hydra runs the container **uncapped**;
  `desktopBillingResources` bills it at the standard preset (12 vCPU / 24 GB) at the
  *desktop* price (`hydra_executor.go:731`).
- The `sandboxes` row is named `Session <id>` (`desktopSandboxName`, `:750`), has no
  bot link, `Runtime=ubuntu-desktop`.
- The org-worker bootstrap (`applySessionBootstrap`, `:820`) injects
  `HELIX_WORKER_ID` and writes `AGENTS.md`/`CLAUDE.md` into the workspace.

Resume paths (`autoStartDevContainerForSession`, `resumeSession`, auto-wake cold start,
container reconciler) rebuild the `DesktopAgent` from session metadata. Since the
session carries no runtime/size either, every path lands on the same uncapped desktop.

### 2.5 Lifecycle HTTP API today

Mounted under `/api/v1/orgs/{org}/` by `api/pkg/org/interfaces/server/api/api.go:323`.
`/agents/*` is canonical; `/bots/*` is an alias with the legacy nested detail shape.

| Verb | Handler | What it does |
|---|---|---|
| `POST /agents/{id}/chat` | `ensureBotChat` | ensure project + App, return ids (no container) |
| `POST /agents/{id}/activate` → 202 | `Activations.Activate` | ensure project synchronously, pre-allocate activation row, enqueue `TriggerManual`. Boots the desktop *as a side effect* of the first prompt |
| `POST /agents/{id}/stop-agent` | `Activations.Stop` | `CancelOutstanding` + `StopExternalAgent` (container down, session kept) |
| `POST /agents/{id}/restart-agent` | `Activations.Restart` | cancel + stop + delete session row + clear pointer + `Activate` → brand-new session |
| `GET /agents`, `GET /agents/{id}` | `listBots`/`getBot` | `agent_status ∈ {running, stopped}` from `session.Metadata.ExternalAgentStatus == "running"` (`helix_org.go:179`), one `GetSession` per bot on list |
| `PATCH /agents/{id}` | `updateBot` | graph fields + App config (harness, credentials, provider, model, reasoning) in one call |

MCP: `start_bot` / `stop_bot` / `restart_bot` call the same `Activations` service.

### 2.6 UI today

- Live surfaces: org chart (`HelixOrgChart.tsx`), chat panel
  (`components/helix-org/HelixOrgChatPanel.tsx`), agent settings via the generic
  `App.tsx` (`org_agent` route). `HelixOrgBots.tsx` and `HelixOrgBotDetail.tsx` are
  orphaned — their routes redirect (`router.tsx:642-661`).
- Status: green/grey dot, `Running`/`Stopped`, polled through the bots list.
- Desktop tab: `ExternalAgentDesktopViewer` with `sandboxId={sessionId}` — the session
  id stands in for a sandbox identity.
- Runtime form (`BotRuntimeForm.tsx`): harness, credential source, model, reasoning
  effort. No environment, no size.
- `AgentRestartRequiredBanner` already exists, driven by `restart_required`
  (`RestartFingerprint`, `orgchart/restart.go`) and `restart_required_container`.

### 2.7 What spec tasks do instead

| Concern | Spec task | Org bot |
|---|---|---|
| Runtime choice | `SpecTask.SandboxRuntime` (`ubuntu-desktop` / `headless-ubuntu`), immutable after launch, project default, browser-stored personal default | none — always desktop |
| Size | `SpecTask.SandboxResourceOverrides`, preset ladder 1/4/8/12/16 vCPU, live resize, operator default | none — uncapped, billed as 12 vCPU |
| Source of truth on every launch path | `resolveSpecTaskLaunchConfig` reloads the task on start / resume / fork / reconcile | nothing; whatever `DesktopAgent` defaults give |
| Sandbox row | named after the task, `SpecTaskID` set, visible in Sandboxes list with a link back | `Session ses_…`, unlinked |
| Status | `SandboxState` absent/starting/running + `SandboxStatusIndicator` | running/stopped |
| Idle | `keep_alive` excludes the task from the idle checker | always idle-stopped after 1h |
| Restart | `restartSessionContainer` keeps the session and threads | deletes the session |
| Picker UI | `SpecTaskExecutionControls` (Compute + Environment), locked after start | harness only |

## 3. API: how the spec-task surface is shaped, and what the bot surface becomes

### 3.1 Spec-task API surface (for comparison)

Spec tasks spread their runtime control across three layers. Bots already sit on the
bottom two; only the top layer is missing.

**Task layer** (`/api/v1/spec-tasks/{id}…`, `server.go:1636-1682`)

| Route | Purpose |
|---|---|
| `POST /spec-tasks` (`CreateTaskRequest`) | `sandbox_runtime` and `sandbox_resource_overrides` are set **here only**; runtime is immutable afterwards |
| `GET /spec-tasks/{id}` | returns `sandbox_runtime`, `sandbox_resource_overrides`, `keep_alive`, derived `sandbox_state` (`absent/starting/running`) |
| `PUT /spec-tasks/{id}` (`SpecTaskUpdateRequest`) | `keep_alive` and other metadata; not runtime/size |
| `PATCH /spec-tasks/{id}/execution-config` | `code_agent_config` and/or `sandbox_resource_overrides`; resizes a **running** container via `UpdateDesktopResources` and reports `sandbox_resources_applied` |
| `POST /spec-tasks/{id}/start-planning` | starts the workflow (creates the planning session, boots the container) |
| `POST /spec-tasks/{id}/stop-agent` | `StopDesktop` on the planning session; session kept |
| `GET/DELETE /spec-tasks/{id}/zed-instance` | legacy status/shutdown of the Zed instance |

**Session layer** (`/api/v1/sessions/{id}…`, `server.go:1080-1099`) — shared by spec
tasks, human desktops and bots already:
`POST /resume` (rebuilds `DesktopAgent` from session metadata), `DELETE
/stop-external-agent`, `POST /restart-agent` (`restartSessionContainer`: recreate the
container, keep session + thread), `POST /messages`, `/cancel`, `/clear`,
`/switch-agent`, `GET /sandbox-state` (thin: `absent|running` from `Session.SandboxID`).

**Sandbox layer** (`/api/v1/organizations/{org}/sandboxes/{id}`, `/billing`) — the
metering row. Spec-task rows show up here named after the task with `spec_task_id`.

The pattern: **the owning entity holds the config; the executor reloads it from the
owning entity on every launch; the session and sandbox layers are generic.**

### 3.2 Bot API after this change

Same pattern, same field names, mounted on the existing org-graph router. No new verbs.

| Route | Change |
|---|---|
| `PATCH /orgs/{org}/agents/{id}` | accepts `sandbox_runtime` (`""`, `ubuntu-desktop`, `headless-ubuntu`) and `sandbox_resource_overrides` (`{vcpus, memory_mb}`, must be a preset). Validation reuses `ValidSpecTaskSandboxRuntime` / `ValidPreset`, and the same "no display-capable host" check spec-task create uses (`spec_driven_task_handlers.go:188`). Stored on `org_bots`. Applied at the **next container start**; if a container is running, `restart_required` is stamped (both fields join `RestartFingerprint`) |
| `POST /orgs/{org}/agents` | same two optional fields at create |
| `GET /orgs/{org}/agents`, `GET …/{id}` | add `sandbox_runtime`, `sandbox_resource_overrides` (effective values, defaults resolved), `sandbox_id`, `sandbox_status` (`pending/running/stopping/stopped/failed` from the `sandboxes` row, `status_message` on failure). `agent_status` stays `running/stopped` so nothing in the UI breaks |
| `POST …/activate`, `…/stop-agent`, `…/restart-agent`, `…/chat` | **unchanged**. `restart-agent` is the way to apply a changed runtime |
| MCP `start_bot` / `stop_bot` / `restart_bot` | unchanged; `create_bot` / `set_bot_*` may take the two fields (optional, cheap) |
| CLI `helix org agents create/update` | `--sandbox-runtime`, `--sandbox-vcpus` |

Mapping onto spec-task verbs:

| Bot | Spec task equivalent | Difference |
|---|---|---|
| `PATCH agents/{id}` with runtime | `POST /spec-tasks` only | bots are long-lived, so runtime is **mutable**, applied on restart. Spec tasks lock it because the workspace/branch state is tied to one container |
| `PATCH agents/{id}` with size | `PATCH /execution-config` | spec tasks resize live; bots apply on restart in this PR. Live resize is a follow-up: call `UpdateDesktopResources` when a container is running, exactly as the execution-config handler does |
| `activate` | `start-planning` + first message | same: boots the container as a side effect of the first prompt |
| `stop-agent` | `stop-agent` | identical semantics (`StopDesktop`, session kept) |
| `restart-agent` | none at task level; `POST /sessions/{id}/restart-agent` at session level | bot restart deletes the session. Kept as-is per decision |
| `GET agents/{id}.sandbox_status` | `GET spec-tasks/{id}.sandbox_state` | bot uses the `sandboxes` row (5 states); task derives 3 states from session config. Either is fine; the row is already there and cheaper on list |

What is deliberately **not** added: a `/sandbox` sub-resource, `keep_alive`, a
session-preserving restart, billing endpoints. All three layers remain reachable for a
bot through the session and sandbox APIs already (`session_id` and `sandbox_id` are
on the DTO).

### 3.3 Org-level defaults

`worker.sandbox_runtime` and `worker.sandbox_vcpus` in the config registry next to the
default agent config, resolved in `resolveWorkerAgentConfig` (`helix_org.go:1614`).
Resolution order: bot → org default → `EffectiveSpecTaskSandboxRuntime` /
`DefaultSpecTaskSandboxResources` (the same helpers spec tasks use; no second ladder).
Global default remains `ubuntu-desktop`.

## 4. Implementation (single PR)

### 4.1 Data

- `org_bots`: `sandbox_runtime varchar(64)`, `sandbox_resource_overrides jsonb`
  (nullable = inherit). `orgchart.Node` gains the two fields; `RestartFingerprint`
  includes them.
- `sandboxes.org_bot_id` (indexed), populated by `BeginSession` via a new
  `BeginSandboxSessionRequest.OrgBotID`. Row name becomes `<Bot name> @ <Org>` in
  `desktopSandboxName` when `Metadata.OrgWorkerID` is set. One-off backfill in the
  migration: join `sessions.config->>'org_worker_id'` for existing session-backed rows.
- `types.SessionMetadata`: `SandboxRuntime`, `SandboxResourceOverrides`. Set only on
  sessions with `OrgWorkerID`; spec-task sessions leave them empty (the task is
  authoritative there, same rule as `CodeAgentConfig`).
- Org config keys `worker.sandbox_runtime`, `worker.sandbox_vcpus`.

### 4.2 Launch path

- Spawner: on every activation, write the bot's effective runtime/size to the session —
  through `StartSessionParams` for a fresh session and through `SyncAgentProfile` for an
  existing one (it already updates session-scoped worker state before each turn).
- Executor: rename `resolveSpecTaskLaunchConfig` → `resolveLaunchConfig`. Branch:
  `SpecTaskID != ""` → task (unchanged); else `Metadata.OrgWorkerID != ""` → session
  metadata (`headless-ubuntu` → `DesktopType="headless"`; preset → `VCPUs/MemoryMB`).
  Every resume path already goes through `StartDesktop`, so
  `autoStartDevContainerForSession`, `resumeSession`, auto-wake cold start and the
  reconciler inherit it. This is the invariant from the headless spec-task doc: "the
  owning entity is the source of truth on every launch path".
- `EnsureAndSend.checkDesktopQuota` (`sessions.go:79`) must skip when the bot is
  headless; `beginSandboxMetering` already enforces the headless cap by pricing type.
- Status: `orgWorkerRuntime.State` reads the bot's `sandboxes` row (`org_bot_id`) for
  `sandbox_id` / `sandbox_status`; list does one `ListSandboxes` per org instead of a
  `GetSession` per bot.

### 4.3 UI: the bot gets the spec-task workspace

Goal: a bot session gets every workspace surface a spec task has — desktop stream, chat,
changes (diff), files, terminal, reverse-tunnel browser, share-preview links, execution
controls, sandbox status, start/stop/restart — minus the things a bot does not have
(specs, design-doc review, approval, PR, attachments, clone groups, thread selector).

**Why this is mostly plumbing.** `SpecTaskDetailContent.tsx` is task-bound only at the
top: it resolves one `activeSessionId` (`:661`) and hands that to every workspace
panel. Each panel calls session-keyed endpoints and none of the handlers consult the
spec task:

| Panel | Component | Endpoint family | Task dependency |
|---|---|---|---|
| Desktop stream / screenshots / drop-zone | `external-agent/ExternalAgentDesktopViewer.tsx` | `/external-agents/{sessionId}/ws/stream`, `/ws/input`, `/screenshot`, `/upload`; `/sessions/{id}/resume`, `/stop-external-agent`, `/cancel` | none |
| Chat | `session/AgentChat.tsx` | `/sessions/{id}/interactions`, `/messages`, `/cancel` | `specTaskId`/`projectId` optional |
| Changes + Files | `tasks/DiffViewer.tsx` → `workspace-inspector/WorkspaceInspector.tsx` | `/external-agents/{sessionId}/workspaces`, `/workspace-review`, `/workspace-files`, `/workspace-file` (`workspace_review_handlers.go`) | none; `baseBranch` is a prop |
| Terminal drawer | `tasks/SpecTaskTerminalDrawer.tsx` → `session/SessionTerminal.tsx` | WS `/sessions/{id}/terminal`, `/terminal-sessions` (`session_terminal_handlers.go`, hydra exec) | none |
| Browser (reverse tunnel) | `tasks/SandboxBrowser.tsx` | `/sessions/{id}/preview-tokens` (`session_preview_handlers.go`), `vhost_routes` → RevDial → hydra dev-container proxy | none; needs `DEV_SUBDOMAIN` |
| Share preview URLs | `tasks/SharePreviewSection.tsx` | same preview-token hooks + rotate/delete | none |
| Sandbox status dot | `tasks/SandboxStatusIndicator.tsx` | pure prop | none |
| Execution controls | `agent/CodeAgentExecutionControls.tsx` | pure value/onChange | none; persistence is the caller's |
| View toolbar | `tasks/SpecTaskViewToolbar.tsx` | pure props: `TaskView` (`chat\|desktop\|browser\|changes\|files\|details`), `hasSession`, `showDesktop`, start/stop/restart, terminal toggle | none |

Session-backed `sandboxes` rows route hydra ops by the **session id**
(`Sandbox.HydraOpsID()`), so a bot session already drives the whole
`/external-agents/{sessionId}/*` surface with no spec task involved.

**What to build.** Extend `components/helix-org/OrgAgentSessionWorkspace.tsx` (today:
chat + desktop split only, used from `pages/Session.tsx:1607` for sessions with
`org_worker_id`) into the bot equivalent of `SpecTaskDetailContent`:

- `SpecTaskViewToolbar` with the same `TaskView` set. `hasSession` = bot has a
  `session_id`; `showDesktop` = effective runtime is `ubuntu-desktop`; start / stop /
  restart wired to `useActivateBot` / `useStopBotAgent` / `useRestartBotAgent`;
  terminal toggle opens `SpecTaskTerminalDrawer` on the bot session. `showKeepAlive`
  stays off (deferred).
- Views: `chat` → `AgentChat` (no task id); `desktop` → `ExternalAgentDesktopViewer`
  (already there); `browser` → `SandboxBrowser`; `changes` / `files` → `DiffViewer`
  with `baseBranch` = the bot repo's default branch; `details` → a bot details pane
  (below).
- Details pane (replaces the task's Details view): sandbox runtime + preset picker
  (`CodeAgentExecutionControls` Compute + Environment, never locked, "takes effect on
  restart"), harness/model (`BotRuntimeForm`), sandbox status / id / host, project and
  repo links, `SharePreviewSection`, `AgentRestartRequiredBanner`, and the activation
  history from `org_activations` (the transcript already exists as
  `s-transcript-<bot>`). Saving the picker calls `PATCH /orgs/{org}/agents/{id}`.
- Mount points: `pages/Session.tsx` (org chat → bot row → session) keeps using this
  component; the `helix_org_bot_detail` route mounts the same workspace so the
  orphaned `HelixOrgBotDetail.tsx` / `HelixOrgBots.tsx` can go. `HelixOrgChatPanel`
  keeps its bot picker and lifecycle menu but delegates the body to the workspace.
- `SandboxStatusIndicator` replaces the local green/grey dot on the chart and chat
  panel; it maps `sandbox_status` → `running/starting/stopped`.
- Org settings: the two sandbox defaults next to the default agent config.
- Sandboxes list: link column to the org agent via `org_bot_id`, mirroring the
  spec-task link.

**Headless bots.** Desktop tab hidden (same `isHeadless` routing as
`SpecTaskDetailContent.tsx:767`). Files/diff work through the workspace-only desktop
bridge, the terminal through hydra exec, and the browser preview through the hydra
dev-container proxy — none need a compositor. Screenshots do not exist.

**One backend nit.** `GET /external-agents/{sessionId}/workspaces` fills
`ExpectedBranch` only from the spec task (`session_workspace_handlers.go:339`); without
one the switch-agent safety net reports "can't save changes". For bot sessions resolve
it from the bot repo's default branch (the session has `OrgWorkerID`), or the
switch-agent dialog on a bot will always refuse to auto-commit.

### 4.4 Compatibility

Existing bots have NULL config → resolve to org default → global default
`ubuntu-desktop` + standard preset. Behaviour is identical to today except the
container is now capped at the preset (12 vCPU / 24 GB) instead of uncapped; that
matches what it is already billed as. Sessions created before the change carry no
metadata until their next activation stamps it, and a restart is the only thing needed
to pick up a changed runtime.

## 5. What must be validated

Live, in the inner Helix at `localhost:8080`, against a connected Zed. Seeded rows do
not exercise the resume paths. Per `CLAUDE.md`, test the operation *after* each state
change, not just the state change.

**Launch config on every path**
1. Create a bot with `headless-ubuntu` / 4 vCPU → first activation: container has
   `HELIX_HEADLESS=1`, rootless isolation, `sandboxes` row `runtime=headless-ubuntu`,
   `vcpus=4`, `org_bot_id` set, name `<Bot> @ <Org>`.
2. Let the idle checker stop it (set `HELIX_DESKTOP_IDLE_TIMEOUT` low) → fire a cron
   trigger → the auto-start path boots it headless at 4 vCPU again. Repeat for
   `POST /sessions/{id}/resume`, the auto-wake cold-start retry, and the container
   reconciler after a `docker kill`.
3. Same bot, switch to `ubuntu-desktop` / 12 vCPU → `restart_required=true`, running
   container untouched → `restart-agent` → new container is a desktop at 12 vCPU, and
   the next trigger produces a reply.
4. Desktop bot on a host with `RenderNode=SOFTWARE` → PATCH to desktop rejected;
   headless placement succeeds on that host.
5. Pre-existing bot with NULL config → activation still boots a desktop; it is now
   capped at the standard preset; org default set to headless → *new* bots inherit,
   existing bots do not.

**Wake loop unchanged**
6. Publish 5 events in a burst → 5 sequential activations, FIFO, one `org_activations`
   row each; `stop-agent` mid-burst drains the consumer and nothing fires after.
7. `PreserveContext=false` → each activation clears the thread; `true` → thread persists
   across an idle stop + wake.
8. Kill the API mid-activation → consumer resumes on boot and redelivers.

**Status and API**
9. `sandbox_status` walks `pending → running → stopped` from the chart and chat panel;
   a failed boot (bad image tag) yields `failed` with the hydra reason in
   `status_message`; the queue Naks with backoff.
10. `activate` / `stop-agent` / `restart-agent` behave exactly as before for a bot that
    never had sandbox config set.
11. MCP `start_bot` / `stop_bot` / `restart_bot` and the CLI produce the same
    transitions; `create_bot` with `sandbox_runtime` lands on the row.
12. Cross-org: bot ids collide across orgs (`b-root`); the Sandboxes list and the bot
    DTO never leak another org's row.

**Workspace UI (bot session, desktop and headless variants)**
13. Chat, Changes, Files, Browser and the terminal drawer all work on a running bot
    session from the org chat and from the bot detail route; Files shows the bot repo,
    Changes diffs against its default branch after the bot commits something.
14. Browser: navigate to a port the bot is serving → a `share-<random>` preview token
    is minted, the iframe loads, the token is listed and revocable under Details.
15. Headless bot: Desktop tab absent, everything in 13–14 still works; desktop bot:
    Desktop tab streams.
16. Toolbar start/stop/restart change `sandbox_status` and the status dot on the chart
    and chat panel within one poll; the restart-required banner appears after changing
    the runtime in Details and disappears after restart.
17. Switch-agent from a bot session with uncommitted changes offers to commit to the
    bot repo branch rather than refusing (the `ExpectedBranch` fix).
18. Mobile width: the toolbar folds like the task page and each view is reachable.

**Regression**
19. A headless bot can still `create_spectask`, `send_spectask_agent_message`, read
    files via the desktop bridge's workspace-only API, and use `sandbox_ssh_access`;
    the org chat Desktop tab is hidden for it and present for a desktop bot.
20. Org delete tears down bot sandboxes and closes their rows. The org-delete sweep
    (`0087ceaeb`) walks the org's external-agent sessions, which includes bot sessions,
    but confirm the session-backed `sandboxes` row is closed too, not just the container.
21. Migration backfill on a Postgres copy with: bots with and without sessions,
    sessions whose sandbox row is already closed, two orgs sharing a bot id.
22. Go: `resolveLaunchConfig` table test covering task / org-worker / plain session on
    each caller; `BeginSession` reuse with `OrgBotID`; status derivation;
    `go test ./pkg/server/ ./pkg/sandbox/ ./pkg/external-agent/ ./pkg/org/...`.
    Frontend: `yarn build`. E2E (`run_docker_e2e.sh`) if the executor change touches
    the WS sync path.

## 6. Deferred (not in this PR)

- Billing block on the bot page, `org_bot_id` in `OrgComputeUsage`, price hints.
- `keep_alive` on bots (idle-checker exclusion).
- Session-preserving restart (`restartSessionContainer`) and a `reset` verb.
- Live resize via `UpdateDesktopResources` on PATCH while running.
- A `/agents/{id}/sandbox` sub-resource; removing the `/bots/*` alias.
- Task-only surfaces that have no bot equivalent: specs / design-doc review, approval
  and PR actions, attachments, clone groups, the Zed thread selector (a bot has one
  session). Multi-session bots would need the thread selector back.

## 7. Implementation notes and verification (2026-09-07)

Implemented as designed in §3–4, with these deviations discovered while building:

- **Sandbox row naming.** `StartExternalAgentSession` named a fresh session from its
  first prompt and the in-proc client renamed it only *after* `StartDesktop`, so the
  sandbox row (named at `BeginSession`) carried the prompt text. `SessionChatRequest`
  now takes an internal `SessionName`, applied before the desktop starts.
- **Org nodes repo update map.** `nodesRepo.Update` writes an explicit column map, so
  new Node fields silently don't persist unless added there. The gorm org tests run on
  the in-memory store and would not have caught it; the live PATCH did.
- **GORM naming.** `SandboxVCPUs` needs an explicit `column:sandbox_vcpus` tag; GORM's
  default produced `sandbox_v_cpus`.
- **Display-host check on PATCH** is not enforced (the org API adapter has no host
  store); a desktop bot on a display-less host fails at start with hydra's placement
  error in `sandbox_status_message` instead.

Verified live in the dev stack (`unmanned-org`, bot `b-mira`):

| Check | Result |
|---|---|
| `PATCH` invalid runtime / off-ladder vCPU | 400 with the ladder in the message |
| `PATCH` headless / 4 vCPU → `GET` | stored + effective values; `sandbox_resource_overrides` reset to inherit with `vcpus: 0` |
| activate stopped bot | container `headless-external-…`, `HELIX_HEADLESS=1`, `HELIX_WORKER_ID=b-mira`, 4 vCPU / 8 GB, rootless; session metadata carries runtime + resources; sandbox row `Software Engineer`, `headless-ubuntu`, `org_bot_id=b-mira`; agent turn completed |
| stop-agent → activate (resume path) | same sandbox row reused, container headless at 4 vCPU again |
| switch to desktop / 8 vCPU while running | `restart_required=true`; container untouched |
| restart-agent | new session, `ubuntu-external-…` container, 8 vCPU / 16 GB, privileged; `restart_required` cleared; sandbox row `ubuntu-desktop` |
| restart-agent after the naming fix | new sandbox row named after the bot |
| org chat → bot session | toolbar Desktop / Browser / Diff / Files / Details, Terminal, Stop, Restart; desktop streams; Diff and Files load the workspace; Details shows sandbox status, environment, compute, sandbox/session/project links, share-preview URLs; terminal drawer opens |
| agent settings | Environment + Compute picker with "Org default (currently …)" and Save sandbox |
| org settings | Default sandbox panel (Helix default hints) |
| Sandboxes list | rows show "Org agent" source linking to the bot |
| Go | `go test ./pkg/org/... ./pkg/sandbox/ ./pkg/external-agent/` and the org/inproc/workspace server tests pass |
| Frontend | `tsc` clean, `yarn build` passes, `vitest` for helix-org / sandboxes / app folders passes |

| chat parity | the bot session page now renders `AgentChat` (the spec-task chat) instead of the legacy Session composer: sandbox file/image attachments via upload, prompt queue, plan progress, cancel, execution controls |
| native SSH (`sandbox_ssh_access` → proxy on :2224) | fixed: `helix/sandboxes.go` opened the hydra terminal with the row id, which 404s for every session-backed row; now `HydraOpsID()`. Verified with a minted user cert: `ssh sandbox@localhost -p 2224` lands in the headless bot container (`HELIX_WORKER_ID=b-mira`, `HELIX_HEADLESS=1`) |
| Sandboxes UI terminal on the bot's row | Terminal tab on `/orgs/…/sandboxes/<id>` opens a shell in the bot container |

NOT tested: headless placement on a display-less host, quota caps, org delete sweep,
the migration backfill on a production copy, mobile layout, and native SSH against a
desktop bot (same docker-exec path; only the id routing differed).

## 8. Follow-up fixes (2026-09-08)

- **Agent-side SSH was unreachable.** `sandbox_ssh_access` advertised the proxy as
  `api:2224` (inferred from `SANDBOX_API_URL`), a name that does not resolve on the
  isolated sandbox bridge, where only `helix-api.internal` (the Hydra gateway) is
  routable and only :18080 was forwarded. Hydra now mirrors the control plane's SSH
  proxy on the gateway (`hydra.SandboxSSHProxyListenAddress`, a raw TCP pipe; the
  control plane still authenticates every connection), the sandbox network policy
  admits gateway:2224, and `ASSET_SSH_PROXY_ADDRESS` defaults to
  `helix-api.internal:2224`. Verified from inside a desktop bot container with a
  minted cert: `ssh sandbox@helix-api.internal -p 2224` lands in the bot's own
  container. Existing sandboxes need the new hydra binary and a network-policy
  re-run (or a sandbox restart) to pick up the rule.
- **Files / Diff stayed on the start placeholder after the sandbox came up.** The
  session page fetched the bot once; the workspace gates those surfaces on
  `agent_status`, so it never noticed the start. The bot is now polled every 5 s
  while the page is open.
- **Diff base branch.** The workspace inspector sent `base=main` for every session.
  Org agents' repos are not necessarily on main (keel is on master); `base` is now
  omitted unless the caller knows a task branch, so the desktop resolves the
  repository's own default.
- **Prompt queue parity.** The org session queue now uses the same attached-header
  chrome and rows as the spec-task composer queue, and hides prompts already handed
  to the agent (`sending`) like the task queue does.
- The trailing status dot on the view toolbar is gone; status lives on the Details
  pane, which was redesigned as a stat strip plus copyable ids.
