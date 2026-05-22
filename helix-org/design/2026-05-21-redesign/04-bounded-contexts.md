# 04 — Strategic Boundaries (Bounded Contexts)

Synthesised from steps 01–03. The goal here is to draw provisional
context boundaries that *describe the responsibilities the system
already has* and then show how today's packages cut across them.
The contexts named below are deliberately fewer and larger than the
current package layout — packages will fall inside contexts, not the
other way round.

Signals used (per the user's brief):

- **What changes together over time** — degraded here because the
  consolidation commit `ee0ecf976` flattened churn. Falling back to
  *what conceptually changes together*: e.g. adding a new TransportKind
  touches `domain/transport.go`, `dispatch/dispatcher.go`,
  `transports/<x>/`, and config — those four files are one context
  whether git knows it or not.
- **What the team intuitively thinks of as one thing** — pulled from
  `CLAUDE.md` ("structural state — Workers, Positions, Roles,
  Channels, Grants, Streams"; "Environment"; "the runtime"), and from
  the role-markdown vocabulary.
- **Where the language genuinely shifts meaning** — the homonyms in
  `03-ubiquitous-language.md §2`. Each is a context seam.
- **Core vs supporting vs generic** — for an AI product, orchestration
  + domain reasoning is the core; identity/billing/eval/tool execution
  are usually supporting or generic.

---

## 1. The seven contexts

| # | Context | Stance | One-line responsibility |
|---|---|---|---|
| 1 | **Org Graph** | Core | Who exists in the org, where they sit, what they're authorised to do. |
| 2 | **Communication** | Core | Asynchronous addressable pub/sub: Streams, Subscriptions, Events, Messages. |
| 3 | **Activation** | Core | Turning a Trigger into a finite agent run that produces a transcript. |
| 4 | **Transports** | Supporting (ACL) | Translating external systems ↔ Messages, in either direction. |
| 5 | **Agent Runtime** | Supporting (port impl) | Where an AI Worker physically executes — `claude` subprocess or Helix session. |
| 6 | **MCP Gateway** | Generic | Per-request, grant-filtered tool surface exposed to LLMs over HTTP. |
| 7 | **Operator Surface** | Generic | Operational config + the owner-facing UI/CLI (chat, settings, org chart). |

A useful frame: contexts 1–3 are *what helix-org is for*; contexts 4–5
are *how it touches the outside world and the LLM*; contexts 6–7 are
*how humans and LLMs reach in*. If the redesign only ever clarified
that 1, 2, and 3 are three separate things, most of the current
confusion would go away.

---

## 2. Context detail

### 2.1 Org Graph (core)

**Responsibility.** The structural state of the organisation: who
exists, who reports to whom, who can do what. Owns lifecycle events
(hire, fire-by-vacate, identity update, grant, revoke). Source of
truth for authorisation.

**Aggregates (target shape).**

- **Worker** (root) — `{ID, Kind, IdentityContent, Positions}`.
  Invariants: every AI Worker has at least one Position; every Worker
  has at most one Identity per row but the projection (`identity.md`)
  is regenerated.
- **Position** (root) — `{ID, RoleID, ParentID}`. Invariants: no
  cycles in `ParentID`; root has `ParentID = nil`.
- **Role** (root) — `{ID, Content}`. Content is markdown; structure
  inside is a deliberate prose/code seam (see §3 below).
- **Grant** (root or value-on-Worker — open question) —
  `{WorkerID, ToolName}`. The system's only authorisation primitive.
  CLAUDE.md's "scope" prose has no implementation and probably
  shouldn't (§03.6 item 3).

**Tools that mutate this context** (current names): `create_role`,
`update_role`, `update_identity`, `create_position`, `hire_worker`,
`grant_tool`, `revoke_tool`. Reads: `list_roles`, `get_role`,
`list_positions`, `get_position`, `list_position_children`,
`list_workers`, `get_worker`, `list_worker_grants`,
`get_worker_environment`, `get_grant`.

**Packages today.** Pieces of `domain/`, `store/sqlite/`, ~10 files
in `tools/`. The mutations are clean except for `hire_worker.go`,
which reaches into the Agent Runtime context — see §3.

### 2.2 Communication (core)

**Responsibility.** Asynchronous addressable pub/sub between Workers
and the outside world. Owns the Stream as a unit of fan-out, the
Subscription as a membership link, and the Event as a row on the log.
Every Event body is a canonical `Message` envelope — this is the
deepest, most under-stated invariant in the system (see
`03-ubiquitous-language.md §1`).

**Aggregates (target shape).**

- **Stream** (root) — `{ID, Name, Description, CreatedBy, Transport,
  Members []WorkerID, EventLog (...)}`. Subscriptions sit *inside*
  the Stream aggregate. Today they are a separate table, which is
  fine for storage, but conceptually a Subscription is a property of
  a Stream (its membership), not of a Worker.
- **Event** (entity, owned-by Stream) — `{ID, Source, Body, CreatedAt}`.
  Body is always a Message.
- **Message** (value object) — the canonical envelope
  `{From, To, Subject, Body, BodyContentType, ThreadID, InReplyTo,
  MessageID, Attachments, Extra}`. Currently lives in `domain/message.go`
  but the schema only knows about a `body TEXT` column — promote
  Message to a first-class value type with explicit accessors and
  remove the "string of JSON" feel from half the codebase.

**Tools that mutate this context**: `create_stream`, `subscribe`,
`unsubscribe`, `invite_workers` (and the TODO.md-item-1 issue —
`subscribe` shouldn't be a self-mutation; see §3), `publish`, `dm`,
`stream_members`. Reads: `list_streams`, `get_stream`,
`list_stream_events`, `read_events`, `worker_log`.

**Notable invariants** (currently unenforced):

- A Stream's `Transport.Kind` decides which `publish` paths are legal.
  `tools/publish.go:71-73` blocks publishing to GitHub-typed Streams.
  This rule belongs *inside* the Stream aggregate, not in a tool.
- The same Stream can have inbound transport, outbound transport, or
  both — `dispatcher.emitOutbound` and the transport handlers express
  this implicitly. The aggregate should make it explicit.
- DM streams are conventional auto-created per-pair Streams
  (`tools/dm.go`). The convention (`s-dm-<a>-<b>`) is invented inside
  the tool, not on the Stream aggregate. Either lift DM-naming into
  the aggregate or accept it stays a tool convention.

**Packages today.** Pieces of `domain/`, `store/sqlite/`, ~12 files
in `tools/`, all of `broadcast/`, the fan-out half of `dispatch/`.

### 2.3 Activation (core)

**Responsibility.** Turning a Trigger into a finite agent run that
produces an Activation transcript. Owns the per-Worker queue +
coalescing, the activation prompt assembly, and the Spawner *port*
(not its implementations).

**Aggregates (target shape).**

- **Activation** (root, new) — `{ID, WorkerID, Triggers, StartedAt,
  EndedAt, Outcome, TranscriptStreamID}`. **Today this isn't a
  struct** (`03-ubiquitous-language §6.8`). Promoting it is the
  single biggest leverage point: it unblocks batching debugging
  (TODO.md item 6), gives `worker_log` a real `activation_id` to
  surface, and clarifies what the dispatcher does ("schedule
  Activations") versus what the Runtime does ("run an Activation").
- **Trigger** (value object) — `{Kind, EventID?, StreamID?, Source,
  SourceKind, Message?, CreatedAt}`. Already exists; keep.

**Ports owned here (consumed by Runtime).**

- `Spawner.Run(ctx, Activation) error`.
- `WorkspaceSync.MirrorRole/Identity/Policy(workerID, content)` —
  renamed from `PublishFile` (`03 §6.7`).

**Packages today.** `agent/spawner.go`, `agent/prompt.go`,
`agent/activations.go`, `agent/policy.go`, the per-Worker queue inside
`dispatch/dispatcher.go`. Cleanly factored *except*:

- The Dispatcher mixes Activation queueing (this context) with
  outbound emit-on-publish (Transports context). Split them.
- `agent.PublishActivationEvent` is referenced in `bootstrap/` comments
  but doesn't exist as an exported symbol (`02 Capability 1, pain
  point 2`). Either export it on the Activation aggregate or stop
  referencing it.

### 2.4 Transports (supporting / anti-corruption layer)

**Responsibility.** Translate external systems ↔ canonical Message,
both directions. Each Transport is its own micro-context (the wire
protocol of one external system) but they share a contract:
inbound-Stream-append and outbound-Event-deliver.

**Per-transport sub-contexts today.**

- **Local** — no-op; events stay in-process.
- **Webhook** — generic HTTP in (`/webhooks/{streamID}`) and out
  (`outbound_url` from `WebhookConfig`).
- **Email (Postmark)** — Postmark JSON in, Postmark API out.
- **GitHub** — HMAC-verified webhook in, deliberately no out.

**Ports owned here (consumed by Communication).**

- `InboundReceiver` — turns an external payload into one
  `(StreamID, Message)` append. (Today this is shaped as HTTP handlers
  + the `webhookHandler`. Lift it to an interface.)
- `OutboundEmitter` — given `(Stream, Event)`, deliver out. (Today:
  `dispatcher.SetEmailEmitter` + an inline goroutine in
  `dispatcher.postOutbound`. Promote to an interface and let the
  dispatcher hold a `map[TransportKind]OutboundEmitter`.)

**Notable cross-cut**: `domain/transport.go` (315 LOC) lives in the
domain package but does HTTP-URL parsing and email-config validation.
The TransportKind enum + Config-blob shape live in the kernel; the
parser/validator should live in `transports/<x>/`. New Transports
shouldn't require touching `domain/`. (`01 §6 bullet 7`.)

### 2.5 Agent Runtime (supporting / port implementation)

**Responsibility.** Where the AI Worker physically runs. Two
implementations of one port:

- `agent/claude` — `exec` the local `claude` CLI per Activation, in
  the Worker's env directory.
- `agent/helix` — open a session against the parent Helix product.

This context owns the projection of canonical Worker state
(`role.md`, `identity.md`, `worker-policy.md` — renamed from
`agent.md` per `03 §6 item 2`) into whatever the runtime needs:
host files for `claude`, a `helix-specs` git branch for `helix`.

**Ports consumed.**

- `Spawner` from Activation.
- `WorkspaceSync` from Activation (called when Role/Identity is
  updated so the runtime sees fresh content).

**Cross-cuts to fix.**

- `tools/hire_worker.go:12-15` imports `agent/helix` and
  `helix/helixclient` directly. This breaks the layering — Org Graph
  mutations know which runtime is active and lazily provision Helix
  projects at hire time. Hire should be a pure Org Graph operation;
  if a runtime needs to react to a hire it should subscribe (e.g.
  to a `WorkerHired` domain event). Put `WorkerProject.Ensure` behind
  a `Runtime.OnWorkerHired` callback or an event subscription.
- `helix/helixclient/client.go` is 1308 LOC. Inside this context, the
  client is fine as-is — the surface it covers (whoami, projects,
  secrets, git files, repos, branches, app lifecycle, models,
  providers, sessions, server status, WebSocket transcript) is the
  full Helix integration. But it should be sub-grouped by concern
  (auth, projects, repos, sessions) within the Runtime context so
  changes localise.

### 2.6 MCP Gateway (generic / infrastructure)

**Responsibility.** Expose grant-filtered tools to an LLM caller over
streamable HTTP. Stateless adapter that resolves `workerID` from URL,
looks up grants, builds a per-request `mcp.Server`, dispatches tool
invocations to the registered tools.

**This is the *only* HTTP/JSON-RPC surface this context cares about.**
The HTML `/ui/*` and the transport-specific receivers are not part of
it (see §3).

**What lives here today** is clean: `server/mcp.go` (192 LOC) +
`server/server.go` (124 LOC). The only smell is that the `Dispatcher`
interface is locally re-declared (`server.Dispatcher`) to avoid an
import cycle (`01 §4.4`) — fold this into a single shared interface
exported from Activation (`03 §6 item 6`).

### 2.7 Operator Surface (generic)

**Responsibility.** Two related-but-distinct sub-surfaces.

- **Operational config** — `(Key, Value)` store + `Specs` registry.
  Bootstraps the process; never mutated by Workers; CLI-only writes.
  `config/`, `domain/config.go`, `store/sqlite/config.go`,
  `cmd/helix-org/configspecs.go`.
- **Owner UI** — HTML page handlers under `/ui/*`, the long-lived
  chat backend driving the owner's claude/Helix session, settings
  forms, org chart, streams view. `server/ui/` (1444 LOC, 0 tests)
  + `server/chat/` (2150 LOC + 404 test).

These could be split into two contexts later; today they share enough
"this is the operator's seat" intent to live together.

**Cross-cut to fix.** `server/ui/ui.go` mutates `roles`, `identities`,
`streams.events`, and `configs` directly via `store.*.Update` calls,
**bypassing the MCP Gateway** (`01 §6 bullet 4`,
`02 Capability 1 pain point 3`). `CLAUDE.md:19` claims "every read
and mutation of the org graph goes through MCP"; the owner UI is the
counter-example. Two options:

1. Honest the docs: the owner UI is server-side and *deliberately*
   bypasses MCP because it has full trust.
2. Make the owner UI call its own MCP endpoint as `w-owner`. Aligns
   surface with policy but adds a hop.

Pick one. Option 1 is the pragmatic choice; option 2 forces the
"every mutation is a tool call" invariant the docs already claim.

---

## 3. Context map (today's reality)

ASCII rendering of upstream/downstream relationships. `→` reads as
"depends on / is downstream of".

```
                                                          .─────────────────.
                                                          │  Operator       │
                                                          │  Surface        │
                                                          │  (UI + CLI +    │
                                                          │   ops config)   │
                                                          ╰──┬──────────┬───╯
                                                             │          │
              ┌──────────────────────────────────────────────┘          ▼ (bypass)
              │                                                   ┌──────────────┐
              ▼                                                   │  Storage     │
       .──────────────.                                           │ (SQLite/GORM)│
       │ MCP Gateway   │                                          └──────▲───────┘
       │ /workers/.../ │                                                 │
       │      mcp      │                                                 │
       ╰──┬───┬───┬────╯                                                 │
          │   │   │                                                      │
   ┌──────┘   │   └─────────┐                                            │
   ▼          ▼             ▼                                            │
 .────────. .──────────────. .────────────.                              │
 │  Org   │ │ Communication│ │ Activation │                              │
 │  Graph │ │  (Streams/   │ │ (Triggers/ │                              │
 │ (Wkrs/ │ │  Subs/Events)│ │ Spawner    │                              │
 │ Roles/ │ │              │ │  port)     │                              │
 │ Pos/   │ │              │ │            │                              │
 │ Grants)│ │              │ │            │                              │
 ╰───┬────╯ ╰───┬──────┬───╯ ╰─────┬──────╯                              │
     │          │      │           │                                     │
     │          │      │           ▼                                     │
     │          │      │   .──────────────.                              │
     │          │      │   │ Agent Runtime│                              │
     │          │      │   │ (claude /    │                              │
     │          │      │   │  helix-      │                              │
     │          │      │   │  client)     │                              │
     │          │      │   ╰──────┬───────╯                              │
     │          │      │          │                                      │
     │          │      ▼          │                                      │
     │          │   .──────────.  │                                      │
     │          │   │Transports│  │                                      │
     │          │   │ (ACL):   │  │                                      │
     │          │   │ webhook, │  │                                      │
     │          │   │ email,   │  │                                      │
     │          │   │ github   │  │                                      │
     │          │   ╰──┬───────╯  │                                      │
     │          │      │          │                                      │
     ▼          ▼      ▼          ▼                                      │
     └──────────┴──────┴──────────┴──────────────────────────────────────┘
                       all read+write via Storage
```

Relationships in DDD terms:

| From → To | Pattern | Notes |
|---|---|---|
| MCP Gateway → Org Graph / Communication / Activation | **Open Host Service** | Tools are the published interface. |
| Activation → Org Graph | **Customer/Supplier** | Activation reads `Worker`, `Environment`, `Grants` to build the prompt. |
| Activation → Communication | **Customer/Supplier** | Activation publishes transcript Events to `s-activations-<wid>`. |
| Activation → Agent Runtime | **Conformist via port** | Runtime conforms to Spawner + WorkspaceSync. Two impls today. |
| Communication → Transports | **Anti-Corruption Layer** | Transports translate canonical Message ↔ external payload. |
| Org Graph ↔ Communication | **Shared Kernel** | Both share `WorkerID`, `Message`'s `From/To`, and Subscription rows. Acceptable. |
| Operator Surface → all of Org Graph / Communication | **Bypass** (today) | UI mutates store directly. Either ratify (doc fix) or route through MCP. |
| Transports → Operational Config | **Conformist** | Postmark/GitHub read config keys live (`transports/github/github.go:110-119`). |

---

## 4. Where today's modules cut across these boundaries (the ugly bits)

The point of step 4 is to surface where the current layout disagrees
with the contexts above. Sorted roughly by cost:

1. **`tools/hire_worker.go` is in Org Graph but imports `agent/helix`
   + `helix/helixclient` directly** (`01 §6 bullet 2`). Hire is a
   pure structural op; the lazy "ensure a Helix Project exists for
   this Worker" should be a Runtime reaction to a `WorkerHired`
   event, not part of the tool body. **Highest-priority cross-cut.**

2. **`dispatch/dispatcher.go` mixes Activation queueing with
   Transports outbound emit** (`01 §6 bullet 9`). The per-Worker
   queue is Activation; `emitOutbound` is Transports. Today they
   share a class because both fire on `Events.Append`. Split:
   `Dispatcher` schedules Activations; `Outbox` (new) drains
   outbound emits.

3. **`server/chat/helix_bridge.go` is in Operator Surface but imports
   `agent/helix` directly** (`01 §6 bullet 3`, 944 LOC). The
   owner-chat UI shouldn't know which Runtime is active any more
   than an AI Worker does. Lift the Helix-session driver behind the
   same Runtime port; the UI then talks to "an agent chat session"
   without naming Helix.

4. **`domain/transport.go` lives in the kernel but does
   transport-specific parsing** (`01 §6 bullet 7`). Move the
   `Webhook/Email/GitHub` config parsers into each
   `transports/<x>/` package. Keep only the discriminator + the
   `Config json.RawMessage` byte slice in `domain/`.

5. **`cmd/helix-org/serve.go` carries Runtime-selection switch
   statements** (`01 §6 bullet 5`). Runtime choice is a
   port-binding decision at startup; it belongs in a thin
   `runtime.NewFromConfig(...)` factory inside `agent/runtime/`, not
   in `main`. `main` then becomes wiring of pre-built objects.

6. **Owner UI bypasses the MCP Gateway** (`01 §6 bullet 4`,
   `02 Capability 1 pain point 3`). Either ratify in docs or route
   through MCP. Currently the architecture doc states a contract the
   code does not keep.

7. **Three Dispatcher interfaces in three packages** (`01 §4.4`,
   `03 §2.11`, `03 §6 item 6`). Collapse to one interface owned by
   Activation; everyone else imports it. The cycle-break that
   justified the split goes away once `tools.hire_worker` stops
   reaching into Runtime.

8. **`bootstrap/bootstrap.go` seeds the owner but is invoked from
   `serve`, not `bootstrap`** (`02 Capability 1 pain point 1`,
   `01 §1.1`). Two different "bootstraps" exist. Rename one — the
   CLI `bootstrap` subcommand is really `helix-runtime preflight`.
   Owner-seeding stays in `serve`'s startup but should be lifted
   into Org Graph as `OrgGraph.SeedOwnerIfEmpty()` rather than a
   peer package.

9. **`server/chat/` has two parallel implementations (`chat.go` and
   `helix_bridge.go`) stapled to a thin `Backend` interface** (`01
   §6 bullet 3`). Either accept that the `Backend` interface is only
   an HTTP-surface adapter (rename it `ChatHTTPAdapter` and document
   it) or fold both impls behind a real chat-session port that
   Runtime owns.

10. **`server/ui/` has 1444 LOC of code and 0 tests** (`01 §5 table`,
    `01 §6 bullet 6`). Not a layering problem, but it means any
    refactor here is unprotected.

---

## 5. Core / supporting / generic — and what's a differentiator

For a redesign budget, where to spend effort:

- **Core (invest)**: Org Graph, Communication, Activation. These three
  are the product. The fact that all three are tangled inside one
  `tools/` package and one `domain/` package is the central problem.
- **Supporting (own deliberately)**: Transports, Agent Runtime. The
  ACL pattern is well-suited; the goal is to make each Transport and
  each Runtime *addable without editing the core* — which today is
  not true (see cross-cuts 1 and 4).
- **Generic (buy / minimise)**: MCP Gateway (use the SDK, don't grow
  it), Operational Config (a flat k/v store with a Specs registry is
  fine — resist features), Owner UI (presentational; keep it thin,
  push contracts through MCP where viable).

---

## 6. Provisional ownership table

For each context, the packages that should live inside it after the
redesign. The arrows on the right are the cross-cuts that need
cutting (numbers refer to §4 above).

| Context | Target packages | Cuts needed |
|---|---|---|
| Org Graph | `org/` (new) — folds `domain/{worker,role,position,grant,identity}.go` + `store/sqlite/{worker,role,position,grant}.go` + the ~10 structural tools | 1 |
| Communication | `comms/` (new) — folds `domain/{stream,subscription,event,message,transport-discriminator}.go` + `store/sqlite/{stream,subscription,event}.go` + `broadcast/` + the ~12 messaging tools | 2, 4 |
| Activation | `activation/` — folds `agent/spawner.go`, `agent/prompt.go`, `agent/activations.go`, `agent/policy.go`, plus the per-Worker queue from `dispatch/` | 2, 7 |
| Transports | `transports/<x>/` — each transport owns its config parser + handlers + emitter | 4 |
| Agent Runtime | `runtime/{claude,helix}/` — `agent/claude`, `agent/helix`, `helix/helixclient`; runtime-selection factory | 1, 3, 5 |
| MCP Gateway | `mcp/` (renamed from `server/mcp.go`) | 7 |
| Operator Surface | `operator/{config,ui,chat}/` | 6, 8, 9, 10 |

---

## 7. What this gives us for step 5

The next step (Tactical Patterns) is now well-scoped: each context above
gets its own pass over entities / value objects / aggregates / domain
events / invariants. The most interesting ones to attack first will be:

- **Worker (Org Graph)** — separating Worker-as-domain-concept from
  Worker-as-runtime-implementation (the "agent" homonym from
  `03 §2.1` lives here).
- **Stream (Communication)** — making the Subscriptions-set + the
  Event-log + the Transport-binding a real aggregate, with the
  publish-rule invariant ("can't publish to a GitHub stream") inside
  it.
- **Activation (new aggregate)** — promoting the activation transcript
  out of "string convention for a Stream ID" into a real entity with
  a lifecycle.
- **Message (value object)** — making `Event.Body` a typed Message
  instead of a `body TEXT` column with implicit JSON.
- **Tool (anti-corruption layer concept)** — the curated semantically-
  meaningful interface to the LLM. Today's `tools/` package collapses
  three contexts' tools into one registry; splitting registries per
  context is on the table.

These five items, plus the cross-cuts in §4, are what step 5 should
expand on.
