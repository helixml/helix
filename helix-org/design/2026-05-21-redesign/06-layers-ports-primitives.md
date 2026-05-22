# 06 — Layers, Ports, and Primitives

Hexagonal-lens audit of helix-org as it stands on `main` (`ee0ecf976`).
The codebase was vibe-coded; the layering is real but not deliberate.
This step names what each package *actually* is, where the boundaries
leak, and the smallest moves that would restore the rule "domain owns
the ports, infrastructure implements them."

Citations: every claim points at `file.go:NN`. Where a span is meant,
it's `file.go:NN-MM`.

---

## 1. Hexagonal lens — package classification

Three tags: **D** = domain (pure types + business rules, no I/O);
**A** = application (use-cases / orchestration; depends on domain +
ports); **I** = infrastructure (talks to the outside world — disk,
HTTP, subprocess, third-party SDK). A `D/A` or `A/I` means the package
mixes two layers and needs splitting.

| Package | Today's tag | Honest reality |
|---|---|---|
| `domain/` | **D** (mostly) | Pure types, IDs, enums — `domain/worker.go`, `domain/role.go`, `domain/stream.go`, `domain/event.go`, `domain/message.go`, `domain/grant.go`, `domain/subscription.go`, `domain/tool.go`. **Exception: `domain/transport.go` (315 LOC) is infrastructure.** It does `net/url` parsing (`domain/transport.go:7`), email-address validation (`domain/transport.go:301-310`), and the discriminator-switch (`domain/transport.go:217-285`) over `TransportKind`. Per `04 §4 cut #4`, this belongs in `transports/<x>/`; only the `TransportKind` discriminator + the opaque `Config json.RawMessage` envelope should live in the kernel. |
| `store/` | **D** (port) | Pure interfaces (`store/store.go:17-80+`). Correct. The "shared kernel" port surface for persistence. |
| `store/sqlite/` | **I** | GORM + `glebarez/sqlite` (`01 §2`). Only place that imports GORM. Correct. |
| `broadcast/` | **I** | In-process pub/sub used as wake-up bus. Tiny (`broadcast/broadcaster.go`). Sits between Communication and Activation as an "edge" — it's an in-memory queue, so it's infra, but it's so minimal it could equally live as a primitive on Communication. Keep as I, expose via interface. |
| `agent/` | **D/A** mixed | The Spawner *contract* (`agent/spawner.go:64`), the `Trigger` value object (`agent/spawner.go:27-48`), `WorkspaceSync` *interface* (`agent/spawner.go:91-93`) — these are domain ports for the Activation context. `agent/prompt.go:24-94` (prompt assembly) is application — it orchestrates a use-case using only domain types. `agent/policy.go` + the embedded `policy.md` is *domain content* (the org-wide policy text). Per `04 §2.3`, this package corresponds to the Activation aggregate; the per-Worker queue currently lives in `dispatch/` instead, splitting the aggregate across two packages. |
| `agent/claude/` | **I** | `exec.Command` of the local `claude` binary (`agent/claude/spawner.go`). Implements `agent.Spawner` + `agent.WorkspaceSync`. Correct infra. |
| `agent/helix/` | **I** | REST + WebSocket against the parent Helix product. Implements `agent.Spawner` + `agent.WorkspaceSync`. Correct infra. |
| `helix/helixclient/` | **I** | 1308 LOC of REST/WS client (`01 §5.2`). Pure infrastructure. Correctly tagged but oversized (see §7). |
| `dispatch/` | **A/D/I** | `dispatch/dispatcher.go:50-64` is application (use-case: turn a publish into N activations). The per-Worker queue + coalescing logic (`dispatcher.go:66-79, 211-226`) is *domain* invariant of the Activation context — that belongs inside Activation. `emitOutbound` + `postOutbound` (`dispatcher.go:275-334`) is *infrastructure* — it does direct `http.Client.Do` and email emission. Three layers in one file, fused by the accident of "both fire on `Events.Append`" (`04 §4 cut #2`). |
| `tools/` | **D/A/I** | The mess. Most tools are pure Org-Graph or Communication use-cases (application orchestrating domain mutations through `store.Store`). But `tools/hire_worker.go:12-15` imports `agent/helix` + `helix/helixclient` directly — domain reaching into infra (`04 §4 cut #1`). `tools.EventDispatcher` (`tools/builtins.go:27-30`) is a *port* declared in the wrong package. Tools-as-MCP-handlers (the `Invoke` method) is the application layer; tools as `domain.Tool` is shared-kernel. |
| `transports/postmark/` | **I** | Postmark REST. Implements an outbound emitter + inbound HTTP handler. Correct. |
| `transports/github/` | **I** | GitHub webhook HMAC + payload parser. Inbound-only. Correct. |
| `server/` | **A/I** | `server/mcp.go` is the MCP HTTP adapter (I) over the MCP Gateway use-case (A). `server/webhook.go` is an inbound transport. `server/server.go:22-25` declares a *duplicate* `Dispatcher` interface (per `01 §4.4`, `03 §2.11`). |
| `server/chat/` | **A/I** | `server/chat/chat.go` shells out to `claude` (I). `server/chat/helix_bridge.go` imports `agent/helix` directly (`01 §6 bullet 3`, `04 §4 cut #3`) — application layer reaching into a specific runtime infra package. Should sit behind a `ChatSession` port. |
| `server/ui/` | **A/I** | HTML handlers (I) over Operator use-cases (A). 1444 LOC, 0 tests (`01 §5 table`). Mutates `store.Roles.Update` / `store.Config.Set` directly (`01 §6 bullet 4`, `02 Capability 1 pain point 3`). |
| `cmd/helix-org/` | **A** (wiring) | `cmd/helix-org/serve.go:36+` is composition root. Should be 100% wiring. Today it carries runtime-selection switch ladders (`serve.go:247-326`, `:343-437`) — that's runtime-construction logic which belongs in a `runtime.New(kind, cfg)` factory. |
| `bootstrap/` | **A** (use-case) | Owner-seeding use-case. Embeds `templates/owner_role.md` (`bootstrap/bootstrap.go:28-29`) — *that file is domain content*, not application config; the embed lives here for convenience but conceptually belongs in the Org Graph domain. |
| `prompts/` | **D/A** | `prompts/role.go:18` embeds `templates/role.md` — the prompt template shown for the `/role` slash command. Domain content + a small registry (A). |
| `config/` | **D/I** | `config/registry.go:36-68` (Specs) is a domain-shaped declaration of operational config; the registry's read/write talks to the `store.Configs` repo so it's also infrastructure-adjacent. |
| `broadcast/` (re-listed) | **I** | (See above.) |

**Honest summary.** Of the 19 packages, 6 are layer-clean (`store`,
`store/sqlite`, `agent/claude`, `agent/helix`, `helix/helixclient`,
`transports/*`). The rest mix at least two layers in one place, and
three of them — `tools/`, `dispatch/`, `server/chat/` — mix all three.
That's where the redesign has to do work.

---

## 2. Ports

A port is an interface owned by domain/application code and implemented
by infrastructure. The columns: *where declared today* (or "missing"),
*who implements*, *who calls*, and whether it's named clearly.

### 2.1 Existing ports today

| Port | Declared | Implementations | Callers | Smell |
|---|---|---|---|---|
| `agent.Spawner` | `agent/spawner.go:64` (function type) | `agent/claude/spawner.go`, `agent/helix/spawner.go` | `dispatch/dispatcher.go:56,238` | A function type, not an interface — fine, but makes it impossible to add new methods (e.g. `Cancel(workerID)`) without breaking every call site. |
| `agent.WorkspaceSync` | `agent/spawner.go:91-93` | `agent/claude/workspace.go`, `agent/helix/workspace.go`, `agent.NoopWorkspaceSync` (`spawner.go:97-102`) | `tools/update_role.go`, `tools/update_identity.go` (via `Deps.Workspace`) | Method `PublishFile` collides verbally with the `publish` MCP tool (`03 §6 item 7`). Rename `MirrorFile` / `SyncFile`. |
| `store.Roles`, `store.Workers`, `store.Positions`, `store.Streams`, `store.Events`, `store.Grants`, `store.Subscriptions`, `store.Environments`, `store.Configs`, `store.WorkerRuntimeState` | `store/store.go:17-80+` | `store/sqlite/*.go` | Everything | Clean. The only repo port that smells is `WorkerRuntimeState` (`store/store.go:59-64`) — it's a sidecar kv store invented for one specific runtime. Per `02 cross-cutting #2` it shouldn't be a first-class core port. |
| `dispatch.Dispatcher` (implicit) + `server.Dispatcher` + `tools.EventDispatcher` | `dispatch/dispatcher.go:54`, `server/server.go:22-25`, `tools/builtins.go:27-30` | `dispatch.Dispatcher` only | `tools/publish.go`, `tools/hire_worker.go`, `server/mcp.go`, UI | **Three interfaces, same actor.** Cycle-break artefact (`01 §4.4`, `03 §6 item 6`). Collapse to one. |
| `dispatch.EmailEmitter` | `dispatch/dispatcher.go:46-48` | `transports/postmark/postmark.go` | `dispatch/dispatcher.go:300` | Declared in dispatcher (consumer), implemented in transport. Fine in principle but it's an after-the-fact injection (`SetEmailEmitter`, `dispatcher.go:103`) because of a constructor cycle the comment honestly flags. Once split into `Outbox`, this becomes one entry in a `map[TransportKind]OutboundEmitter` (see §6). |
| `domain.Tool` | `domain/tool.go:440-456` | 30 tool structs in `tools/*.go` | `tools/registry.go`, `server/mcp.go` | Clean. The shared-kernel surface to the LLM. |

### 2.2 Missing ports — the next refactor wants these

| Port (proposed) | Why needed | Today's situation |
|---|---|---|
| `Runtime` | Unify `agent/claude` and `agent/helix` behind one named type, not "a `Spawner` plus a `WorkspaceSync` plus an out-of-band `WorkerProject`". | Today the two runtimes implement the same *pair* of unrelated funcs (Spawner + WorkspaceSync), and the helix runtime also adds `WorkerProject.Ensure` which has no counterpart on the claude side. There is no `Runtime` type; the two implementations agree only by convention. Lift to: `Runtime interface { Spawn(ctx, Activation) error; Workspace() WorkspaceSync; OnWorkerHired(ctx, Worker) error }` — the last method swallows the hire-time project provisioning that `tools/hire_worker.go` currently does inline. |
| `InboundReceiver` | Per `04 §2.4`. Today each transport mounts its own HTTP handler (`transports/github/github.go:137`, `transports/postmark/postmark.go:239`, `server/webhook.go:33`). The contract — "turn external payload into one `(StreamID, Message)` append" — exists in prose, not as a type. | Concrete handlers are HTTP-shaped. The port should be HTTP-agnostic: `InboundReceiver interface { Receive(ctx, payload) (StreamID, Message, error) }`. The HTTP framing stays in the transport package; the contract is hoistable for testing. |
| `OutboundEmitter` | Per `04 §2.4`. `EmailEmitter` (`dispatch/dispatcher.go:46-48`) is one instance of the shape. Webhook emit (`dispatch/dispatcher.go:314-334`) is inlined into the dispatcher. | Generalise: `OutboundEmitter interface { Emit(ctx, Stream, Event) error }`. Dispatcher holds `map[TransportKind]OutboundEmitter`; webhook is just another entry. The `switch` in `emitOutbound` (`dispatcher.go:285-305`) dies. |
| `ChatSession` | `server/chat/helix_bridge.go` imports `agent/helix` + `helix/helixclient` directly (`01 §6 bullet 3`, `04 §4 cut #3`). The owner-chat UI shouldn't know which runtime is active. | Lift the 5-method `chat.Backend` *or* create a deeper `ChatSession` that wraps "an LLM-driven conversation with MCP back-pressure" and let `claude` / `helix` both implement it. Today the 5-method `Backend` is an HTTP adapter, not a domain abstraction. |
| `ActivationLog` (sketch) | The activation transcript currently appears as a magic stream ID `s-activations-<workerID>` (`agent/prompt.go:176-178`). Per `03 §6 item 8` and `04 §2.3`, an `Activation` aggregate should own the transcript port. | Either a `MessageBus.Publish(ctx, ActivationStreamID, Event)` (currently `tools.Deps.Broadcaster` + the dispatcher), or a richer `ActivationLog.Append(ctx, ActivationID, Frame)`. Pick after deciding whether Activation is a real aggregate. |
| `IdentityProvider` (future, not now) | Auth is deferred (`CLAUDE.md` "Auth: Deferred"). Today every caller is `w-owner`. When auth lands, the MCP gateway needs a port to resolve the principal — `helixclient.WithBearerToken` (`helix/helixclient/...`) does the lift today but it's positioned as plumbing, not a contract. | Defer. Listed for completeness. |

---

## 3. LLM-SDK / external-client placement violations

This codebase has no OpenAI/Anthropic SDK import — every LLM call goes
through (a) the local `claude` CLI subprocess or (b) the parent Helix
server via `helixclient`. So the "domain importing the LLM SDK"
anti-pattern doesn't manifest as imports. It manifests as
**`os/exec`-of-claude** and **HTTP-against-Helix** appearing inside
packages that are nominally domain or application.

| Site | Behaviour | Behind a port? | Verdict |
|---|---|---|---|
| `tools/hire_worker.go:12-15` | Imports `agent/helix` and `helix/helixclient` directly. At hire time, `WorkerProject.Ensure` is called to materialise a Helix Project — an action that *only* makes sense for the Helix runtime. | **No.** This is the largest layering violation in the codebase (per `04 §4 cut #1`). Hire is an Org-Graph mutation; it should not know which runtime is running. | Violation. Move project-provisioning into a `Runtime.OnWorkerHired(Worker)` callback or a domain-event subscription. The tool body becomes pure Org-Graph. |
| `server/chat/helix_bridge.go:15-17` | Imports `agent/helix` and `helix/helixclient` directly. 944 LOC of "drive the owner's chat session against a Helix WebSocket". | **No.** The `chat.Backend` interface (`server/chat/backend.go`) exists but it's a thin HTTP-handler-shaped adapter (5 methods, all `http.HandlerFunc`), not a real chat-session abstraction. | Violation. Lift to a `ChatSession` port owned by the Operator Surface; both `chat.go` and `helix_bridge.go` implement it. |
| `cmd/helix-org/serve.go:247-326` (`buildSpawner`) | A switch over `spawner.kind` config key. `case "claude":` constructs `agentclaude.Spawner(...)`; `case "helix":` constructs `helixclient.New(...)` + validates provider/model + builds `agenthelix.Spawner(...)`. | **No.** This is the *composition root* deciding which runtime infra to wire — fine for `main`, but the duplication with `buildChatBackend` and the inline config-read-and-validate logic mean any third runtime requires three new branches across two functions. | Refactor. Push runtime selection into `runtime.NewFromConfig(ctx, cfg) Runtime`; `main` becomes wiring of already-built objects. |
| `cmd/helix-org/serve.go:343-437` (`buildChatBackend`) | Same shape as above for `chat.backend` config key. Reads `helix.url`, `helix.api_key`, `helix.org_url`, `chat.session_role`, `chat.provider`, `chat.model`, builds a `helixclient.Client`, then a `chat.HelixBridge`. | **No.** Same violation; same fix. | Refactor. |
| `cmd/helix-org/chat.go:106` | `syscall.Exec`s the local `claude` CLI for the interactive subcommand. | **Behind no port; doesn't need one.** This is a process-replacement at the top level — the binary IS the LLM client. The single-binary `helix-org chat` is supposed to be a thin shim. | OK. Leave it; document that this is intentional. |
| `agent/claude/spawner.go:124` | `exec.Command(claudeBin, ...)` per activation. | **Yes — it's already the infra side of the `agent.Spawner` port.** | OK. |
| `agent/helix/spawner.go:98+` | Opens Helix WebSocket per activation; polls for completion. | **Yes — same.** | OK. |
| `server/chat/chat.go` (whole file) | Long-lived `claude` subprocess for the owner UI. | **Half.** It's behind `chat.Backend`, but `chat.Backend` isn't a real abstraction (see §2.2 `ChatSession`). | Violation, same as `helix_bridge.go`. |
| `agent/helix/project.go:21-60` | `WorkerProject.Ensure` calls `helixclient` to provision Project + AgentApp + git repo per Worker. | **Yes**, this is firmly infra inside the helix runtime sub-package. The leak is that it's *called from outside the runtime* by `tools/hire_worker.go`. | Violation in the *caller*, not here. |

Net: three concrete leaks (`tools/hire_worker.go`,
`server/chat/helix_bridge.go`, the two `serve.go` switch ladders). Every
other LLM/external-client call site is correctly behind a port.

---

## 4. Prompt placement

The user's framing: *prompts are domain language but the call site is
infrastructure, so the prompt template lives in the domain layer and is
rendered at the boundary*. Today's reality:

| Template | Embedded in | Layer of file | Layer of content | Mismatch? |
|---|---|---|---|---|
| `agent/policy.md` (the canonical "agent.md" every AI Worker reads) | `agent/policy.go:22-23` | `agent/` is D/A mixed. | **Domain.** This is org-wide policy text — it defines what an AI Worker *is*. | No, but the embed sits next to the Spawner port. If `agent/` splits into `activation/` (D/A) + `runtime/*` (I), the policy text belongs in `activation/policy.md`. |
| `bootstrap/templates/owner_role.md` | `bootstrap/bootstrap.go:28-29` | `bootstrap/` is application (use-case). | **Domain.** This is the seed Role content for the Org Graph. The fact that it's embedded next to the seeding code is a convenience, not a layer fact. | **Yes — minor.** Move to the Org Graph domain (e.g. `org/seed/owner_role.md`), let the bootstrap use-case embed it from there. |
| `prompts/templates/role.md` | `prompts/role.go:18` | `prompts/` is D/A. | **Domain.** This is the schema for every Role's markdown body — what the `/role` slash command produces and what the LLM is expected to follow. | Borderline. The template is domain; the `Registry` is application. They're co-located, which is OK *if* `prompts/` stays a domain-content package and the registry's HTTP-binding (none today) lives elsewhere. |
| Role markdown in DB (`roles.content`) | n/a | n/a | **Data.** | OK. Roles are user-data, not embedded. |
| `agent/prompt.go:24-94` `BuildPrompt` + `renderTrigger` | `agent/prompt.go` | A (use-case orchestration) | Builds the activation prompt from `mandate` + `[]Trigger`. | Correct — application code rendering domain templates at the boundary into the infra-bound Spawner call. |

**Recommended placement** (matches `04 §6` target structure):

```
activation/
  prompt.go               # BuildPrompt, renderTrigger (A)
  policy.md   + policy.go # the agent-policy embed   (D content)
  spawner.go              # Spawner type + Trigger    (D port)
  activations.go          # Activation aggregate     (D entity)
org/
  seed/
    owner_role.md         # bootstrap content        (D content)
    owner_role.go         # //go:embed              (A glue)
prompts/
  templates/role.md       # /role template          (D content)
  registry.go             # /role + /help registry  (A)
runtime/
  claude/                 # implements Runtime port  (I)
  helix/                  # implements Runtime port  (I)
```

The rule "prompts are domain language, rendered at the infra boundary"
then holds: all four templates are domain content, each lives in the
context whose language it speaks, each is rendered by application code
just before crossing into an infra port.

---

## 5. Primitive obsession audit

Strings, `map[string]any`, and bare-stringly-typed kinds that want
real types.

| Today | Where | Should be | Why |
|---|---|---|---|
| `Event.Source string` (interpreted as `WorkerID`-or-empty), `Message.From string`, `Message.To []string` | `domain/event.go`, `domain/message.go:799-810`, `dispatcher.go:152-156, 174-182` | A `Principal` (or `Identity`) value object that wraps *either* a `WorkerID` (internal) *or* an external address (`alice@example.com`, `github:octocat`). Today the same string field holds both. | `03 §2 homonyms`, `02 cross-cutting #3`. Lookup in `dispatcher.go:153` (`store.Workers.Get(ctx, e.Source)`) silently fails for external addresses — that's by design but only because the call ignores the error. A `Principal.IsInternal()` accessor would make the branch explicit. |
| `Message.Extra json.RawMessage` (or `map[string]any` per call site) | `domain/message.go:799-810` | Typed extension points keyed by `TransportKind`: `GitHubExtra { Event, Delivery, ... }`, `EmailExtra { MessageID, References, ... }`. The discriminator is the carrying Stream's transport. | `03 §2.6` — GitHub event types are smuggled through `Message.Extra.event`. Today the agent's prompt renders the raw JSON (`prompt.go:112-115`); a typed `Extra` would let `renderTrigger` produce a richer view per transport without `prompt.go` switching on kinds. |
| `Trigger.Source domain.WorkerID` + `Trigger.SourceKind domain.WorkerKind` | `agent/spawner.go:33-40` | One `Principal` value object with an accessor `.Kind()` returning Human/AI/External. | Two related fields that must always be set together (per `dispatcher.go:152-156`), with the constraint "if Source != \"\" then SourceKind should resolve" encoded only by convention. A single value type makes the invariant a compile-time fact. |
| `ActivationStreamID(workerID) → "s-activations-" + workerID` | `agent/prompt.go:176-178` | A typed `ActivationID` (per `03 §6 item 8`). Today an activation has *no* identity beyond "the stream where its transcript lives." Promote: `Activation { ID ActivationID; WorkerID; Triggers; StartedAt; EndedAt; Outcome; TranscriptStreamID }`. | `04 §2.3`. Until Activation is a real noun, batch debugging (TODO.md item 6) and `worker_log`'s activation grouping have no anchor. |
| Transport `Config json.RawMessage` discriminated by `Kind` | `domain/transport.go:557-560`, `WebhookConfig`/`EmailConfig`/`GitHubConfig` at `:571-604` | The shape is fine; the **discriminator → impl mapping** is hand-rolled in a switch (`domain/transport.go:217-285`) and again in `dispatch/dispatcher.go:285-305`. Per `04 §4 cut #4`, this should be a registry indexed by `TransportKind`. | The current shape forces every new transport to edit `domain/transport.go` (parser/validator switch) **and** `dispatch/dispatcher.go` (emit switch) **and** the inbound HTTP mounting in `cmd/helix-org/serve.go:188-189`. Three places per transport. |
| `WorkerRuntimeState` "backend" string label | `store/store.go:59-64` (`backend string` param), `agent/helix/state.go:23` (`Backend = "helix"`) | Either drop entirely (per `03 §2.10` — "premature plural") or type as `RuntimeKind`. Today the *only* value is `"helix"`. | Either it's never going to have a second value (delete the parameter) or it's a runtime identity that should be a typed enum. |
| `tools.Deps.EnvsDir string` | `tools/builtins.go:34-46` | `EnvironmentRoot` type with `Join(WorkerID) Path`. Today `filepath.Join(deps.EnvsDir, string(workerID))` appears throughout. | Trivial typing win; eliminates one class of "is this a workerID or a path" bugs. |
| `chat.backend`, `spawner.kind` config strings | `cmd/helix-org/serve.go:243, 344` | A `RuntimeKind` enum exported by the runtime package, validated at config-set time. | Today the switch's `default:` case (`serve.go:326-328, 434-436`) is the only typo-catch — and it fires at startup, not at `helix-org config set`. |

---

## 6. Open-closed / strategy-pattern audit

Switch-over-a-kind ladders that should be a registry. Each is a place
where adding a new variant means editing core code instead of dropping
in a new file.

### 6.1 `dispatch/dispatcher.go:285-305` — `emitOutbound` switch over `Stream.Transport.Kind`

```go
switch stream.Transport.Kind {
case domain.TransportWebhook:
    ...
case domain.TransportEmail:
    ...
}
```

Two cases today (`Webhook`, `Email`); `GitHub` is deliberately
unsupported for outbound (`domain/transport.go:78-80`); `Local` is
no-op. Adding outbound Slack means editing this file. **Become:**
`map[TransportKind]OutboundEmitter` injected at startup; each
transport's package registers itself. The switch in `emitOutbound`
becomes `if emitter, ok := d.emitters[stream.Transport.Kind]; ok {
go emitter.Emit(...) }`. Webhook stops being special-cased.

### 6.2 `cmd/helix-org/serve.go:247-326` (`buildSpawner`) + `:343-437` (`buildChatBackend`)

Both switch on a config key (`spawner.kind`, `chat.backend`). Today's
two cases (`claude`, `helix`) duplicate provider/model validation, URL
reads, and applier construction. **Become:** `runtime.NewFromConfig(ctx,
cfg) (Runtime, error)` with a package-level registry
`map[RuntimeKind]Factory`; each runtime sub-package registers itself
via `init()`. Owner-chat then takes a `Runtime` (`chat.NewFromRuntime(r
Runtime)`) and the wiring file shrinks to one factory call.

### 6.3 `agent/prompt.go:35-42` `renderTrigger` switch over `TriggerKind`

```go
switch t.Kind {
case TriggerHire:    ...
case TriggerEvent:   ...
default:             ...
}
```

Only two cases today (`hire`, `event`). A third kind ("schedule"?
"manual"?) would require a third arm. **Become:** `Trigger` becomes
sealed-ish (one type per kind) and each implements `Render() string`.
This is a smaller win than the others — the surface is tight and
prompt-rendering is rare to extend — but it's the same shape.

### 6.4 `domain/transport.go:217-285` `Transport.Validate` + per-kind config getters at `:165-215`

`Validate` switches on `Kind`; `WebhookConfig()`, `EmailConfig()`,
`GitHubConfig()` are three separate methods each switching on `Kind`
internally. Per `04 §4 cut #4`, these belong in each transport's own
package. **Become:** each transport package registers a
`TransportSpec { Parse(raw) (any, error); Validate(any) error;
Inbound() InboundReceiver; Outbound() OutboundEmitter }`; the kernel
holds only `map[TransportKind]TransportSpec`. New transports add a file
under `transports/<x>/`; `domain/transport.go` stays a 50-LOC
discriminator type.

### 6.5 Implicit: three `Dispatcher` interfaces in three packages

Not a switch ladder, but the same shape — three interface declarations
because the cycle-break forced it. Collapse to one when the
`tools/hire_worker.go → agent/helix` cycle gets cut (`04 §4 cut #1`).

---

## 7. Single-responsibility audit

Five files do too many things. In order of cost:

**`helix/helixclient/client.go` (1308 LOC).** One `Client` interface
with method-groups for whoami, projects, secrets, git-files, repos,
branches, app lifecycle, models, providers, sessions, server status,
and WebSocket transcripts (`01 §5.2`). The package is correctly
infrastructure, but the file is the entire Helix-integration surface
under one interface. Sub-group by concern in *separate files*:
`client_auth.go`, `client_projects.go`, `client_repos.go`,
`client_sessions.go`, `client_models.go`. Keep the `Client` interface
if callers want one handle, but split the method-set by responsibility
so changes localise. Tests already at 544 LOC are a guide for the seams.

**`server/chat/helix_bridge.go` (944 LOC).** Per `01 §6 bullet 3`,
this file conflates: (a) HTTP/SSE fan-out (5 `*Handler` methods);
(b) Helix-session lifecycle (open WebSocket, poll, close); (c) frame
translation (claude-stream-json ↔ Helix-frame); (d) prompt rendering
(slash-command interception). The `chat.Backend` interface this
implements is itself only an HTTP shape. **Split:** lift `ChatSession`
as a real port, factor frame translation into a separate file, leave
the bridge as the thin glue between the HTTP handlers and the port.

**`server/ui/ui.go` (878 LOC).** Every HTML handler in one file —
chat, org, settings, streams, orgchart, plus four direct mutation
endpoints. Plus zero tests in the whole `server/ui/` package (`01 §5
table`). Per-page handlers should split into `chat_handlers.go`,
`org_handlers.go`, `settings_handlers.go`, `streams_handlers.go`. The
four mutation endpoints (`POST /ui/org/roles/set`,
`POST /ui/org/identity/set`, `POST /ui/streams/publish`,
`POST /ui/settings/set`) need a separate decision: either ratify that
the UI bypasses MCP, or route them through `tools.Registry.Invoke` as
`w-owner` (`04 §4 cut #6`).

**`cmd/helix-org/serve.go` (447 LOC).** Mixes (a) flag parsing,
(b) store open + bootstrap, (c) registry/dispatcher/spawner wiring,
(d) the two runtime-selection switch ladders, (e) UI handler
registration. **Split:** extract `buildSpawner` and `buildChatBackend`
into `runtime.NewFromConfig` (§6.2). `serve.go` shrinks to ~150 LOC of
wiring.

**`dispatch/dispatcher.go` (334 LOC).** Two responsibilities welded:
per-Worker activation queueing + outbound webhook/email emit
(`01 §6 bullet 9`, `04 §4 cut #2`). The `SetEmailEmitter` callback at
`:103` is honest about why constructor injection isn't possible (email
transport takes a Dispatcher for inbound) — once the Dispatcher is
split into `Dispatcher` (Activation queue) and `Outbox` (Transports
emit), the cycle goes away.

**`agent/helix/spawner.go` (421 LOC).** Not on the SRP list above but
worth noting: it does prompt-build, semaphore-take, session-ensure, WS
open, poll-for-completion, transcript-publish. Each is a step in one
use-case; the file is at the natural size for the responsibility (one
runtime's spawn pipeline). Leave it.

---

## 8. Concrete recommendations (punch list for step 8 migration)

In rough order of "smallest fix unlocks the most subsequent work."
Every item names the target file path and the line range where the
move lands.

1. **Cut `tools/hire_worker.go` off from `agent/helix` and `helix/helixclient`.** Drop the imports at `tools/hire_worker.go:12-15`. Move the conditional `WorkerProject.Ensure(...)` call body (currently around `tools/hire_worker.go:150-220`) into a new `Runtime.OnWorkerHired(ctx, Worker) error` callback. The runtime registers a handler with the dispatcher (or a new `org.Events`) at startup. **Unblocks:** cuts #1, #5, #7 from `04 §4`; collapses the three `Dispatcher` interfaces.

2. **Promote the Activation aggregate.** Add `domain/activation.go` (or `activation/activation.go` if step 8 commits to renaming the package): `Activation { ID ActivationID; WorkerID; Triggers []Trigger; StartedAt, EndedAt time.Time; TranscriptStreamID StreamID }`. Replace `agent.ActivationStreamID(workerID)` (`agent/prompt.go:176-178`) with `(a Activation) TranscriptStreamID`. Move the per-Worker queue from `dispatch/dispatcher.go:66-79, 211-226` into the Activation context. **Unblocks:** `03 §6 item 8`, the batch-debugging TODO, and clean `worker_log` activation grouping.

3. **Split `dispatch/dispatcher.go` into `Dispatcher` + `Outbox`.** Move `emitOutbound` + `postOutbound` (`dispatcher.go:275-334`) into a new `outbox/outbox.go`; replace the switch with `map[TransportKind]OutboundEmitter`. Keep the per-Worker queue + Spawner-call code in `dispatcher.go` (or move it into Activation per (2)). **Unblocks:** cut #2; open-closed for new transports.

4. **Move transport-specific parsing out of `domain/transport.go`.** `domain/transport.go:165-285` (`WebhookConfig`/`EmailConfig`/`GitHubConfig` accessors + `Validate` switch) moves to per-transport files in `transports/<x>/`. Domain keeps `TransportKind` + `Config json.RawMessage` only (~50 LOC). Each transport registers a `TransportSpec` at init. **Unblocks:** cut #4; open-closed.

5. **Lift `ChatSession` as a port.** New `chat.Session` interface in `server/chat/session.go`. Both `chat.Bridge` (claude subprocess) and `chat.HelixBridge` implement it. The 5 `*Handler` methods on `chat.Backend` (`server/chat/backend.go`) shrink to one `HTTPAdapter` that takes a `Session`. Then `server/chat/helix_bridge.go:15-17` drops the `agent/helix` import — the chat surface stops naming Helix. **Unblocks:** cut #3.

6. **Collapse the three Dispatcher interfaces.** Once (1) lands, `tools/builtins.go:27-30` and `server/server.go:22-25` can both import the same `dispatch.Dispatcher` (renamed `activation.Dispatcher` per (2)). Delete the duplicates. **Unblocks:** cut #7.

7. **Introduce `Runtime` port + factory.** `runtime/runtime.go` declares `interface { Spawn(ctx, Activation) error; Workspace() WorkspaceSync; OnWorkerHired(ctx, Worker) error }`. `runtime/claude/` and `runtime/helix/` (renamed from `agent/{claude,helix}`) implement it; each registers via `init()` into a `map[RuntimeKind]Factory`. `runtime.NewFromConfig(ctx, cfg) Runtime` replaces `buildSpawner` (`cmd/helix-org/serve.go:247-326`) and absorbs `buildChatBackend`'s runtime-side construction (`:343-437`). `serve.go` shrinks below 200 LOC. **Unblocks:** cuts #5, #6 partially.

8. **Move prompt templates into the contexts whose language they speak.** `bootstrap/templates/owner_role.md` → `org/seed/owner_role.md` (Org Graph). `agent/policy.md` → `activation/policy.md` (Activation). `prompts/templates/role.md` stays in `prompts/` since `prompts/` *is* a domain-content package. Embeds follow the files.

9. **Introduce `Principal` value object.** Replace `Event.Source string`, `Message.From string`, `Trigger.Source + Trigger.SourceKind` with one `Principal { id string; kind PrincipalKind }`. Migration: keep DB column as `source TEXT`; serialise via `Principal.String()`/`ParsePrincipal()`. `dispatcher.go:152-156` becomes one `if p.IsInternal() { ... }` branch. **Unblocks:** the bare-string identity drift across §3 of `03`.

10. **Sub-group `helix/helixclient/client.go` (1308 LOC).** Mechanical split into `client_auth.go`, `client_projects.go`, `client_repos.go`, `client_sessions.go`, `client_models.go`, keeping the `Client` interface intact. Pure file move, no behavioural change. **Unblocks:** future runtime work; localises future changes.

11. **Owner-UI MCP-vs-direct-store decision.** Either rewrite `POST /ui/org/roles/set` (`server/ui/ui.go:78`), `POST /ui/org/identity/set` (`:80`), `POST /ui/streams/publish` (`:83`), `POST /ui/settings/set` (`:76`) to invoke their MCP counterparts through `tools.Registry.Invoke` as `w-owner` *or* fix `CLAUDE.md:19` to say "every MCP-driven mutation goes through MCP; the owner UI is a privileged bypass." Pick one. Code change is small either way; the docs-vs-code drift is the bug.

12. **Rename `WorkspaceSync.PublishFile` → `MirrorFile`.** `agent/spawner.go:92`. Pure rename, but kills the worst verb-overload in the codebase (`03 §6 item 7`).

Items 1–7 are the load-bearing structural moves; items 8–12 are
clean-ups that make the result legible. Steps 7 and 8 of the redesign
(strategic patterns + migration plan) will sequence these properly.
