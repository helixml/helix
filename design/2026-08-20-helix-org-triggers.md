# Helix-Org Triggers

## Proposal

Remove the Topic abstraction. It currently combines unrelated concerns:
inbound events, outbound delivery, Worker messaging, processor wiring, and
transcripts. All outbound communication is performed by MCP or CLI tools. Do
the backend work first; update the frontend later.

Replace it with four concepts:

- **Trigger** — one inbound event source, such as GitHub, Slack, email,
  webhook, cron, or a Helix event. Eventually merge this with the existing
  Helix Trigger definition when Helix-Org Workers and Helix agents converge.
- **Processor** — filters, transforms, or splits events from a Trigger. A
  Processor emits events through the same output interface as a Trigger, so
  Workers and other Processors can attach to it directly.
- **Tool** — an action initiated by a running Worker, including email, Slack,
  DMs, team briefings, and other internal or external actions.
- **Session (later)** — the existing Helix Session owned by a Worker. Trigger
  events and internal messages become Session interactions; Session history is
  the transcript and activity record.

The Activation/log-to-Helix-Session work is independent follow-up work. It
does not need to be part of the Topic-to-Trigger backend rework.

There is no generic publish/subscribe channel between these concepts.

```text
external event -> Trigger -> optional Processors -> Worker

running Worker -> MCP tool or sandbox CLI/API -> action

internal communication tool -> recipient Worker
```

For example, one GitHub Trigger can feed two Processors:

- pull-request filter -> Review Worker
- merge-to-`main` filter -> Documentation Worker

These Processors are visible and independently attachable, but use the same
event interface as the GitHub Trigger rather than introducing another concept.

Triggers are strictly inbound. They never send email, post to Slack, or call
outbound webhooks. Those are tool or sandbox actions. GitHub and GitLab already
mostly follow this rule; outbound webhook delivery, email delivery, and Slack
delivery through Topic publishing do not.

## Code scope

The implementation design needs to inspect:

- Topic/event lifecycle: `domain/streaming`, `application/topics`,
  `application/publishing`, `application/dispatch`, and
  `application/subscriptions`.
- Trigger sources: `domain/transport`, `infrastructure/transports/*`, and the
  wiring in `api/pkg/server/helix_org.go`.
- Processors: `domain/processor`, `application/processing`,
  `application/processors`, and `application/slackrouting`. Replace
  the current output Topics with the common Trigger/Processor event interface.
- Internal messaging: `domain/channels`, lifecycle/reconciliation, and the
  `dm`, `reports`, and `ask_human` MCP tools.
- Public surfaces: Topic MCP tools, Topic REST APIs and DTOs, persistence,
  and generated API clients. Frontend changes to the Topics UI, Worker
  assignments, and org chart follow the backend rework.
- Existing documentation and tests, especially `api/pkg/org/QA.md` and the
  Topic, Processor, Slack, and human-delivery designs.

Topic CRUD, publishing, subscriptions, repositories, and tables are all in
scope for replacement. Worker attachments should point directly to a Trigger
or Processor. Shared low-level storage and queue helpers may remain, but they
must not recreate Topic as a domain abstraction.

This document defines the implementation order and the compatibility boundary,
but deliberately does not prescribe every outbound tool, the detailed frontend
design, or the later Activation/log-to-Session consolidation. The central rule
is: attaching a Trigger or Processor controls what can start a Worker; it never
grants or performs an outbound action.

## Implementation plan

Use four PRs. The first three must preserve the existing Topic REST DTOs and
frontend behaviour. Topic compatibility in those PRs is a temporary adapter
over the new services, not a second implementation of the domain. The final PR
updates the frontend and removes that adapter.

### PR 1: Event sources, Processor outputs, and Worker attachments

Add the new backend model without changing the active Topic data path:

- Define the common event envelope and source reference used by Triggers and
  Processors.
- Give every Processor branch a stable output ID. An attachment to a
  multi-output Processor must identify the output; attaching only to the
  Processor ID cannot express different Workers consuming different branches.
- Add the Trigger aggregate and persistence contracts.
- Add Worker attachments to a Trigger or a specific Processor output, including
  persistence, tenant isolation, lifecycle cleanup, and graph validation.
- Add source-based dispatch behind the current Topic dispatch path and prove
  activation queue ordering and source-Worker suppression remain unchanged.
- Keep the existing Topic, Subscription, Processor, REST, MCP, and frontend
  contracts working.

This PR is additive. It establishes the target vocabulary and data-plane ports
before transports or public APIs move to them.

#### PR 1 implementation plan

PR 1 introduces internal domain and persistence surfaces only. It does not add
Trigger REST or MCP endpoints, migrate rows, change Topic DTOs, or make
transports emit Trigger events. Existing Topic subscriptions remain the
authoritative production routing path until PR 3.

1. **Add the source and event domain types.**

   - Add a small domain package for event sources rather than extending
     `domain/streaming`, whose types remain the Topic compatibility model.
   - Define `SourceRef` as a closed, validated reference to either a Trigger or
     one stable Processor output. A Processor reference contains both the
     Processor ID and output ID; an output label or slice index is never an
     identity.
   - Define the new event envelope with event ID, organization ID, source
     reference, canonical `streaming.Message`, optional originating Worker ID,
     and creation time. Keep the originating Worker separate from the source
     reference so source-Worker suppression still works for events emitted by
     a Worker through an internal compatibility path.
   - Add constructors and validation for empty IDs, invalid source-kind/field
     combinations, empty messages, zero timestamps, and organization IDs.
     Do not add a generic source kind for Topic: Topic-to-source conversion is
     migration work for PR 3.

2. **Give Processor outputs durable identities.**

   - Add an `ID` to `processor.Output` and make Processor validation require
     IDs to be non-empty and unique within the Processor. Keep `TopicID`,
     `Owned`, and `ManagedFor` intact for the compatibility implementation.
   - Allocate output IDs in `application/processors` when an output is first
     created. Preserve them on name, predicate, configuration, input, and
     output-Topic changes. Reordering outputs must not change identity.
   - Treat an update without an ID as creation of a new branch; never infer
     identity from label, match expression, or array position. During the
     compatibility period only, the existing Processor application adapter may
     recover an omitted ID by an exact match on the branch's immutable owned
     output Topic ID. Reject ambiguous, duplicate, or foreign output IDs rather
     than silently repairing them.
   - Continue storing outputs in the existing Processor JSON column. Add an
     idempotent data migration that assigns IDs to every pre-existing output
     before the stricter domain mapper reads those rows; do not rely on lazy
     read-time mutation. GORM and memory round-trip tests must prove IDs survive
     create/update/reload and upgrade. Existing REST/MCP requests remain valid
     without IDs and existing response fields remain compatible; exposing the
     IDs for graph editing is deferred to the public cutover.

3. **Add the Trigger aggregate and repositories.**

   - Define Trigger by deriving from the shared generic aggregate type. The
     generic aggregate owns common identity, organization, creator, and
     timestamp state; Trigger adds only its name, inbound transport kind, and
     configuration. Reuse the existing `domain/transport` validation
     strategies; do not copy provider-specific configuration or execution
     logic into the Trigger aggregate.
   - Add `Triggers` to `domain/store.Store` with create, update, delete, and one
     `Find(QueryBuilder)` retrieval method. Express get-by-ID, list-by-org,
     list-by-kind, ordering, and pagination through typed QueryBuilder filters;
     do not add bespoke `Get`, `List`, or `ListBy...` repository methods. The
     organization predicate is mandatory for tenant-scoped application calls.
   - Implement the same contract in memory and GORM with a new
     `org_triggers` table, composite `(org_id, id)` identity, and an
     organization-scoped unique name. Both implementations consume the query
     object rather than maintaining separate retrieval APIs.
   - Register the row in the canonical AutoMigrate/FK lists and cover clean
     creation, upgraded-database creation, conflict mapping, round trips, and
     colliding IDs in different organizations. Do not populate this table from
     Topics in this PR.

4. **Add Worker attachments as their own aggregate.**

   - Derive the attachment aggregate from the same generic aggregate type and
     add Worker ID plus `SourceRef`; `(organization, worker, SourceRef)` is its
     natural uniqueness constraint. One Worker may attach to a Trigger and to
     multiple outputs of the same Processor.
   - Add create and delete mutations plus one `Find(QueryBuilder)` retrieval
     method. Typed query filters cover attachment ID, Worker, exact source,
     Trigger, Processor, and Processor output; application code composes these
     filters instead of adding `Find`, `ListForWorker`, or `ListForSource`
     methods. Implement memory and GORM storage in a new
     `org_worker_attachments` table. Store Trigger ID, Processor ID, and output
     ID in explicit nullable columns with a database check constraint matching
     the `SourceRef` variants; do not store an opaque JSON reference.
   - Put referential validation in an application `attachments` service. On
     create, require the Worker and referenced Trigger/Processor output to
     exist in the same organization, reject human nodes, and return conflict
     for a duplicate. Repository methods must always include organization ID,
     including lookups on the dispatch hot path.
   - Extend Worker deletion to remove its attachments. Trigger deletion removes
     Trigger attachments. Processor deletion removes all attachments to its
     outputs; Processor update rejects removal of an output that still has
     attachments, unless the calling application operation explicitly removes
     those attachments in the same lifecycle operation. Mirror these rules in
     memory tests; install database cascades where the referenced row is a
     first-class table and keep application cleanup for Processor outputs,
     which remain JSON-owned in this PR.

5. **Extract one delivery core and add source dispatch.**

   - Refactor `application/dispatch.Dispatcher` so Message parsing, Worker
     lookup, human-node rejection, originating-Worker suppression, activation
     construction, and `Queue.Enqueue` live in one ordered delivery function.
   - Keep `Dispatch(streaming.Event)` behaviour intact: outbound emitters,
     Topic subscription lookup, and Processor runner invocation remain in the
     same order and continue to use Topic IDs. It calls the shared delivery
     function with the current subscription targets.
   - Add an internal source-event entry point that resolves exact Worker
     attachments through `Find(QueryBuilder)` with organization and source
     filters and calls that same delivery function. It does not emit outbound
     traffic or invoke Topic-based
     Processor traversal. No production transport is wired to this entry point
     yet; PR 3 performs that cutover.
   - Preserve repository ordering through enqueue calls and deduplicate a
     Worker defensively before delivery. Do not run attachment dispatch in
     parallel with subscription dispatch for the same Topic event; that would
     create duplicate activations during the compatibility period.

6. **Validate the graph boundary.**

   - Extend Processor application validation to ensure every referenced input
     and compatibility output Topic belongs to the same organization, while
     retaining the existing cycle guard and runtime hop guard.
   - Validate that an attachment names an exact, current source endpoint.
     Attachments are terminal edges to Workers and therefore cannot create a
     Processor cycle. Processor-to-Trigger/Processor source wiring remains PR
     3 work and must not be partially introduced here.

7. **Prove compatibility and the new path.**

   - Domain tests cover SourceRef/event validation and stable Processor-output
     identity.
   - Repository suites cover Trigger and attachment CRUD, uniqueness,
     cross-tenant isolation, exact-output lookup, and cleanup.
   - Dispatcher tests cover attachment fan-out from a Trigger and from two
     different outputs of one Processor, ordering into a single Worker's
     activation queue, originating-Worker suppression, human-node rejection,
     missing/deleted endpoints, and colliding IDs across organizations.
   - Existing publishing, dispatch, Processor runner, Topic REST/MCP, Slack,
     cron, GitHub, GitLab, Postmark, and webhook tests must pass unchanged in
     observable behaviour. Add a regression test showing a legacy Topic event
     still performs append -> notify -> subscription dispatch -> Processor
     traversal without consulting attachments.
   - Run the org package tests and the required server/store build. Because PR
     1 has no user-visible route to the new model, browser E2E is not an
     acceptance gate; the existing Topics UI must still load against both a
     clean database and an upgraded database.

PR 1 is complete when the new Trigger and attachment records can be created and
dispatched in tests, Processor branch identity is durable, and every existing
Topic workflow behaves exactly as before. It must contain no Topic migration,
transport cutover, public Trigger API, outbound-action replacement, or Session
logging.

### PR 2: Outbound actions

Remove outbound communication from Topic semantics while retaining the current
frontend:

- Inventory every outbound use of Topic publishing, including Slack, email,
  webhook delivery, DMs, team briefings, and human delivery.
- Provide an MCP tool or sandbox CLI/API replacement for every supported
  outbound workflow. The exact tool surface is decided in this PR.
- Update Worker tool grants, prompts, and documentation for those replacements.
- Remove outbound emitters from dispatch and make Trigger transports strictly
  inbound.
- Retain Topic publishing only where it is still needed by the temporary
  frontend compatibility path or the not-yet-converted internal data path.

The removal gate is behavioural: no outbound workflow may be removed until it
has a named MCP or sandbox CLI/API replacement and an end-to-end test.

### PR 3: Trigger and Processor backend cutover behind the Topic API

Move the complete backend data path to the new model while projecting the old
API shape for the unchanged frontend:

- Persist Triggers and convert GitHub, GitLab, Slack, email, webhook, cron, and
  Helix-event ingress to emit from Trigger source references.
- Change Processors to consume a Trigger or Processor output and emit directly
  from their stable output IDs. Remove auto-provisioned Processor output
  Topics.
- Dispatch to Worker attachments rather than Topic subscriptions.
- Convert Slack routing and other reconcilers to Processor outputs and Worker
  attachments.
- Convert internal Worker messaging away from Topics, including `dm`,
  `reports`, `ask_human`, team briefing, and lifecycle reconciliation. Keep
  Session transcript consolidation as later work.
- Migrate existing Trigger-like Topics, Processor edges, and Worker
  subscriptions to the new records. Classify legacy local and outbound Topics
  explicitly rather than silently guessing their meaning.
- Keep the existing Topic REST responses, Processor DTOs, event/history calls,
  and graph operations working through a narrow projection adapter so the
  current frontend does not break.

The new Trigger, Processor, attachment, and action services are authoritative
after this PR. The compatibility adapter may translate requests and responses,
but must not dual-write or retain separate business rules.

### PR 4: Frontend cutover and Topic removal

Change the public model and frontend together, then delete the compatibility
surface:

- Replace the Topics UI with Triggers and update Worker assignments and the org
  chart for direct Trigger and Processor-output attachments.
- Update Processor editing and graph wiring to use source references and stable
  output IDs.
- Replace Topic and Subscription REST/MCP DTOs with Trigger and attachment
  surfaces, then regenerate the API client.
- Remove generic Topic publishing after all remaining callers use explicit
  actions or the new event path.
- Delete Topic CRUD, subscriptions, publishing services, repositories, tables,
  frontend components, and the temporary projection adapter.
- Reshape retained event storage around source references so a low-level event
  log does not recreate Topic as a domain abstraction.
- Update `api/pkg/org/QA.md` and the older Topic, Processor, Slack, and human
  delivery designs to describe the final model.

This is the only PR allowed to break the old Topic API, and it must include the
corresponding frontend changes. Verify both an upgraded database and a clean
installation end to end: external event -> Trigger -> optional Processor chain
-> attached Worker.

## Deferred work

The following work is not part of these four PRs:

- Activation and event logging into Worker-owned Helix Sessions.
- Historical transcript migration and retention policy.
- Further convergence with the existing non-org Helix Trigger definition.
- Additional outbound tools that are not required to preserve an existing
  workflow.
