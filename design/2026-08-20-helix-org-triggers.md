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
  DMs, Chats, and other internal or external actions.
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
but deliberately does not prescribe every outbound tool, the detailed Topic
frontend design, or the later Activation/log-to-Session consolidation. The
central rule is: attaching a Trigger or Processor controls what can start a
Worker; it never grants or performs an outbound action.

## Implementation plan

Use five PRs. PRs 1 through 3 preserve the existing Topic REST DTOs and
Topic/graph frontend behaviour while the backend foundations are added. PR 2
may add the independent Worker-secret configuration surface described below.
PR 4 deliberately breaks and removes the Topic backend after converting
existing data to Triggers. PR 5 replaces the frontend and publishes the final
Trigger API.

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
authoritative production routing path until PR 4.

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
     migration work for PR 4.

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
     yet; PR 4 performs that cutover.
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

### PR 2: Dynamic Worker secrets

Give Workers one source-agnostic secret namespace and resolve values through
MCP at the point of use:

- `list_secrets` lists the secret names available to the calling Worker without
  returning values.
- `get_secret` returns the current credential value for one name.
- Each name is backed either by a project-scoped Helix Secret or by an
  explicitly granted export from an organization-connected account. Connected
  Accounts are the organization-scoped credential source; this PR does not add
  organization-scoped Helix Secrets.
- There is no literal-value source. A user who wants to supply a value creates
  a project Secret through the existing Helix Secrets flow and grants it to the
  Worker.
- Connected Account values are never injected into the sandbox environment at
  boot. Existing project-Secret injection remains as a compatibility surface
  until Worker binding management is user-visible; new dynamic access uses
  `get_secret`. Helix-owned bootstrap variables remain a separate system concern.

The Worker does not need to know the source. It may export a returned token,
construct an authentication header, write a JSON credential file, pass the
value to a subprocess, or store it elsewhere. A refreshable or minted account
credential is resolved again on every `get_secret` call, which makes this model
usable by the long-running Sessions that are the default.

This deliberately gives the Worker the credential value. The value enters the
MCP result and therefore the agent's context; the Worker can read, retain,
print, or exfiltrate it. Redaction reduces accidental disclosure but is not a
security boundary. HTTPS credential brokering remains deferred.

#### PR 2 implementation plan

1. **Define a stable Worker secret binding.**

   - Add `WorkerSecretBinding` with organization ID, Worker ID, unique name,
     optional description/usage metadata, and exactly one typed source:
     `helix_secret` or `connected_account`. Do not store a resolved value on the
     binding and do not use `map[string]interface{}` for the source union.
   - A Helix Secret source stores only an immutable project `secret_id`. A
     Connected Account source stores a typed account reference plus a stable
     export key, for example `slack_workspace/bot_token`,
     `github_app/installation_token`, `postmark/server_token`, or
     `oauth/access_token`.
   - The binding name is the agent-facing interface and should normally match
     the provider convention, such as `GH_TOKEN`, `SLACK_BOT_TOKEN`,
     `POSTMARK_SERVER_TOKEN`, or `GOOGLE_APPLICATION_CREDENTIALS_JSON`.
     Renaming or replacing a backend account must not silently rename the
     Worker's secret.
   - Allow string values, including structured JSON. Add non-secret metadata
     such as `content_type`, `suggested_filename`, `usage`, and optional expiry
     so the agent can decide whether to export the value, put it in a header,
     or write it to a file. Binary values are out of scope unless a concrete
     existing workflow requires an explicit base64 encoding contract.
   - Reject empty/duplicate names and reserve Helix bootstrap names such as
     `USER_API_TOKEN` and `HELIX_*`. Only explicitly configured bindings appear
     in the Worker namespace; system credentials are never discoverable through
     `list_secrets`.

2. **Persist grants at the Worker boundary.**

   - Add an `org_worker_secret_bindings` table with composite organization/
     Worker ownership, a unique `(organization_id, worker_id, name)` constraint,
     explicit source columns, and database checks matching the source union.
     Register it in AutoMigrate and the canonical foreign-key cleanup path.
   - Treat a binding as a grant of raw-value access, not merely display
     configuration. Creating or updating a Helix Secret binding requires access
     to the Secret's project and permission to configure the target Worker.
     Creating a Connected Account binding requires membership of the account's
     organization and permission to configure the target Worker. Neither path
     requires Helix installation-administrator authority.
   - Re-authorize the binding and source on every `get_secret` call. Save-time
     validation is only fast feedback: Worker configuration permission, source
     project access, organization membership, account connection, and OAuth
     scopes may change during a long-running Session.
   - Deleting a Worker deletes its bindings. Deleting or disconnecting a source
     revokes it immediately and removes or marks dependent bindings unavailable
     in the same application operation; never leave a stale cached value usable
     after revocation.
   - Do not put these bindings on `BotDTO`, project settings, or Sandbox rows as
     a second source of truth. Expose them through a dedicated Worker-secret
     application service used by REST and MCP.

3. **Resolve values through explicit source adapters.**

   - Define one resolver contract that accepts the authenticated caller and
     binding and returns a typed result such as `{value, expires_at,
     content_type, suggested_filename, usage}`. The raw value exists only for
     the duration of the MCP request and any bounded provider cache.
   - Helix Secret resolution loads the exact project Secret, rechecks access to
     its project, and decrypts it. It does not infer identity by name and does
     not introduce an organization Secret scope.
   - Connected Account exports are registered adapters. Each adapter owns its
     safe export names, account/resource lookup, authorization, recommended
     secret name, metadata, and resolution:
     - Slack workspace: the installed bot token for the bound workspace.
     - GitHub App: a newly minted or still-valid installation token, never the
       App private key.
     - Postmark: the configured server token.
     - OAuth: refresh when necessary and return only the current access token,
       never the refresh token or provider client secret.
     - Structured credentials such as a GCP service account: the approved JSON
       document, never unrelated fields from the backing connection.
   - Only organization-owned accounts are eligible. A member's personal OAuth
     connection does not become an organization credential merely because the
     member can configure a Worker; exposing it requires a separate explicit
     delegation policy and is not part of this PR.
   - Never reflect credential-bearing `ServiceConnection`, OAuth, or config
     structs into the export catalog. Adding a provider requires an explicit
     adapter; this prevents accidentally exposing signing secrets, refresh
     tokens, private keys, or admin-only fields.
   - Serialize refresh-token rotation and persist the replacement refresh token
     before returning the new access token. A concurrent pair of `get_secret`
     calls must not invalidate the account or return a token from a lost refresh
     race.
   - Do not copy dynamic provider values into the Helix Secrets table. The
     binding remains stable while each call resolves the provider's current
     value.

4. **Make `list_secrets` metadata-only and add `get_secret`.**

   - Change the existing `list_secrets` MCP tool, which currently returns the
     calling Worker's decrypted project-secret map, to return stable descriptors
     only: name, description/usage, content type, suggested filename, and
     optional availability/expiry metadata. It must not return a value or reveal
     whether the source is stored, static, refreshable, or minted.
   - Add `get_secret(name)` as the single credential-retrieval primitive.
     Resolve the caller's organization and Worker identity from
     `tool.Invocation`; accept no
     organization, Worker, Secret, account, provider, or resource ID from the
     model. The name must resolve through that Worker's explicit binding.
   - Return `{name, value}` plus only useful optional metadata. The credential
     string is intentionally available to the model. Register it with the
     existing Session redactor before returning, but do not claim that redaction
     prevents deliberate disclosure.
   - Resolve on every call. A short bounded cache inside a provider adapter may
     reuse a still-valid token, but `get_secret` must never serve a stale value
     cached in the Worker binding, Session, MCP server, or sandbox environment.
   - Delete `mint_credential` and fold its GitHub/Slack provider resolvers,
     authorization, expiry metadata, prompts, and auditing into
     `get_secret`. Do not retain two generic raw-credential primitives.
   - Update the tool descriptions to tell the Worker to call `get_secret`
     immediately before an authenticated operation and again after a 401/403.
     The agent remains free to retain a value when the provider semantics make
     that appropriate.

5. **Do not expand environment injection.**

   - Do not add bindings to `CreateSandboxRequest`, `ExternalAgentConfig`,
     `DesktopAgent.Env`, Hydra requests, Docker configuration, startup scripts,
     or Session metadata.
   - Preserve existing project-Secret injection for helix-org Worker desktops
     until binding management is user-visible, so deploying this backend does
     not silently break existing Workers. Do not inject Connected Account
     values or newly configured bindings. The later UI/configuration cutover
     removes this compatibility path after users can grant replacements.
   - Do not prefetch values during activation or sandbox creation. A stopped or
     warm long-running Worker has the same secret interface, and rotating a
     Secret or account takes effect on the next `get_secret` call without a
     restart.
   - Preserve Helix-owned runtime variables needed for the desktop, API proxy,
     model access, and Worker bootstrap. “No environment injection” here means
     no user-configured or Connected Account secret material, not removal of the
     sandbox's system configuration.

6. **Make direct credential access explicit and auditable.**

   - Register `list_secrets` and `get_secret` for Workers by default. Do not add
     a feature flag or a second disabled mode: direct retrieval is the PR 2
     behaviour, while HTTPS credential brokering is separate deferred work.
   - Audit every list and get with organization, Worker, Session, binding name,
     source identifier, success/failure, and provider expiry. Never log the
     value, a prefix, structured credential fields, request headers, or tool
     result.
   - Record resolved values with the existing per-Session redactor. Ensure
     metrics and traces use binding/source identifiers rather than values.
   - State the boundary in the UI, tool description, and operator
     documentation: granting a binding authorizes that Worker to obtain and use
     the credential value however it chooses.

7. **Expose binding management without exposing values.**

   - Add generated-client CRUD endpoints under the existing helix-org Worker
     API for secret bindings, plus
     `GET /api/v1/helix-org/{org_id}/workers/{worker_id}/available-secrets`.
     Resolve every `{org_id}` through `lookupOrg`. Responses contain
     binding/source metadata only.
   - Put listing bindings, listing grantable sources, projecting descriptors,
     authorizing sources, and resolving values in one Worker-secret application
     service. The REST handlers and MCP tools are adapters over that service;
     they must not implement separate Secret/Connected Account joins or
     authorization rules. `list_secrets` returns descriptors for sources
     already bound to the calling Worker, while `available-secrets` returns the
     larger set the authenticated user may bind. They share domain code but are
     not the same authorization view.
   - The available-secrets response is the Worker's source catalog. It returns
     a normalized union of project Secrets the caller may grant and approved
     exports from Connected Accounts owned by the organization. It groups them
     as `Helix Secrets` and `Connected Accounts`, includes proposed aliases and
     whether each source is already bound, and never contains stored or
     resolved values, refresh tokens, private keys, or provider client secrets.
     Existing generic Secret and Service Connection list endpoints are not a
     substitute: the backend must apply source-specific authorization and
     export-adapter rules for this exact Worker.
   - Build one Worker Secrets editor with name, optional usage metadata, Source
     (`Helix secret` or `Connected account`), source selector, and remove
     action. Selecting a source proposes its conventional name and usage, but
     the user may change the alias.
   - Do not offer a literal value field. Link to the Helix Secret creation flow
     when a user needs to provide a new value. Helix Secrets remain
     project-scoped and retain their existing non-Worker environment-injection
     behaviour. Account connection/installation remains a separate
     organization operation; binding one of its approved exports is the
     explicit grant to the Worker.
   - Use React Query and the generated API client. Show disconnected/deleted
     sources as unavailable without revealing prior values, and always show the
     warning that granting a binding exposes its credential value to the
     Worker.

8. **Cut over the Worker tool surface without migration.**

   - Do not backfill Worker bindings. Helix-org Workers do not currently use
     project Secrets through this model, so there is no project-secret state to
     migrate. Connected Account values remain in their existing source records
     and are resolved through the new adapters rather than copied or migrated.
   - Remove `mint_credential`, register `list_secrets` and `get_secret` in the
     default Worker MCP surface, and update seed Roles, prompts, MCP help, and
     `api/pkg/org/QA.md` to describe the new interface. Workers discover the
     changed surface when they next read the MCP tool list; no Session or
     Worker migration is required.
   - Make the tool-description cutover atomic: no shipped prompt should tell a
     Worker to call removed `mint_credential` or expect values from
     `list_secrets`.

9. **Test the backend through its user-visible use cases and trust
   boundaries.**

   - Write application-service tests at the Worker-secret use-case boundary,
     using fake repositories and source adapters rather than testing private
     helper methods. Cover: listing grantable sources, granting each source
     type, listing only the Worker's bindings, resolving a project Secret,
     resolving static and refreshable Connected Account exports, replacing an
     alias, revoking a binding, source deletion/disconnection, membership or
     project-access removal after grant, expired access-token refresh, refresh
     failure, and two concurrent refreshes. Assert the complete use-case result
     and persisted state, not merely that a repository method was called.
   - Return typed application errors with stable machine codes and safe,
     actionable user messages. When the caller can repair the problem, say
     what to do, for example: add the secret in **Worker > Secrets**, reconnect
     the account in **Organization Settings > Connected Accounts**, request
     access to the source project, or select another available source. Do not
     expose provider responses, credential material, or whether a guessed
     cross-tenant source ID exists. Unauthorized/not-found cases must use a
     uniform response where distinguishing them would create an enumeration
     oracle.
   - Add REST integration tests through the real router and authorization
     middleware for binding CRUD and
     `GET /api/v1/helix-org/{org_id}/workers/{worker_id}/available-secrets`.
     Cover an organization ID and slug, unauthenticated requests, a valid
     organization member, a member who cannot configure the target Worker, and
     an authenticated user outside the organization. Verify the catalog and
     binding responses contain metadata only and that no REST route returns a
     credential value.
   - Exercise confused-deputy and cross-tenant attacks explicitly. An attacker
     must not be able to combine their own authenticated identity with another
     organization's `org_id`, Worker ID, project Secret ID, Connected Account
     ID, export key, binding ID, or personal OAuth connection. Also reject a
     same-organization Secret for which the caller lacks source-project access,
     a Connected Account not owned by the organization, an unregistered export
     key, mass-assigned organization/Worker/source fields, and a binding that
     targets a reserved Helix runtime name. Re-run authorization during
     `get_secret` so removing membership, project access, or the backing
     connection immediately denies an already-created binding.
   - Use a unique canary credential in boundary tests and assert it appears
     only in the successful `get_secret` result. It must be absent from
     `list_secrets`, `available-secrets`, binding CRUD responses, error bodies,
     audit records, application logs, metrics, and traces. After revocation, no
     provider cache may continue serving it.
   - Test MCP discovery through the real in-process MCP server with a
     Worker-bound Session identity. The advertised tool list must contain
     `list_secrets` and `get_secret`, must not contain `mint_credential`, and
     must not advertise Worker-secret tools to an unrelated non-Worker
     Session. Invoke both tools through that boundary, proving that caller
     organization and Worker identity come from the authenticated Session and
     cannot be supplied or overridden in tool arguments. A Session-token/org
     mismatch and a Worker ID from another organization must fail without
     revealing bindings or source metadata.
   - Defer browser/UI tests until the Worker Secrets editor exists. PR 2's
     backend acceptance does not depend on a mocked or placeholder frontend;
     the REST and MCP boundary tests define the contract the later UI consumes.

PR 2 is complete when Workers have one explicitly granted secret namespace,
`list_secrets` never returns values, `get_secret` resolves current credential
values from project Helix Secrets and organization-connected accounts in the
same long-running Session, Connected Account values are never injected at
sandbox launch, and existing project-Secret injection remains unchanged until
the user-visible binding cutover.
It must remove `mint_credential`, add no provider-action MCP wrapper, preserve
the unrelated Topic compatibility surface, and introduce no Trigger/Processor
cutover logic.

### PR 3: Outbound actions

Remove outbound communication from Topic semantics while retaining the current
Topic frontend:

- Inventory every outbound use of Topic publishing, including Slack, email,
  webhook delivery, DMs, Chats, and human delivery.
- Replace external delivery with native sandbox commands using values obtained
  through PR 2 `get_secret`. Add no provider-specific MCP send wrappers.
- Keep MCP tools only where Helix itself must enforce org-graph policy, such as
  Worker-to-Worker or Worker-to-human communication.
- Update Worker grants, prompts, and documentation for those replacements.
- Remove outbound emitters from dispatch and make Trigger transports strictly
  inbound.
- Retain Topic publishing only where the temporary Topic compatibility path or
  not-yet-converted internal data path still needs it.

The removal gate is behavioural: no outbound workflow may be removed until it
has a named native sandbox or Helix-internal action and an end-to-end test.

#### PR 3 implementation plan

PR 3 separates an explicit Worker action from an event arriving on a source.
It does not migrate ingress, Processor wiring, Worker attachments, DM/team
delivery, or the Topic REST DTOs. Those remain on the compatibility path until
PR 4. In particular, this PR must not introduce an outbound Trigger kind or an
`action` source reference: actions do not start Workers and are not graph
edges.

1. **Freeze the outbound inventory and replacement matrix.**

   - Trace every call to `Publishing.Publish`, `PublishWithReceipt`,
     `Dispatcher.Dispatch`, `RegisterOutbound`, and `RegisterDeliverer`, plus
     every direct provider call. Classify each path as inbound activation,
     internal Worker messaging, external action, transcript/event logging, or
     temporary Topic API compatibility. Record the caller, authorization
     boundary, provider configuration, delivery guarantees, receipt shape,
     retry behaviour, and current tests.
   - The initial replacement matrix is:
     - Slack text, replies, reactions, uploads, and edits: call
       `get_secret` for the explicitly granted workspace token and use the
       Slack Web API from the sandbox. Bind each permitted workspace under a
       stable name with non-secret team metadata so an inbound `team_id` can be
       matched without accepting a resource ID in `get_secret`.
     - GitHub and GitLab actions: call `get_secret` for the appropriate token,
       then use `gh`, `glab`, git HTTPS, or the provider API. A GitHub App
       binding returns an installation token, never the App private key.
     - Email: call `get_secret` for `POSTMARK_SERVER_TOKEN` (or the configured
       alias), then invoke the provider REST API. Do not add `send_email` or an
       equivalent Helix email proxy endpoint.
     - Outbound webhook: use `curl`, retrieving any authentication value
       through `get_secret`. Non-secret URLs stay in Role/configuration data.
       Do not add a generic server-side URL-fetch MCP tool.
     - Human contact: keep `ask_human` and the `HumanDelivery` port. Its in-app
       and Slack-DM routes are explicit Helix delivery, not Topic publishing.
     - One-to-one Worker messaging and Chats: keep the existing `dm`
       and `reports` + `publish` compatibility paths in this PR. They are
       Helix-internal delivery, not external provider actions; PR 4 decides the
       smallest direct-messaging surface while replacing their Topic storage.
   - Fail the inventory gate if a current outbound route has no row, named
     replacement, owner, and acceptance test. Provider webhook management such
     as installing a GitHub webhook is control-plane ingress setup, not
     outbound messaging, and stays with Trigger provisioning.

2. **Use `get_secret` through native provider interfaces.**

   - A Worker calls `get_secret` immediately before an authenticated operation,
     then uses the raw value however the provider requires: environment
     variable, authentication header, SDK configuration, subprocess input, or
     credential file. API-shape knowledge belongs in Role text or reusable
     skills, not Go wrappers.
   - The secret name is stable while PR 2 resolves stored, static, refreshable,
     minted, or structured values behind it. Provider/resource IDs never appear
     in the action prompt or tool arguments.
   - Do not imply the value is protected after retrieval. Prompts should avoid
     accidental printing or command-line exposure, and Helix redacts known
     values from logs and transcripts, but the Worker is deliberately allowed
     to store or disclose the value after retrieval.
   - On a 401/403, call `get_secret` again. If a non-idempotent request may
     already have reached the provider, inspect provider state before retrying;
     do not blindly duplicate an email or Slack post.
   - External API actions are independent of `eventsource.Event`, attachments,
     subscriptions, Processor traversal, the wake bus, and activation queues.
     They must not manufacture a Topic event merely to obtain an audit trail.
     Provider responses are the delivery receipts; prompts must tell Workers
     how to distinguish transport success from application success, such as
     Slack's HTTP 200 with `ok=false`.

3. **Move email to the dynamic-secret/API path.**

   - Split `transport.postmark` so inbound alias and configured From identity
     remain transport configuration, while the outbound server token becomes
     an explicit Connected Account export or Helix Secret binding. Migrate the
     existing token without copying it into a Worker binding or API response.
   - Document Postmark's canonical `/email` request, retrieving the token with
     `get_secret`, placing it in `X-Postmark-Server-Token`, and inspecting both
     HTTP status and response body. Do not add a `send_email` MCP action.
   - Remove sending mechanics from `postmark.Transport.Emit`; retain its
     inbound parsing and alias lookup untouched for PR 4. Update seed Roles and
     examples that currently use an email Topic for delivery.
   - Tests cover tenant and Worker authorization, secret resolution,
     From/To/Reply-To/thread mapping, text and HTML, malformed addresses and
     headers, provider non-2xx and timeout errors, attachment limits, and
     absence of any Topic append or Worker activation.

4. **Keep internal and human actions at the correct boundary.**

   - Keep `dm`'s reporting-line authorization and `ask_human`'s human-node
     validation unchanged. These tools enforce Helix graph/contact policy and
     therefore are legitimate domain primitives, unlike provider-specific send
     wrappers.
   - Keep Chats on `reports` + `publish` until PR 4. Designing their
     direct replacement before Topic subscriptions become attachments would
     create another temporary abstraction. PR 4 should decide whether `dm` can
     remain separate from one generic `chat` action.
   - Do not migrate DM channels, team channels, human reply routing,
     reconcilers, event history, or their current response fields in this PR.
     Compatibility tests must keep these workflows unchanged, including the
     immediately following reply after a DM or Chat message.

5. **Move legacy Topic delivery behind one compatibility adapter.**

   - Split the current publish pipeline into an internal Topic event core
     (validate -> append -> notify -> Topic subscription dispatch -> Processor
     traversal) and a narrow legacy-delivery decorator. The core never calls an
     external provider. The decorator exists only so the unchanged Topic MCP
     and REST APIs, Processor output path, and current frontend retain their
     observable PR 1 behaviour until the PR 4 cutover.
   - The decorator owns the current direct provider clients for legacy Slack,
     email, and outbound-webhook Topic configuration. It preserves current
     inbound-loop suppression, synchronous/asynchronous behaviour, and response
     receipts. There must be one translation point, not delivery checks
     scattered across publishing, dispatch, and transports.
   - Mark the adapter and its tests as PR 4 deletion targets. New MCP tools,
     prompts, and application services must not call it. Do not add new
     outbound fields to Trigger or Processor DTOs. The unchanged Topic API may
     still create legacy bidirectional configuration until PR 4; no new API or
     domain service may expose that configuration as part of the target model.

6. **Remove outbound behaviour from dispatch and target transports.**

   - Delete `Dispatcher.outbound`, `RegisterOutbound`, and `emitOutbound`.
     `Dispatch` becomes activation and Processor traversal only; delivery
     latency or provider failure can no longer affect or race source fan-out.
   - Remove `streaming.Outbound` once no caller remains. Keep the Postmark,
     webhook, and Slack clients only behind the temporary compatibility
     decorator; native Worker actions call providers directly from the sandbox.
     Keep inbound Slack, Postmark, GitHub, GitLab, webhook, cron, and Helix-event
     handlers unchanged for PR 4.
   - Update `domain/transport` comments and validation so kinds describe
     inbound routing on the target model. Legacy `outbound_url`, email send
     behaviour, and Slack `channel_id` remain parseable only for the Topic
     compatibility adapter; no Trigger service may consume them as outbound
     configuration.
   - Remove the composition-root outbound registrations in `helix_org.go`.
     Connected Account export resolvers belong to the PR 2 Worker-secret
     service, while legacy provider clients are injected only into the
     compatibility decorator. Do not create a control-plane provider adapter
     for the new native sandbox path.

7. **Update grants, prompts, documentation, and compatibility tests.**

   - Replace instructions that say `publish` posts to Slack or briefs a team
     with the replacement matrix. Document `list_secrets`, `get_secret`, native
     CLI/REST calls, provider-specific success checks, materialization examples,
     and the re-fetch-on-401 path without printing returned values.
   - Update seed Roles, role-drafting prompts, MCP help, tool descriptions, and
     `api/pkg/org/QA.md`. Add no provider-specific send tools. Preserve
     `publish` only for temporary internal/event compatibility cases and
     describe that boundary plainly.
   - Add regression tests proving a Topic event activates subscriptions and
     Processors without consulting an outbound registry, while each legacy
     Topic API route still produces exactly one compatible external delivery.
     Cover inbound Slack/email/webhook events to prove they never echo out.
   - Run all org package tests and the required server/store builds. From one
     live long-running Worker Session, exercise email and Slack delivery, `gh`,
     token refresh through a second `get_secret`, `ask_human`, a controlled
     webhook, and writing a structured credential to a file. Exercise `dm` and
     a Chat message through the unchanged compatibility path, including the
     recipient's next normal operation. Also exercise the unchanged frontend
     Topic publish flow for each externally delivering legacy kind. Tests must
     assert provider receipt and the specified absence/presence of Topic events
     and Worker activations; a mocked state transition alone does not satisfy
     the removal gate.

PR 3 is complete when every existing outbound workflow has a tested named
action, external provider actions use native sandbox tools with values obtained
through `get_secret`, `Dispatcher` and the event-source path contain no
external delivery, and all remaining Topic-driven delivery is isolated in the
deletion-marked compatibility adapter. It must contain no ingress cutover,
Trigger migration, Processor source rewiring, attachment migration, internal
DM/team storage replacement, Topic API removal, or Session logging.

### PR 4: Trigger and Processor backend cutover

Move the complete backend data path to the new model and delete the old Topic
runtime rather than maintaining a compatibility projection. The existing
Topics frontend may break between this PR and PR 5; PR 5 replaces it with the
Trigger UI.

Every existing workflow Topic converts one-to-one into a Trigger with the same
organization and ID. The only structural exception is an implementation-owned
Processor output Topic, which becomes its owning Processor's existing stable
output. Preserve only a Topic's inbound transport configuration; outbound
configuration and delivery are deliberately discarded. This is safe because
outbound Topic routes are unsupported after PR 3. Keeping the source identity
is the critical history invariant: existing persisted events are not copied or
rewritten.

Processors remain Processors. Their inputs move from Topic IDs to exact Trigger
or Processor-output references, and their existing stable output IDs replace
auto-provisioned output Topics. Existing subscriptions become Worker
attachments to the corresponding Trigger or Processor output.

#### PR 4 implementation plan

PR 4 is one deliberately breaking data-plane cutover. It uses idempotent Go
application migration code over repository interfaces, not database-specific
migration tables, checkpoints, cutover flags, dual writes, or a runtime
fallback to Topics.

1. **Define and test the one-to-one conversion.**

   - Add a pure conversion from every persisted Topic to a Trigger. Preserve
     organization, ID, name, description, creator, timestamps, and the inbound
     part of the transport configuration.
   - Map every current Topic transport kind explicitly. Local Topics become
     local/internal Triggers; GitHub, GitLab, Slack, Postmark email, webhook,
     cron, and Helix-event Topics retain their inbound configuration. Strip
     Slack channel delivery, email sending, outbound webhook URLs, and every
     other outbound-only field.
   - Treat an unknown transport kind or malformed inbound configuration as an
     error. Do not infer meaning from a Topic name and do not silently skip a
     row.
   - Preserve the Topic ID as the Trigger ID. Existing `Store.Events` rows and
     event history remain unchanged and continue to use that identifier. For a
     Processor-owned output Topic, retain the deterministic mapping from its
     stable output ID to the existing event-store key. This PR changes the
     domain overlay and event-service lookup, not event persistence or rows.
   - Convert subscriptions to attachments. A subscription to an ordinary Topic
     becomes an attachment to its same-ID Trigger. A subscription to a
     Processor-owned output Topic becomes an attachment to that Processor's
     exact stable output ID.
   - Convert each Processor input Topic ID to the matching Trigger or exact
     Processor output. Preserve Processor identity, kind, configuration,
     output identity, and branch order.
   - Implement conversion as repeat-safe application code using the Topic,
     Trigger, Processor, subscription, and attachment repositories. Existing
     target rows with the expected values are success; conflicting target rows
     are errors. Add no migration-specific database schema or state machine.

2. **Make Triggers own inbound transports.**

   - Add one Trigger application service for create, update, delete, list/get,
     validation, provisioning, and reconciliation. Provider-specific webhook,
     socket, scheduler, and alias setup remains behind the existing transport
     adapters.
   - Convert GitHub, GitLab, Slack, Postmark inbound email, generic webhook,
     cron, and Helix-event handlers to resolve an organization-scoped Trigger
     and emit one canonical `eventsource.Event`.
   - Preserve existing authentication, parsing, provisioning, reconciliation,
     and failure behaviour. This PR changes domain ownership and routing; it
     does not add new ingress security or reliability features.
   - Scheduler and socket managers enumerate Triggers rather than Topics.

3. **Move Processor execution to source references.**

   - Replace `Processor.InputTopicID` with an exact `eventsource.SourceRef`. A
     Processor consumes either one Trigger or one exact output of another
     Processor.
   - Make every branch emit an `eventsource.Event` from its durable output ID.
     Preserve the input event metadata, organization, originating Worker, and
     deterministic branch order.
   - Retain existing cycle validation and the runtime hop guard, expressed in
     terms of source references.
   - Remove Processor output-Topic creation, update, publication, and deletion.
     Processor lifecycle owns the Processor and its stable outputs only.

4. **Cut dispatch from subscriptions to attachments exactly once.**

   - Wire every Trigger and Processor output to source dispatch. Resolve
     attachments by `(organization, exact SourceRef)`, preserve repository
     ordering, deduplicate Workers, suppress the originating Worker, reject
     human nodes, and enqueue through the existing activation queue.
   - Remove subscription lookup and Topic Processor traversal when attachment
     dispatch becomes active. Do not run both paths for one event.
   - Keep durable delivery on Priya's existing agent-delivery queue. Its NATS
     durable stream, per-Worker FIFO consumer, acknowledgement, retry, restart
     recovery, and Worker cleanup remain the delivery mechanism; PR 4 does not
     create another queue or persistence layer.
   - Preserve the current ordering contract for one source and one Worker.

5. **Move internal messaging off Topics.**

   - Model DMs and Chats as system-managed Triggers using the same event,
     attachment, dispatch, history, and durable activation path. A send remains
     an explicit action by the sender; the Trigger is the inbound source from
     each recipient's perspective.
   - A DM Trigger represents one reporting-line conversation. A Chat Trigger
     represents one named multi-Worker conversation. Only their lifecycle and
     membership services may create them or manage attachments, and only `dm`
     and `chat` may append Worker-authored events.
   - Replace the `reports` plus `publish` sending workflow with `chat`. Update
     MCP tools, prompts, tests, and backend API language now; the replacement UI
     belongs to PR 5.
   - Keep `ask_human` and `HumanDelivery`. Move human replies to a dedicated
     system-managed conversation Trigger only where required to remove the
     remaining Topic dependency.
   - Preserve existing event history by retaining the converted Topic ID for
     each existing DM or Chat Trigger.

6. **Delete the active Topic backend.**

   - After successful conversion, make Trigger, Processor, attachment, and
     internal-action services the only runtime path.
   - Remove Topic CRUD, generic Topic publishing, subscriptions, Topic-based
     dispatch, Processor output Topics, reconcilers that create Topics, and the
     PR 3 legacy-delivery adapter. Do not add a read/write Topic projection.
   - Old Topic persistence may remain temporarily only as the input read by the
     repeat-safe conversion. No runtime service may create or update Topic or
     subscription rows after the cutover. Physical table cleanup can follow
     once deployed data has converted.
   - Remove or disable old Topic REST and MCP operations rather than translating
     them. PR 5 introduces the public Trigger and attachment surfaces used by
     the replacement frontend.

7. **Verify migration and the new path.**

   - Test conversion from every Topic transport kind, including removal of all
     outbound fields, same-ID history preservation, subscriptions to Trigger
     attachments, Processor-output subscriptions to exact-output attachments,
     Processor chains, repeat execution, malformed rows, conflicts, dangling
     references, and colliding IDs in different organizations.
   - Test Trigger create/update/delete and reconciliation for each existing
     inbound kind without expanding its security or reliability semantics.
   - Test complete routing through a Trigger, zero/one/many Processor branches,
     chained Processors, attachment fan-out, originating-Worker suppression,
     human-node rejection, queue ordering, restart recovery, and source/output
     deletion.
   - Test DM then reply, Chat message then recipient response, reporting-line
     removal then denied DM, participant removal then denied Chat send/no wake,
     human question then reply, and Worker deletion then recreation.
   - Run all org package tests and required server/store builds. Run an upgraded
     local stack with existing Topic data, execute the conversion, send real
     inbound events through every configured transport available in the test
     environment, inspect retained history, and exercise the immediately
     following Worker operation.
   - Do not make the old Topics UI an acceptance gate. Confirm that its removed
     calls fail deliberately rather than mutating the retired Topic model; PR 5
     supplies the replacement browser acceptance test.

PR 4 is complete when every existing workflow Topic has a same-ID Trigger
containing only inbound configuration, every Processor-owned output Topic has
become its stable Processor output, Processors use source references,
subscriptions have become attachments, all activation uses the existing
durable delivery queue, internal messaging no longer relies on Topics, and no
runtime code reads or writes Topic semantics. It contains no compatibility
projection, dual write, runtime fallback, new migration database machinery,
Session transcript consolidation, or unrelated ingress hardening.

#### Future work after the Topic removal lands on `main`

- Strengthen ingress verification consistently across providers, including
  signature handling, replay windows, request-size limits, and content-type
  enforcement.
- Add provider-delivery idempotency where existing provider semantics do not
  already supply it.
- Standardize typed application errors, HTTP status mapping, correlation IDs,
  safe operator diagnostics, and error redaction across Trigger endpoints.
- Physically remove retired Topic and subscription tables after deployed data
  has been converted and verified.

### PR 5: Trigger frontend and public API

Publish the new public model and replace the retired Topics frontend:

- Replace the Topics UI with Triggers and update Worker assignments and the org
  chart for direct Trigger and Processor-output attachments.
- Update Processor editing and graph wiring to use source references and stable
  output IDs.
- Add Trigger, source-reference, and attachment REST surfaces, then regenerate
  the API client.
- Delete retired Topic frontend components and routes.
- Update `api/pkg/org/QA.md` and the older Topic, Processor, Slack, and human
  delivery designs to describe the final model.

PR 4 has already removed the old Topic API, so this PR does not provide a
compatibility surface. Verify both converted existing data and a clean
installation end to end: external event -> Trigger -> optional Processor chain
-> attached Worker.

#### PR 5 implementation plan

1. **Publish the final Trigger and attachment API contract.**

   - Add generated-client REST CRUD for Triggers, Processor source/output
     wiring, Worker attachments, source event history, provider setup/status,
     and graph reads. Resolve every `{org_id}` with `lookupOrg`; ignore or
     reject ownership fields supplied in request bodies.
   - Use source references as a closed discriminated union. Processor output
     IDs are required wherever a Processor is selected. Responses include the
     display metadata the UI needs without forcing it to join or interpret
     provider secrets.
   - Specify pagination, stable ordering, optimistic-concurrency behaviour,
     provider status, and typed error bodies in Swagger; regenerate the API
     client and use it everywhere. Do not retain raw frontend `fetch` calls or
     hand-maintained duplicate DTOs.

2. **Replace Topics navigation and list/detail surfaces with Triggers.**

   - Replace the Topics sidebar route, list, cards, create flow, detail editor,
     status, setup actions, and event tail with Trigger equivalents. Preserve a
     deliberate redirect or explanatory tombstone for old bookmarked Topic
     URLs; never render a blank page or silently select the wrong Trigger.
   - Use `SimpleTable`, `CardGrid`, `ViewModeToggle`, Lucide toolbar icons, and
     generated-client React Query hooks. Invalidate queries after mutations and
     use `matchesAllTokens()` for filtering.
   - Provider forms expose inbound fields only. Secrets use the existing
     Connected Account/Worker-secret surfaces and never round-trip through a
     Trigger form. Show provisioning state and actionable repair steps without
     displaying webhook secrets, tokens, or raw provider failures.

3. **Make source attachments explicit in Worker and graph editing.**

   - Replace Worker Topic subscriptions with attachments to Triggers or exact
     Processor outputs. Selectors group sources by type, show stable output
     labels plus IDs where needed for disambiguation, and prevent selecting a
     whole multi-output Processor.
   - Update the org chart so Trigger and Processor-output edges are visually
     distinct from reporting lines. Edge creation/deletion calls attachment or
     Processor APIs directly; no graph gesture publishes, grants a tool, or
     authorizes outbound access.
   - Handle concurrent deletion and stale output IDs: refresh the source list,
     preserve unsaved unrelated edits, identify the missing source, and tell
     the user to select a current Trigger/output. Never silently retarget by
     name or array position.

4. **Publish the final MCP and REST surfaces atomically.**

   - Add only MCP tools that express required org-graph primitives. Do not
     restore generic Topic publish/subscribe under Trigger names.
   - Add Trigger, Processor-source, attachment, graph, and history routes in the
     same PR as their frontend consumers. Old Topic routes remain absent.
   - Update Role prompts, MCP help, examples, API docs, and tests together.
     MCP discovery tests assert retired Topic tools remain absent and current
     tools have no legacy Topic arguments.

5. **Delete retired frontend and finish storage cleanup.**

   - Delete frontend Topic components, hooks, DTOs, routes, tests, and dead
     generated-client methods. Use repository search and compile-time interface
     removal to prove no first-party caller remains.
   - Physical removal of retired Topic and subscription tables is future work.
     Event rows remain in the existing event store and retain their source IDs;
     do not copy or reshape history in this PR.

6. **Make UI and API errors resolve the user's problem.**

   - Render field-level validation beside the offending Trigger/source field.
     Conflicts identify the name/output involved. Provider setup failures link
     to **Organization Settings > Connected Accounts** or the Trigger's repair
     action. Attached-output deletion tells the user which Workers to detach.
   - Permission and cross-tenant failures remain indistinguishable where
     disclosure would create an oracle. Unexpected errors show a correlation ID
     and preserve the user's form input for retry.
   - Test the mapping from every stable backend code to its UI resolution text.
     A new user-fixable backend error cannot ship without a mapped resolution;
     non-user-fixable errors require a meaningful summary and correlation ID.

7. **Verify the final product, security boundary, and upgrade.**

   - High-level frontend tests cover the user use cases rather than component
     internals: create each Trigger kind, repair a failed provider setup, edit
     while preserving managed fields, connect Trigger -> Processor output ->
     Worker, detach/delete, filter/list, view live history, follow an old URL,
     and recover from a stale concurrent edit.
   - Playwright runs against the real local stack for both a clean database and
     a database upgraded from the last Topic release. Create two organizations
     with colliding Trigger, Processor, output, and Worker IDs. Verify each
     user's list, graph, selectors, event history, provider status, and network
     responses contain only their organization.
   - Exercise every ingress API with a valid event and the existing provider
     authentication and tenant-isolation cases. Exercise
     every Trigger/Processor/attachment REST mutation with unauthenticated,
     underprivileged, authorized, and foreign-organization callers.
   - Complete the canonical live flow for every transport:
     `external event -> Trigger -> optional multi-stage Processor -> attached
     Worker -> activation -> next Worker operation`. Assert branch selection,
     ordering, source metadata, and absence of outbound side effects.
   - Inspect browser console, network payloads, application/audit logs, metrics,
     traces, and database rows using unique canaries. Provider secrets,
     signatures, credential values, and foreign-tenant metadata must be absent.
   - Run `go test ./api/pkg/org/...`, required server/store builds, frontend
     unit tests, `yarn build`, generated-client drift checks, conversion tests,
     API integration tests, and the updated `api/pkg/org/QA.md` Playwright
     suite. Record any unavailable external-provider E2E as **NOT tested**;
     mocks do not satisfy that provider's release gate.

PR 5 is complete when the product exposes only Trigger/Processor/attachment
concepts, every old Topic caller and table has been intentionally handled, the
clean-install and upgrade Playwright suites pass, and API/security tests prove
authorization and tenant isolation at every public and ingress boundary.

## Deferred work

The following work is not part of these five PRs:

- HTTPS credential brokering that keeps real Connected Account tokens outside
  the sandbox, including placeholder capabilities, TLS interception, CA
  distribution, provider header transforms, and transparent token refresh.
- Externally enforced egress policy for privileged/nested sandboxes, whether
  implemented with a proxy gateway, network namespace controls, or eBPF. Do
  not rely on in-sandbox policy for this boundary.
- Activation and event logging into Worker-owned Helix Sessions.
- Historical transcript migration and retention policy.
- Further convergence with the existing non-org Helix Trigger definition.
- Additional outbound tools that are not required to preserve an existing
  workflow.
