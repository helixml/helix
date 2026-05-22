# 03 — Ubiquitous Language

Step 3 of the DDD-style excavation. Goal: produce a glossary, expose the
homonyms and synonyms, and surface the places where prompts and code
disagree about what a term means. This is the linguistic backbone the
rest of the redesign hangs off.

Sources walked:

- `domain/*.go` — canonical structs / interfaces / IDs.
- `store/sqlite/*.go` — GORM rows and table names.
- `tools/*.go` — every built-in `ToolName` constant and its
  Description string (what the LLM actually reads).
- `prompts/templates/role.md`, `bootstrap/templates/owner_role.md`,
  `agent/policy.md` (the embedded `agent.Policy`), every
  `demos/*/roles/*.md`, every `demos/*/workers/*.md` (the user-shaped
  intent).
- `CLAUDE.md` (project intent), `TODO.md` (acknowledged drift).
- `server/server.go`, `server/mcp.go`, `dispatch/dispatcher.go`,
  `broadcast/broadcaster.go`, `agent/spawner.go`,
  `agent/helix/state.go` (architectural prose attached to the
  scaffolding).

---

## 1. Glossary

Each term is keyed by its dominant code-side name. Citations point at
the declaration site that anchors the term.

### Org-graph nouns

| Term | Definition | Citation | Load-bearing? |
|---|---|---|---|
| **Worker** | The principal that occupies a Position and holds Grants. The single subject of authorisation, activation, and addressing. Two subtypes via marker interface: `HumanWorker`, `AIWorker`. | `domain/worker.go:51`, `store/sqlite/worker.go:25` (table `workers`, col `kind` ∈ "human","ai") | **Load-bearing.** Every other concept hangs off Worker. |
| **WorkerKind** | Enum: `"human"` \| `"ai"`. Controls dispatcher behaviour (only `ai` gets a Spawner activation) and in-prompt priority (see `source_kind`). | `domain/worker.go:10-15`, surfaced in trigger as `source_kind:` (`agent/prompt.go:90`) | **Load-bearing.** |
| **WorkerID** | String identifier, convention `w-<lowercase-firstname>` (or `w-<uuid>` fallback). Naming is enforced socially via the `hire_worker` Description prose, not the schema. | `domain/id.go:953`, `tools/hire_worker.go:58-67` | Load-bearing. |
| **IdentityContent** | Per-Worker free-text persona/profile. Lives in the `workers` row, NOT on disk; spawners project it to `identity.md` at activation. | `domain/worker.go:41-46`, `agent/spawner.go:66-73` | Load-bearing. |
| **Position** | A slot in the org tree that instantiates a Role. `(ID, RoleID, ParentID)`. `p-root` is conventional root. | `domain/position.go:146-150`, `store/sqlite/position.go` table `positions` | Load-bearing. |
| **PositionID** | String, convention `p-<slug>`. `p-root` is referenced as the default parent in the owner role template (`bootstrap/templates/owner_role.md`). | `domain/id.go:952` | Load-bearing. |
| **Role** | A job description: `(ID, Content, CreatedAt, UpdatedAt)`. Owner-edited markdown. Multiple Workers can fill the same Role via Positions. | `domain/role.go:192-197`, `store/sqlite/role.go` table `roles` | Load-bearing. |
| **RoleID** | String, convention `r-<kebab-case>`. | `domain/id.go:951`, `prompts/templates/role.md:67-69` | Load-bearing. |
| **Grant** / **ToolGrant** | `(ID, WorkerID, ToolName)`. Authorisation. There is no scope field — granularity is per-tool, by design. | `domain/grant.go:327-331`, `store/sqlite/grant.go` table `grants` | **Load-bearing**, but the comment explicitly rejects a `Scope` field (`domain/grant.go:5-8`). |
| **GrantID** | String. | `domain/id.go:954` | Incidental. |
| **Stream** | Named source of events: `(ID, Name, Description, CreatedBy, CreatedAt, Transport)`. The unit of fan-out and the unit of audit. Workers publish to / subscribe to Streams. | `domain/stream.go:235-242`, `store/sqlite/stream.go` table `streams` | **Load-bearing.** |
| **StreamID** | String, convention `s-<slug>`. Activation streams are `s-activations-<workerID>` (`agent/prompt.go:176`). | `domain/id.go:956` | Load-bearing. |
| **Event** | One entry on a Stream: `(ID, StreamID, Source, Body, CreatedAt)`. Body is always Message-JSON in practice. | `domain/event.go:286-292`, `store/sqlite/event.go` table `events` | Load-bearing. |
| **EventID** | String, convention `e-<uuid>`. | `domain/id.go:957` | Incidental. |
| **Subscription** | `(WorkerID, StreamID, CreatedAt)` link from a Worker to a Stream. Composite key — no synthetic ID. Drives both `read_events` and dispatcher fan-out. | `domain/subscription.go:358-362`, `store/sqlite/subscription.go` table `subscriptions` | Load-bearing. |
| **Environment** | `(WorkerID, Path, CreatedAt)` — directory on the host where a Worker's working files live (`role.md`, `identity.md`, `agent.md`, plus whatever the role writes). Mirror of identity into a filesystem. | `domain/environment.go:394-398`, `store/sqlite/environment.go` table `environments` | Load-bearing. Note the term doubles up — see "Environment" in homonyms. |
| **Tool** | Generic capability interface: `Name()`, `Description()`, `InputSchema()`, `Invoke()`. Exposed to the calling Worker over MCP, gated by Grants. | `domain/tool.go:440-456`, registered in `tools/builtins.go:81-115` | **Load-bearing**, but split semantically — see homonyms. |
| **ToolName** | String — both stable identifier in `grants.tool_name` and the MCP tool name. | `domain/id.go:955`, all `XxxName domain.ToolName = "…"` constants in `tools/*.go` | Load-bearing. |
| **Invocation** | `(Caller Worker, Args json.RawMessage)` — the per-call bundle handed to `Tool.Invoke`. | `domain/tool.go:430-433` | Incidental. |
| **Message** | Canonical Stream payload — JSON envelope `{from, to, subject, body, body_content_type, thread_id, in_reply_to, message_id, attachments, extra}`. Always wrapped as `Event.Body`. Transports translate provider-native ↔ Message at the boundary. | `domain/message.go:799-810` | **Load-bearing** — the deepest unspoken assumption: "all events are messages". |
| **Attachment** | `(Filename, ContentType, URL, SizeBytes)`. Pointer-only, system never owns the bytes. | `domain/message.go:817-821` | Incidental today; load-bearing once object storage lands. |
| **Transport** | `(Kind TransportKind, Config json.RawMessage)` attached to every Stream. Decides how Events move to/from the outside world. | `domain/transport.go:557-560` | **Load-bearing.** |
| **TransportKind** | Enum: `local` \| `webhook` \| `email` \| `github`. | `domain/transport.go:475-541` | Load-bearing. |
| **WebhookConfig** / **EmailConfig** / **GitHubConfig** | Per-kind shape of `Transport.Config`. | `domain/transport.go:571-604` | Load-bearing per transport. |
| **Config** (operational) | `(Key, Value, UpdatedAt, UpdatedBy)`. Server-level operational config (e.g. `claude.bin`, `transport.postmark`). Set via CLI only — never MCP. Distinct from org-graph state. | `domain/config.go:890-895`, `store/sqlite/config.go` table `configs`, `config/registry.go:36-68` | Load-bearing for runtime ops; orthogonal to the org graph. |
| **Spec** (config) | Subsystem's declaration of one config key — `Key`, `Type` (`string|int|object`), `Default`, `Required`, `Secrets`, `Description`. | `config/registry.go:36-68` | Incidental. |
| **WorkerRuntimeState** / **WorkerState** | Sidecar key/value store keyed by `(WorkerID, Backend, Key)`. Holds Helix runtime pointers (`project_id`, `agent_app_id`, `repo_id`, `session_id`, `hiring_user_id`) per Worker. | `agent/helix/state.go:65-71`, `store/sqlite/worker_runtime.go` table `worker_runtime_state` | Load-bearing for the helix runtime; invisible to prompts. |

### Runtime / activation nouns

| Term | Definition | Citation | Load-bearing? |
|---|---|---|---|
| **Trigger** | Per-activation context: `{Kind, EventID, StreamID, Source, SourceKind, Message, CreatedAt}`. Tells the agent why it was woken. | `agent/spawner.go:25-46` | Load-bearing — appears verbatim in every activation prompt (`=== Trigger ===`). |
| **TriggerKind** | Enum: `"hire"` \| `"event"`. | `agent/spawner.go:14-22` | Load-bearing. |
| **Spawner** | Function `func(ctx, workerID, envPath, []Trigger) error`. Runs an AI Worker for ONE activation and blocks until exit. Two implementations: `agent/claude` (local `claude` CLI exec), `agent/helix` (Helix chat session). | `agent/spawner.go:60`, `agent/claude/spawner.go`, `agent/helix/spawner.go` | **Load-bearing.** |
| **Activation** | One Spawner invocation = one fresh agent run = one turn. Each activation publishes events to the Worker's activation Stream. No long-running agent loops. | `agent/prompt.go:11-44`, `dispatch/dispatcher.go:1-22` | **Load-bearing** in prose; never a struct. Implicitly defined by the lifecycle of a Spawner call. |
| **Activation Stream** | `s-activations-<workerID>` — deterministic per-Worker Stream that carries the transcript (assistant text + tool calls + tool results). Read by the hiring Worker, NOT subscribed to by the Worker itself. | `agent/prompt.go:172-177`, `tools/hire_worker.go:40-46` | Load-bearing for `worker_log` and the chat UI. |
| **Dispatcher** | The component that turns a publish into N activations (one per subscribed AI Worker). Owns per-Worker queue + coalescing. Also drives outbound webhook POSTs and outbound email Emit. | `dispatch/dispatcher.go:50-58` | Load-bearing. |
| **EventDispatcher** | Tool-side interface (`Dispatch`, `DispatchHire`) used by tools to fan out — defined separately to break the import cycle. | `tools/builtins.go:27-30` | Incidental — duplicate of `Dispatcher` interface in `server/server.go:22-25`. |
| **Broadcaster** | Tiny in-process pub/sub used by long-poll readers (e.g. `read_events(wait=…)`, the live UI) to wake on Notify. Distinct from the Dispatcher. | `broadcast/broadcaster.go:11-22` | Load-bearing. Easy to confuse with Dispatcher — see homonyms. |
| **WorkspaceSync** | Interface that mirrors canonical Role/Identity content into the agent's working location. Two backends: `claude` writes to `<envsDir>/<workerID>/<name>`, `helix` pushes to a `helix-specs` branch. | `agent/spawner.go:62-83` | Load-bearing — but its name overlaps with `Environment` and with `helix.Workspace`. |
| **Workspace** (claude backend) | `agent/claude.Workspace` — concrete file-mirroring `WorkspaceSync`. Unrelated to the Helix "workspace" concept. | `agent/claude/workspace.go` | Incidental. |
| **WorkerProject** | Helix-runtime component that ensures each Worker has a Helix Project + auto-provisioned Agent App + git repo, and pushes role/identity/agent.md to the `helix-specs` branch. | `agent/helix/project.go:21-60` | Load-bearing inside the helix runtime; invisible elsewhere. |
| **Backend** (runtime) | String label namespacing `WorkerRuntimeState` rows. Today exactly `"helix"`. | `agent/helix/state.go:23` | Incidental — premature plural. |
| **Backend** (chat) | Interface in `server/chat/backend.go` — abstraction over a chat driver (claude / helix). Different "Backend" entirely from the runtime one. | `server/chat/backend.go` | See homonyms. |

### Tools (every `ToolName` is a domain term)

The 30 built-in tools are the surface every prompt names. They split
into three rough tiers — but the code makes no such distinction:

| Tier (informal) | Tools |
|---|---|
| **Structural mutations** | `create_role`, `update_role`, `update_identity`, `create_position`, `hire_worker`, `grant_tool`, `revoke_tool`, `create_stream`, `subscribe`, `unsubscribe`, `invite_workers` |
| **Communication** | `publish`, `dm` |
| **Reads** | `list_roles`, `get_role`, `list_positions`, `get_position`, `list_position_children`, `list_workers`, `get_worker`, `list_worker_grants`, `get_worker_environment`, `list_streams`, `get_stream`, `list_stream_events`, `get_grant`, `read_events`, `worker_log`, `stream_members` |
| **Test-only** | `ping` |

Notable named primitives in tool prose:

- `DM` (tool) — described as "Direct message (DM/PM/private message)" — implemented as a per-pair Stream that gets auto-created+auto-subscribed. So a DM is a Stream of two members. `tools/dm.go:13-25`.
- `invite_workers` — the only third-party-subscribe primitive; the bare `subscribe` is self-only. `tools/invite_workers.go:14-19`. **TODO.md item 1 admits this split is wrong** (workers shouldn't subscribe themselves).
- `worker_log` — sugar over `subscribe + read_events` against `s-activations-<workerID>`. `tools/worker_log.go:16-23`.
- `publish` vs `dm` — both append a Message-event; `dm` resolves a deterministic per-pair Stream first.

### Prompt-only / external-host nouns

| Term | Definition | Citation | Load-bearing? |
|---|---|---|---|
| **owner** | The seeded root Worker `w-owner`. Holds every tool. Drives the org via `helix-org chat`. | `bootstrap/templates/owner_role.md:1`, `CLAUDE.md:30-31`, `tools/publish_test.go:32` | **Load-bearing** in prompts; encoded only as a literal `w-owner` ID in code. No Owner type, no role discriminator. |
| **operator** | Synonym of "owner" used in the owner role template ("You are the operator — you hire, set direction, decide, unblock"). | `bootstrap/templates/owner_role.md:9-22`, `prompts/templates/role.md:40` | Synonym — see §3. |
| **manager** | Used in prose to mean "any Worker calling `hire_worker` or `invite_workers`". No code construct. | `tools/invite_workers.go:14-16`, `prompts/templates/role.md:99`, `demos/newsroom/roles/recruiter.md:25` | Synonym for "hiring caller". |
| **hiring manager** | Same thing as "manager". | `CLAUDE.md:20`, `tools/hire_worker.go:38-39` | Synonym. |
| **agent** (prompt sense) | What `agent.md` calls an AI Worker ("You are an AI Worker… **Default to acting.**"). The shared org-wide policy text. | `agent/policy.md:1-8` | Load-bearing as a policy text; see homonyms. |
| **agent** (code sense) | Go package `agent/` — runtime/activation scaffolding. Sub-packages `agent/claude`, `agent/helix` are LLM driver runtimes. | `agent/spawner.go`, `agent/claude/spawner.go`, `agent/helix/spawner.go` | See homonyms. |
| **Agent App** | Helix-side abstraction — every Helix Project auto-provisions one; helix-org wires its per-Worker MCP endpoint into the Agent App. | `agent/helix/project.go:17-21`, `agent/helix/state.go:50` (`AgentAppID`) | Load-bearing inside the helix runtime. Different "agent" entirely. |
| **role.md / identity.md / agent.md** | The trio of files projected into a Worker's Environment on activation. role.md = Role.Content; identity.md = Worker.IdentityContent; agent.md = embedded `agent.Policy`. | `agent/spawner.go:73-83`, `agent/policy.md` | Load-bearing — the **actual** interface between the Go code and the LLM. |
| **helix-log.md** | Per-Worker rolling log the agent itself maintains in its Environment. Not a system construct — pure prompt convention. | `agent/policy.md:21-37` | Load-bearing for behaviour, invisible to code. |
| **Triggers section** | Markdown convention inside Role.Content: `**On <event>.** ...` blocks. Drives how a Worker reacts. | `prompts/templates/role.md:26-58`, every demo role | Load-bearing in prompts; code does not parse this — the LLM does. |
| **Constraints section** | Markdown convention inside Role.Content. Refusals / forbidden actions. | `prompts/templates/role.md:39-45` | Load-bearing in prompts; invisible to code. |
| **Streams section** | Markdown convention inside Role.Content listing the `s-…` IDs the Worker reads/writes. Per `CLAUDE.md`: "reference data the hiring manager's prompt reads, not triggers the code acts on." | `prompts/templates/role.md:21-25`, `CLAUDE.md:20` | Load-bearing in prompts; the code deliberately does not act on it. **This is a deliberate code/prompt asymmetry.** |
| **Tools (MCP) section** | Markdown convention inside Role.Content listing the toolnames the hiring manager should grant. | `prompts/templates/role.md:17-19`, every demo role | Same asymmetry: prompt-only. |
| **shell tool** | Anything in the Environment's shell (`bash`, `curl`, `gh`, `git`, `python`). Not an MCP tool, not modelled in the domain — just executables Claude finds on `$PATH`. | `CLAUDE.md:19`, `demos/email/roles/customer-service.md`, `demos/newsroom/roles/editor-in-chief.md:11`, `demos/github-engineer/roles/software-engineer.md:18-22` | Load-bearing as a tier in the design philosophy; invisible to code. |
| **dispatch / dispatcher** (prompt sense) | Used in `CLAUDE.md` and `demos/email/roles/customer-service.md:73` ("The dispatcher will reactivate you") to name the wake-up actor. | `CLAUDE.md:30`, `demos/email/roles/customer-service.md:73`, `agent/policy.md` (implicit "runtime") | Same noun as the Go `Dispatcher`. Aligned. |
| **scope** | Used in `CLAUDE.md` ("scope value", "scope shapes") and in role prose to mean a per-grant constraint. **No Scope type exists.** `domain/grant.go:5-8` deliberately argues against it. | `CLAUDE.md:17,18`, `demos/github-engineer/roles/software-engineer.md:217` | **Pure prompt term — see §5.** |
| **persona / profile** | Used in `domain/worker.go:41` and `demos/newsroom/roles/recruiter.md:36` to mean what the code calls `IdentityContent`. | `domain/worker.go:41-46`, `demos/newsroom/roles/recruiter.md:36,54` | Synonyms — see §3. |
| **candidate** | Recruiter-prompt term for an `IdentityContent` blob written to disk before a hire. Not a domain type. | `demos/newsroom/roles/recruiter.md:31-44` | Prompt-only. |
| **handle** | Slug-form alias for a `WorkerID` used in recruiter prose. | `demos/newsroom/roles/recruiter.md:39-46`, `tools/hire_worker.go:58-67` | Synonym for `WorkerID`. |
| **Org graph** | Phrase covering "structural state — Workers, Positions, Roles, Channels, Grants, Streams". | `CLAUDE.md:19`, `store/store.go:127` | **Load-bearing** boundary concept (decides MCP vs CLI); never a type. |
| **org chart** | UI rendering of Positions+Workers as an SVG tree. | `server/ui/ui.go:74,180-181,220-226` | Incidental — UI affordance. |
| **subscribe** (verb) | (1) MCP tool `subscribe`. (2) `broadcast.Broadcaster.Subscribe` for long-poll wakeups. Two different actions sharing a name — see homonyms. | `tools/subscribe.go`, `broadcast/broadcaster.go:55-60` | Both load-bearing. |
| **publish** (verb) | (1) MCP tool `publish` (append a Message-event to a Stream). (2) `WorkspaceSync.PublishFile` (mirror canonical content into the agent's working location). | `tools/publish.go`, `agent/spawner.go:62-83` | Both load-bearing. **Different meanings.** |
| **hire** | (1) MCP tool `hire_worker`. (2) `TriggerKind = "hire"` activation. (3) Prompt-verb in role text ("On hire."). | `tools/hire_worker.go:51`, `agent/spawner.go:17`, every demo role | Consistent meaning. |
| **fire** | Used in `bootstrap/templates/owner_role.md:21` ("hiring, firing, reshaping reporting lines"). **No code construct.** | `bootstrap/templates/owner_role.md:21` | Prompt-only — see §5. |
| **chat** | (1) The `helix-org chat` CLI subcommand wrapping the local `claude` exec. (2) The web `/ui/chat` surface. (3) A Helix-side "chat session" (`helixclient.SessionInfo`). | `cmd/helix-org/chat.go`, `server/chat/`, `helix/helixclient/session_send.go` | See homonyms. |
| **session** | Almost entirely about LLM/UI surfaces — claude `.jsonl` per-cwd sessions; Helix chat session ID. **Not** an authentication or authorisation concept here. | `server/chat/sessions.go`, `helix/helixclient/session_send.go:13-19` | See homonyms. |

---

## 2. Homonyms (terms with multiple meanings)

This is the messy bit. Most overload is concentrated on 4 words.

### 2.1 "agent"

| Meaning | Where | How to tell it apart |
|---|---|---|
| The org-wide policy text every AI Worker reads at activation. | `agent/policy.md`, embedded as `agent.Policy`. Surfaces as `agent.md` in the Worker's Environment. | Lowercase, file-shaped. Always paired with `role.md` / `identity.md`. |
| The Go package containing Spawner + activation scaffolding. | `agent/spawner.go`, `agent/prompt.go`, `agent/activations.go`. | Import path `helix-org/agent`. Talks about Triggers, Spawners. |
| The two runtime sub-packages that exec the LLM. | `agent/claude/`, `agent/helix/`. | Sub-packages. "Spawner" lives here. |
| Helix-side "Agent App". | `agent/helix/project.go:17-21`, `agent/helix/state.go` (`AgentAppID`). | Always prefixed "Agent App". Helix-runtime-only concept. |
| Synonym for AI Worker in human-facing prose. | `CLAUDE.md:20` ("orchestrate multi-step sequences on behalf of an agent"), `demos/getting-started/README.md:55`, `agent/policy.md:11`. | Context: "the agent calls", "the agent does". Always means the AI side of a Worker. |
| Synonym for the LLM client tool (Claude Code, Qwen). | `CLAUDE.md`-style prose about runtimes. | Context: a binary, not an in-org actor. |

**Mitigation**: agent.md is the worst because the file ships INTO the Worker's environment and is named after the package. Most readers will conflate "agent (the policy)" with "agent (the AI Worker)" — `agent/policy.md` itself plays both sides ("You are an AI Worker… this file… tells you how to *be* an agent"). Pick one canonical term — "Worker" if we want the file to be `worker-policy.md`, or "agent" if we re-label `AIWorker → Agent` in the domain.

### 2.2 "session"

| Meaning | Where | How to tell apart |
|---|---|---|
| Claude CLI's per-cwd `.jsonl` conversation file. | `server/chat/sessions.go:14-21`, `helix-org chat --continue`. | Has a UUID-shaped `sessionId`, lives in `~/.claude/...`. |
| Helix-side chat session — a long-running model conversation backed by Helix. | `helix/helixclient/session_send.go:13-19`, `WorkerState.SessionID`. | Always paired with `Helix`. Sent over `helixclient`. |
| Sandbox lifetime ("frozen for the lifetime of their first desktop session"). | `bootstrap/templates/owner_role.md:62-66`. | Means the Helix sandbox boot. Rare. |
| UI-side "chat session" the web surface tracks. | `server/chat/chat.go`. | Same as Helix session usually, but in browser context. |

There is **no** auth/HTTP-session concept in helix-org. Every meaning above is a model-conversation lifetime of one shape or another.

### 2.3 "worker"

| Meaning | Where | How to tell apart |
|---|---|---|
| The domain principal `Worker`. | `domain/worker.go:51`. | The canonical sense. |
| The owner Worker `w-owner` — a `HumanWorker` row in the same table. | `bootstrap/bootstrap.go:148-149`, `CLAUDE.md:30`. | Same type, just a privileged ID. |
| A goroutine queue inside the Dispatcher — `workerQueue` (per-Worker activation queue). | `dispatch/dispatcher.go` (struct `workerQueue`). | Lowercase, internal. |
| HumanWorker (the type) vs AIWorker (the type). | `domain/worker.go:61-114`. | Concrete impls. |

Cleaner than "agent". The only landmine is `workerQueue` (an in-process Go construct) sharing the word with `Worker` (the domain principal).

### 2.4 "tool"

| Meaning | Where | How to tell apart |
|---|---|---|
| MCP tool — implements `domain.Tool`. The 30 built-ins. | `domain/tool.go:440`, `tools/*.go`. | The canonical sense. Reaches the LLM through `tools/list` / `tools/call`. |
| Shell tool — `bash`, `gh`, `curl`, `git`, `python` in the Worker's Environment. | `CLAUDE.md:19`, every demo role's "The Environment has …" block. | Always introduced by "the Environment has" or "your shell". NOT registered, NOT granted. |
| `Tool` in the `## Tools (MCP)` markdown section of a Role. | every role.md | A naming-only convention; describes what to grant, doesn't act. |

The MCP-vs-shell split is *architecturally load-bearing* (`CLAUDE.md:19`) but invisible to the codebase — `tools/` only knows about MCP tools, and shell tools have no representation at all. The Role markdown is the only place the boundary is described.

### 2.5 "stream"

| Meaning | Where | How to tell apart |
|---|---|---|
| `domain.Stream` — the org-graph noun. | `domain/stream.go:235`. | Always has an `s-` ID. |
| Verb — "stream events" via long-poll `read_events(wait=…)`. | `tools/read_events.go`, `broadcast/`. | Verb form. |
| Activation Stream — `s-activations-<workerID>`. A normal Stream with a magic ID convention. | `agent/prompt.go:172-177`. | Still a `Stream`; only the ID is special. |
| HTTP Streamable MCP transport. | `server/mcp.go:23-29`. | Network layer. |
| "Stream" the user types in role markdown ("which channels they read/write" — note channel/stream mixup). | `prompts/templates/role.md:82`. | Markdown. |

Aligned enough — they all converge on "ordered sequence of things you can subscribe to and replay". The risk is conflation with "Channel" — see §3.

### 2.6 "event"

| Meaning | Where | How to tell apart |
|---|---|---|
| `domain.Event` — a row in `events`. | `domain/event.go:286`. | Has a `streamId`. |
| GitHub webhook event type — `X-GitHub-Event` ("issues", "pull_request", …). | `domain/transport.go:599-617`, `demos/github-engineer/roles/software-engineer.md:114`. | Carried in `Message.Extra.event`. |
| Activation event — assistant message / tool call / tool result published to the activation Stream. | `agent/claude/spawner.go:130`, `agent/activations.go`. | Still a `domain.Event`; just a sub-genre. |
| Trigger event — the `domain.Event` that woke a Worker up. | `agent/spawner.go:25-46`. | Same row; context-dependent. |
| UI `EventCard` — render shape. | `server/ui/pages.go`. | UI-only. |

`Event` is reasonably well-disciplined — every meaning resolves back to a `domain.Event` row. The exception is GitHub-event-type, which is metadata *inside* a `domain.Event`. Two layers of "event" stacked here.

### 2.7 "channel"

There is no `Channel` type. The word means:

| Meaning | Where | What it actually maps to |
|---|---|---|
| Synonym for Stream in `CLAUDE.md:19`. | `CLAUDE.md`. | `Stream`. |
| "Output channel" / "post to s-… channel" in role prose. | `prompts/templates/role.md:29-57`, `demos/manufacturing/roles/quality-bot.md:5`. | `Stream`. |
| "Slack DM channel", "SMS channel", "email channel" — a stream of one transport kind. | `demos/manufacturing/roles/quality-bot.md:14-19`. | `Stream` with non-local `Transport`. |
| "Env channel" — what a backend's WorkspaceSync writes to. | `domain/worker.go:43`. | `WorkspaceSync`. |

**`CLAUDE.md` itself spells the structural primitives as "Workers, Positions, Roles, Channels, Grants, Streams" — separating "Channels" and "Streams" as if they were distinct, which they are not in the code.** Pick one term. See §6.

### 2.8 "subscribe" / "publish"

| Meaning | Where |
|---|---|
| MCP tool `subscribe` (self-subscribe a Worker to a Stream). | `tools/subscribe.go`. |
| `Broadcaster.Subscribe` (register wake-up channel for long-poll). | `broadcast/broadcaster.go:55-60`. |
| MCP tool `publish` (append Event to Stream). | `tools/publish.go`. |
| `WorkspaceSync.PublishFile` (mirror role.md/identity.md to runtime workspace). | `agent/spawner.go:62-83`. |
| `Broadcaster.Notify` is also colloquially called "publish" in dispatcher prose. | `dispatch/dispatcher.go`. |

Acceptable within their own packages, but `WorkspaceSync.PublishFile` is a particularly bad name — the same verb used in `tools/publish.go` for completely unrelated semantics. Rename candidate.

### 2.9 "environment"

| Meaning | Where |
|---|---|
| `domain.Environment` — `(WorkerID, Path)` row. | `domain/environment.go:394`. |
| "The Environment" in role prose — meaning "your working directory and the shell tools available there". | every demo role ("The Environment has `gh`, `git`…"). |
| `EnvsDir` in `tools.Deps` — host filesystem root. | `tools/builtins.go:42-46`. |
| `--envs` Make flag. | `Makefile`, `cmd/helix-org/serve.go`. |
| Workspace (claude backend) vs Helix repo (helix backend) — both materialise "the Environment" but in different shapes. | `agent/claude/workspace.go`, `agent/helix/project.go`. |

The shapes diverge: for claude it's a host directory; for helix it's a git branch on a helix-managed repo. The domain `Environment` row only describes the first. The helix runtime smuggles its equivalent through `WorkerRuntimeState`. This is the biggest unspoken polymorphism in the system.

### 2.10 "backend"

| Meaning | Where |
|---|---|
| Runtime label (`agent/helix.Backend = "helix"`). | `agent/helix/state.go:23`. |
| Chat backend interface (claude vs helix-bridge). | `server/chat/backend.go`. |
| Transport backend (provider — postmark, github). | `transports/postmark/`, `transports/github/`. |

Three separate things; all called "backend". Acceptable inside packages, dangerous in cross-cutting prose.

### 2.11 "dispatcher"

Two implementations under one name:

| Meaning | Where |
|---|---|
| The `dispatch.Dispatcher` — fans events to subscribed Workers. | `dispatch/dispatcher.go:50`. |
| The duplicate `server.Dispatcher` interface — narrower subset for breaking the import cycle. | `server/server.go:22-25`. |
| `tools.EventDispatcher` — yet another narrow subset for the same reason. | `tools/builtins.go:27-30`. |

Same idea, three interfaces. Architectural smell more than a homonym.

---

## 3. Synonyms (one concept, many words)

| Concept | Variants | Where each | Recommended canonical |
|---|---|---|---|
| Owner / Operator / The Human | "owner" (most places), "operator" (`prompts/templates/role.md:40`, `bootstrap/templates/owner_role.md:9`), "the user" (UI prose) | mostly prompts | **owner** — already the literal worker ID `w-owner`. |
| Hiring caller | "manager", "hiring manager", "the one calling hire_worker", "the operator who hires" | `tools/hire_worker.go:38`, `tools/invite_workers.go`, `demos/newsroom/roles/recruiter.md:25`, `CLAUDE.md:20` | **hiring Worker** (consistent with everyone-is-a-Worker stance). |
| Worker identity content | `IdentityContent` (code), "persona" (`domain/worker.go:41`), "identity" (`role.md`), "profile" (`demos/newsroom/roles/recruiter.md:36`), "voice / stance / personality refusals" (worker .md files), "candidate" (recruiter prose, pre-hire) | scattered | **Identity** (matches the `identity.md` file and `update_identity` tool). |
| AI Worker | `AIWorker` (struct), `WorkerKind = "ai"` (enum), "agent" (everywhere in prose), "AI Worker" (`agent/policy.md`), "software agent" (`domain/worker.go:88`), "the agent" (CLAUDE.md), "claude session" (helix-org chat) | scattered | **AI Worker** in prose; **agent** can stay for the LLM-side actor if the runtime layer wants it (but pick one and stick to it). |
| Stream / Channel | "Stream" (code, mostly), "channel" (`CLAUDE.md:19`, role markdown, manufacturing demo), "thread" (Message field `ThreadID`, GitHub demo "#42") | code uses Stream; prompts mix | **Stream**. Delete "Channel" from `CLAUDE.md`'s structural list. |
| Subscription / Membership | "Subscription" (struct), "stream members" (tool name `stream_members`), "subscribers" (broadcaster prose), "invite_workers" (the verb form of "add to a stream") | scattered | **Subscription** for the row, **stream membership** for the read view. Reconsider `invite_workers` as `subscribe_workers`. |
| Activation transcript | "activation Stream", "activation transcript" (`agent/prompt.go:171-177`), "Worker log" (the tool `worker_log`), "activation log" (`tools/worker_log.go:37`) | scattered | **Activation log** OR **Worker log** — pick one; `worker_log` is the tool name so probably that. |
| Role markdown sections | "Tools", "Tools (MCP)", "DefaultTools" (CLAUDE.md), "MCP scope" (`prompts/templates/role.md:82`) | mixed | **Tools** (MCP-tier) and **Shell tools** (`bash` etc.) as two distinct sub-sections. |
| Working directory | "Environment" (domain), "envs dir" (Make), "workspace" (helix backend), "the directory" (claude backend), "Path" (column) | mixed | **Environment** in the domain; **runtime workspace** for backend-specific materialisations. |
| The dispatcher | `Dispatcher` (dispatch pkg), `Dispatcher` (server interface), `EventDispatcher` (tools interface), "the runtime" (`agent/prompt.go:37`) | code | **Dispatcher** as the concrete; collapse the two narrow interfaces into one shared `tools.EventDispatcher` (move out of `tools`). |
| Activation | "activation" (everywhere), "spawn" (`agent.Spawner`), "wake" (broadcaster prose), "fire" (TODO item 4 — "spawn"), "reactivate" (`demos/email/...`) | mixed | **Activation** for the noun, **Spawn** for the verb-on-Spawner. "Wake" should be reserved for the broadcaster wake-up of long-polls, not for activations. |

---

## 4. Term sources

| Source | Style | Authority |
|---|---|---|
| `domain/*.go` | Tight, type-anchored. Every term has exactly one struct or one ID type. | **Strongest concrete source.** What's not here is not a first-class concept. |
| GORM rows in `store/sqlite/*.go` | Same vocabulary as `domain/`. No drift. Tables: `workers`, `positions`, `roles`, `grants`, `streams`, `events`, `subscriptions`, `environments`, `configs`, `worker_runtime_state`. | Aligned with domain — column `kind` enum-validated, etc. |
| Tool Descriptions in `tools/*.go` | Long prose blocks meant for the LLM. Use a mix of capitalised domain terms ("Worker", "Stream", "Position") and verbs ("DM", "publish"). Mostly consistent. | Strong — these are what the LLM literally sees in `tools/list`. |
| `prompts/templates/role.md` | Markdown template used by `/role`. Introduces the role-section vocabulary: `## Tools (MCP)`, `## Streams`, `## Triggers`, `## Constraints`, `## Files`. | Strong — this is the schema for every Role.Content the org creates. |
| Demo roles `demos/*/roles/*.md` | Real-world usage. Use the role.md schema with minor drift ("channel", "shell tools", "Other workers" section). | Strong — these are the in-use examples agents are trained on by `/role`. |
| `agent/policy.md` (= `agent.Policy`) | Org-wide AI policy. Defines "activation", "trigger", "source_kind", "speaking discipline", "helix-log.md". | Strong — every AI Worker reads it at every activation. |
| `bootstrap/templates/owner_role.md` | The seed Role for `w-owner`. Introduces "operator", "hiring playbook". | Strong — every fresh org gets this verbatim. |
| `CLAUDE.md` | Architectural intent. Introduces "org-graph", "scope", "Channels" (sic), "shell tool" tier. | **Strongest intent source, but drifts from code on "Channel" and "scope".** |
| `TODO.md` | Admitted bugs. "Workers should not subscribe themselves" — the `subscribe`/`invite_workers` split is wrong. | Authoritative on what's broken. |
| Go log messages, CLI flag names | Casual. `--envs`, "Dispatcher", "Broadcaster", "Spawner" — match code. | Aligned. |
| Helix-runtime sub-package (`agent/helix/`) | Imports a different vocabulary: `Project`, `Agent App`, `RepoID`, `SessionID`, `helix-specs branch`. | Internal to the helix runtime. **Does not leak into the prompt surface.** |

### Notable cross-source disagreements

1. **`CLAUDE.md` lists "Channels" as a structural primitive; the code has no Channel.** Demos and role-markdown also say "channel" for what the schema calls Stream. The `## Streams` section is the canonical name; "channel" is a leak.
2. **`CLAUDE.md` references "scope value" / "scope shapes" but `domain/grant.go:5-8` is an explicit polemic *against* a Scope field.** The two documents directly contradict each other.
3. **`tools/hire_worker.go:36` says "hire_worker does not subscribe to Channels"** — capitalised, in a comment about Streams. The author uses "Channel" and "Stream" interchangeably in code comments.
4. **`domain/worker.go:41-46` calls IdentityContent the "persona for AI, profile for a human"** — three different words for one column. The recruiter role builds "candidate profiles". The owner role talks about "identity". Pick one.
5. **`bootstrap/templates/owner_role.md` introduces "fire" as a verb** in the same sentence as hire — but no MCP tool exists for it. Closest is `hire_worker` updating positions to `[]` (vacate), which the domain anticipates (`domain/worker.go:117-118`).
6. **`prompts/templates/role.md:40` says "Default tools: pick from what the org has — typically `subscribe`, `publish`, `read_events`, `dm`."** But there's no schema-level concept of "DefaultTools" — the Role.Content `## Tools (MCP)` section is reference text the hiring manager's prompt reads (per `CLAUDE.md:20`). Code never parses it.

---

## 5. Concepts in prompts but absent from code (or vice versa)

These are the highest-signal items for the redesign.

### In prompts, absent from code

| Concept | Where it appears | What's missing in code |
|---|---|---|
| **Scope** of a Grant | `CLAUDE.md:17,18`; `demos/github-engineer/roles/software-engineer.md:217` ("scoped to the repo") | `domain.ToolGrant` has no scope field. Per-grant restrictions are talked about in CLAUDE.md but implemented only via tool-design (`grant_tool` for everyone or no-one). |
| **Channel** as a distinct structural primitive | `CLAUDE.md:19` lists it alongside Streams; role markdown uses it constantly | Doesn't exist. Always means Stream. |
| **DefaultTools / DefaultStreams** on a Role | `CLAUDE.md:20` mentions these explicitly | Not on `domain.Role`. The Role's markdown body is parsed (by the LLM, not the code) for `## Tools (MCP)` / `## Streams` sections. |
| **Firing** a Worker | `bootstrap/templates/owner_role.md:21` | No `fire_worker` tool. Closest is updating Positions to `[]` (vacate) — the domain comment in `domain/worker.go:117-118` anticipates this but no MCP tool surfaces it. |
| **Candidate / Recruiter shortlist** | `demos/newsroom/roles/recruiter.md` | Pure prompt convention — the candidates are markdown files in the Environment, not domain rows. |
| **helix-log.md** — per-Worker memory file | `agent/policy.md:21-30` | Just a file the agent writes itself. No system involvement. Yet the policy treats it as foundational. |
| **Triggers / Constraints / Files sections** of Role markdown | `prompts/templates/role.md`, all roles | Schema-free markdown. Code doesn't parse, doesn't enforce, doesn't validate. |
| **Source priority** (human-origin vs AI-origin) | `agent/policy.md:58-73` ("source_kind: human" vs "source_kind: ai") | This DOES exist in code as `Trigger.SourceKind` (`agent/spawner.go:33-37`) and is rendered into the prompt — but priority enforcement is entirely prompt-side. |
| **Hiring playbook ordering** (create_role → create_position → hire_worker → subscribes) | `bootstrap/templates/owner_role.md`, `prompts/templates/role.md:94-106` | Each step is a separate idempotent tool; the orchestration is in the prompt by design (`CLAUDE.md:20`). But there's no test that the playbook actually composes — TODO.md hints this is partly broken. |
| **"Operator"** as a noun | role.md template | Code uses "owner". |
| **"Hold" a message** ("Hold the supplier email until 'implicate supplier' is said") | `demos/manufacturing/roles/quality-bot.md:19-22` | Pure agent-side discipline. No domain concept of held/queued/draft messages. |

### In code, absent from (or invisible to) prompts

| Concept | Where in code | What prompts say |
|---|---|---|
| **TransportLocal** as a default Kind | `domain/transport.go:480` | Demos treat "local" as the unmarked default; never names it. |
| **`WorkerRuntimeState` sidecar** + the `helix` Backend label | `agent/helix/state.go`, `store/sqlite/worker_runtime.go` | Completely invisible to prompts. Yet `HiringUserID` here decides which API key the activation runs under. |
| **`WorkerProject` + per-Worker Helix project + Agent App + `helix-specs` branch** | `agent/helix/project.go` | Demos and roles say "your Environment is the current working directory" — false on the Helix runtime, where it's a git branch. |
| **`Broadcaster`** (wake-up channels for long-poll) | `broadcast/broadcaster.go` | Prompts say "the dispatcher will reactivate you" — collapsing Dispatcher + Broadcaster + Spawner into one "the runtime" actor. |
| **`workerQueue` + coalescing** of bursts into one activation | `dispatch/dispatcher.go`, `agent/prompt.go:28` (the "%d triggers have queued" line) | The agent.md policy mentions it ("most cascades resolve to a single response or to silence") but doesn't explain the mechanism. |
| **Per-Worker MCP server**, tools filtered by Grant | `server/mcp.go` | Prompts say "you have these tools" and list them by name; no awareness that the very list is gated by `grants` rows. |
| **Activation-stream subscription rules** (Worker NOT subscribed to own activation Stream) | `tools/hire_worker.go:41-46` | Prompts never mention this; could be surfaced explicitly in `agent.md`. |
| **`update_role` / `update_identity` triggering `WorkspaceSync.PublishFile`** | `agent/spawner.go:62-83` | Prompts assume edits land "live"; the file-mirror is silent infrastructure. |
| **Config / `config` CLI vs MCP split** | `config/registry.go`, `domain/config.go` | Roles never reference operational config (e.g. `transport.postmark`); the boundary lives only in `CLAUDE.md`. |
| **Two transports for the same Stream direction** (webhook in/out, email in/out) | `domain/transport.go:481-516`, `dispatch/dispatcher.go` | Roles say "subscribe on hire" without distinguishing inbound vs outbound. |
| **Conventional Message envelope fields** (`thread_id`, `in_reply_to`, `extra`) | `domain/message.go:799-810` | Some roles use them faithfully (`demos/email/roles/customer-service.md` is a masterclass), others ignore them entirely. |

---

## 6. Resolve these first

Ordered by how much architectural movement each unblock enables. Each
of these is **a naming decision plus a small ripple-edit**, not a
re-architecture — but every later refactor benefits from doing them up
front.

1. **Pick one of {Stream, Channel}. Delete the other.** `CLAUDE.md`'s
   structural-primitives list says "Workers, Positions, Roles,
   Channels, Grants, Streams" as if these were six things; they are
   five. Demos call s-… IDs "channels"; code calls them Streams. Until
   this is fixed, every conversation about pub/sub semantics has two
   words for the same concept. **Recommendation: keep `Stream`** —
   already the type, the table, the ID prefix, the `## Streams`
   section header, and the MCP tool family (`create_stream`,
   `list_streams`, `stream_members`). Edit `CLAUDE.md:19` and the
   "channel" leak in `tools/hire_worker.go:36`.

2. **Resolve "agent" overload.** It currently means (a) the AI side of
   a Worker, (b) the org-wide policy file `agent.md`, (c) the Go
   package, (d) the LLM client binary (Claude Code), and (e) Helix's
   "Agent App". Either rename `agent.md` → `worker-policy.md` and
   reserve "agent" for the LLM-client sense, or rename the domain's
   `AIWorker` → `Agent` and remove "AI Worker" from prose. Right now
   `agent/policy.md` swerves between both senses *in the same
   paragraph*. **Recommendation: keep "AI Worker" in the domain, rename
   the file to `worker-policy.md`** — pushes the overload up to the LLM
   ecosystem layer where it's unavoidable anyway.

3. **Decide whether "scope" is a real concept or a `CLAUDE.md`
   hallucination.** `domain/grant.go:5-8` is a direct rebuttal of
   `CLAUDE.md:17,18` — the latter says "scope value, scope shapes",
   the former says granularity comes from tool design. **Recommendation:
   delete the scope language from `CLAUDE.md` and align everything on
   "the only authorization primitive is `(WorkerID, ToolName)`."** If
   per-grant scoping is wanted later, give Grant a `Constraints`
   JSON-blob field; do not retrofit prose first.

4. **Promote "Identity" as the canonical name; retire "persona",
   "profile", "candidate".** They all denote the same string
   (`Worker.IdentityContent`, projected to `identity.md`, set by
   `update_identity`, returned by recruiter prompts). Fix
   `domain/worker.go:41` ("persona for AI, profile for a human") and
   teach the recruiter role to call its outputs "identities" not
   "profiles".

5. **Decide where the Role → Worker contract lives, and write it
   down.** The `## Tools (MCP)` and `## Streams` sections inside a
   Role's markdown body are referenced by the owner's prompt
   ("`grants` set to **every MCP tool listed in the Role's
   `## Tools (MCP)` section**") but the code does not parse them. This
   is **deliberate** (CLAUDE.md:20: "reference data the hiring
   manager's prompt reads, not triggers the code acts on") but it
   creates a silent contract. Options: (a) leave it prose-only and
   document the schema explicitly in design/; (b) lift the lists onto
   `domain.Role` as `DefaultTools []ToolName` + `DefaultStreams
   []StreamID` and let the hiring manager read them via a structured
   tool. Pick one and write it down.

6. **Collapse Dispatcher / EventDispatcher / server.Dispatcher into a
   single named interface in one package.** Three interface
   declarations exist (`dispatch.Dispatcher`, `server.Dispatcher`,
   `tools.EventDispatcher`) to dodge import cycles, but all three
   describe the same actor. This is purely linguistic / structural
   tidying — but it makes the architecture map readable.

7. **Make `WorkspaceSync.PublishFile` not "publish".** `publish` is
   the canonical MCP tool meaning "append an Event to a Stream";
   re-using the verb for "mirror role.md to the agent's runtime
   workspace" causes everyone reading `agent/spawner.go` to do a
   double-take. Rename to `MirrorFile` or `SyncFile`.

8. **Name the Activation as a first-class noun (even if it stays
   ephemeral).** Today `Activation` exists only in prose and as the
   string `s-activations-<workerID>`. The Spawner func type encodes it
   implicitly. Promote it: a tiny `agent.Activation` struct (just
   `{WorkerID, Triggers, StartedAt, ActivationID}`) would let
   `worker_log` events carry a real `activation_id`, would let the
   dispatcher emit a single audit row per activation, and would unblock
   the TODO item about batching ("there's quite a lag when multiple
   events are published at the same time. Each agent goes through
   events one by one. When there is a queue of events, they should be
   batched into one spawn."). Right now there is no shared term for
   "the thing being batched", so the discussion can't even be had
   precisely.

---

**Net assessment.** The domain layer's vocabulary is unusually clean
— one struct per noun, one ID type per row, enums centralised. The
**prompt layer is also clean** — the role.md template establishes a
consistent five-section schema (Tools/Streams/Triggers/Constraints/Files)
that every demo follows. **All the linguistic mess lives at the seam
between them**: `CLAUDE.md` calls things by names the code doesn't use
(Channel, scope, DefaultTools); the agent runtime layer (`agent/helix/`)
imports a separate vocabulary (Project, Agent App, helix-specs) that
prompts never see; and "agent" / "tool" / "stream" / "publish" each do
double duty across the seam. Resolving §6 items 1–3 alone would remove
most of the apparent confusion.
