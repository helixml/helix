# 05 — Tactical Patterns Inside Each Context

Per-context pass at the DDD building blocks (entities, value objects,
aggregates, domain events, services), with explicit invariant gaps and
their target home. Every claim is cited to `file.go:NN` relative to
`/home/phil/helix/helix-org/`. Skeptical-staff-engineer voice: where
today's code does the right thing, I say "keep". Where it is propped up
by an LLM following prose in `role.md`, I name the prose and propose
either a code-side invariant or an honest "stay social" call.

The seven contexts come from `04-bounded-contexts.md §1`. Three
agent-specific wrinkles — working memory, tools-as-ACL, orchestration —
are pulled into §8 so they don't get diluted across seven sections.

---

## 1. Org Graph

The structural state of the org. Currently it's the only context whose
DB schema and domain types actually agree.

### 1.1 Entities

| Name | Today | Target shape | Lifecycle / transitions |
|---|---|---|---|
| **Worker** | `domain/worker.go:51` (`Worker` interface; `HumanWorker`, `AIWorker` impls at `:61-114`). Row in `workers` table (`store/sqlite/worker.go:25`). | Keep the interface. Push `Kind` into a sealed enum (it is one, today as a `string`-typed enum at `domain/worker.go:10-15`). | `Created` (= hired or seeded) → `IdentityUpdated*` → `Vacated` (= positions emptied; soft delete anticipated by `domain/worker.go:117-118` but no `fire_worker` tool exists, per `03 §5`). |
| **Position** | `domain/position.go:146-150`. Row in `positions`. | Keep. | `Created → Reassigned (RoleID swap) → Deleted`. Today `ParentID` mutation is allowed without cycle check — see invariants below. |
| **Role** | `domain/role.go:192-197`. Row in `roles`. | Keep. **Content is markdown** — the Role aggregate's invariant is "Content parses as Role-Markdown" — which today is a prose schema (`prompts/templates/role.md`) the LLM honours, not parsed code. Live with that, but write the schema down (see §8.3). | `Created → ContentUpdated*`. No deletion path (no `delete_role` tool). |
| **Grant** | `domain/grant.go:327-331` (`ToolGrant`). Row in `grants`. | Borderline entity/VO — see §1.3. | `Issued → Revoked`. |

### 1.2 Value objects

| VO | Today | Primitive obsession to kill |
|---|---|---|
| `WorkerID` | typed string `domain/id.go:953`. | Already a type — keep. But the **naming convention** (`w-<firstname>`) is enforced socially in `tools/hire_worker.go:58-67` prose, not by code. Either lift convention into a `NewWorkerID(slug string) (WorkerID, error)` constructor or accept the slug-fallback at `tools/hire_worker.go:139` as the documented escape hatch. |
| `PositionID`, `RoleID`, `GrantID`, `StreamID`, `EventID`, `ToolName` | typed strings `domain/id.go:951-957`. | Keep. Six aliases for `string`, all load-bearing in `grants.tool_name`. |
| `WorkerKind` | typed string `domain/worker.go:10-15` (`"human"` / `"ai"`). | Promote to a sealed enum with exhaustive `switch`. Today the dispatcher branches at `dispatch/dispatcher.go:166`, the trigger renders `source_kind` at `agent/prompt.go:90`, and the bootstrap mints human at `bootstrap/bootstrap.go:88-96`. Three sites; one missed branch silently degrades. |
| `IdentityContent` | bare `string` field on Worker (`domain/worker.go:41-46`). Projected to `identity.md` at activation. | Wrap as `Identity` value type — markdown body + last-updated. The current name fight ("persona" / "profile" / "identity" — `03 §3`) is downstream of it being a raw string. |
| `(WorkerID, ToolName)` Grant tuple | composite primary key with a separate `GrantID` synthetic | Reasonable today. If the `Scope` debate is reopened (`03 §5` item 1), this becomes a real VO: `Grant{WorkerID, ToolName, Constraints}`. |

### 1.3 Aggregates

- **Worker (root).** Contains Identity (VO), its set of Position links,
  and — if we lean to "Grant as VO" — its set of Grants. The trade-off:
  treating Grants as part of the Worker aggregate makes `grant_tool` /
  `revoke_tool` mutations naturally serialised through Worker; treating
  them as their own aggregate (which today's `grants` table does) is
  fine for storage but loses the invariant locality. **Recommendation:
  Grant stays its own row in storage but lives *logically* inside the
  Worker aggregate** — `grant_tool` mutates Worker, returns the new
  Grant rows. Today `tools/grant_tool.go` and `tools/revoke_tool.go`
  mutate the `grants` table without ever loading a `Worker`, which is
  fine for SQLite but means **the invariant "Workers cannot hold a
  grant whose `ToolName` is not in the registry" is enforced lazily** —
  `server/mcp.go:91-102` silently skips unknown grants. That should be
  a constraint at issue time, not a silent skip at request time.

- **Position (root).** Sits beside Worker because Positions can be
  reassigned independently. The aggregate's invariants are
  acyclic-`ParentID` and root-uniqueness (`p-root` is conventional but
  not enforced, `bootstrap/bootstrap.go:80-86`).

- **Role (root).** Independent. The "Content is the schema" point
  matters: the Role aggregate must own whatever validation we want
  ("must contain a `## Tools (MCP)` section", "must declare at least
  one Trigger") if we ever lift it from prose. Today: no validation,
  per `prompts/templates/role.md`. Recommend leaving prose, but write
  the schema as a design doc — the asymmetry must be acknowledged.

### 1.4 Domain events (proposed; none exist today)

| Event | Implicit emitter today | Implicit listeners today |
|---|---|---|
| `WorkerHired` | `tools/hire_worker.go:170` (`Workers.Create`) + `:220-222` (`Dispatcher.DispatchHire`) | The dispatcher (queues a `TriggerHire`). And `agent/helix/project.go` lazily ensures a Helix Project — today via direct import (`01 §6 bullet 2`), should be via event subscription. |
| `IdentityUpdated` | `tools/update_identity.go` → `Workers.Update` → `WorkspaceSync.PublishFile` (today inlined). | Runtime backends mirror the file (`agent/spawner.go:62-83`). |
| `RoleUpdated` | `tools/update_role.go` (same shape). | Same. |
| `PositionCreated`, `PositionReassigned`, `PositionVacated` | Position tools. | Nothing today; surfacing for org-chart UI consumers. |
| `GrantIssued`, `GrantRevoked` | `tools/grant_tool.go`, `tools/revoke_tool.go`. | Nothing reacts today; the per-request MCP rebuild (`server/mcp.go:67`) re-reads on every call so it's eventually-consistent for free. |
| `WorkerVacated` | Anticipated by `domain/worker.go:117-118`; no tool. | Would let the dispatcher reap the per-Worker queue at `dispatch/dispatcher.go:74`. |

Domain events are worth promoting **only if** the cross-cuts in `04 §4`
(items 1 and 7) are tackled — that's the whole point. If Runtime
provisioning stays inside the hire tool, events buy nothing.

### 1.5 Domain services

None warranted. Hiring, granting, revoking, updating identity — all
naturally belong on Worker. The current "shape" of one tool per
mutation is fine; the **tool** is the application service. Don't
introduce a separate domain-service layer.

### 1.6 Invariants that ought to hold but currently don't

1. **A hired AI Worker has at least one Position.** Today `hire_worker`
   takes positions inline (`tools/hire_worker.go:117-145`) but does not
   reject empty. Belongs on the `Worker` aggregate's constructor.
2. **Position graph is acyclic.** No check anywhere; would belong on
   `Position` aggregate (or on a `PositionGraph` domain service).
3. **Grants are bundled with hire.** Per `tools/hire_worker.go:31-34`
   and `bootstrap/templates/owner_role.md:58-63`, granting *after* hire
   means the new MCP session has stale tools. Encoded today only as
   prose in the owner Role and a comment in the hire tool. Should be:
   `hire_worker` rejects when `Grants` is empty AND the Role declares a
   non-empty `## Tools (MCP)` section — but that requires parsing the
   Role markdown (see §8.3 below).
4. **Grants only reference registered ToolNames.** Today silently
   skipped (`server/mcp.go:91-102`). Move the check to `grant_tool` —
   reject at issue time.
5. **WorkerID has a single, validated shape.** Today the convention is
   social prose (`tools/hire_worker.go:58-67`), with a UUID fallback
   (`:139`). Either enforce or document the fallback.
6. **Worker has at most one Identity.** Schema guarantees this (single
   column), so it's enforced today.

---

## 2. Communication

Pub/sub. The deepest underspecified place: everything on a Stream is a
Message envelope (`domain/message.go:799-810`), but the storage type is
`event.body TEXT` (`domain/event.go:286-292`). The whole codebase
implicitly trusts that the JSON in there is a Message.

### 2.1 Entities

| Name | Today | Target |
|---|---|---|
| **Stream** | `domain/stream.go:235-242` + row in `streams`. Carries `Transport`. | Aggregate root. The Stream owns its Subscriptions and Events (`§2.3`). |
| **Subscription** | `domain/subscription.go:358-362`, composite PK `(WorkerID, StreamID)`. | Inside the Stream aggregate as `Membership`. Today an independent table — that's fine for storage; conceptually it belongs to a Stream. |
| **Event** | `domain/event.go:286-292`. Has `ID, StreamID, Source, Body, CreatedAt`. | Entity owned by Stream. Body **becomes** a `Message` VO (not a `string`). |

### 2.2 Value objects (the killer section)

| VO | Today (primitive obsession) | Proposed |
|---|---|---|
| **`Message` envelope** | Declared as a struct at `domain/message.go:799-810`, but stored as opaque JSON in `event.body TEXT`. Every consumer parses defensively (`dispatch/dispatcher.go:137-141` falls back to a synthetic `{Body: e.Body}` when parsing fails). | First-class VO on `Event.Body Message`. Marshal at write, unmarshal at read — once. Kill the synthetic-fallback path. The parse-failure branch in the dispatcher is exactly where this leak shows up. |
| **`MessageHeaders`** sub-VO (`From, To, Subject, ThreadID, InReplyTo, MessageID`) | Flat fields on `Message`. | Group into a sub-VO; lets transports map provider-native headers (Postmark `MessageID`, GitHub `delivery_id`) onto a single typed slot. Today `MessageID` is set from a string in `transports/postmark/postmark.go` and from nothing in webhook (`server/webhook.go:70-77`). |
| **`Attachment`** | `domain/message.go:817-821`. URL-only. | Keep. Note the comment in `03 §1` — load-bearing once object storage lands. |
| **`Source`** (event-level) vs **`Message.From`** (envelope-level) | Two separate `string`s (`domain/event.go:286-292`, `domain/message.go:799-810`). Inbound transports set `Source=""` and `From=<external>` (`server/webhook.go:70-77`, `transports/github/github.go:212-219`); in-org publishes set both. Roles must know which to read (`02 cross-cut 3`). | Replace with a single `Sender` VO: `Sender { Internal *WorkerID; External *string; Kind SenderKind }`. The "is this from an AI?" check at `dispatch/dispatcher.go:148-156` becomes a method, not a four-step lookup. |
| **`TransportConfig`** | A `json.RawMessage` with a `TransportKind` discriminator (`domain/transport.go:101`, four typed parsers at `domain/transport.go:475-541`). | Promote to a sum type via interface + per-kind structs. Move per-kind parsers into `transports/<kind>/` packages (`04 §4` item 4). |
| **`StreamID` conventions** (`s-activations-<workerID>`, `s-dm-<a>-<b>`) | Built by string concat in `agent/prompt.go:176` and `tools/dm.go`. | Either lift to constructors on the Stream aggregate (`Stream.ActivationStreamID(WorkerID)`, `Stream.DMStreamID(a,b)`) or accept they're tool conventions and stay in tools. Currently they live in *two* places — that's the problem. |

### 2.3 Aggregates

- **Stream (root).** Owns Memberships (Subscriptions) and the EventLog
  (Events). Invariants the root enforces in target shape:
  - `publish(Stream, Message, Sender)` rejects when `Stream.Transport.Kind`
    forbids local publishes — today this is at `tools/publish.go:71-73`
    for the GitHub case. **Move the rule onto the aggregate** so
    every caller (the publish tool, the dm tool, and the bootstrap
    seed) gets it.
  - `subscribe(Stream, Worker)` is idempotent — today at
    `tools/subscribe.go:51-54`. Already idempotent; just relocate.
  - **DM Streams have exactly two members** — today an invariant of the
    `dm` tool's convention (`tools/dm.go`), not the Stream aggregate.
    If we keep DM as "a Stream with two Memberships", lift the
    invariant.
  - **A Worker's activation Stream has exactly one member: the hiring
    Worker.** Today enforced at `tools/hire_worker.go:41-46` and re-
    asserted by the bootstrap (`bootstrap/bootstrap.go:163-171`). The
    "Worker NOT subscribed to own activation Stream" rule lives in two
    files; should live on Stream.

### 2.4 Domain events

| Event | Implicit today |
|---|---|
| `StreamCreated` | `tools/create_stream.go`. |
| `MemberJoined` / `MemberLeft` | `tools/subscribe.go`, `tools/unsubscribe.go`, `tools/invite_workers.go`. |
| `EventAppended` | `tools/publish.go:100-101` + `dispatch/dispatcher.go:128`. This is **the** load-bearing event — the dispatcher listens, the broadcaster listens. |
| `TranscriptSegmentAppended` | `agent/activations.go:31` (`PublishActivationEvent`) — a *deliberately* second-class append that does NOT fire `Dispatch` (`02 Capability 3 pain point 1`). Today it bypasses the dispatcher to avoid infinite recursion. If we model events properly, this is a distinct domain event from `EventAppended` and the bypass is explicit. |

### 2.5 Domain services

- **Broadcaster** (`broadcast/broadcaster.go:11-22`) — in-process
  wake-up channels for long-polls. It is genuinely a domain service:
  it doesn't belong on any one Stream and it doesn't carry state worth
  persisting. Keep as-is.

### 2.6 Invariants that ought to hold but currently don't

1. **`Event.Body` parses as a Message.** Today the dispatcher
   tolerates parse failure (`dispatch/dispatcher.go:137-141`). Should
   be a write-time invariant on the Event entity.
2. **A Stream of kind `github` rejects local publishes.** Today
   enforced in the publish tool (`tools/publish.go:71-73`); should be
   on the Stream aggregate.
3. **Inbound POST idempotency.** `02 Capability 5 pain point 3` — no
   delivery ID. Belongs on Stream-with-Transport (transports supply a
   provider-native delivery ID; the aggregate dedupes). Today not
   implemented anywhere.
4. **A Worker cannot subscribe itself to its own activation Stream.**
   Anticipated by `hire_worker` not subscribing the new Worker
   (`tools/hire_worker.go:41-46`), but `tools/subscribe.go` does not
   reject the self-loop — an AI Worker could subscribe itself and
   start an infinite cascade. Belongs on Stream's `subscribe`.
5. **Outbound emit only fires for in-org-published events.** Today
   `dispatcher.emitOutbound` checks `e.Source == ""` (`dispatch/dispatcher.go:276`)
   to skip re-emitting inbound events. The check is right; its home is
   wrong — it lives in the dispatcher, not on the Stream.

---

## 3. Activation

Where the biggest leverage hides. Today there is **no `Activation`
type** (`03 §6` item 8). The concept is implicit in:

- the `Spawner` function signature (`agent/spawner.go:64`),
- the per-Worker `workerQueue` in the dispatcher (`dispatch/dispatcher.go:74`),
- the convention `s-activations-<workerID>` (`agent/prompt.go:176`),
- the inline `=== activation: hire ===` / `=== exit: ok ===` markers
  in the transcript (`agent/claude/spawner.go:139,182`).

### 3.1 Entities

| Name | Today | Target |
|---|---|---|
| **Activation** | Does not exist as a struct. | New aggregate root: `Activation { ID, WorkerID, Triggers []Trigger, StartedAt, EndedAt *time.Time, Outcome ActivationOutcome, TranscriptStreamID }`. Persisted? Yes, even as a thin row — it would give every transcript event a real `activation_id` to filter by, which unblocks `worker_log` (`tools/worker_log.go`) returning per-activation slices instead of the union-of-streams firehose (`02 Capability 4 pain point 1`). |

### 3.2 Value objects

| VO | Today | Target |
|---|---|---|
| **Trigger** | `agent/spawner.go:27-48`. Tagged union via `Kind`. | Keep, but tighten: `Trigger.Source` is currently `domain.WorkerID` (empty when external) — replace with the `Sender` VO from §2.2 so external triggers carry external sender identity in a typed slot. |
| **TriggerKind** | enum `"hire"` / `"event"` (`agent/spawner.go:14-22`). | Keep. Asymmetric to `Worker.Kind` (no `"vacate"`/`"fire"` — see `02 cross-cut 3`). Either accept the asymmetry or add lifecycle triggers when `fire_worker` lands. |
| **ActivationOutcome** | The string `=== exit: ok ===` / `=== exit: error: ... ===` published as the last transcript event (`agent/claude/spawner.go:182`). | Lift to `ActivationOutcome { Status, Error, ExitedAt }`. Replaces the marker-string convention with a typed slot. |
| **Mandate** | The static "what is your job" text — `Role.Content`, plus `Identity.Content`, plus `agent.Policy` (`agent/policy.go:22-23`). Assembled into the prompt at `agent/prompt.go:24`. | The assembly is a domain service; the three sources are already VOs in other contexts. No new VO. |
| **TranscriptSegment** | An `Event` on `s-activations-<workerID>` with body shape `assistant: …` / `tool_use foo: …` / `tool_result: …`. The format is invented in `agent/claude/spawner.go:368-416` and `agent/helix/spawner.go:371-385` and read by `tools/worker_log.go`. | Promote to a real VO `TranscriptSegment { Kind, Speaker, Body, ToolCall *ToolCall, ToolResult *ToolResult }`. Currently the string-parsing in `worker_log` is the consumer; one new field on a struct removes a parser. |

### 3.3 Aggregates

- **Activation (new root).** Holds the Triggers it was woken with,
  the transcript-stream pointer, the outcome. Invariants:
  - At least one Trigger (an Activation with empty Triggers makes no
    sense; today the Spawner can technically be called with `[]`).
  - Outcome is set exactly once (today: the `=== exit: ===` marker is
    appended exactly once by the Spawner; not enforced by anything but
    the Spawner code path).
  - `TranscriptStreamID == ActivationStreamID(WorkerID)` —
    deterministic from WorkerID (`agent/prompt.go:176`).

  The Activation is **ephemeral by lifecycle but persistent by audit**.
  Whether to store it: yes. A 32-byte row per agent run, indexed by
  WorkerID + StartedAt, unlocks audit and the TODO-item-2 polling
  story.

### 3.4 Domain events

| Event | Implicit emitter today |
|---|---|
| `ActivationScheduled` | `dispatch/dispatcher.go:191-205` (`enqueue`). |
| `ActivationStarted` | `dispatch/dispatcher.go:231` (`activate`) + `agent/claude/spawner.go:139`. |
| `ActivationSegmentAppended` | `agent/activations.go:31` (`PublishActivationEvent`) — already a real function, just not framed as an event. |
| `ActivationCompleted` | `dispatch/dispatcher.go:211-216` (loop sees pending empty) + the `=== exit: ===` marker. |

### 3.5 Domain services

- **Spawner port** — `agent/spawner.go:64`. Single-method function
  type. Keep. The two implementations (`agent/claude/spawner.go`,
  `agent/helix/spawner.go`) live in §5.
- **WorkspaceSync port** — `agent/spawner.go:91-93`. Rename
  `PublishFile` → `MirrorFile` per `03 §6` item 7 (the `publish` verb
  is already taken).
- **PromptBuilder** — `agent/prompt.go:24` (`BuildPrompt`). Already a
  pure function; that's the right shape.
- **CoalescingQueue** — the per-Worker queue inside
  `dispatch/dispatcher.go:74-78`. This is genuinely a domain service:
  it expresses the "bursts coalesce into one Spawner call" rule
  (`02 Capability 3 pain point 2`) and that rule isn't a property of
  any single Activation. Extract from the Dispatcher.

### 3.6 Invariants that ought to hold but currently don't

1. **Every hired AI Worker has an Activation Stream.** Today
   `tools/hire_worker.go:199-203` creates it; bootstrap creates one
   for the owner (`bootstrap/bootstrap.go:149-164`). Two code paths
   do it; neither is "the" enforcement site. Move to Worker creation.
2. **Activation segments only get appended by the Activation owner.**
   Today: `PublishActivationEvent` is package-private to `agent`
   (`agent/activations.go:31`) so it's de-facto enforced. Should be an
   aggregate method.
3. **Activation does not re-enter Dispatch.** The transcript-publish
   path deliberately bypasses Dispatch (`02 Capability 3 pain point
   1`); the rule is encoded by *not calling* `Dispatch`. Should be
   `Activation.AppendSegment(...)` that explicitly does not fire an
   `EventAppended` domain event — make the asymmetry typed.
4. **Hire activation produces a one-time setup, then the Worker is
   ready.** Today no signal — owner polls `worker_log` for `=== exit:
   ok ===` (`02 Capability 2 pain point 3`). With a real Activation
   aggregate, the hiring caller can subscribe to `ActivationCompleted`
   for `activation_id=<the one returned by hire_worker>`.

---

## 4. Transports

ACLs to the outside world. Each is a small sub-context; they share two
ports.

### 4.1 Entities

None internal to the transport context. A transport is **stateless** —
it owns parsing config blobs and translating payloads. State (delivery
IDs, retries) does not exist today. If retries land, we'd want a
`DeliveryAttempt` entity.

### 4.2 Value objects

| VO | Today | Target |
|---|---|---|
| `TransportKind` | enum `domain/transport.go:18,475-541`. | Keep. |
| `WebhookConfig` / `EmailConfig` / `GitHubConfig` | typed structs `domain/transport.go:571-604`. | Move into each `transports/<x>/` package — they live in the kernel today purely because the discriminator does (`04 §4` item 4). |
| `WebhookDeliveryHeaders` (`X-Helix-Stream`, `X-Helix-Event`) | hard-coded in `dispatch/dispatcher.go:314-334`. | Lift to a VO; the inbound side at `server/webhook.go:33` doesn't read them today but should (idempotency). |
| `OutboundEnvelope` | inline `(targetURL, eventBody)` in `dispatcher.postOutbound`. | Real VO. |

### 4.3 Aggregates

A transport is closer to a **domain service** than an aggregate — see
§4.5. The Stream aggregate owns the `Transport` value (`domain/stream.go`);
the transport-as-actor sits outside aggregates.

### 4.4 Domain events

| Event | Today |
|---|---|
| `InboundReceived` | `server/webhook.go:33`, `transports/github/github.go:137`, `transports/postmark/...`. Each transport currently does its own `Events.Append + Broadcaster.Notify + Dispatcher.Dispatch`. Should hand a single `InboundReceived` to Communication and let Communication do the rest. |
| `OutboundEmitted` / `OutboundFailed` | inline goroutine in `dispatch/dispatcher.go:300-334`. No retry, just logging on 5xx (`02 Capability 3 pain point 3`). With events: `OutboundFailed` becomes the retry trigger. |

### 4.5 Domain services (ports)

- **`InboundReceiver`** — given an external payload + headers,
  produces `(StreamID, Sender, Message)`. Today shaped as HTTP
  handlers in each transport package.
- **`OutboundEmitter`** — given a `Stream` + `Event`, deliver to the
  outside world. Today: `dispatcher.SetEmailEmitter` (`dispatch/dispatcher.go:103`)
  + an inline goroutine for webhook (`:300`). Two transports use
  setter injection, two don't. Promote to one `map[TransportKind]OutboundEmitter`.

### 4.6 Invariants that ought to hold but currently don't

1. **Outbound never fires for inbound-origin events.** Today via the
   `Source == ""` check at `dispatch/dispatcher.go:276`. Right rule,
   wrong home (lives in the dispatcher, not the transport). Move to
   the Stream/Communication boundary.
2. **Inbound deliveries are idempotent.** Not enforced anywhere
   (`02 Capability 5 pain point 3`). The right home is on the inbound
   side of each transport — provider gives us a delivery ID; the
   transport dedupes against a `transport_deliveries` table keyed by
   `(kind, delivery_id)`.
3. **Each transport's config validates on write, not on first
   delivery.** Today validation is in `domain/transport.go:217` and
   GitHub re-reads config on every delivery (`transports/github/github.go:110-119`).
   Splitting `domain/transport.go` per `04 §4` item 4 keeps the
   validators near the parsers.

---

## 5. Agent Runtime

The two Spawner implementations — `claude` (local subprocess) and
`helix` (Helix chat session). Conformist to the Activation context's
ports.

### 5.1 Entities

| Name | Today | Target |
|---|---|---|
| **WorkerRuntimeState** | `agent/helix/state.go:65-71` + row in `worker_runtime_state`. Holds `ProjectID, AgentAppID, RepoID, SessionID, HiringUserID` keyed by `(WorkerID, Backend, Key)`. | Sidecar — keep, but bound to the **helix runtime sub-context**. Today the table is generic (`Backend` is a column) but `Backend == "helix"` is the only value (`agent/helix/state.go:23`, `03 §1` "Backend (runtime)"). The premature plural can stay as a single typed table per backend; today's flat KV is fine. |

### 5.2 Value objects

| VO | Today | Target |
|---|---|---|
| **Backend label** | bare string (`agent/helix/state.go:23`). | Typed `RuntimeKind` — there are exactly two values today (`"claude"` / `"helix"`); the chat-backend interface (`server/chat/backend.go`) reuses the word for a third thing (`03 §2.10`). Disambiguate. |
| **MCPConfig** | written as JSON in `agent/claude/spawner.go:276-296`; inlined as JSON in `cmd/helix-org/chat.go:56-67`. | One VO, two sites. |
| **CommandSpec** (the claude argv) | hard-coded array in `agent/claude/spawner.go:124-128` and `cmd/helix-org/chat.go:72-104`. | Same — extract. |
| **ProjectRef** (helix-side) | `{ProjectID, AgentAppID, RepoID}` stored in `WorkerRuntimeState`. | Group into a sub-VO. |

### 5.3 Aggregates

None *inside* this context — Runtime is a port implementation, not a
domain in its own right. The `WorkerRuntimeState` row is a sidecar
projection of the Worker aggregate.

### 5.4 Domain events

- Listens for `WorkerHired` (today by direct import — `01 §6 bullet 2`,
  `04 §4` item 1).
- Listens for `IdentityUpdated` / `RoleUpdated` (today via
  `WorkspaceSync.PublishFile`).
- Emits `RuntimeProvisioned` (helix only — when `ProjectApplier.Ensure`
  succeeds). Today this is just a side-effect of the first activation.

### 5.5 Domain services

- **Spawner impl** (`agent/claude/spawner.go`, `agent/helix/spawner.go`).
- **ProjectApplier** (`agent/helix/project.go:21-60`) — helix-only.

### 5.6 Invariants that ought to hold but currently don't

1. **`hire_worker` is a pure Org Graph mutation.** Today it imports
   `agent/helix` directly (`tools/hire_worker.go:12-15`, cited in `04
   §4` item 1). Should be: hire emits `WorkerHired`; the helix runtime
   listens. Highest-priority cross-cut.
2. **A given Worker activates against exactly one runtime at a time.**
   Today the runtime is chosen at startup by `cmd/helix-org/serve.go`
   and applied to every Worker. There is no per-Worker override; that
   property should be stated and (probably) kept.
3. **`server/chat/helix_bridge.go` does not know the runtime kind.**
   Today it imports `agent/helix` directly (`04 §4` item 3). Same fix
   as invariant 1 — make Runtime a port the chat surface consumes
   without naming.

---

## 6. MCP Gateway

The HTTP/JSON-RPC adapter to the LLM. Stateless, per-request rebuild
(`server/mcp.go:67`).

### 6.1 Entities

None. Every per-request `mcp.Server` is throwaway.

### 6.2 Value objects

| VO | Today | Target |
|---|---|---|
| **Invocation** | `domain/tool.go:430-433` — `(Caller Worker, Args json.RawMessage)`. | Keep. The "Args is raw JSON" is correct (the schema lives on the Tool); no need to box. |
| **GrantSet** | derived per-request from `s.store.Grants.ListByWorker(workerID)` (`server/mcp.go:80`). | Optional VO. If we want "the set of tools this Worker can see" to be reusable across MCP, UI, and audit, lift it. Today three places independently materialise it. |

### 6.3 Aggregates

None — this is a generic adapter.

### 6.4 Domain events

`ToolInvoked` could be useful for audit but is purely instrumentation;
not core to anyone.

### 6.5 Domain services

- **Tool registry** (`tools/registry.go`). Genuine domain service.
  Single instance per process; lookups by `ToolName`. Splitting it
  per-context is the §8.2 discussion.

### 6.6 Invariants that ought to hold but currently don't

1. **Unknown ToolNames in `grants` are rejected at issue time.**
   Today silently skipped at request time (`server/mcp.go:91-102`). See
   §1.6 item 4 — this is the same invariant viewed from the request
   side.
2. **There is exactly one `Dispatcher` interface.** Today three
   (`dispatch.Dispatcher`, `server.Dispatcher`, `tools.EventDispatcher`
   — `03 §2.11`, `04 §4` item 7). Pure structural tidying; the only
   reason for the split is an import cycle that goes away once Runtime
   provisioning leaves `tools/hire_worker.go`.

---

## 7. Operator Surface

Owner UI + CLI + ops config. Two sub-surfaces sharing the "operator's
seat" intent (`04 §2.7`).

### 7.1 Entities

| Name | Today | Target |
|---|---|---|
| **ConfigEntry** | `domain/config.go:890-895`. Row in `configs`. KV with audit columns. | Keep. |
| **ChatSession** | claude `.jsonl` path on disk OR Helix `SessionInfo` — two implementations behind `server/chat/backend.go`. | Acknowledge it's an *infrastructure adapter*, not a domain entity. The system has no auth/session concept (`03 §2.2`). |

### 7.2 Value objects

| VO | Today | Target |
|---|---|---|
| **`ConfigKey`** | bare string with a registry of specs (`config/registry.go:36-68`). | Typed `ConfigKey` with the Specs registry as its repo. Today the strings flow through `domain/config.go` as-is. |
| **Spec** | `(Key, Type, Default, Required, Secrets, Description)` — `config/registry.go:36-68`. | Keep. |
| **`OwnerWorkerID`** | the literal string `"w-owner"` referenced in five places (`02 §observations` item 8). | A single `WorkerID` constant. Five sites today; one constant tomorrow. |

### 7.3 Aggregates

- **ConfigStore** as an aggregate root over `ConfigEntry`. Invariants:
  required keys non-empty, secrets never logged. Today no enforcement
  on the latter beyond convention.

### 7.4 Domain events

`ConfigChanged` — already half-implicit: GitHub re-reads on every
delivery (`transports/github/github.go:110-119`) instead of subscribing
to changes. With events: subscribers re-resolve once on change, not on
every request.

### 7.5 Domain services

- **OwnerSeeder** — today shoved into `bootstrap/bootstrap.go` but
  invoked from `serve` (`02 Capability 1 pain points`). Lift into
  `OrgGraph.SeedOwnerIfEmpty()` and let the CLI's `bootstrap` mean
  something else (preflight). Cited in `04 §4` item 8.
- **OwnerChatBridge** — `server/chat/chat.go` + `helix_bridge.go`. A
  parallel implementation pair (`04 §4` item 9).

### 7.6 Invariants that ought to hold but currently don't

1. **Owner UI mutations go through MCP** *or* the docs admit they
   don't. Today `server/ui/ui.go` mutates `roles`, `identities`,
   `streams.events`, `configs` directly (`04 §4` item 6,
   `CLAUDE.md:19` contradicts code). Pick option 1 in `04 §2.7` —
   ratify the bypass — and document it. Less load-bearing than the
   other cross-cuts but the easiest to fix today by editing CLAUDE.md.
2. **Owner Role is replaceable at bootstrap.** Today
   `bootstrap/templates/owner_role.md` is `//go:embed`ed
   (`bootstrap/bootstrap.go:28`); every install gets the same prose.
   Per `02 §observations` item 1, this contradicts "no workflow in
   code" because the owner playbook *is* workflow in code (embedded
   data, but compiled in). Either accept (it's a seed; seeds are code)
   or expose as a config-driven template.

---

## 8. Agent-specific wrinkles

This is the section the rest of the doc supports. Three calls.

### 8.1 Conversation / working memory — where the seam sits

Today a Worker's "memory" is spread across **four** surfaces with no
named owner:

- **`Worker.IdentityContent`** — DB row, projected to `identity.md` at
  every activation (`agent/spawner.go:66-73`).
- **`Role.Content`** — DB row, projected to `role.md`.
- **`agent.Policy`** — embedded const, projected to `agent.md`
  (`agent/policy.go:22-23`).
- **`helix-log.md`** — *the agent writes this itself* in its
  Environment. Pure prompt convention (`agent/policy.md:21-37`). The
  Go code has no concept of it.
- **The activation transcript Stream** — `s-activations-<workerID>`
  events. This is the closest thing to "what the Worker said and did".

The seam question: where does **the Worker's state I own** end, and
**the LLM's scratchpad** begin?

My answer, in two parts.

**(a) `helix-log.md` should stay on the LLM's side of the seam.** The
agent.md policy frames it as a self-managed memory file the LLM
appends to between activations. Promoting it into the domain would
either (i) require the LLM to call an MCP tool to write it — slowing
every activation and tying memory to MCP availability — or (ii) require
us to parse the file from the host filesystem on the system's behalf,
which is brittle and runtime-specific (the Helix runtime has no
filesystem to read). **Keep it prompt-only.** Document in the design
that this is a deliberate "the agent owns its own working memory"
choice.

**(b) The activation transcript should become a first-class Activation
aggregate (§3.1).** The Stream that today carries the transcript is
already a domain object; promoting Activation makes the *aggregate of
turns* visible — which lets `worker_log` filter by activation, lets
the hiring caller wait for `ActivationCompleted`, and lets multi-
activation analyses (rate-limiting, audit, replay) compose. This is
the work the system needs to own.

So the seam is: **I (helix-org) own the structural state, the Stream
contents, the Activation runs, and the events that announce them. The
LLM owns its own `helix-log.md` scratchpad, its tool-call reasoning,
its prose.** The Worker aggregate does not contain `helix-log.md`.

### 8.2 Tools as an ACL — what each context's surface should look like

The `tools/` package today is one registry with 30 entries (`03 §1`
"Tools" table). Per `04 §6 bullet 6` it crosses three contexts. The
right question isn't "should we split the registry" — it's "what does
each context's *tool surface* mean as a curated, semantic interface to
the LLM?"

I think the answer is **don't split the registry, but split the
interfaces it implements**. Today every tool implements
`domain.Tool` (`domain/tool.go:440-456`). Lift to three context-shaped
interfaces:

| Context-interface | Tools today | Why this carves correctly |
|---|---|---|
| `OrgGraphTool` | `create_role`, `update_role`, `update_identity`, `create_position`, `hire_worker`, `grant_tool`, `revoke_tool`, all reads on roles/positions/workers/grants/environment (~15 tools). | These all mutate or read the Org Graph aggregates. Their Args/Returns reference WorkerID/RoleID/PositionID/GrantID and nothing else. |
| `CommunicationTool` | `create_stream`, `subscribe`, `unsubscribe`, `invite_workers`, `publish`, `dm`, `stream_members`, `list_streams`, `get_stream`, `list_stream_events`, `read_events`, `worker_log` (~12 tools). | All mutate or read Streams/Subscriptions/Events. `worker_log` is communication-shaped today (it reads the activation stream); when Activation lands it'd migrate to an `ActivationTool`. |
| `MetaTool` | `ping`. | The test-only escape hatch. Lives here because it can't be classified. |

What this argues *for* over today: each tool's `Description()` (what
the LLM literally reads in `tools/list`) gets a context-flavoured prose
template. Today the descriptions drift in style (compare
`tools/hire_worker.go:38-39` to `tools/read_events.go:128`); a typed
interface lets us standardise.

What this argues *against*: there are not enough tools today to
justify three separate registries, three separate dispatchers, three
separate import graphs. The MCP gateway should keep its single
`tools.Registry` lookup. The split is about which struct field your
tool's `Invoke` reaches for (the Worker aggregate, the Stream
aggregate), not about which package it imports from.

So: **type-split, not package-split**. Today's import cycle (the
reason `tools.EventDispatcher`, `server.Dispatcher`, and
`dispatch.Dispatcher` all exist) is the *artefact* of an
under-articulated split — once Org Graph, Communication, and
Activation are real packages with one Dispatcher interface, the cycle
goes.

### 8.3 Orchestration graph — code, prompt, or aggregate?

Today the orchestration ("on hire do X, on event Y do Z, escalate to
W") lives in **two** places:

- Per-Role: the `## Triggers` / `## Constraints` sections of
  `Role.Content` markdown (`prompts/templates/role.md:26-58`). The
  hiring manager's prompt and every demo role rely on this; the Go
  code does not parse it (`03 §5` "Triggers / Constraints / Files
  sections").
- Org-wide: `agent/policy.md` — the activation-shape doctrine, the
  speaking-discipline rules, the AI-vs-human-source priorities.

Should any of this become a first-class artefact?

**No, mostly.** Three reasons.

1. The Role-markdown sections are the **prompt-driven design's whole
   point** (`CLAUDE.md:14-26`). Lifting them into typed
   `Trigger`/`Constraint`/`Policy` aggregates would re-introduce the
   "workflow in code" the project explicitly rejects.

2. There is no orchestration the **system** runs across multiple
   Workers. The dispatcher fans events out; each Worker independently
   reads its role markdown and decides. There is no global
   "playbook" the system is running. Trying to model orchestration as
   a state machine would have nothing to put in it.

3. The one place that *is* code-shaped — the dispatcher's coalescing
   batch (`02 §observations` item 7) — is already extractable to a
   single `CoalescingQueue` domain service (§3.5). That doesn't
   require an "orchestration graph" abstraction.

**The exceptions** (the small bit that should be lifted):

- **The Role's `## Tools (MCP)` list is a soft contract.** Today the
  owner's prompt reads it and grants accordingly
  (`bootstrap/templates/owner_role.md`). If the LLM forgets a tool,
  the hire half-succeeds. **Promote this to a typed
  `Role.DefaultTools []ToolName` field** (`03 §6` item 5 option b) and
  have `hire_worker` reject when `Grants` is empty and `DefaultTools`
  is non-empty. This is the §1.6 item 3 invariant from earlier with
  teeth. Same for `## Streams` → `Role.DefaultStreams []StreamID`
  driving auto-subscription, killing TODO.md item 1.

- **`agent.Policy` should stay as `worker-policy.md`** (`03 §6` item
  2). It's data the LLM reads; no need to model it. Renaming addresses
  the homonym, no further surgery needed.

So: **keep orchestration in prose, but make the Role contract enforce
two `[]ToolName` and `[]StreamID` lists.** That's the smallest move
that closes the biggest invariant gap and stays consistent with the
philosophy.

### 8.4 Agent-as-concept vs agent-as-implementation

The `Worker` / `AIWorker` / `agent` overload (`03 §2.1`) is the same
thing seen from two layers:

| Layer | Object | What it represents |
|---|---|---|
| **Domain** | `AIWorker` (struct) | A *role-occupier in the org* — has Identity, has Positions, holds Grants. Subject of authorisation, addressing, hiring. **Has no behaviour of its own**; behaviour lives in its Role's prompt. |
| **Implementation** | `agent/claude` spawner + `claude` subprocess (or `agent/helix` + Helix session) | The *runtime process* that physically executes the Role's prompt for one Activation. Has a stdin, a stdout, an exit code. |

These two should never share a name and today they do. Possible
renames:

- **Domain layer: keep `Worker`/`AIWorker`. Drop "agent" entirely from
  the domain.** The struct is a `Worker`; nothing in `domain/`
  references "agent" today, so this is just stopping the prose-layer
  bleed (`agent/policy.md:1-8`: "You are an AI Worker… this file
  tells you how to be an agent").
- **Implementation layer: keep `agent/` as the Go package, but
  consistently mean "the runtime that physically executes a Worker"**.
  Rename `agent.md` → `worker-policy.md` (`03 §6` item 2). Rename
  `WorkspaceSync.PublishFile` → `MirrorFile`.

Net: the **AI Worker** is a role-occupier with Identity + Position +
Grants + a Role's prompt. The **agent runtime** is a port (Spawner)
with two implementations. The pair `(Worker, Activation)` together is
where you go when you need both the *who* and the *what just
happened*. Today there's no `Activation` so the pair collapses onto
Worker, which is why "agent" gets dragged into everything.

---

## 9. If we only do five things — ordered

The user's willingness to invest is "considerable", so this list is
**ordered for leverage**, not safety. The earlier moves enable the
later ones; the later moves are skippable.

1. **Promote `Activation` to a first-class aggregate.** §3. The single
   biggest leverage point. Unblocks: per-activation transcript
   filtering, hire-completion signalling for the owner, audit row per
   run, batching-debug visibility (TODO.md item 6), the
   `worker_log`/`read_events` filter awkwardness (`02 Capability 4
   pain point 1`), and a clean place to put the
   "transcript-append-doesn't-redispatch" rule that today is encoded
   by *not* calling a function.

2. **Promote `Message` to a typed VO on `Event.Body`, and add the
   `Sender` VO.** §2.2. Kill the dispatcher's parse-fallback
   (`dispatch/dispatcher.go:137-141`), collapse `Event.Source` and
   `Message.From` into one typed thing, give external sender identity
   a typed slot. Touches every transport but each touch is local.

3. **Lift `WorkerHired` and friends as real domain events; pull
   runtime provisioning out of `tools/hire_worker.go`.** §1.4 + §5.6
   invariant 1. Cuts the highest-priority cross-cut in `04 §4`,
   collapses the three Dispatcher interfaces into one, and makes
   adding a new Runtime (or a new "react to hire" listener) a
   subscription instead of a code edit.

4. **Lift `Role.DefaultTools []ToolName` and `Role.DefaultStreams
   []StreamID` onto the domain.** §8.3. Close the
   "hire-without-grants" and "hire-without-subscriptions" invariants
   (§1.6 item 3, TODO.md item 1) in code, not prose. Makes the social
   contract testable.

5. **Move publish-rule and subscribe-rule invariants onto the Stream
   aggregate, and extract the dispatcher's `CoalescingQueue` into a
   named domain service.** §2.3 + §2.6 + §3.5. Stream invariants are
   currently scattered across `tools/publish.go`, `tools/dm.go`, and
   `tools/hire_worker.go`; one home, one set of tests. The
   `CoalescingQueue` extraction is the natural next step once
   Activation exists (item 1) — it's the thing operating on
   `Activation.Triggers`.

What I'd **defer** even with a big budget: splitting the tools
registry per context (§8.2 — type-split is enough), promoting
`helix-log.md` into the domain (§8.1 — wrong side of the seam),
modelling orchestration as a state machine (§8.3 — nothing to put in
it), and rewriting the owner UI to go through MCP (§7.6 invariant 1 —
ratify in docs instead).
