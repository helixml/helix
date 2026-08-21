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
but deliberately does not prescribe every outbound tool, the detailed Topic
frontend design, or the later Activation/log-to-Session consolidation. The
central rule is: attaching a Trigger or Processor controls what can start a
Worker; it never grants or performs an outbound action.

## Implementation plan

Use five PRs. The first four must preserve the existing Topic REST DTOs and
Topic/graph frontend behaviour. PR 2 may add the independent Worker-secret
configuration surface described below. Topic compatibility in those PRs is a
temporary adapter over the new services, not a second implementation of the
domain. The final PR updates the Topic frontend and removes that adapter.

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
- No user-configured Secret or Connected Account value is injected into the
  sandbox environment at boot. Helix-owned bootstrap variables required to run
  the desktop remain a separate system concern.

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

5. **Keep environment injection out of the design.**

   - Do not add bindings to `CreateSandboxRequest`, `ExternalAgentConfig`,
     `DesktopAgent.Env`, Hydra requests, Docker configuration, startup scripts,
     or Session metadata.
   - Stop injecting project Secrets into helix-org Worker desktops. Existing
     non-org development workflows and production web-service Secret injection
     are separate compatibility surfaces and are not redesigned in this PR;
     the Worker path must use `get_secret`.
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
same long-running Session, and no user/account value is injected at sandbox
launch.
It must remove `mint_credential`, add no provider-action MCP wrapper, preserve
the unrelated Topic compatibility surface, and introduce no Trigger/Processor
cutover logic.

### PR 3: Outbound actions

Remove outbound communication from Topic semantics while retaining the current
Topic frontend:

- Inventory every outbound use of Topic publishing, including Slack, email,
  webhook delivery, DMs, team briefings, and human delivery.
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
     - One-to-one Worker messaging and team briefing: keep the existing `dm`
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
   - Keep team briefing on `reports` + `publish` until PR 4. Designing its
     direct replacement before Topic subscriptions become attachments would
     create another temporary abstraction. PR 4 should decide whether `dm` can
     grow a direct-reports target or whether one generic internal-message tool
     should replace both DM and briefing.
   - Do not migrate DM channels, team channels, human reply routing,
     reconcilers, event history, or their current response fields in this PR.
     Compatibility tests must keep these workflows unchanged, including the
     immediately following reply after a DM or briefing.

5. **Move legacy Topic delivery behind one compatibility adapter.**

   - Split the current publish pipeline into an internal Topic event core
     (validate -> append -> notify -> Topic subscription dispatch -> Processor
     traversal) and a narrow legacy-delivery decorator. The core never calls an
     external provider. The decorator exists only so the unchanged Topic MCP
     and REST APIs, Processor output path, and current frontend retain their
     observable PR 1 behaviour through PR 4.
   - The decorator owns the current direct provider clients for legacy Slack,
     email, and outbound-webhook Topic configuration. It preserves current
     inbound-loop suppression, synchronous/asynchronous behaviour, and response
     receipts. There must be one translation point, not delivery checks
     scattered across publishing, dispatch, and transports.
   - Mark the adapter and its tests as PR 5 deletion targets. New MCP tools,
     prompts, and application services must not call it. Do not add new
     outbound fields to Trigger or Processor DTOs. The unchanged Topic API may
     still create legacy bidirectional configuration until PR 5; no new API or
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
     team briefing through the unchanged compatibility path, including the
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

### PR 4: Trigger and Processor backend cutover behind the Topic API

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

### PR 5: Frontend cutover and Topic removal

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
