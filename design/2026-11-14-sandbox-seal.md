# Per-task sandbox seal (#3622)

Issue: https://github.com/helixml/helix/issues/3622. Builds on https://github.com/helixml/helix/pull/3611
(`confine_agent_key`) and https://github.com/helixml/helix/pull/3613 (turn provenance).

Status: design only, nothing implemented. Code references are to `origin/main` at `d3e922636`.
The files cited in §1f are byte-identical at `66c7cdd`, so their line numbers hold there too.
Paths are relative to `api/pkg/server/` unless they say otherwise. A † marks a reference that came
from a delegated code search and was not re-read line by line.

Revision: round 7 (cyber review). The seal is now **default-deny**. The changes were:

- a central guard, plus a route-policy test that fails on any unclassified route;
- the routes missed at `66c7cdd`, listed in §1f;
- an `outcome` field on access records;
- the sealing key K, and the control/twin probe (§3.7);
- access-record scope now includes task-level routes.

The break-glass option section (§4) is unchanged; the owner decision on it is still open.

## Goal

PTaaS verifier and finder lanes need a sandbox that nobody can act on out of band. A sealed task
is **default-deny**: every route that resolves its id, its sessions' ids or its sandbox row refuses
every caller, except an explicit allow-list.

| allowed caller | allowed routes |
|---|---|
| **The sealing key K** (§3.7): the credential that created the task | stop, archive (the delete precondition), delete; reading the seal state and the access record |
| **The sandbox's own identity**: its session key, and a per-runner token bound to its host | its agent channel (sync, RevDial, config, credentials, daemon callbacks) and its own drive operations |
| **Platform internals**: in-process callers with no HTTP route (orchestrator, pollers, hydra, reapers, credit exhaustion) | everything they do today |

Everyone else is refused on every such route. That includes org owners, project members, platform
admins acting through the API, and K on any route not in its allow-list. A route added later is
refused until someone classifies it (§3.3).

**What the seal is not.** It does not protect against the platform operator. Anyone with database,
host, hydra or `docker exec` access on the sandbox host can always reach the container. The seal
protects against tenant-side actors and against admins acting through the Helix API. The portal and
the verifier must not claim more than that.

---

## 1. Inventory

### Who passes `authorizeUserToSession` today

`authorizeUserToSession` is at `authz.go:589`. "S(U) set" below means every caller it admits:

1. **The session owner, with any of their credentials.** That includes a browser session, their
   personal key, a key an admin mints for them (`/admin/users/{id}/api-keys`†), and the session's
   own key. The session key is minted with `Owner = session.Owner`
   (`services/spec_driven_task_service.go:2076`), so it authenticates *as* the creator.
2. **Any org owner** (`authz.go:618`).
3. **Any platform admin.** `AuthorizeOrgMember` returns a synthetic owner membership for
   `user.Admin` (`store/orgstore/authz.go:101-106`).
4. **Project members with the requested action.** When the project has `OrgMembersAccess`,
   `ActionUpdate` is downgraded to project `ActionGet` (`authz.go:631-633`), so **every org member**
   passes.
5. **The unconfined session key of any sandbox in the same project.** `enforceKeySessionScope`
   (`authz.go:204-231`) admits it, so sibling task sandboxes pass. A task-confined key reaches only
   its own task (#3611).

S(G) is the same set with `ActionGet`.

### Seal class (last column of every table)

Default-deny applies: anything not marked `SELF` or `K: …` is refused to every non-platform caller
of a sealed task.

| class | meaning on a sealed task |
|---|---|
| **SELF** | Only the sandbox's own identity may call it (its session key, or a per-runner token bound to its host). Everyone else, K included, is refused. |
| **K: stop / archive / delete / read seal** | Allowed to the sealing key K, and to SELF. Everyone else is refused. |
| **DENY** | Refused to everyone except platform internals. "(candidate: …)" records what an owner could later choose to add to the allow-list (a read view, a provenance-recorded turn). Nothing is added by default. |
| **freeze** | Not a route on the sealed task: a project- or app-level edit whose effect is held back from the sealed sandbox (§3.4). |
| **n/a** | Cannot reach the sandbox. |

Every refusal and every allowed call on a sealed task writes an access record (§4).

### 1a. Desktop, container and agent channel (`/external-agents/*`, `/sessions/*`)

| route / path | handler file:line | what it can do to the sandbox or agent | authz today | who passes today | creates interaction / provenance? | seal |
|---|---|---|---|---|---|---|
| `POST /external-agents/{sid}/command` | `external_agent_handlers.go:218` | Sends any `ExternalAgentCommand` verbatim to Zed: chat_message, open_thread, cancel, respond_question, …. A chat_message that reuses an in-flight `request_id` matches a pre-created interaction (`websocket_external_agent_sync.go:1640-1653`), so no new interaction is created. Can also auto-start the container (`websocket_external_agent_sync.go:2491`). | S(U) `:240` | S(U) set | **n** | SELF (better: delete the route, see §5) |
| `POST /external-agents/{sid}/exec` | `external_agent_handlers.go:403` | Arbitrary command in the desktop through RevDial `/exec`. | S(U) `:427` | S(U) set | n | SELF |
| `POST /external-agents/{sid}/upload` | `external_agent_handlers.go:836` | Writes files into the container. | S(U) `:855` | S(U) set | n | SELF |
| `GET /external-agents/{sid}/file` | `external_agent_handlers.go:1014` | Reads uploaded chat attachments. | S(G) `:1027` | S(G) set | n | DENY (candidate: read) |
| `GET` / `POST /external-agents/{sid}/clipboard` | `external_agent_handlers.go:603` / `:714` | Reads / writes the desktop clipboard. Pasting from it is input. | S(G) `:622` / S(U) `:733` | S(G) / S(U) set | n | DENY (GET candidate: read; POST SELF) |
| `GET /external-agents/{sid}/ws/input` | `external_agent_handlers.go:1153` | Keyboard and mouse input. Typing into Zed's agent panel creates a turn stamped `external_agent`, with no actor (`websocket_external_agent_sync.go:1101`, `:1675`). | `authorizeDesktopID(Update)` `:46` | S(U) set | turn y, actor **n** | SELF |
| `GET /external-agents/{sid}/ws/stream` | `external_agent_handlers.go:1332` | Video stream, **and input**. Input is dropped only when `X-Helix-Readonly` is set, i.e. for a non-interactive embed key (`desktop_stream_readonly.go:45`, `api/pkg/desktop/ws_stream.go:1893`, `:2128`). | `authorizeDesktopID(Get)` | S(G) set, all of whom can **drive** | n | DENY (candidate: read-only stream, §6 bug 1) |
| `GET /external-agents/{sid}/screenshot` | `external_agent_handlers.go:268` | Screenshot. | `authorizeDesktopID(Get)` | S(G) set | n | DENY (candidate: read) |
| `POST /external-agents/{sid}/configure-pending-session` | `external_agent_handlers.go:1109` | No-op. | user only | anyone | n | n/a (delete it) |
| `GET /external-agents/{sid}/workspace-review`, `/workspace-review/turn/{iid}`, `/workspace-files`, `/workspace-file`, `/workspace-file/download`, `/workspace-skills`, `/workspaces` | `workspace_review_handlers.go:65`, `:253`, `:93`, `:112`, `:130`, `:219`; `external_agent_handlers.go:1600` | Reads the workspace through the desktop's HTTP API. | S(G) (`authorizeWorkspaceReviewRequest` `:17`) | S(G) set | n | DENY (candidate: read) |
| `PUT /external-agents/{sid}/workspace-file` | `workspace_review_handlers.go:186` | Writes a file into the agent's working tree. | S(U) | S(U) set | n | SELF |
| `GET /sessions/{id}/terminal` (WebSocket) | `session_terminal_handlers.go:28` | Interactive tmux shell in the container, through hydra RevDial. | S(U) `:88` | S(U) set | n | SELF |
| `GET /sessions/{id}/terminal/sessions`, `DELETE …/{ts}` | `session_terminal_handlers.go:46`, `:64` | Lists / kills tmux sessions. | S(G) / S(U) | S(G) / S(U) set | n | GET DENY (candidate: read); DELETE SELF |
| `GET /mcp/desktop?session_id=` | `mcp_backend_desktop.go:57-90`†, registered `server.go:704` | Desktop MCP: `type_text`, `mouse_click`, `set_clipboard`, window control, and `save_screenshot` to **any path** (`api/pkg/desktop/mcp_server.go:66`). | S(U)† | S(U) set | n | SELF |
| `GET /sessions/{id}/zed-config` | `zed_config_handlers.go:37` | Returns the full agent config. **It embeds the session API key** (`:139`, used in MCP headers `:164`). | `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54`): bound runner, or `session.Owner == user.ID` plus S(G) | per-runner token for the host; the **owner with any credential** | n | SELF (closes the key leak) |
| `POST /sessions/{id}/zed-config/user` | `zed_config_handlers.go:426` | Saves overrides. Inert inside the sandbox (§2). | same | same | n | SELF |
| `GET /sessions/{id}/zed-settings` | `zed_config_handlers.go:484` | Merged view. Also builds config with the session key (`:517`). | same, runner refused | owner | n | SELF |
| `GET` / `PUT /sessions/{id}/claude-credentials`, `/codex-credentials` | `claude_subscription_handlers.go:768`, `:851`; `codex_subscription_handlers.go:307`†, `:346` | Read / replace the decrypted agent subscription. A PUT reaches the sandbox on the next 30s poll. | inline: runner bound to the host, the owner in key scope, or the session key; then `canUseSessionAgentCredential` | owner (only for their own subscription), session key, bound runner | n | SELF |
| `GET /external-agents/sync` (agent WebSocket) | `external_agent_thread_routes.go:97` | **The agent channel itself.** `registerConnection` is last-writer-wins (`websocket_external_agent_sync.go:2703`), so a caller displaces the real Zed, receives every command, and can inject agent events. `validateExternalAgentToken` is a stub returning true (`:4981`). | runner token (shared **or** per-host, **any** session), `user.Admin`, or `ownsSessionInKeyScope` | runner-token holders, **all admins**, the **owner with any credential**, same-owner sibling project keys | n | SELF |
| `GET /revdial` (`desktop-{sid}`) | `server.go:2767`; check at `revdial_registration.go:32` | Registers the tunnel the API uses for *every* desktop operation. `connman.Set` is last-wins (`server.go:~2958`), so a caller intercepts exec, upload, file writes, input, stream and MCP traffic. | `revDialKeyMayRegisterDesktop` + `session.Owner == user.ID` | the **owner's personal key**, project keys of the owner | n | SELF (session key only) |
| `GET /ws/user?session_id=` | `websocket_server_user.go:91`† | Outbound events only; incoming frames are discarded (`:203-212`†). | S(G) | S(G) set | n | SELF (the daemon's event feed); others DENY |
| `GET /sessions/{id}/rdp-connection`, `/sandbox-state`, `/workspace-status`, `/step-info`, `/output`, `/usage`, `/toc`, `/search`, `/turns/{n}`, `/interactions*`, `GET /sessions/{id}` | `session_handlers.go:2281`, …; `agent_sandboxes_handlers.go:446` | Read-only. | S(G) | S(G) set | n | DENY (candidate: read) |
| `GET /sessions/{id}/operator-access` | `session_operator_access.go:10` | Probe only (returns 204). | S(U) + project U | — | n | DENY |

### 1b. Session lifecycle and the turn path

| route | handler | effect | authz | who passes | provenance? | seal |
|---|---|---|---|---|---|---|
| `POST /sessions/{id}/messages` | `session_handlers.go:2717` | Turn through the prompt queue (`enqueueAgentMessage`). | S(U) | S(U) set | **y** (`prompt_history_handlers.go:589`) | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /sessions/chat` (existing session) | `session_handlers.go:644` (S(U) at `:894`) | Turn through `RunExternalAgent`, **bypassing the queue**. | S(U) | S(U) set | y (`:1416`, `:1470`†) | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /prompt-history/sync` | `prompt_history_handlers.go:71` | Durable prompt queue. Rows are delivered and the desktop is woken. | `authorizeUserToPromptQueue` — **ActionGet** for the scope's own session (`:436`, `:446`, `:471`). Fix in progress in a separate PR. | S(G) set | y | DENY (candidate: provenance-recorded turn; owner decision) |
| `POST /interactions/{iid}/questions/{rid}/respond` | `interaction_question_handlers.go:31` | Sends **caller-chosen answers** to the agent. | S(U) `:127` | S(U) set | **n** (`ResolvedQuestion` has no actor, `types/agent_question.go:29`) | DENY |
| `POST /interactions/{iid}/questions/{rid}/cancel` | `interaction_question_handlers.go:76` | Fixed-form `cancel_question`; no caller content. | S(U) | S(U) set | n | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/cancel` | `session_handlers.go:2600` → `prompt_history_handlers.go:702` | Fixed-form cancel of the current turn. | S(U) | S(U) set | n | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/clear` | `session_clear.go:185` | Cancels and closes the Zed thread, losing agent context. | S(U) | S(U) set | n | DENY |
| `POST /sessions/{id}/restart-agent` | `session_handlers.go:2873` | Restarts the container; may reset the thread. | S(U) | S(U) set | n | DENY |
| `POST /sessions/{id}/resume`, `/ensure-agent` | `session_handlers.go:2363`, `:3619` | Starts the container. | S(U) | S(U) set | n | DENY (platform internals may still start it) |
| `DELETE /sessions/{id}/stop-external-agent` | `session_handlers.go:2636` | `StopDesktop`. | S(U) | S(U) set | n | **K: stop** |
| `DELETE /sessions/{id}` | `session_handlers.go:448` | Destroys the desktop. | S(Delete) | — | n | **K: delete** |
| `PATCH /sessions/{id}/archive` | `session_handlers.go:567`† | `StopDesktop`. | S(U) | — | n | DENY |
| `PUT /sessions/{id}` | `session_handlers.go:502` | DB only: name, provider, model. Not pushed live†. | S(U) | — | n | DENY |
| `POST /sessions/{id}/foreground-thread` | `session_handlers.go:2796` | `open_thread` command; switches the agent's thread. | S(U) | — | n | DENY |
| `POST /sessions/{id}/fork` | `session_fork_handlers.go:61` | Runs commit and push **inside the parent sandbox**, stops it, repoints the task. | S(U) | — | y (`:397`, `:439`†) | DENY |
| `POST /sessions/{id}/switch-agent` | `session_switch_agent_handlers.go:62` | Hot-switches the live agent. | S(U) | — | y | DENY |
| `PATCH /sessions/{id}/execution-config` | `session_execution_config_handlers.go:118` (S(U) `:140`; spec-task branch `applySpecTaskExecutionConfig` `:175`) | Hot-switches runtime, model, provider or credential, or restarts the agent. | S(U) | S(U) set | y (`execution_config.go:328`†) | **DENY** (§3.4) |
| `POST /sessions/{id}/agent-config-applied`, `/agent-startup-error` | `session_switch_agent_handlers.go:833`, `session_agent_startup_error_handlers.go:40` | Daemon callbacks. The first delivers the waiting turn early; the second **marks a turn as errored**. | S(U) | S(U) set | n | SELF |
| `GET` / `POST` / rotate / `DELETE /sessions/{id}/preview-tokens` | `session_preview_handlers.go:61` | Mints a **public, unauthenticated** `share-*` host for **any port 1-65535** in the container (`vhost_middleware.go:122`, `hydra/server.go:754`†). The caller can then drive the agent's dev servers. | S(U) | S(U) set | n | DENY (all four verbs; §1f) |
| `POST /api_keys?type=embed&interactive=true` | `handlers.go:~965` | Embed key that can drive the session's desktop (`auth_embed_key.go:237`). | project/task authz | project members | n | DENY for sealed tasks (any embed key, interactive or not) |
| `GET /api_keys?types=all` | `handlers.go:1296` | Lists the caller's keys **in plaintext**, including session keys, which are owned by the creator. Redaction only applies to scoped callers (`:1247`). | user | the creator | n | exclude or redact sealed session keys for every caller |

### 1c. Org Sandboxes API on session-backed rows (cyber R013 point 1)

Spec-task desktops, exploratory sessions and subscription desktops also have a `sandboxes` row
(`sbx_…` id) with `SessionID` set (`types/sandbox.go:92-107`). `HydraOpsID()` returns the
**session id** for those rows (`types/sandbox.go:147-152`), so every handler below operates on the
spec-task desktop. `loadAuthorizedSandbox` (`sandboxes_api_handlers.go:1004`) checks **org
membership only**. A fix for that is in progress in a separate PR. The seal must refuse this route
family regardless, because org owners and admins pass any membership check.

| route | handler | effect on a session-backed row | authz | who passes | provenance? | seal |
|---|---|---|---|---|---|---|
| `POST …/sandboxes/{id}/commands` | `sandboxes_api_handlers.go:226` (`:262`) | Arbitrary exec in the spec-task desktop. | org member | **every org member**, admins | n | DENY (the sandbox's own key has no use for it) |
| `GET …/commands`, `…/{cmd}`, `…/{cmd}/logs` | `:290`, `:320`, `:354` | Read command output. | org member | org | n | DENY (candidate: read) |
| `POST …/commands/{cmd}/kill` | `:391` (`:402`) | Signals processes in the desktop. | org member | org | n | DENY |
| `GET` / `PUT` / `DELETE …/files`, `GET …/files/list` | `:422` (`:440`, `:463`, `:470`), `:501` | Read / **write** / **delete** files. | org member | org | n | DENY (all verbs) |
| `GET …/terminal`, `…/terminal/sessions` | `:521` (`:531`), `:880` | Interactive shell. | org member | org | n | DENY |
| `GET …/screenshot` | `:683` (`desktop-{HydraOpsID}` `:694`) | Screenshot. | org member | org | n | DENY (candidate: read) |
| `PATCH …/sandboxes/{id}` | `:175` | Changes the row (TTL, …). | org member | org | n | DENY (§1f) |
| `DELETE …/sandboxes/{id}` | `:202` → `sandbox/controller.go:293` (`stopSessionDesktop`) | Stops the spec-task desktop. | org member | org | n | DENY (K stops and deletes through the task and session routes; §1f) |

### 1d. Spec-task routes

| route | handler | effect | authz | provenance? | seal |
|---|---|---|---|---|---|
| `POST /spec-tasks/from-prompt`, `/clone`, the helix-org `create_spectask` tool, the PM skill | `spec_driven_task_handlers.go`, `spec_task_clone_handlers.go:33` | Creates the task. | project/task create; `denyTaskCreationToConfinedKey` | y | sets the seal (§3.1); the creating key becomes K (§3.7) |
| `PUT /spec-tasks/{id}` | `spec_driven_task_handlers.go:1384`† | `status` drives the orchestrator (approve, start implementation), and the resulting prompt is attributed to the old `QueuedBy`†. `backlog` and `keep_alive=false` stop the desktop. **`agent_tools` hot-applies** through the MCP rev (`external-agent/zed_config.go:405-411`†). | T(U) | status path **n** (misattributed) | DENY (every field; `sandbox_sealed` immutable) |
| `PATCH /spec-tasks/{id}/execution-config`, `/interactive-embed-model` | `spec_task_execution_config_handlers.go:73`†, `spec_task_interactive_embed_model_handlers.go:54`† | Hot-switches agent, model or provider; live resource change. | T(U) | y | **DENY** (§3.4) |
| `POST /start-planning`, `/approve-specs`, `/approve-implementation`, `/pr-proposals/{p}/decide`, `/completion/decide` | `spec_driven_task_handlers.go:1209`†, `:699`†; `spec_task_workflow_handlers.go:30`†; `spec_task_pr_proposal_handlers.go:56`†, `:154`† | Workflow decisions that send templated system turns. | T(U) / project(U); agent keys refused except on approve-specs (gap noted in §6) | y | DENY (a sealed task must need no human decisions, §3.1) |
| design reviews: `/submit`, `/comments` POST, `/comments/{c}/resolve` | `spec_task_design_review_handlers.go:511`, `:722`, `:890` | `revision_requested` / `design_review_comment` turns, which interrupt the agent. | "task creator always has access" skips authz (`:539`, `:745`). Fix in progress in a separate PR. | y (`:767`†) | DENY |
| design review `/document` PUT | `spec_task_design_review_handlers.go:318`† | Commits to helix-specs, **which the agent reads**. No turn. | T(U) | n | DENY |
| `POST /spec-tasks/{id}/attachments` | `spec_task_attachments_handlers.go:324`† | Commits a file into the repo and sends an `attachment_added` turn. | T(U) | y | DENY after creation |
| `POST /refresh-pull-request` | `spec_task_pull_request_refresh_handler.go:25`† | Forces a PR poll, which can trigger `pr_settled` / `ci_status` turns. | **T(Get)** | partial | DENY (the PR and CI pollers still run as platform internals) |
| `POST /stop-agent`, `PATCH /archive`, `DELETE` | `spec_task_workflow_handlers.go:951`†; `spec_driven_task_handlers.go:1728`†, `:1648`† | Stop the desktop; delete. | T(U) | n | **K: stop** (`/stop-agent`), **K: archive and delete** (archive is the delete precondition†) |
| `DELETE /spec-tasks/{id}/zed-instance`, `POST /work-sessions/{s}/zed-thread`, `/zed/events`, `/zed/instances/{i}/threads/{t}/events`, `/zed/threads/{t}/activity` | `zed_event_handlers.go:314`, `:566`, `:106`, `:171`, `:435`† | Legacy protocol: fake agent state events, instance shutdown. | `authorizeUserToZedSpecTask(U)` | n | SELF; everyone else DENY (§1f) |

### 1e. Git, MCP, and live config

| path | where | effect | who passes | provenance? | seal |
|---|---|---|---|---|---|
| git push to the task branch | `services` `GitHTTPServer` `handleReceivePack` (`git_http_server.go:637`†); post-push hook `:1300`† | Silently changes the branch the agent works on and pulls. Branch limits apply only to agent keys. | anyone with repo write | n | SELF for the task's branch and its helix-specs task directory |
| `PUT /git/repositories/{id}/contents` | `git_repository_handlers.go:1172`† | Commits through the API. | repo(U) | n | same branch rule |
| `POST /pulls/{n}/request-rebase` | `git_pull_request_handlers.go:443`† | `rebase_required` turn. Has **no task-scope check** (a confined key can target another lane). | repo(U) | y | DENY (refused at the turn sink, §3.3) |
| `POST /pulls/{n}/merge`, GitHub review webhook | `:404`†; `spec_task_github_review_webhook.go:178`† | Merge; `pr_settled` / `review_comment` turns. | repo(U) / HMAC | partial (no `triggered_by`) | merge of the sealed task's PR: DENY. Webhook `review_comment` turns are dropped at the turn sink and recorded as `denied`. Poller `pr_settled` / `ci_status` turns: platform |
| MCP `session` backend agent-only tools: `propose_pull_request`, `mark_task_complete`, `task_completed` | `mcp_backend_session.go:215`, `:339`† | Acts **as the agent**. | `session.Owner == user.ID` plus key scope, so the creator's personal key passes | n | SELF |
| MCP `helix-tasks`: `SendSpecTaskAgentMessage`, Start/Stop/Restart | `mcp_backend_spectask.go:62-127`†; tools `org/interfaces/mcptools/defaults.go:186-200`† | One sandbox messages, stops or restarts **another** task's agent in the same project. | non-confined session keys in the project | system turn | DENY at the turn sink when the target is sealed |
| helix-org bot tools that steer spec tasks | `mcp_backend_helix_org.go`† | Bot messages a task's agent. | org-bot session key | y (`org_bot_spec_task_message`) | DENY at the turn sink when the target is sealed |
| **Live config: project skills and MCPs** | `getZedConfig` `zed_config_handlers.go:123-130`, `:164`; project MCPs `external-agent/zed_config.go:389-393`, stdio `command` `:445-486`; daemon poll `cmd/settings-sync-daemon/main.go:35` | A project member adds a stdio MCP or changes an MCP URL; Zed runs it **inside the running sandbox** within about 30s. This is **remote code execution with no record.** | project(U) (`project_handlers.go:683`, `:845`†) | n | **freeze** (§3.4) |
| Live config: app/agent MCPs, provider endpoints, `agent_tools` | `getZedConfig` → `GenerateZedMCPConfig` | Changing the agent's MCPs or the provider base URL (lets the editor intercept LLM traffic) reaches the running sandbox. | app editors, org owners | n | **freeze** (§3.4) |
| Start-time inputs: project secrets (they can set `HELIX_SKILLS_REPO`, `_REF` and the like†), `.helix/startup.sh`, skills ref | `hydra_executor.go:925-936`†; `helix-workspace-setup.sh:301-320`† | Apply on the next start or resume. | project(U) | n | snapshot hashes at seal time; flag drift on every start (§3.4) |
| hydra registry and admin debug routes | `sandbox_handlers.go:27-35`†; `/admin/agent-sandboxes/*`, `/admin/runners/{id}/logs` | Read-only, or runner/admin only. No path into a desktop. | admin / runner | n | n/a |
| `/ws/external-agent-runner` | `server.go:~2167`† | Runner token only. | runner | n | n/a (platform) |
| Skills routes, `/skills/reload` | admin† | Start-time only. | admin | n | n/a |

There is no dedicated admin desktop viewer. Admins use the normal session page through the routes
above (`frontend/src/components/admin/Runners.tsx` only stops sessions†), so sealing those routes
covers admins too.

### 1f. Routes missed at `66c7cdd` (round-7 list), and how the guard reaches each

Each row gives the route registration (`server.go`), the handler, and the line where today's authz
helper resolves the sealed object. The last two columns say how the guard reaches the route. "Path
guard" is the router middleware (§3.3, gate 1). "Resolver gate" is the seal check inside the shared
authz helper (§3.3, gate 2). Because the resolver gate sits in the helper, a handler that resolves
the id from its body is still covered.

| route | registered | handler | object resolved at | id comes from | covered by | seal |
|---|---|---|---|---|---|---|
| `PATCH /spec-tasks/{taskId}/execution-config` | `server.go:1756` | `spec_task_execution_config_handlers.go:73` | `authorizeUserToSpecTask` `:111` | path `taskId` | path guard + resolver gate | DENY |
| `POST /sessions/{id}/cancel` | `server.go:1295` | `session_handlers.go:2600` | `authorizeUserToSession` `:2611` | path `id` | path guard + resolver gate | DENY (candidate once the actor is recorded, §3.6) |
| `POST /sessions/{id}/foreground-thread` | `server.go:1297` | `session_handlers.go:2796` | `authorizeUserToSession` `:2813` | path `id` | path guard + resolver gate | DENY |
| `PATCH /organizations/{org_id}/sandboxes/{id}` | `server.go:1498` | `sandboxes_api_handlers.go:175` | `loadAuthorizedSandbox` `:176` (`:1004`) | path `id` → row `SessionID` / `SpecTaskID` (`types/sandbox.go:102`, `:107`) | path guard + resolver gate (in `loadAuthorizedSandbox`) | DENY |
| `DELETE /organizations/{org_id}/sandboxes/{id}` | `server.go:1499` | `sandboxes_api_handlers.go:202` | `loadAuthorizedSandbox` `:203` | same | same | DENY (K stops and deletes through the task and session routes) |
| `POST /organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/kill` | `server.go:1504` | `sandboxes_api_handlers.go:391` | `loadAuthorizedSandbox` `:392` | same | same | DENY |
| `POST /zed/events` | `server.go:1797` | `zed_event_handlers.go:106` | `authorizeZedSpecTask` `:129` → `authorizeUserToZedSpecTask` `:76` | **body** `spec_task_id` (decoded `:110`, checked `:125`) | resolver gate only (no id in the path) | SELF; everyone else DENY |
| `POST /zed/instances/{instanceId}/threads/{threadId}/events` | `server.go:1798` | `zed_event_handlers.go:171` | `authorizeZedSpecTask` `:216` | **body** `spec_task_id`, else the thread's record (`:208-214`) | resolver gate only | SELF; everyone else DENY |
| `POST /zed/instances/{instanceId}/heartbeat` (same family) | `server.go:1799` | `zed_event_handlers.go:501` | `authorizeZedSpecTask` `:527` | body | resolver gate only | SELF; everyone else DENY |
| `POST /zed/threads/{threadId}/activity` (same family) | `server.go:1800` | `zed_event_handlers.go:435` | `authorizeZedSpecTask` `:458` | the thread's record | resolver gate only | SELF; everyone else DENY |
| `GET /spec-tasks/{taskId}/zed-instance`, `GET …/zed-threads` (same family) | `server.go:1801`, `:1803` | `zed_event_handlers.go:264`, `:371` | `authorizeUserToZedSpecTask` `:286`; `authorizeUserToSpecTask` `:393` | path `taskId` | path guard + resolver gate | DENY |
| `POST /work-sessions/{sessionId}/zed-thread` | `server.go:1804` | `zed_event_handlers.go:566` | `GetSpecTaskWorkSession` `:588` → `GetSpecTask` `:595` → `authorizeUserToZedSpecTask` `:602` | path `sessionId` is a **work-session** id, not a session id | path guard (resolves through the work session) + resolver gate | DENY |
| `DELETE /spec-tasks/{taskId}/zed-instance` | `server.go:1802` | `zed_event_handlers.go:314` | `authorizeUserToZedSpecTask` `:336` | path `taskId` | path guard + resolver gate | DENY |
| `GET /sessions/{id}/preview-tokens` | `server.go:1305` | `session_preview_handlers.go:30` | `authorizeUserToSession` `:37` | path `id` | path guard + resolver gate | DENY |
| `POST /sessions/{id}/preview-tokens` | `server.go:1306` | `session_preview_handlers.go:61` | `authorizeUserToSession` `:68` | path `id` | same | DENY |
| `POST /sessions/{id}/preview-tokens/{token_id}/rotate` | `server.go:1307` | `session_preview_handlers.go:114` | `authorizeUserToSession` `:123` | path `id` | same | DENY |
| `DELETE /sessions/{id}/preview-tokens/{token_id}` | `server.go:1308` | `session_preview_handlers.go:186` | `authorizeUserToSession` `:195` | path `id` | same | DENY |

`authorizeUserToZedSpecTask` has a non-project branch (`zed_event_handlers.go:80-89`) that does not
call `authorizeUserToSpecTask`. The resolver gate must therefore go into `authorizeUserToZedSpecTask`
itself, not only into `authorizeUserToSpecTask`.

---

## 2. How `/sessions/{id}/zed-config/user` overrides are applied

Short answer: **they aren't applied to a running sandbox.** The route is a write-only store. The
sandbox's own settings-sync daemon uses it to report local `settings.json` edits.

1. **Authorization.** `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54-71`) admits:
   - a per-runner token bound to the session's host (`canRunnerReadSession`,
     `runner_identity.go:177`);
   - the session owner with any credential, followed by S(G).

   Bot-instance keys are allowlisted for it (`auth_bot_instance_key.go:35-38`).
2. **Storage.** The body is any JSON object, unvalidated (`zed_config_handlers.go:449-457`). It is
   upserted into `zed_settings_overrides(session_id, overrides jsonb)` via
   `external_agent.SaveUserZedOverrides` (`api/pkg/external-agent/zed_config.go:974`).
3. **Readers.** The only reader is `GetUserZedOverrides` (`external-agent/zed_config.go:989`), and
   its only caller is `getMergedZedSettings` (`zed_config_handlers.go:555`). That handler serves
   `MergeContextServers(helix, user)` to the UI's "MCP Tools" panel
   (`frontend/src/components/session/ZedSettingsViewer.tsx`†). **`getZedConfig` never reads the
   overrides.**
4. **The daemon** (`api/cmd/settings-sync-daemon/main.go`):
   - It authenticates with `USER_API_TOKEN`, the session key.
   - It fetches `GET /zed-config` at start (with retries), polls it every 30s (`:35`), and reacts to
     `config_changed` on `/ws/user`. `field=agent_restart` runs `pkill -x zed` (`:1721`).
   - It writes `/home/retro/.config/zed/settings.json` (`:28`), the qwen settings (`:31`),
     `/etc/claude-code/managed-settings.json` (`:945`), and the codex, goose and claude credential
     and config files.
   - It watches `settings.json` with fsnotify and POSTs the user-edited part back to
     `/zed-config/user` (`syncToHelix`, `:~2342-2372`). **That push-back is the route's main
     caller.**
5. **Merge rules inside the sandbox** (`mergeSettings`, `:1970`):
   - Helix-owned keys win: `agent_servers`, `telemetry`, `default_agent` and `external_sync`
     (`:1845`). Inside `agent`, the default and side models and `sandbox_permissions` are also
     Helix-owned. So are the context servers `chrome-devtools`, `helix-session`, `helix-desktop`,
     `helix-viz` and `helix-tasks` (`:1899`).
   - Any other key the user edits locally wins until the next full `syncFromHelix` resets the
     overrides. That includes `context_servers` (arbitrary stdio commands), `language_models`,
     `assistant`, and `agent.tool_permissions`.
   - Zed hot-reloads the file. Only someone who can already write `settings.json` *inside* the
     sandbox can use this, i.e. the exec/terminal/input/upload/workspace-file routes.

**Consequences for the seal:**

- The POST cannot change agent behaviour by itself. Under the seal it becomes SELF, so the panel
  cannot show fabricated entries.
- The real behavioural channel is `GET /zed-config`. It carries project and app MCPs, including
  stdio commands, plus provider endpoints and `agent_tools`, and re-pulls every 30 seconds. That is
  what §3.4 freezes.
- `GET /zed-config` and `/zed-settings` also hand the creator the session key, so they must be SELF.

---

## 3. Design

### 3.1 Model

- **`SpecTask.SandboxSealed bool`**: `gorm:"not null;default:false"`, AutoMigrate.
  - It can be set on every creation path that takes `confine_agent_key`: from-prompt, clone (a
    clone of a sealed task is always sealed), helix-org `create_spectask`, and the PM skill
    `CreateSpecTask`.
  - It is immutable, exactly as in #3611. `PUT /spec-tasks/{id}` returns 400 on a differing value,
    and `PostgresStore.UpdateSpecTask` omits the column.
- **`sandbox_sealed` implies `confine_agent_key`.** A create request with `sandbox_sealed: true`
  forces confinement on, and returns 400 if the caller explicitly sent `confine_agent_key: false`.
  Without confinement, the lane's own key can reach sibling sessions, so the seal would protect the
  sandbox but not what it does.
- **The sealing key K is recorded.** Add `SpecTask.SealedByKeyID` (the non-secret `key_…` id from
  `types.APIKeyID`, `types/api_key_summary.go:40`), plus `SealedByUserID` and `SealedAt`. All three
  are write-once and omitted from updates.
  - A sealed task must be created with an API key, so that K is a single credential.
  - A create request with `sandbox_sealed: true` from a browser session, a runner token, or a
    session/project/task-scoped key returns 400.
  - Other credentials of the same user are **not** K (§3.7).
- **A sealed task must not need a human decision to make progress.** Default-deny refuses
  approve-specs, design-review submit, approve-implementation and the PR decisions to everyone, so a
  task that waits on one of them would never finish.
  - A create request with `sandbox_sealed: true` requires `just_do_it_mode: true`
    (`types/simple_spec_task.go:467`) and auto-approval of its own completion path.
  - Otherwise it returns 400, naming the missing setting.
  - Whether this holds for every completion path (PR auto-merge, completion decide) needs checking
    during implementation.
- **The seal is derived, never denormalised.** `sealOf(session)` reads
  `session.Metadata.SpecTaskID` and then the task's column. A copy on session metadata would be
  writable by `PUT /sessions/{id}` paths. Because the flag never changes, `sealed=true` can be
  cached per task id for the life of the process, so the check costs one lookup per task.
- **`SpecTask.SealedConfig`** (jsonb, written once, omitted from updates). See §3.4.
- **Break-glass fields** (§4): `SealBrokenAt`, `SealBrokenBy` and `SealBrokenReason`. Write-once.

### 3.2 Who counts as the sandbox's own identity

`isSandboxSelf(user, session, task)` is true only in these two cases:

1. **The session key itself.**
   - `TokenType == api_key`, `user.SessionID == session.ID`, `user.SpecTaskID == task.ID` and
     `user.TaskConfined`.
   - The same session only. A sibling session of the same task is a different sandbox and does not
     count.
2. **A per-runner token bound to the session's host** (`canRunnerReadSession`). This applies only
   to the routes a host uses: `/zed-config` GET, the credential GET and PUT, and `/external-agents/sync`.
   The **shared** runner token is refused on every sealed route.

Identity is decided by the **credential, never by `user.ID`**. The session key and the creator share
a user id, so any rule based on `user.ID == session.Owner` admits the creator. Admins get no
exemption. An admin can mint a personal key for any user, but a personal key is never "self".

**Keeping the session key secret is a prerequisite.** Without it, "self" equals "anyone who can read
the key":

- Mint sealed session keys with a new type, `APIkeytypeSandboxSession`. Keep `Owner = session.Owner`
  so existing RBAC, git and billing keep working.
- `GET /api_keys` never returns that type's secret to anyone; it returns the prefix only.
- `GET /zed-config`, `/zed-settings` and the credential routes become SELF for sealed sessions.
- `DELETE /api_keys?key=` on a sealed session key is refused. Revoking happens when the desktop
  stops.
- `/revdial` `desktop-{sid}` and `/external-agents/sync` accept only SELF for sealed sessions. This
  closes the takeover paths, where today the creator, admins and any runner-token holder qualify.

### 3.3 Enforcement: default-deny, decided centrally

There is **one decision function and one allow-list**. Three gates call the function. None of them
has a route-by-route opt-in, so a route that is not listed is refused.

```go
// sealGrant is who a sealed-task allow-list entry admits.
type sealGrant string // "self" | "self_runner" | "sealing_key"

// sealAllowList is the ONLY place a route is permitted on a sealed task.
// Key: METHOD + " " + mux path template. A template that is absent is refused.
var sealAllowList = map[string][]sealGrant{
    "POST /api/v1/spec-tasks/{spec_task_id}/stop-agent":       {"sealing_key"},
    "DELETE /api/v1/sessions/{id}/stop-external-agent":        {"sealing_key"},
    "PATCH /api/v1/spec-tasks/{taskId}/archive":               {"sealing_key"}, // delete precondition
    "DELETE /api/v1/spec-tasks/{taskId}":                      {"sealing_key"},
    "DELETE /api/v1/sessions/{id}":                            {"sealing_key"},
    "GET /api/v1/spec-tasks/{taskId}/seal":                    {"sealing_key"}, // §3.7
    "GET /api/v1/spec-tasks/{taskId}/sandbox-access":          {"sealing_key"}, // §4
    "GET /api/v1/sessions/{id}/zed-config":                    {"self", "self_runner"},
    "POST /api/v1/sessions/{id}/zed-config/user":              {"self"},
    "GET /api/v1/external-agents/sync":                        {"self", "self_runner"},
    "GET /api/v1/revdial":                                     {"self"},
    // … every other SELF row of §1 (credentials, daemon callbacks, zed events,
    // the agent's own drive routes) …
}

// sealDecision is called by all three gates. It returns the outcome and the reason it records.
func (s *HelixAPIServer) sealDecision(ctx context.Context, user *types.User,
    taskID, sessionID string) (outcome sealOutcome, reason string, err error)
```

`sealDecision` works in this order:

1. If the object belongs to no task, or its task is not sealed, return allowed and record nothing
   for the seal. Normal authz has already run, or runs next.
2. If the task's seal is broken (§4), return allowed.
3. Read the route key from the request context. A middleware stamps it from
   `mux.CurrentRoute(r)`, the same way `stampRequestAuditRoute` does at `request_audit.go:132`.
   - **No route in the context means a platform-internal caller** (orchestrator, pollers, hydra,
     reapers). Return allowed.
4. Look the route key up in `sealAllowList`. Allow only if the caller matches one of that entry's
   grants:
   - `self`: the session key (§3.2);
   - `self_runner`: a per-runner token bound to the session's host;
   - `sealing_key`: `types.APIKeyID(user.Token) == task.SealedByKeyID` (§3.7).
5. Otherwise return denied with error code `sandbox_sealed`.
6. Write the access record (§4) synchronously, for both outcomes. If the write fails, the request
   fails closed.

**Gate 1, the path guard (router middleware).** It runs on the auth, user-or-runner and runner
routers, after authentication. It resolves the sealed object from the path or query and calls
`sealDecision` before the handler runs.

| route family | how the id resolves |
|---|---|
| `/sessions/{id}` | session → `Metadata.SpecTaskID` |
| `/external-agents/{sessionID}` | session |
| `/spec-tasks/{taskId\|spec_task_id\|id}` | task |
| `/interactions/{interaction_id}` | interaction → session |
| `/organizations/{org_id}/sandboxes/{id}` | sandbox row → `SessionID` / `SpecTaskID` |
| `/work-sessions/{sessionId}` | work session → `SpecTaskID` |
| `/mcp/desktop`, `/mcp/session`, `/revdial`, `/external-agents/sync` | the session id in the query |

Gate 1 refuses before body parsing and before any side effect. So the sealed twin in a probe
(§3.7) answers `sandbox_sealed` even to an empty body.

**Gate 2, the resolver gate (inside the shared authz helpers).** This covers ids that are not in
the path, such as body-addressed routes (§1f: `/zed/events`, the thread events, heartbeat), and a
secondary object a handler loads. These helpers call `sealDecision` right after their own checks
pass:

- `authorizeUserToSession` (`authz.go:589`)
- `authorizeUserToSpecTask` (`authz.go:300`)
- `authorizeUserToZedSpecTask` (`zed_event_handlers.go:76`), whose non-project branch does not
  reach `authorizeUserToSpecTask`
- `authorizeSessionSandboxConfig` (`session_credentials_authz.go:54`)
- `authorizeDesktopID` (`external_agent_handlers.go:46`)
- `loadAuthorizedSandbox` (`sandboxes_api_handlers.go:1004`)
- `loadAuthorizedTerminalSession`, `authorizeWorkspaceReviewRequest`
- the desktop and session MCP backends' authorize callbacks
- the `ownsSessionInKeyScope` paths (`/external-agents/sync`, credentials)
- the `/revdial` desktop registration (`revdial_registration.go:32`)

Every handler that touches a session, task or session-backed sandbox already has to call one of
these helpers; the route-policy test enforces that today. So gate 2 makes default-deny hold even
for a route gate 1 cannot parse.

**Gate 3, the agent-input sinks.** Turns can be produced by routes that resolve a repo or PR rather
than the task (request-rebase, merge, the GitHub review webhook, `helix-tasks` and helix-org MCP
tools). These sinks refuse a sealed target:

- `sendCommandToExternalAgent`
- `enqueueAgentMessage` / `persistQueuedPrompt`
- `RunExternalAgent` for an existing session
- `GitHTTPServer` receive-pack on the sealed task's branch or its helix-specs task directory

The sink cannot use "no route in the context" to recognise platform internals.
`sendCommandToExternalAgent` takes no context at all (`websocket_external_agent_sync.go:2483`), and
internal turns are often produced while a request's context is live. So the sink decides on the
turn's **provenance** (#3613), which every turn already carries:

| provenance on the turn | sealed target |
|---|---|
| `system` with a kind Helix composes itself, with no caller text (`planning_prompt`, `implementation_prompt`, `ci_status`, `pr_settled`, `push_required`, `completion_sent_back` when auto-approved) | allowed |
| `system` with a kind that relays caller-supplied text or decisions (`design_review_comment`, `revision_requested`, `review_comment`, `attachment_added`, `rebase_required`, `pr_proposal_outcome`, `org_bot_*`, `cron_trigger`, chat-channel kinds) | denied |
| `user`, or `api` with an `api_key_id` other than the session's own key | denied |
| `api` with the session's own key | allowed |

Non-turn commands (`cancel`, `open_thread`, `respond_question`, `cancel_question`) come only from
HTTP routes that gates 1 and 2 already deny. The `/command` route is deleted (§5).

When the sink drops a turn, it writes a `denied` access record. Examples are a GitHub reviewer's
`review_comment`, and a sibling task's `SendSpecTaskAgentMessage`.

**Route-policy test (fails on unclassified routes).** Add a test in `route_auth_policy_test.go`
that walks the production router (`buildRouter`). A route **resolves a sealable id** if either:

- (a) its template has one of the gate-1 prefixes; or
- (b) an AST walk of its handler finds a call to any gate-2 helper, or to
  `Store.GetSession`, `GetSpecTask`, `GetSandbox`, `GetInteraction`, `GetSpecTaskWorkSession`,
  `GetSpecTaskZedThreadByZedThreadID`.

Every such route must declare `.seal(...)` in its `route_policy_*_test.go` entry. The value is
either `sealDeny()` or `sealAllow(grants...)`. The test fails if:

- a route that resolves a sealable id has no declaration;
- a `sealAllow` declaration disagrees with `sealAllowList`;
- `sealAllowList` names a template the router does not serve.

A second test runs every declared `sealDeny()` route against a sealed fixture with K, an org owner
and an admin. It expects 403 `sandbox_sealed` and a `denied` access record, with the fake store and
connman recording no write, dial or send.

Cost: one cached lookup per request on the gate-1 prefixes. `sealed=true` is cached forever. For
`false`, cache only the session→task mapping, never the negative seal.

Internal callers are allowed by construction. They don't call the gate-2 helpers, and at gate 3
they carry system provenance:

- the orchestrator;
- `execCommandInDesktop` (sets git identity, `services/spec_driven_task_service.go:1721`);
- `ensureFeatureBranchInContainer`;
- workspace checkpoint capture (`interaction_code_changes.go:69`);
- prompt-queue delivery;
- the CI and PR pollers;
- hydra.

Each of these runs a fixed, server-built action.

### 3.4 Config that changes mid-run (cyber R013 point 2): freeze inherited config, refuse own config

There are two kinds of config a running sealed sandbox can receive.

| source | owner | recommendation | why |
|---|---|---|---|
| Project skills and MCPs, app/agent MCPs, provider endpoints, re-pulled by `getZedConfig` (`zed_config_handlers.go:123-130`, `:164`, `:200`) every 30s | the project, app or org, shared with other tasks | **Freeze** | Refusing a project edit because one task is sealed would block the whole project. Freezing scopes the effect to the sealed task. |
| `PATCH /sessions/{id}/execution-config` (`session_execution_config_handlers.go:118-180`), `PATCH /spec-tasks/{id}/execution-config`, `/interactive-embed-model`, `PUT agent_tools`, switch-agent | the sealed task itself | **Refuse** (not on the allow-list: `sandbox_sealed`, §3.3) | An explicit 403 is honest. Accepting a write and then silently serving the old config would mislead whoever made the change. |

**Freeze mechanics:**

- At task creation, when the seal is set, resolve and store the declarative inputs in
  `SpecTask.SealedConfig`. These are:
  - the assistant config (runtime, model, provider_ref and reasoning effort, i.e. the
    `CodeAgentConfig`);
  - the agent's MCP definitions, the project `Skills.MCPs`, and `agent_tools`;
  - the provider endpoint **base URL and id**. The API key stays by reference so it can rotate.
  - SHA-256 hashes of the project secret set (names and value hashes), `.helix/startup.sh` at the
    base commit, and the skills ref.
- `getZedConfig` (and every container start, which on a sealed task is platform-initiated only) build from `SealedConfig` when the
  task is sealed. Tokens and URLs are still rendered per request, so the session key and OAuth
  tokens rotate normally, but the *set* of MCP servers, their commands and URLs, the model and the
  endpoint URL are fixed.
- If a frozen dependency disappears (the endpoint is deleted, an MCP's OAuth connection is revoked),
  the sandbox fails visibly. It never falls back to live config.
- Start-time inputs (secrets, startup script, skills ref) are re-hashed on every container start. A mismatch
  writes a `config_drift` access event and shows on the task. Freezing secret *values* is out of
  scope for v1: secrets are encrypted per project and rotated by design.
- `config_changed` publishes to a sealed session are dropped, except `color_scheme`
  (`user_handlers.go:564`), which is cosmetic.

Recommendation: freeze inherited config and refuse own config, as above. Freeze-only would leave the
execution-config routes reporting success on no-ops. Refuse-only cannot stop project-level edits.

### 3.5 What still works under the seal (the whole allow-list)

- **K** may:
  - stop (`POST /spec-tasks/{id}/stop-agent`, `DELETE /sessions/{id}/stop-external-agent`);
  - archive, the precondition for delete (`PATCH /spec-tasks/{id}/archive`);
  - delete (`DELETE /spec-tasks/{id}`, `DELETE /sessions/{id}`);
  - read the seal (`GET /spec-tasks/{id}/seal`) and the access record
    (`GET /spec-tasks/{id}/sandbox-access`).

  That is all K may do.
- **The sandbox itself** (SELF rows in §1): the daemon, sync, RevDial, config and credentials,
  its callbacks and zed events, the LLM proxy, git push to its own branch, and its own MCPs.
- **Platform internals:** orchestrator, hydra, pollers, checkpoints, reapers, credit-exhaustion
  stop, and the system turns listed under gate 3.

Nothing else is allowed by default. That covers read views (interactions, screenshot, stream,
workspace), further turns (`/messages`, the prompt queue, approvals, comments), resume, restart,
cancel, and every org-owner and admin action through the API. The "(candidate: …)" notes in §1
list what an owner could later choose to add, one entry at a time, to `sealAllowList`. Each
addition needs a review, and its route-policy entry changes visibly.

Under the seal, nobody but the agent can type into Zed. So an interaction with origin
`external_agent` comes from the agent itself, which makes it a useful signal for the verifier.

**Open question for the owner (not decided here).** Should org owners get `stop` for cost control?
Today a runaway sealed sandbox is stopped by K, by its TTL or credit exhaustion (platform), or by
break-glass. Adding `{org_owner: stop}` is a single allow-list entry, should it be wanted.

### 3.6 Stop, delete and cancel (cyber R013 point 3)

**Stop and delete survive, for K only.** These routes are allow-listed for K:

- `POST /spec-tasks/{id}/stop-agent`
- `DELETE /sessions/{id}/stop-external-agent`
- `PATCH /spec-tasks/{id}/archive` (the delete precondition)
- `DELETE /spec-tasks/{id}`
- `DELETE /sessions/{id}`

None of them forwards caller-supplied content to the agent. Each writes an access record with
`outcome: allowed` and `kind: lifecycle`, and the task shows "stopped by K at T".

Every other lifecycle route is denied, by default-deny:

- `PUT status=backlog`;
- archive through `PUT`;
- the org Sandboxes `DELETE` / `PATCH`;
- resume, restart and ensure-agent.

These are equivalent to routes K already has, or they restart the agent at a time the caller
chooses. Platform internals (TTL, credit exhaustion, reapers) still stop and destroy the sandbox.

**Cancel is denied by default.** It becomes an allow-list candidate only when it is recorded and
content-free, which matches the reviewer's position.

- **Turn cancel** (`POST /sessions/{id}/cancel`, `session_handlers.go:2600` →
  `cancelActiveTurn` at `prompt_history_handlers.go:702`):
  - It sends a fixed, server-built cancel for the current request id and carries no caller content.
  - It qualifies once it records `cancelled_by` (a `TurnProvenance`) on the interaction it ended.
  - Cancelling at a strategic moment changes an outcome. An unrecorded cancel cannot be told apart
    from an agent failure.
- **Question cancel** (`POST /interactions/{iid}/questions/{rid}/cancel`,
  `interaction_question_handlers.go:76`):
  - It sends a fixed `cancel_question` for a `request_id` that is validated against the pending
    question, and carries no content.
  - It qualifies once `ResolvedQuestion` records the actor (`ResolvedBy TurnProvenance`).
- Even once qualified, cancel is added to `sealAllowList` (for K) only by an owner decision.
- **Question respond** carries caller-chosen `answers` into the agent. It stays denied.
- **`/clear`** destroys agent context. It stays denied.

### 3.7 The sealing key K, and the control/twin probe

K is the API key that created the sealed task. Its non-secret id is stored in `SealedByKeyID`
(§3.1).

**K can set the seal and read it back.**

- **Set:** `POST /spec-tasks/from-prompt` (or `/clone`) with `sandbox_sealed: true`,
  `just_do_it_mode: true`, using K. The response returns the task with `sandbox_sealed: true`,
  `sealed_at`, and `sealed_by_key_id` (K's own `key_…` id, never a secret).
- **Read back:** `GET /spec-tasks/{taskId}/seal` (new, allow-listed for K) returns
  `{spec_task_id, sandbox_sealed, sealed_at, sealed_by_key_id, confine_agent_key, seal_broken_at, seal_broken_by, seal_broken_reason}`.
  - The same route works on an **unsealed** task for anyone with normal read authz. It returns
    `sandbox_sealed: false`, so a probe reads both tasks the same way.
  - `GET /spec-tasks/{taskId}` is **not** on the allow-list; it returns task content. On an
    unsealed task it also includes `sandbox_sealed` and `sealed_by_key_id`.

**A sealed task refuses K on every non-allow-listed route.** Being K does not widen anything except
the entries in §3.5. K gets 403 with:

```json
{"error": "sandbox_sealed", "route": "<METHOD template>", "spec_task_id": "spt_…"}
```

That is the same body an org owner or admin gets.

**The probe this design supports.** K creates two tasks in the same project with identical inputs:

- **A**, an unsealed control (`sandbox_sealed: false`);
- **B**, the sealed twin (`sandbox_sealed: true`).

Then, using K throughout:

1. `GET /spec-tasks/{A}/seal` → `sandbox_sealed: false`. `GET /spec-tasks/{B}/seal` →
   `sandbox_sealed: true`, with `sealed_by_key_id` equal to K's id.
2. **Enumerate the probe set.** `GET /api/v1/seal-policy` (new, any authenticated caller, static) is
   generated from the same `sealAllowList` and the route-policy declarations. It returns every
   `METHOD template` that resolves a sealable id, with its grants (`deny`, `self`, `sealing_key`).
   The probe iterates every `deny` route, so it needs no hand-kept route list, and it covers
   routes added after the probe was written.
3. **For each `deny` route, call it on A and on B with K.**
   - **A must accept.** Any response other than `403 sandbox_sealed` counts, including a 400 for an
     empty body: that means the request got past authz and the seal check. K must hold project
     update rights for this to be meaningful, and the probe should check that first with one
     known-good write on A.
   - **B must refuse** with exactly `403` and `error: "sandbox_sealed"`.
   - Path-addressed routes refuse before body parsing (gate 1), so an empty body is enough.
   - Body-addressed routes (`/zed/events`, the thread events, heartbeat; §1f) need a body that names
     B's task id. The resolver gate then refuses, before any side effect.
4. `GET /spec-tasks/{B}/sandbox-access` must show one `outcome: denied` record per probe call.
   `seal_broken_at` must still be null: denied attempts are not breaches (§4).
5. The `sealing_key` routes (stop, archive, delete) must succeed on B with K, and must be refused
   on B for a different credential of the same user. That last check proves the grant is bound to
   the key, not to the user.

The control matters because a refusal on B alone could come from ordinary RBAC. Acceptance on A,
with the same key and route, shows that the seal is what refused B.

---

## 4. Break-glass and the access log

**Recommendation: ship both.** The access log comes first and is the more important of the two.

### Access log

Add a table, `spec_task_sandbox_access_events`, written by the server and append-only.

**Columns:**

| column | values / notes |
|---|---|
| `id`, `created` | |
| `spec_task_id` | **always set.** It is resolved from the task, session, sandbox row, interaction or work session. |
| `session_id`, `sandbox_id` | nullable. Set when the route resolved one. |
| `scope` | `task`, `session` or `sandbox`: which kind of id the route addressed |
| `outcome` | **`allowed` or `denied`**. Required on every row. |
| `kind` | `access` (a route call), `turn` (a gate-3 sink decision), `lifecycle` (an allowed stop/archive/delete), `config_drift`, `break_glass` |
| `reason` | `sealing_key`, `self`, `self_runner`, `platform`, `not_allowlisted`, `turn_provenance_denied`, `drift`, `break_glass` |
| `actor_user_id`, `credential_kind` (browser, personal_key, org_key, session_key, runner, admin), `api_key_id` | `api_key_id` is the non-secret `key_…` id |
| `route` | `METHOD template`, or the sink name for gate 3 |
| `status` | the HTTP status returned |

**Scope.** The record covers all three route families, not only routes that touch the sandbox:

- **task-level routes**, i.e. anything resolving `/spec-tasks/{taskId|spec_task_id|id}` (execution
  config, approvals, design reviews, attachments, PR decisions, zed-instance, the seal read, stop,
  archive, delete), plus the body-addressed task routes in §1f;
- **session-level routes:** `/sessions/{id}/…`, `/external-agents/{sessionID}/…`, `/interactions/…`,
  the MCP backends, sync and RevDial;
- **sandbox-level routes:** `/organizations/{org}/sandboxes/{id}/…` on session-backed rows.

`sealDecision` writes one row per decision, whichever gate called it, so all three families land in
the same table keyed by `spec_task_id`.

**A denied attempt is not a breach.**

- `outcome: denied` means the seal *held*. The UI shows such rows as "blocked attempts", and they
  never change the task's seal state.
- `outcome: allowed` rows are expected, and are attributed by `reason`: K's stop or delete, the
  sandbox's own traffic, or platform work.
- The **only** event that changes the seal state is `kind: break_glass`, which sets
  `seal_broken_at` (§ Break-glass).
- `GET /spec-tasks/{id}/seal` reports `seal_broken_at`; it is not derived from counting rows. So a
  verifier reading the record can tell three states apart: intact and untouched, intact with
  blocked attempts, and broken.

**Volume.**

- On sealed tasks every decision is written, `allowed` and `denied` alike.
- Allowed SELF traffic (sync reconnects, the 30s config polls) is deduplicated: one `allowed` row
  per route, credential and hour, with a count. Otherwise the record would fill with the sandbox's
  own polling.
- On unsealed tasks, only non-read operations are written.

- **Written** synchronously by `sealDecision` and the gate-3 sinks. A failed write fails the
  request closed.
- **Exposed** as `GET /spec-tasks/{id}/sandbox-access`: allow-listed for K on sealed tasks, T(Get)
  on unsealed ones. Filters: `outcome`, `kind`, `scope`, `since`. Also shown as a panel on the
  task. The portal and verifier read it alongside `/interactions`.
- **Kept** for the task's lifetime and never purged by the audit retention job.
- **Why not reuse the request audit log:**
  - It is admin-only and global, with no session column.
  - Its recorder **drops events when its buffer is full** (`api/pkg/auditlog/recorder.go:55`).
  - Its retention is 180 days (`config.go:1379`).

  An evidentiary record must not drop rows. Keep the request audit as the second, independent
  trail.

### Break-glass

Add `POST /spec-tasks/{id}/break-seal`.

- It is on `adminRouter`, needs a reason, and is not reversible.
- It sets `SealBrokenAt`, `SealBrokenBy` and `SealBrokenReason`, writes a `break_glass` event, and
  publishes a `seal_broken` state shown on the task and in the API.
- After that the task is treated as **unsealed** under normal authz. It never becomes "sealed with
  an exception", so no reader can mistake a broken seal for an intact one.

**Why keep it:** without an API path, operators who need to debug a stuck verifier will edit the
database or `docker exec` on the host, which leaves no trace in Helix. A loud, irreversible
break-glass is strictly better than that.

**Why not an exemption:** a per-request admin bypass is exactly the out-of-band access the seal
exists to rule out, and it would be invisible to the verifier.

**What it cannot catch:** host or database access by the platform operator. State that limit in the
portal (see the Goal section).

---

## 5. Implementation plan

**Phase 1: default-deny on every route, key secrecy, access record.** About 7 days.

1. **Model** (0.5d).
   - Add `SandboxSealed`, `SealedByKeyID`, `SealedByUserID`, `SealedAt` and the break-glass fields.
   - Plumb them through every creation path.
   - Enforce immutability in PUT and the store, force confinement, and require a key credential and
     `just_do_it_mode`.
   - Update the OpenAPI spec (`./stack update_openapi`); add the frontend create option and a badge.
2. **Decision function** (1d). Add the route-key context stamp; `sealOf` with its cache;
   `isSandboxSelf`; the K match on `SealedByKeyID`; `sealAllowList`; `sealDecision`; and the
   synchronous access-record write with `outcome`.
3. **Gate 1, the path guard** (0.5d). The middleware resolves every family in §3.3, including
   `/work-sessions/{sessionId}` → work session and `/organizations/{org}/sandboxes/{id}` → row
   `SessionID`.
4. **Gate 2, the resolver gate** (1d). Call `sealDecision` from every helper listed in §3.3,
   including `authorizeUserToZedSpecTask` and `loadAuthorizedSandbox`.
   - Force the stream read-only in all cases (bug 1 in §6).
   - **Delete `POST /external-agents/{sid}/command`**: it is labelled "manual command sending for
     testing" (`external_agent_handlers.go:217`), there are no callers in `frontend/src`,
     `api/pkg/cli` or `api/pkg/client` (grep at `d3e922636`; re-check the e2e test server before
     deleting), and it is the widest primitive on the list. Delete the no-op
     `configure-pending-session` too.
5. **Route-policy test and seal declarations** (1d).
   - Add `.seal(...)` to every `route_policy_*_test.go` entry that resolves a sealable id.
   - Add the unclassified-route test, plus the deny-sweep test against a sealed fixture (§3.3).
   - Add `GET /api/v1/seal-policy`, generated from the same data.
6. **Key secrecy** (1d).
   - Add the `APIkeytypeSandboxSession` key type.
   - Exclude sealed session keys from `GET /api_keys` and refuse deleting them.
   - Restrict `/external-agents/sync` to SELF or a bound runner, and replace the
     `validateExternalAgentToken` stub.
   - Restrict `/revdial` `desktop-{sid}` to SELF.

   These are allow-list entries plus the fixes that make "self" mean only the sandbox.
7. **Seal read and access record** (1d). Add `GET /spec-tasks/{id}/seal`,
   `GET /spec-tasks/{id}/sandbox-access` with filters, the task panel ("blocked attempts" vs. seal
   state), and SELF deduplication.
8. **Break-glass** (0.5d), if the owner chooses to ship it (§4).

**Phase 2: the turn sink, config freeze and git.** About 4-5 days.

9. **Gate 3, the turn sink** (1d). Add provenance-based refusal in `enqueueAgentMessage`,
   `persistQueuedPrompt` and `RunExternalAgent` for an existing session, covering request-rebase,
   the review webhook, and the `helix-tasks` and helix-org MCP tools. Each refusal writes a
   `denied` record with `kind: turn`.
10. **Config freeze** (2d). Build the `SealedConfig` snapshot at creation; make `getZedConfig` and
    platform-initiated restarts build from it; add drift hashing; drop `config_changed`.
11. **Git branch protection** (1d). In `GitHTTPServer` pre-receive and the `/contents` PUT, only
    the task's own session key may write the sealed task's branch or its helix-specs task
    directory.
12. **Provenance for cancel and questions** (0.5-1d). Record `cancelled_by` and `ResolvedBy`. This
    makes cancel *eligible* for the allow-list; adding it is an owner decision (§3.6).

**Tests, the probe harness and live verification:** about 3 days, spread across both phases.

**Total: about 14-15 engineer-days.** Phase 1 alone puts every route that names a sealed id under
default-deny. Phase 2 closes the paths that reach the agent without naming one (sinks, config, git).
Ship them as separate PRs.

## 6. Independent bugs this inventory found

These need fixing whether or not a task is sealed. Two are already in progress in a separate PR and
appear here only as inventory rows: prompt-history sync on `ActionGet`, and the design-review
"creator always has access" shortcut.

1. **The stream WebSocket accepts input from any reader.** `/ws/stream` is authorized with
   `ActionGet`, but forwards input unless the caller holds a non-interactive embed key
   (`desktop_stream_readonly.go:55`). Fix: set read-only unless the caller passes `ActionUpdate`.
2. **The agent channel can be taken over.**
   - `/external-agents/sync` admits **all admins**, any runner-token holder for any session, and
     the owner with any credential.
   - `validateExternalAgentToken` is a stub that returns true.
   - The connection registry is last-writer-wins and never closes the displaced connection.
3. **The owner's personal key can register `/revdial` `desktop-{sid}`**, and the registration is
   last-wins. This allows interception of all API→desktop traffic.
4. **The creator can read session keys in plaintext,** from `GET /api_keys?types=all` and from
   `GET /sessions/{id}/zed-config`.
5. **Org Sandboxes routes on session-backed rows admit any org member.** A fix is in progress in a
   separate PR.
6. **Project members can run code live in running sandboxes.** A project stdio MCP reaches every
   running sandbox in the project within 30s. Even unsealed, at minimum record an access event, and
   consider requiring a restart for a new stdio command.
7. **Preview tokens can expose any port publicly.** They accept any port from 1 to 65535 on an
   unauthenticated `share-*` host. Consider a port allowlist.
8. **The desktop MCP `save_screenshot` writes to any path the caller gives.**
9. **The session MCP's agent-only tools accept the owner's personal key.**
10. **Spec-task workflow gaps:**
    - `approve-specs` has no agent-key guard.
    - `request-rebase` has no task-scope check.
    - `PUT status` attributes the resulting prompt to the old `QueuedBy`†.
11. **`POST /interactions/…/questions/…/respond` records no provenance.**

## 7. Test plan

**Route policy** (`route_auth_policy_test.go` and the `route_policy_*_test.go` files):

- Add a `Seal` field and the `.seal(sealDeny())` / `.seal(sealAllow(grants...))` builders to
  `routePolicy`.
- **Unclassified-route test (default-deny guard).** Walk the production router. A route resolves a
  sealable id if its template has a gate-1 prefix, **or** an AST walk of its handler finds a call to
  a gate-2 helper or to the `Store.Get{Session,SpecTask,Sandbox,Interaction,SpecTaskWorkSession,
  SpecTaskZedThreadByZedThreadID}` lookups (§3.3). The test fails if:
  - such a route has no `.seal(...)` declaration;
  - a `sealAllow` declaration disagrees with `sealAllowList`;
  - `sealAllowList` names a template the router does not serve.

  Verify the test by deleting one declaration (for example the one for
  `POST /api/v1/zed/events`) and watching it fail.
- **Coverage of the 66c7cdd list.** A table-driven test asserts that each §1f route is declared
  `sealDeny()` (or SELF-only for the zed event family). For those routes it also asserts that a
  sealed fixture refuses K: the body-addressed ones through gate 2, and the rest through gate 1.
- An AST test checks that every handler referencing a desktop primitive reaches `sealDecision`
  through a gate-2 helper. The primitives are:
  `connman.Dial`, `dialDesktopExec`, `callSessionDesktopJSON`, `sendCommandToExternalAgent`,
  `hydra.NewRevDialClient`, `openPersistentTerminal`, `uploadToSandbox`, and the
  `hydra` `RunSandboxCommand` / `WriteSandboxFile` / `DeleteSandboxFile` / `KillSandboxCommand`
  calls.

**`SandboxSealSuite`** (gomock store, plus a fake `connman` that counts `Dial` and `Set`):

- **Callers:**
  - session owner via browser;
  - owner's personal key;
  - the key that created the task;
  - an admin-minted key for the owner;
  - org owner;
  - platform admin;
  - project member with update;
  - an org member on an `OrgMembersAccess` project;
  - a sibling task's confined key;
  - an unconfined sibling session key;
  - the shared runner token;
  - a per-runner token bound to another host.
- **Every `sealDeny()` route** × every caller (K included):
  - assert 403 with `error: "sandbox_sealed"` for callers inside the tenant, and 404 for callers
    outside it;
  - assert **no side effect**: no store write (gomock has no expectation), no `Dial`, no `Set`, no
    `SendChan` send;
  - assert an access record with `outcome: denied` and `reason: not_allowlisted`, and that
    `seal_broken_at` is still null.
- **Allowed cases:**
  - the session's own key on the SELF routes reaches the dial;
  - a bound runner reads `/zed-config`;
  - K's stop, archive and delete succeed and write `outcome: allowed`, `kind: lifecycle`;
  - K reads `/seal` and `/sandbox-access`;
  - another credential of K's user is refused on those same routes.
- **Denied by default (regression guard):** K's `/messages`, resume, cancel, screenshot and stream
  on the sealed task all return `sandbox_sealed`.
- **Gate 3:** a `review_comment` webhook turn, a `rebase_required` from request-rebase, and a
  sibling `SendSpecTaskAgentMessage` aimed at the sealed task are dropped, each with an
  `outcome: denied`, `kind: turn` record. A `ci_status` poller turn is delivered.
- **Control/twin probe (§3.7), automated.** Using K: create the unsealed control A and the sealed
  twin B. Iterate `GET /api/v1/seal-policy` for every `deny` route.
  - A must not return `403 sandbox_sealed`.
  - B must return exactly that.
  - B's access record must gain one `outcome: denied` row per call, and B's `seal_broken_at` must
    stay null.

  Run it with `sealDecision` stubbed to "allowed" and confirm it fails, so the test proves the seal
  rather than the existing authz.
- **Immutability and K:**
  - a sealed create through a browser session or a scoped key returns 400;
  - a sealed create without `just_do_it_mode` returns 400;
  - `GET /seal` on B returns `sealed_by_key_id` equal to K's `key_…` id.
- **Immutability:**
  - PUT `sandbox_sealed` with a differing value returns 400;
  - the store update omits the column;
  - a clone of a sealed task is sealed;
  - `sandbox_sealed` with `confine_agent_key: false` returns 400.
- **Key secrecy:**
  - `GET /api_keys?types=all` as the owner shows only the prefix for the sealed session key;
  - the owner is refused `/zed-config`;
  - `/external-agents/sync` and `/revdial` reject the owner's personal key and an admin.
- **Freeze:**
  - change a project MCP and the execution config after creation; `getZedConfig` for the sealed
    session is unchanged and the execution-config PATCH returns 403;
  - an unsealed sibling task does see the change;
  - drift in secrets or the startup script on a restart writes `config_drift`.
- **Git:** a push to the sealed task's branch by the owner's personal key is rejected; a push by the
  session key is accepted.
- **Org Sandboxes:** on a session-backed `sbx_` row for a sealed session, every route (commands,
  logs, kill, files in every verb, terminal, screenshot, PATCH, DELETE) returns `sandbox_sealed` to
  K, org owners and admins. Each refusal writes a record with `scope: sandbox`.

**Live, in the inner Helix** (mandatory, because this changes the agent-channel lifecycle):

1. Create a sealed spec task with a live Zed and confirm `zed_thread_id` is set.
2. As the owner in the browser, and as K through the CLI:
   - the stream, screenshot, terminal, upload, exec, workspace-file, messages and resume all return
     `sandbox_sealed`;
   - K's stop works;
   - the access-record panel shows the blocked attempts while the seal state stays intact.
3. Confirm the sandbox still works by itself: the daemon syncs, MCPs connect, the agent pushes its
   branch, and credentials refresh.
4. Add a project stdio MCP mid-run and confirm it does **not** appear in the sealed sandbox's
   `settings.json` after 60s, while an unsealed task in the same project picks it up.
5. After K's stop, confirm that platform-initiated restart, if any, builds from `SealedConfig`.
   Then confirm K's archive and delete remove every resource (§ lifecycle checklist in
   CLAUDE.md).
6. Break the seal, confirm the state is visible, and confirm normal access returns.
