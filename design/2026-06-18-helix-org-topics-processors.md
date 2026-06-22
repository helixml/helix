# Helix-Org Topics & Processors (filtering, routing, and the dataflow model)

**Date:** 2026-06-18
**Status:** Design — proposed, not yet implemented
**Component:** `api/pkg/org/` (streaming aggregate, dispatch, REST) × `frontend/src` (org chart)
**Problem:** Today a Stream wires *directly* to a Worker via a Subscription. There is no
way to (a) reshape the messages a Worker sees, or (b) route a Stream's events to different
Workers by rule. We want to interpose processing nodes — **transforms**, **filters**, and
**routers** — on the edge between a source of events and a Worker, using one coherent model.

---

## TL;DR

We adopt the standard **dataflow** model and align our names to it:

- **Topic** (renamed from **Stream**) — the *wire*. A named, durable, multi-subscriber log
  of `Message`s. Publish in, subscribe out. This is the **only** thing nodes connect through.
- **Processor** (new) — a *node* that reads Topic(s) and writes Topic(s) with logic inside.
  A `Kind` registry (mirroring `transport`) gives **`transform`** kinds (`template`,
  `truncate`) that *mutate*, and a **`filter`** kind that *selects* by predicate — where **a
  router is just a `filter` with more than one output** (one predicate → one output each).
  Two operations (mutate, select); routing falls out of selection for free.

> **The invariant:** nodes connect only through Topics. A Processor isn't a Topic — it
> *speaks Topic on both ends*. That is the "clean interface" we were reaching for.

The flagship v1 Processor is the **`template`** transform (`{{ .Message.body }}` → a string
that becomes the new body). **`truncate`** ships alongside it purely to prove the registry is
generic (a new kind = one file + one map entry, no core edits). External systems (github,
email, webhook, cron) stay as Topic **transports** for now — they are the **sources/sinks**
of the graph and already encode direction; unifying them into Processor kinds is noted as
future work, not v1.

New React Flow **processor nodes** sit on the chart between a topic and a worker, with a
config drawer (template editor + live preview that pulls real recent messages from the input
topic via the existing messages API and renders them server-side). All new HTTP endpoints are
**JSON:API**. Built **TDD-first**, phase by phase, with a mechanical **Stream→Topic rename**
as Phase 0.

---

## Why this vocabulary

This is textbook **dataflow / event-driven integration**. The terms below are the canonical
ones; we pick the pair that fits Helix and avoids local collisions.

| Concept | EIP (Hohpe) | Stream processing (Flink/Kafka/Rx) | Reactive Streams / FBP | **Helix (new)** |
|---|---|---|---|---|
| the wire events flow along | Message Channel | Stream / **Topic** | Connection / ports | **Topic** |
| produces only | inbound Channel Adapter | **Source** | Publisher | Topic + inbound transport (github, cron) |
| consumes only | outbound Channel Adapter | **Sink** | Subscriber | Topic + outbound transport |
| consumes **and** emits | Translator / Router | operator (`map`/`filter`/`branch`) | **Processor** | **Processor** |
| the whole graph | Pipes-and-Filters | topology / pipeline / DAG | flow graph | the org chart |
| the data unit | Message | event / record | Information Packet | `Message` / `Event` |

- **"Processor"** is the exact term for "a receiver *and* an emitter": Reactive Streams
  defines `Processor<T,R> extends Subscriber<T>, Publisher<R>`, and Apache NiFi (a canvas of
  wired boxes — our UI) calls every box a Processor. **"Topic + Processor"** is the Kafka
  Streams pairing (topic → stream processor → topic).
- **Why Topic, not Channel:** Helix already has **Slack channels** (a transport) and Go has
  channels; "Channel" would collide. A Helix wire is a durable, multi-subscriber, retained
  log — that *is* a topic — and "subscribe to a topic" matches the existing `Subscription`.

### One naming correction baked into the schema

"Filter" is overloaded. In **Pipes-and-Filters** a "filter" is *any* processing box; in
**EIP / stream processing** a **Filter = selective drop** and a **transform = map /
Translator**. Our templating box is a **transform**, not a filter. There are two **families**
of operator, by what they do to the event:

| Kind | Family | Semantics | Ports | Phase |
|---|---|---|---|---|
| `template` | **transform** (mutate) | rewrite the body via Go `text/template` | 1 in → 1 out (always passes) | **v1** |
| `truncate` | **transform** (mutate) | cap body bytes | 1 in → 1 out (always passes) | v1 (minor) |
| `filter` | **select** (predicate) | publish to each output whose predicate matches; drop if none match | 1 in → **0..N** out | near-term |

**A router is not a separate kind — it is a `filter` with more than one output.** Each output
carries its own predicate (`Output.Match`), so **one output = a classic keep/drop filter, and
N outputs = a content-based router**. This is exactly your "a router is a specialised filter…
one per filter": in the model, **each `Output` *is* one filter** (a predicate + a destination
topic). Mutation and selection stay separate boxes (one thing each); you compose them by
chaining through topics (`transform → filter → worker`, or `filter → transform → …`). The UI
may still *call the boxes* "filters" colloquially, but the kinds mean what they say.

---

## Current state (what we're interposing on)

Publish → deliver today (verified in code):

```
Worker/transport ──publish──▶ Publishing.Publish()            api/pkg/org/application/publishing/publishing.go
                                 1. Events.Append(event)       // event.Body = canonical Message JSON
                                 2. Hub.Notify(stream)          // wake long-poll observers
                                 3. Dispatcher.Dispatch(event)  api/pkg/org/application/dispatch/dispatcher.go
                                      ├─ emitOutbound (webhook/email)
                                      ├─ msg := event.Message()
                                      └─ for sub in Subscriptions.ListForStream(stream):
                                             if worker is AI → Queue.Enqueue(TriggerEvent{Message: msg})
```

Key types (`api/pkg/org/domain/streaming/`): `Stream`, `Event` (`Body` = canonical `Message`
JSON), `Message` (`From/To/Subject/Body/…`), `Subscription` (worker-anchored
`(orgID, workerID, streamID)`). The edge we interpose on is the Subscription. **The transport
layer already encodes source/sink direction** via the [`streaming.Inbound`/`Outbound`](api/pkg/org/domain/streaming)
ports (github = inbound-only, email = in+out, local = neither) — so "source vs sink" exists
in the code today, just implicitly, glued onto a Stream by its `Transport` field.

---

## Design decisions

1. **Topic is the universal wire; Processors connect only through Topics.** No node→node
   edges, ever. A worker subscribes to a topic; a processor reads a topic and writes a
   topic; a source writes a topic; a sink reads a topic. This keeps the existing
   publish/subscribe/dispatch backbone as the *one* mechanism for everything.

2. **A Processor's output Topic(s) are auto-provisioned and owned by it.** Creating a
   processor creates its output Topic(s) (`transport.KindLocal`). This is *structural
   derivation*, not workflow — the output topic **is** part of what a Processor means,
   exactly as `Role.Tools` **is** a Worker's capability (CLAUDE.md org philosophy). (Open
   Question 1 keeps the explicit-output alternative on the table.)

3. **Processor kinds use the `transport` registry shape.** `domain/processor` mirrors
   `domain/transport`: a `Kind` enum, a `Strategy`/`Config` interface, a `strategies` map,
   one file per kind. **Adding a kind = new file + new constant + one map entry**, with no
   edits to `Processor.Validate`, the runner, or the dispatcher ("keep the core generic").

4. **Transforms are pure domain functions.** Template rendering and truncation are
   deterministic, no I/O (`text/template` is stdlib) → they live in the **domain** layer and
   unit-test with no store or HTTP. (Contrast: transports do real network I/O →
   `infrastructure/`.)

5. **One execution interface covers mutation and selection.** A processor `Process`es an
   input message into **zero or more** `(output topic, message)` results: a `transform`
   returns one (mutated); a `filter` returns one result per output whose predicate matched
   (zero = a drop). **A router is a `filter` with several outputs** — same interface, no
   special case. The runner just publishes each result.

6. **Execution is a new fan-out arm of the existing Dispatcher,** injected late (like the
   outbound emitters). Output is published via `publishing.Publishing`, re-entering the same
   path → the output topic dispatches to its subscribers. Recursion = processor chains
   (desired); cycles are prevented at create-time (DAG check) + a runtime hop guard.

7. **External systems stay as Topic transports in v1.** github/email/webhook/cron are the
   graph's sources/sinks and already work. Reframing them as `Source`/`Sink` processor kinds
   is future work (noted), not this change.

8. **All new endpoints are JSON:API.** Extends `api/pkg/org/interfaces/jsonapi` (today
   emit-only) with request-document binding. Processor resources are `type: "processors"`.

9. **No MCP tools for processors in v1.** Keep the MCP surface small (org-graph primitives
   only). Operator/UI-assembled for now; add `create_processor` only on demonstrated agent
   need.

10. **`text/template`, not `html/template`.** Output is plain text consumed by an agent, not
    browser HTML. Documented; revisit if output is ever HTML-rendered.

---

## Phase 0 — Rename Stream → Topic (mechanical, behaviour-preserving)

Land this **first, as its own PR**, so all subsequent code is born with the right names and
we don't write `Stream`-named code we immediately rename. No behaviour change.

**Scope of the rename:**
- Domain: `streaming.Stream` → `Topic`, `StreamID` → `TopicID`; `Event.StreamID`,
  `Subscription.StreamID` → `TopicID`. (Keep the `streaming` package name — topics stream
  events; renaming the package is extra churn for no gain.)
- `orgchart.Role.Streams` → `Role.Topics`.
- Store: `store.Streams` → `store.Topics`; `Store.Streams` field → `Topics`. GORM
  `streamRow` → `topicRow`, **table `org_streams` → `org_topics`** (rename migration; update
  `orgRowTypes` + `orgTableNames` in `gorm.go:20-48`). Memory repo renamed.
- App services: `application/streams` → `application/topics`; references in `publishing`,
  `subscriptions`, `queries`, `dispatch`.
- REST: `/streams…` → `/topics…` (incl. `/topics/{id}/messages`, `/topics/{id}/events`,
  `/topics/{id}/publish`). Regenerate the client.
- Frontend: `StreamNode` → `TopicNode`, `HelixOrgStreamDetail` → `HelixOrgTopicDetail`,
  route `helix_org_stream_detail` → `helix_org_topic_detail`, `helixOrgService` hooks/keys.

**The sharp edges (call these out in the PR):**
- **MCP tool renames are agent-facing.** `create_stream`/`list_streams`/`get_stream`/
  `list_stream_events`/`stream_members` → `…_topic(s)`. (`subscribe`/`unsubscribe`/`publish`
  don't name streams — unchanged.) These names appear in **Role prompt text**, which the
  compiler will not catch — grep roles/profiles and update prose. *This is the riskiest part
  of the rename.*
- **ID prefix stays `s-`.** Topic IDs remain `s-general` etc. The prefix is opaque; changing
  it to `t-` means migrating every `event.topic_id` / `subscription.topic_id` /
  `Role.Topics` reference. Not worth it; note `t-` as an optional later cleanup.
- **Timing/merge risk.** This is a wide mechanical diff against an active repo — land it on a
  quiet window and rebase aggressively. (Alternative sequencing: defer the rename to *after*
  the Processor feature and do it once at the end. Recommended order is rename-first, but
  this is resequenceable if a big rename PR is inconvenient right now.)

**Done when:** build + full test suite green, no behaviour change, no `Stream` identifiers
left in `api/pkg/org` or the org frontend.

---

## Proposed architecture (Processors)

### Before / after

```
BEFORE   [Topic s-inbox] ───subscription───▶ [Worker w-triage]

AFTER    [Topic s-inbox] ──input──▶ [Processor p-fmt] ──(output s-p-fmt-out)──▶ [Worker w-triage]
            (kind=template)            template: "From {{.Message.from}}: {{.Message.subject}}"
                                       output topic auto-created; w-triage subscribes to it

FUTURE   [Topic s-inbox] ──▶ [Processor p-route] ─┬─ match "*@vip"  ─▶ s-...-vip   ─▶ w-senior
            (kind=filter,                          ├─ match "invoice"─▶ s-...-bill  ─▶ w-billing
             3 outputs                             └─ default (empty)─▶ s-...-gen   ─▶ w-triage
             = 3 filters)            each output above is "one filter": a predicate + a destination
```

### Domain: `api/pkg/org/domain/processor/` (new, mirrors `domain/transport`)

```go
// processor.go — the umbrella
type Processor struct {
    ID             ProcessorID         // "p-<slug>"
    OrganizationID string
    Name           string
    InputTopicID   streaming.TopicID   // 1 input in v1 (merge = N inputs, future)
    Outputs        []Output            // 1 for transform/filter; N for router
    Kind           Kind                // "template" | "truncate" | "filter"  (router = filter w/ N outputs)
    Config         json.RawMessage     // per-Kind opaque config
    CreatedBy      string              // cosmetic anchor, like Topic.CreatedBy
    CreatedAt      time.Time
}
type Output struct {
    TopicID streaming.TopicID // auto-provisioned local topic
    Match   string            // routing predicate; empty = unconditional (transform/filter)
    Label   string            // UI label for the branch
}

// Strategy/Config mirror transport's. Process returns 0..N results so ONE
// interface serves transform (1), filter (0/1), and router (N).
type Strategy interface { ParseConfig(json.RawMessage) (Config, error) }
type Config interface {
    Validate(outputs []Output) error
    Process(in streaming.Message, outputs []Output) ([]Result, error)
}
type Result struct { TopicID streaming.TopicID; Message streaming.Message }

var strategies = map[Kind]Strategy{ KindTemplate: template{}, KindTruncate: truncate{} }
func KindValues() []Kind { /* canonical order, like transport.KindValues */ }
func (p Processor) Validate() error { /* dispatch via strategies[p.Kind]; no switch */ }
func (p Processor) Process(m streaming.Message) ([]Result, error) { /* parse cfg → Config.Process */ }
```

```go
// template.go — flagship v1 kind
const KindTemplate Kind = "template"
type templateConfig struct { Template string `json:"template"` } // Go text/template
func (c templateConfig) Validate(out []Output) error {
    // len(out)==1; non-empty; template.New("proc").Funcs(fns).Parse(c.Template) must succeed
}
func (c templateConfig) Process(in streaming.Message, out []Output) ([]Result, error) {
    // data = { Message: <in decoded to map[string]any, lowercase JSON keys>, Event: <meta> }
    // render → m := in; m.Body = rendered; m.BodyContentType = "text/plain"
    // return []Result{{TopicID: out[0].TopicID, Message: m}}
}
```

```go
// truncate.go — proves the registry is generic
const KindTruncate Kind = "truncate"
type truncateConfig struct { MaxBytes int `json:"max_bytes"` } // > 0
func (c truncateConfig) Process(in streaming.Message, out []Output) ([]Result, error) {
    m := in; m.Body = runeSafeTruncate(in.Body, c.MaxBytes)
    return []Result{{TopicID: out[0].TopicID, Message: m}}, nil
}
```

```go
// filter.go — selection. ONE kind serves both filter (1 output) and router (N outputs):
// "each Output is one filter" — a predicate + a destination. No mutation here.
const KindFilter Kind = "filter"
type filterConfig struct{} // predicates live on Output.Match; this kind just evaluates them
func (c filterConfig) Validate(out []Output) error {
    // len(out) >= 1; each non-empty Output.Match must be a valid predicate (see OQ7)
}
func (c filterConfig) Process(in streaming.Message, out []Output) ([]Result, error) {
    var res []Result
    for _, o := range out {
        if matches(o.Match, in) {          // empty Match = unconditional (a "default"/else branch)
            res = append(res, Result{TopicID: o.TopicID, Message: in}) // pass-through, unchanged
        }
    }
    return res, nil // empty slice = dropped (no predicate matched)
}
```

**Template data context** (matches your `{{ .Message.body }}` exactly — `.Message` is a
`map[string]any`, so lowercase wire keys resolve): `.Message.body`, `.Message.from`,
`.Message.to`, `.Message.subject`, `.Message.thread_id`, … plus `.Event.ID`,
`.Event.TopicID`, `.Event.Source`, `.Event.CreatedAt`. Stdlib `text/template` + a small fixed
FuncMap (`trunc`, `lower`, `upper`, `default`); no arbitrary user funcs in v1.

### Store: `api/pkg/org/domain/store/store.go`

```go
type Processors interface {
    Create(ctx, p processor.Processor) error
    Get(ctx, orgID string, id processor.ProcessorID) (processor.Processor, error)
    List(ctx, orgID string) ([]processor.Processor, error)
    ListByInputTopic(ctx, orgID string, in streaming.TopicID) ([]processor.Processor, error) // dispatch hot path
    Update(ctx, p processor.Processor) error
    Delete(ctx, orgID string, id processor.ProcessorID) error
}
// + Store.Processors field. GORM processorRow (table org_processors, outputs as JSON column),
// + memory processorsRepo (clone the topic repos).
```

### Application: `api/pkg/org/application/processors/` (CRUD)

`processors.New(Deps{Processors, Topics, Now, NewID})`: **Create** validates config +
**cycle-checks** the processor graph + auto-creates the output Topic(s) via the topics service
+ persists. **Update** re-validates/re-cycle-checks. **Delete** removes the processor and its
auto-created output topics (cascading their subscriptions, as topic delete already does).
**Preview** renders a candidate config against caller-supplied or fetched sample messages
**without persisting** — server-side, so there's no Go↔JS template drift.

### Execution: `api/pkg/org/application/processing/` + dispatcher hook

```go
// processing/runner.go
type Publisher interface { // satisfied by publishing.Publishing; declared here to break the cycle
    Publish(ctx, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}
type Runner struct { procs store.Processors; publisher Publisher; logger *slog.Logger }

func (r *Runner) Run(ctx, e streaming.Event, msg streaming.Message) {
    for _, p := range mustList(r.procs.ListByInputTopic(ctx, e.OrganizationID, e.TopicID)) {
        results, err := p.Process(msg)        // pure; 0..N results
        if err != nil { /* log, skip */ continue }
        for _, res := range results {
            r.publisher.Publish(withHop(ctx), e.OrganizationID, res.TopicID, procSource(p), res.Message)
        }
    }
}
```

Dispatcher integration mirrors `RegisterOutbound` (`dispatcher.go:75`) — late-bound because
`publishing` is built *after* the dispatcher at the composition root:

```go
func (d *Dispatcher) RegisterProcessorRunner(r ProcessorRunner) { d.processorRunner = r }
// inside Dispatch(), after the worker fan-out loop:
if d.processorRunner != nil { d.processorRunner.Run(ctx, e, msg) }
```

**Wiring** (`helix_org.go`): build `store.Processors`; build
`processing.Runner{Processors, Publisher: orgServices.Publishing}`; call
`dispatcher.RegisterProcessorRunner(runner)` where outbound emitters are registered. No import
cycle: `publishing` doesn't import `processing`; `processing` depends on the `Publisher`
interface it declares.

### REST (JSON:API) — `api/pkg/org/interfaces/server/api/`

Org resolved via `resolveOrgID(r)` like every other org handler. Resource type
`"processors"`.

| Method & path | Purpose |
|---|---|
| `GET /processors` | list |
| `POST /processors` | create (auto-creates output topics) — `data.attributes`: name, input_topic_id, kind, config, outputs |
| `GET /processors/{id}` | one |
| `PUT /processors/{id}` | update name/config/outputs |
| `DELETE /processors/{id}` | delete + output topics |
| `POST /processors/preview` | dry-run render — `data.attributes`: kind, config, outputs, and `samples:[…]` or `input_topic_id`(+count) |

`jsonapi` additions (TDD in `jsonapi_test.go`): `jsonapi.Bind(r, &attrs)` to decode
`{data:{type,attributes}}`. Single-resource docs already work (`Document.Data any`). Then
swagger annotations → `./stack update_openapi` → generated client.

### Frontend — `frontend/src/pages/HelixOrgChart.tsx` + service + drawer

- **Node type:** add `processor` to `nodeTypes` (`HelixOrgChart.tsx:423`). `ProcessorNode`
  clones `TopicNode` (post-rename) with a **left/target** handle (input from a topic) and
  **right/source** handle(s) (output → worker). Distinct accent so it reads as a node, not a
  topic. Router later renders one source handle per output.
- **`buildGraph()`** (`:494`): emit a processor node; edge `inputTopic → processor`
  (`'proc_in'`); edge `processor → worker` for each worker subscribed to an output topic
  (`'proc_out'`). Auto-created output topics are **not** drawn as their own boxes (collapsed
  into the processor) so the box visibly sits *in between* the edge; their messages stay
  inspectable via the detail view.
- **`onConnect`** (`:937`): `topic → processor` ⇒ set `input_topic_id` (PUT). `processor →
  worker` ⇒ subscribe worker to the output topic (reuse the existing worker→topic
  subscription mutation — no new endpoint). `onEdgesDelete` mirrors.
- **Config drawer** (`ProcessorConfigDrawer`, clone `HireWorkerDrawer.tsx`): name, kind
  select, template editor, **live preview**: (1) fetch recent messages from the input topic
  via the existing messages API (`v1OrgsTopicsMessagesDetail(topicId, org, {page[size]:5})`
  → `data[].attributes.{from,subject,body}`) to show "what a message looks like"; (2) on edit
  (debounced) POST `/processors/preview` and show before → after. Server-render is
  authoritative.
- **Service hooks** + query keys in `helixOrgService.ts`; invalidate topics + processors +
  worker-subscriptions on mutate.

---

## Implementation plan (phased, TDD-first)

Each phase states **what we want** (the capability), **the red test** (the failing check that
defines done — behavioural, not file-level), and **suggested steps**. Per CLAUDE.md: write the
red test first; lifecycle phases (2, 4, 5) need a **live** end-to-end run, not just a unit test
asserting a state change.

### Phase 0 — Rename Stream → Topic
- **What we want:** the wire is called *Topic* everywhere — domain, store, REST, MCP tools,
  frontend — with zero behaviour change.
- **Red test:** not classic TDD (no new behaviour); the net is two checks. The **existing
  suite stays green** (regression guard), and a **grep guard** that fails while any
  `Stream`/`StreamID`/`org_streams`/`*_stream` identifier survives in the org packages and the
  org frontend — it goes green only when the rename is complete.
- **Steps:** (1) rename the domain types + the table (with a rename migration); (2) sweep the
  store, app services, REST routes, and MCP tool names; (3) sweep the frontend + regenerate the
  client; (4) update Role prompt text that names the old tools (what the compiler can't catch);
  (5) run the full suite + grep guard.

### Phase 1 — Processor domain + storage (no execution)
- **What we want:** processors can be defined, validated, and persisted, and the pure
  transforms produce correct output — all in isolation, nothing wired into the live publish
  path yet.
- **Red test:** unit tests that render a `template` against a sample message and assert the
  resulting body (`{{.Message.body}}`, `.subject`, a missing key, a malformed template rejected
  at validation); `truncate` caps a body rune-safely; the registry lists its kinds in canonical
  order and rejects an unknown kind. Then a store round-trip (create → get → list → update →
  delete) on the in-memory store. All fail because neither the package nor the store exists.
- **Steps:** (1) define the Processor entity + the Strategy/Config registry (mirroring
  `transport`) with the `template` and `truncate` kinds; (2) add the `Processors` store
  interface + in-memory implementation; (3) add the persistent implementation + migration;
  (4) make the domain tests pass, then the store tests.

### Phase 2 — Execution (the input → output hop)
- **What we want:** publishing a message to a processor's input topic transforms it and the
  result reaches the workers subscribed to the output topic — live, through the real dispatcher
  — with cycles rejected and chains bounded.
- **Red test:** an integration test that wires a `template` processor between an input topic and
  a worker (worker subscribed to the output topic), publishes one message to the input, and
  asserts both that a transformed event appears on the output topic *and* that the worker is
  activated with the rendered body. Two more: creating a processor that would close a cycle is
  rejected; a chain deeper than the guard aborts rather than looping. All fail because the
  runner and wiring don't exist.
- **Steps:** (1) add the execution runner — list processors by input topic, `Process`, publish
  each result through a `Publisher` port; (2) hook it into the dispatcher as a late-bound
  fan-out arm; (3) auto-provision output topics + cycle-check in the create use case; (4) wire
  it at the composition root; (5) make the integration tests pass; (6) **verify live in the
  inner Helix** with a real subscribed worker — and confirm the *next* publish after setup
  actually flows through.

### Phase 3 — REST (JSON:API) CRUD + preview
- **What we want:** processors are fully manageable over HTTP in JSON:API, including a
  **preview** that renders a candidate config against real recent messages without persisting
  (this is what the UI's live preview calls).
- **Red test:** handler tests against the in-memory-backed test server: create returns a
  JSON:API `processors` resource and the auto-created output topic id; list/get/update/delete
  behave; bad input, missing id, and cycle attempts map to the right status codes; preview
  returns before/after pairs for supplied samples. Plus request-binding tests for the JSON:API
  decode helper. All fail because the routes, handlers, and binding don't exist.
- **Steps:** (1) add the JSON:API request-binding helper; (2) add CRUD handlers + routes that
  delegate to the Phase-1/2 services; (3) add the preview endpoint (pull samples from the input
  topic, render with the same engine the runner uses); (4) annotate for swagger + regenerate the
  client; (5) make the handler tests pass.

### Phase 4 — Frontend (chart node + drawer + live preview)
- **What we want:** a user can drop a processor box on the chart, wire a topic into it and it
  into a worker, edit a template, see real sample messages render live, save, and have the
  worker receive transformed messages.
- **Red test:** the binding check here is an **end-to-end acceptance run in the inner Helix**
  (the UI doesn't exist yet, so the scenario currently can't be performed): drop a processor,
  wire topic → processor → worker, enter a template, confirm the preview shows real
  before/after, save, publish to the input topic, observe the worker activate with the
  transformed body. (Optionally a component test for the connect/disconnect routing.)
- **Steps:** (1) add the processor node type + graph building (input/output edges); (2) wire
  connect/disconnect to the CRUD + existing subscription mutations; (3) build the config drawer
  — template editor + live preview (samples via the messages API, render via the preview
  endpoint); (4) run the acceptance scenario; (5) ask the user to confirm the UI while you
  verify the data path via DB + logs.

### Phase 5 — `filter` kind (which *is* routing)
- **What we want:** predicate-based selection with 1..N outputs — a keep/drop filter at one
  output, a content-based router at many — usable end-to-end with no new core machinery.
- **Red test:** domain tests first — a one-output filter keeps a matching message and drops a
  non-matching one; a multi-output filter sends each message to exactly the outputs whose
  predicate matches, with an empty-predicate output catching the rest; a malformed predicate is
  rejected at validation. Then an integration test: publish a spread of messages and assert each
  lands on the correct output topic(s) and reaches the right worker(s). All fail because the
  kind and the predicate evaluator don't exist.
- **Steps:** (1) decide the predicate language (OQ7); (2) add the `filter` kind that evaluates
  each output's predicate; (3) extend the chart node to render N output handles + a rule editor;
  (4) reuse the runner, CRUD, and preview unchanged; (5) make the domain then integration tests
  pass; (6) verify live.

---

## How this honours the org philosophy

- **Data/text over code:** the behaviour *is* the template / the match rule (data the user
  writes); the Go is a generic, stateless engine.
- **Core stays generic:** new kinds need no edits to the dispatcher, store, or
  `Processor.Validate` — one file + one map entry (the `transport` registry pattern).
- **Not workflow-in-code:** the Runner does exactly one structural thing — `Process` then
  publish results. No agent decisions, no implicit subscribing/chaining beyond what the data
  declares. Output topics are intrinsic to a Processor (structural derivation, like
  `Role.Tools`).
- **Small MCP surface:** no new MCP tools in v1.

---

## Open questions

1. **Auto-create vs explicit output topics.** Spec assumes auto-create. Explicit (user points
   at an existing topic) enables fan-in to a shared topic but muddies delete/chart ownership.
   *Lean: auto-create v1; allow "publish into an existing topic" later.*
2. **Output attribution.** `Event.Source`/`Message.From` on a processed event? `Source=""`
   (system-emitted; won't be re-delivered to a worker) with `Message.From` carried through,
   vs a synthetic `processor:<id>` principal for audit. *Lean: `Source=""`, preserve From.*
   Interacts with the dispatcher's "don't deliver to publisher" check + `SourceKind`.
3. **Runtime failure policy.** Template errors at runtime → drop / pass-through / dead-letter
   topic? *Lean: log + drop, surface in the processor detail view; revisit dead-letter with
   routing.*
4. **Template power.** Fixed FuncMap only, or a vetted sprig-lite set? Any access to prior
   messages / accumulator state? *Lean: stateless, small fixed FuncMap v1.*
5. **Rename sequencing & ID prefix.** Rename-first (Phase 0) vs rename-last; keep `s-` IDs vs
   migrate to `t-`. *Lean: rename-first, keep `s-`.* (See Phase 0.)
6. **Channel vs Topic.** Spec uses **Topic** (avoids Slack/Go "channel" collisions; matches
   the durable-log semantics). Switch to **Channel** if preferred — one-line change here.
7. **Filter predicate language & polarity.** Boolean `text/template` (`Output.Match` renders
   non-empty/"true" ⇒ match — one engine, consistent with the `template` transform) vs a
   structured match (field/op/value — friendlier for a UI rule-builder) vs an expression lang
   (CEL/JMESPath). Polarity: an output publishes **when its predicate is true**
   (keep-on-match); "filter *out* X" = a negated predicate or a per-output `negate` flag.
   *Lean: boolean `text/template` + keep-on-match for v1; revisit structured match when the
   rule-builder UI lands.*

---

## Touch list

**Phase 0 (rename):** `api/pkg/org/domain/streaming/*`, `domain/orgchart` (Role.Topics),
`domain/store/store.go`, `infrastructure/persistence/{gorm,memory}/*`, `application/{streams→topics,publishing,subscriptions,queries,dispatch}`,
`interfaces/{server/api,mcptools}/*`, `frontend` (TopicNode, detail page, routes, service),
`frontend/src/api/api.ts` (regenerated), Role prompt text referencing `*_stream` tools.

**Phases 1–4 (processors) — new:** `domain/processor/{processor,template,truncate}.go`,
`application/processors/processors.go`, `application/processing/runner.go`,
`infrastructure/persistence/gorm/processor.go`, `interfaces/server/api/processors.go`,
`frontend/src/components/helix-org/ProcessorConfigDrawer.tsx` (+ tests throughout).
**Changed:** `domain/store/store.go` (+Processors), `gorm.go` (orgRowTypes/orgTableNames),
`memory/memorystore.go`, `dispatch/dispatcher.go` (RegisterProcessorRunner + call),
`jsonapi/document.go` (Bind), `server/api/api.go` (routes) + `helix_org.go` (wiring),
`HelixOrgChart.tsx` (node type, buildGraph, onConnect), `helixOrgService.ts`, `api.ts`.
