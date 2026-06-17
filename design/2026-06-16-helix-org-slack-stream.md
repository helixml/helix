# Helix-org Slack Stream — User Story & Requirements

- **Date:** 2026-06-16
- **Status:** Implemented (TDD) — pending live Slack credentials for full e2e. See "Implementation status" below.
- **Area:** `api/pkg/org/` (Streams / Transports), Slack integration

> **Implementation status (2026-06-16).** All phases of §11 are built test-first
> and green (36 tests in `api/pkg/org/infrastructure/transports/slack/`, plus the
> domain Kind tests and the `Dispatcher.DispatchTo` dispatcher tests). The
> transport is wired into the composition root (`helix_org_slack.go`) and mounted
> in `server.go`: `POST /api/v1/slack/events` (REST ingress), `GET
> /api/v1/slack/oauth/callback` (install callback), `GET
> /api/v1/orgs/{org}/slack/oauth/start` (membership-authorised install start), and
> the Socket Mode runner goroutine. The admin OAuth Providers panel gained the
> three Slack fields (signing secret / app token / ingress mode). Verified
> end-to-end at the API+DB level on localhost:8080: creating an enabled
> `type=slack` provider flips `/slack/events` from 503 (inert, FR-3) to 401
> (signature check active, NFR-4), and the three columns persist + round-trip.
> **Not yet exercised:** a real Slack workspace round-trip (OAuth install, inbound
> message → Worker activation, outbound persona post) — blocked on live Slack app
> credentials. Deferred per §13: human linking (FR-21/22/24), Worker-avatar
> source, and the `LLMRouter`.

> This document captures *what* we want and *why*. It deliberately does **not**
> describe how to build it. Implementation design follows in a separate doc once
> these requirements are agreed.

---

## 1. Context

Helix-org is the org-graph runtime: **Workers** fill **Positions** bound to
**Roles**, and communicate over **Streams** (named event channels, each backed by
a **Transport**). Transports already exist for `local`, `webhook`, `email`,
`github`, and `cron`.

We want a new **Slack** capability so that Helix-org Workers can participate in a
Slack workspace — posting into channels under their own identities and receiving
messages from those channels — without provisioning a real Slack account per
Worker.

### Key decisions already made (constraints on these requirements)

There are **three layers**, and the tenancy differs per layer:

1. **One global Slack *app* per Helix instance**, configured once by an
   administrator: client id, client secret, signing secret. One app definition
   for the whole installation.
2. **Per-org installation.** Each org OAuth-installs that one app into **its
   own** Slack workspace, producing a per-org **bot token + workspace/team id**.
   The app is global; the installation (and its token) is org-scoped.
3. **Org isolation.** Workers in an org act only against that org's installed
   workspace — its bot token, its channels. A Worker in org A cannot reach org
   B's Slack.
4. **Workers are personas of their org's bot**, not real Slack user accounts. A
   Worker's name and avatar are applied per message; a channel has exactly one
   real Slack member (the bot).
5. **Multiple Workers may operate in the same channel** — this is expected and
   acceptable.
6. **Two inbound ingress modes must both be supported** from a single
   implementation: an HTTP **REST** path (Slack Events API) and a **WebSocket**
   path (Slack Socket Mode). The operator/deployment chooses which is active.
   Their tenancy reach differs: **REST serves many per-org installs** (one
   endpoint, route by team id); **Socket Mode suits the single-workspace
   on-premise case**.

---

## 2. Actors

| Actor | Description |
|---|---|
| **Administrator** | Operates the Helix instance. Registers the one global Slack app (instance-wide). Decides whether to use the REST API or WebSocket path. |
| **Hiring manager / Worker** | A Helix-org Worker that creates Slack streams and subscribes other Workers (per existing org-graph conventions). |
| **AI Worker** | A software Worker that posts to and receives from Slack channels as a persona. |
| **Human Worker** | A person represented in the org graph who may also exist as a real user in the connected Slack workspace. |
| **Slack participant** | Any human in the connected Slack workspace interacting in a channel the bot is in. |

---

## 3. User Stories

**US-1 — Connect Slack (admin + org).**
As an **administrator**, I want to register one Slack app for the whole Helix
instance, and have each org install that app into its own Slack workspace, so
that Workers can use Slack — scoped to their own org — without per-Worker setup.

**US-2 — Bind a channel to a stream.**
As a **hiring manager**, I want to create a Slack stream tied to a specific
channel, so that I can route a conversation between that channel and the org
graph.

**US-3 — Connect Workers to a channel.**
As a **hiring manager**, I want to subscribe Workers to a channel-bound Slack
stream, so that those Workers send and receive messages in that channel.
Subscribing several Workers to the same channel is allowed.

**US-4 — Post as a persona.**
As an **AI Worker**, I want my messages in a channel to appear under my own name
and avatar, so that Slack participants can tell Workers apart even though we
share one bot.

**US-5 — Receive channel messages.**
As an **AI Worker** subscribed to a channel-bound stream, I want to receive
messages posted in that channel, so that I can act on them.

**US-6 — Deploy behind a firewall.**
As an **administrator** of an on-premise install that cannot expose inbound HTTP
endpoints, I want Slack to work over an outbound WebSocket connection, so that I
can use the integration without opening inbound network access.

**US-7 — Link an existing human (secondary).**
As an **administrator**, I want a Human Worker to be associated with their
existing Slack user account, so that the org graph knows which Slack person maps
to which Human Worker.

---

## 4. Functional Requirements

Priority: **MUST** / **SHOULD** / **MAY**.

### 4.1 Global app and per-org installations

**Global app (instance-wide, admin):**

- **FR-1 (MUST).** An administrator can configure exactly one global Slack app
  for the Helix instance (client id, client secret, signing secret).
- **FR-2 (MUST).** Configuring or changing the global Slack app is restricted to
  administrators.
- **FR-3 (MUST).** The integration is inert when no global app is configured —
  no errors surfaced to Workers, no partial behaviour.

**Per-org installation:**

- **FR-4 (MUST).** Each org can install the global app into its own Slack
  workspace, producing an org-scoped installation that stores the credentials
  required by the active ingress mode (at minimum a bot token + workspace/team
  id; plus what each mode needs — the app's signing secret for REST, an
  app-level token for WebSocket).
- **FR-5 (MUST).** An org has at most one Slack installation. Workers act only
  against their own org's installation; there is no cross-org access to Slack.
- **FR-6 (SHOULD).** An administrator (or org owner/admin, per existing org
  authorization) can see the health/status of an org's installation (e.g.
  installed / misconfigured / auth failed).

### 4.2 Channels and streams

- **FR-7 (MUST).** A Slack stream is bound to a single Slack channel within its
  org's installed workspace.
- **FR-8 (MUST).** A Worker is "connected to a channel" by subscribing to the
  corresponding channel-bound Slack stream, using existing org-graph
  subscription semantics.
- **FR-9 (MUST).** Multiple Workers may be subscribed to the same channel-bound
  stream simultaneously.

### 4.3 Outbound (Worker → Slack)

- **FR-10 (MUST).** A Worker can post a message to its channel through its org's
  bot.
- **FR-11 (MUST).** Outbound messages display the posting Worker's identity
  (name and avatar) derived from the Worker's identity, not the bot's default
  identity.
- **FR-12 (MUST).** Outbound behaviour is identical regardless of which inbound
  ingress mode is active.
- **FR-13 (SHOULD).** Replies preserve Slack threading where a thread context
  exists.

### 4.4 Inbound (Slack → Worker)

- **FR-14 (MUST).** The system supports an HTTP REST ingress (Slack Events API)
  with request authenticity verification.
- **FR-15 (MUST).** The system supports a WebSocket ingress (Slack Socket Mode)
  requiring no inbound HTTP endpoint.
- **FR-16 (MUST).** Exactly one ingress mode is active per deployment, chosen by
  an explicit administrator toggle — not inferred from which credentials are
  present.
- **FR-17 (MUST).** Inbound events are routed to the correct org by
  workspace/team id, and delivered only to that org's Workers (enforcing org
  isolation, FR-5).
- **FR-18 (MUST).** Messages posted in a bound channel are delivered to all
  Workers subscribed to that channel-bound stream.
- **FR-19 (MUST).** Both ingress modes deliver inbound events to a single shared
  processing path, so message handling is identical across modes.
- **FR-20 (SHOULD).** The bot's own messages do not re-enter the system as
  inbound events (no self-echo loops).

### 4.5 Human linking (secondary capability)

- **FR-21 (SHOULD).** Human Workers are automatically associated with existing
  Slack users in their org's workspace by matching email addresses. (Depends on
  a Human Worker email attribute that does not yet exist — deferred to a future
  task; see §7 Dependency.)
- **FR-22 (MAY).** The mapping (Slack user ↔ Human Worker) is queryable for use
  by other org-graph features.

### 4.6 Messaging scope & routing

- **FR-23 (MUST).** Worker-to-Worker communication uses internal org streams,
  not Slack. Slack is used only for communication involving Slack participants.
- **FR-24 (SHOULD).** A Worker uses a Slack DM to communicate with a human only
  when that human is a linked org member with known Slack details (depends on
  FR-21).
- **FR-25 (MUST).** Helix delivers inbound channel messages to all subscribed
  Workers (FR-18) and does not itself decide which Worker should respond.
  Disambiguation among multiple Workers is an org-graph concern (e.g. a routing
  Worker), not a Helix code responsibility.
- **FR-26 (SHOULD).** The setup UI presents the configuration appropriate to the
  selected ingress mode (OAuth "Add to Slack" for REST; app manifest + token
  entry for Socket Mode).

---

## 5. Non-Functional Requirements

- **NFR-1 (MUST).** A single implementation serves both ingress modes; the
  modes differ only at the ingress seam, not in message handling or outbound
  logic. We do not maintain two parallel Slack integrations.
- **NFR-2 (MUST).** The WebSocket ingress tolerates a multi-replica deployment
  without duplicate event processing (a single connection owner).
- **NFR-3 (MUST).** Slack credentials are stored as secrets and never exposed to
  Workers or non-admin users.
- **NFR-4 (SHOULD).** Inbound request authenticity is verified (signing secret
  for REST; Slack-managed authentication for Socket Mode).
- **NFR-5 (SHOULD).** The integration degrades gracefully on Slack API
  rate-limiting and transient connection loss (reconnect for WebSocket; retry
  semantics for REST), without losing the deployment's single-connection
  invariant.
- **NFR-6 (SHOULD).** Channel and persona configuration follow the existing
  helix-org philosophy: behaviour expressed as data/text (stream config, Role
  text) rather than new code paths per channel or Worker.

---

## 6. Out of Scope

Explicitly excluded from this body of work:

- **More than one workspace per org, or more than one Slack app per instance.**
  One global app; each org installs it into exactly one Slack workspace. Sharing
  a single workspace across multiple orgs, an org spanning multiple workspaces,
  or registering multiple apps, are excluded. (Per-org installs of the one app
  *are* in scope — see FR-4/FR-5.)
- **Real per-Worker Slack accounts** (SCIM provisioning, paid seats, Business+ /
  Enterprise Grid requirements). Workers are personas, not members.
- **Per-Worker OAuth or per-Worker tokens.** Workers share the one global bot.
- **Addressing a specific Worker via Slack `@mention`** — personas are not real
  Slack users and cannot be mentioned as members (see Open Questions).
- **Slack Marketplace distribution.**

---

## 7. Resolved Questions & Accepted Limitations

Resolved during review (2026-06-16):

- **OQ-1 — Addressing a specific Worker. (Accepted limitation.)** There is no
  native way to address one of several Workers in a channel — personas cannot be
  `@mentioned`. Helix does not solve this in code: inbound messages go to all
  subscribed Workers (FR-18, FR-25), and disambiguation is left to the org graph
  — e.g. an org-defined **routing Worker** that reads the channel and decides who
  responds. Accepted as a known limitation, not a blocker.
- **OQ-2 — Direct messages. Resolved.** Worker-to-Worker messaging uses internal
  org streams, never Slack. Slack DMs are used only for Worker↔human
  communication, and only when the human is a linked org member with known Slack
  details (FR-23, FR-24).
- **OQ-3 — Ingress mode selection. Resolved.** An explicit administrator toggle
  selects REST vs WebSocket; it is not inferred from credentials (FR-16).
- **OQ-4 — Human-linking trigger. Resolved.** Automatic email matching against
  the workspace member list, where possible (FR-21).
- **OQ-5 — Credential acquisition per mode. Resolved.** The setup experience may
  differ per mode; the frontend shows the configuration appropriate to the
  selected ingress mode (FR-26).

**Dependency / future work.** FR-21 (automatic human linking) requires a **Human
Worker email attribute**, which does not exist in the org graph today. Adding
that attribute is a prerequisite future task; until it lands, human linking
cannot be implemented.

---

## 8. Glossary

- **Persona** — a Worker's name + avatar applied to a message sent through the
  shared bot; not a real Slack account.
- **Global app** — the single, instance-wide, admin-configured Slack app
  (client id, secret, signing secret).
- **Installation** — an org's OAuth install of the global app into its own
  workspace, yielding that org's bot token + workspace/team id.
- **Channel-bound stream** — a helix-org Stream whose Transport targets one
  specific Slack channel.
- **Ingress mode** — how inbound Slack events reach Helix: REST (Events API) or
  WebSocket (Socket Mode).

---

## Part II — Implementation Plan

The Slack transport is a new `transport.Kind` plus one infrastructure package,
reusing the existing append→notify→dispatch fan-out and subscription model.

## 9. Architecture

### 9.1 One processing path, two ingress sources

Two ingress sources feed one shared path (**NFR-1 / FR-19**).

```text
        REST (Events API)            Socket Mode (WebSocket)
   POST /api/v1/slack/events      outbound WS, single owner
   verify signing-secret sig      Slack-managed auth
   answer url_verification         reconnect/backoff
            │                              │
            └──────────────┬───────────────┘
                           ▼
              Ingest.Receive(teamID, slackEvent)     ← the ONE path
                           │
       1. team_id → orgID            (per-org install lookup; FR-17)
       2. drop bot's own events      (self-echo guard; FR-20)
       3. channel → matching streams (org-scoped, KindSlack; FR-7/FR-18)
       4. build streaming.Message envelope
       5. Events.Append → Hub.Notify → Dispatcher.Dispatch   (existing fan-out)
                           │
                           ▼
          every subscribed Worker activates (existing dispatch; FR-18/FR-25)
```

Outbound is **ingress-agnostic** (FR-12): the dispatcher calls
`streaming.Outbound.Emit` for every appended Event on a `KindSlack` stream, as it
does for `webhook`/`email`.

```text
  Worker → publish MCP tool → Append → Dispatcher → Outbound.Emit (KindSlack)
                                                          │ chat.postMessage
                                                          ▼  username+icon (persona)
                                                       Slack channel
```

### 9.2 Three config layers → two storage homes

Three tenancy layers (§1) map onto config mechanisms that already exist. The
global app is admin-editable in the UI.

| Layer | What | Storage | Precedent | Reqs |
|---|---|---|---|---|
| **Global app** | client id/secret, signing secret, app token (`xapp-`), **ingress-mode toggle** | **`OAuthProvider` row** (`type="slack"`), edited in the admin panel | the existing admin **OAuth Providers** section | FR-1/2/3, FR-16, NFR-3 |
| **Per-org install** | bot token (`xoxb-`), team/workspace id | **configregistry** key `transport.slack`, `Secrets:["bot_token"]`, per-org row | `transport.github`, `transport.postmark` | FR-4/5/6, NFR-3 |
| **Channel binding** | one Slack channel id | per-**stream** `SlackConfig.Channel` (transport config JSON) | `EmailConfig.Alias`, `GitHubConfig.Repo` | FR-7 |

Global app = `OAuthProvider` row, edited in the admin **OAuth Providers** panel
(`frontend/src/components/dashboard/OAuthProvidersTable.tsx` →
`/api/v1/oauth/providers`, handlers in `api/pkg/server/oauth.go`). The
`types.OAuthProvider` model is instance-wide (no `OrganizationID`), admin-gated
(`user.Admin` on create/update/delete — FR-2), already carries
`ClientID`/`ClientSecret`/`Enabled`, and `type="slack"` is already defined.
`Enabled=false` (or no row) ⇒ subsystem inert (FR-3). Ingress mode is a stored
field, not inferred (FR-16).

Add three nullable fields to `OAuthProvider`:

```go
// api/pkg/types/oauth.go
SlackSigningSecret string `json:"slack_signing_secret" gorm:"type:text"` // REST authenticity (NFR-4)
SlackAppToken      string `json:"slack_app_token"      gorm:"type:text"` // xapp-… for Socket Mode
SlackIngressMode   string `json:"slack_ingress_mode"`                     // "rest" | "socket" | ""
```

- Redact the two secrets for non-admins (as `ClientSecret` already is) and
  encrypt at rest via `crypto.EncryptAES256GCM` + `getEncryptionKey` (as
  `ServiceConnection`/`GitProviderConnection` do). Retrofitting `ClientSecret`
  encryption is a follow-on.
- The New/Edit Provider dialog shows these inputs only when `type === "slack"`
  (FR-26).

Per-org `transport.slack` config (slack infra package, `postmark.Config` shape):

```go
type Config struct {
    BotToken string `json:"bot_token"`        // xoxb-…
    TeamID   string `json:"team_id"`          // T0123… — routing key (FR-17)
}
func (c Config) Validate() error { /* both non-empty */ }
```

The slack package reads the global app via an injected `GlobalApp(ctx) (App, error)`
port, backed by `store.ListOAuthProviders(type=slack, enabled=true)` at the
composition root.

### 9.3 Persona model (FR-4 / FR-11)

A Worker posts under its own name+avatar through the shared bot. The persona is a
lookup keyed on the Worker, modelled as an injected port (like
`gitHubTokenResolver`/`TokenResolver`), not a field on the shared
`streaming.Message`:

```go
type PersonaResolver func(ctx context.Context, orgID, workerID string) (Persona, error)
type Persona struct { Username, IconURL string }
```

`Outbound.Emit` reads `event.Source` + `event.OrganizationID`, calls the
resolver, and sets `chat.postMessage`'s `username` + `icon_url`. The default
returns `Username` from the Worker's identity (bare id today) and `IconURL == ""`
⇒ Slack falls back to the bot avatar. `IconURL` plumbing ships now; its source
(a Worker-avatar attribute) is deferred like FR-21's human-email attribute.

### 9.4 Inbound routing to a Worker (FR-25 / OQ-1) — pluggable

A bound channel may have several subscribed Workers (FR-9); personas cannot be
`@mentioned` (OQ-1). A `Router` port selects which subscriber(s) an inbound
message activates, behind one port:

```go
// Router narrows the subscriber set; it never invents a target outside it.
type Router interface {
    Route(ctx context.Context, in Inbound) ([]orgchart.WorkerID, error)
}

type Inbound struct {
    OrgID       string
    Stream      streaming.Stream
    Message     streaming.Message    // the new message
    Thread      []streaming.Message  // prior thread messages, oldest-first
    Subscribers []Candidate          // WorkerID + identity/role text
}
```

Implementations, selected **per org** (default broadcast):

- **`BroadcastRouter` (default).** Returns every subscriber (FR-25).
- **`FuzzyRouter` (this task).** Code-only. Scores each candidate by keyword
  overlap of message+thread text against the Worker's identity/role text; returns
  the best match, or all on a tie/low-confidence. No LLM, no network.
- **`LLMRouter` (future work).** Same port; an LLM picks from the thread + roster.

**Per-org selection.** A configregistry key `slack.router` (`TypeString`,
`Default: "broadcast"`) names the implementation (`"broadcast"` | `"fuzzy"`). The
ingest is built per-org (`New(orgID, …)`), reads `slack.router`, and resolves the
`Router` from a `map[string]Router` registry; unknown/unset ⇒ `BroadcastRouter`.

**Pipeline seam.** Add `Dispatcher.DispatchTo(ctx, event, targets []orgchart.WorkerID)`;
the existing `Dispatch` becomes `DispatchTo(ctx, e, allSubscribers)` (same
fan-out, outbound emit, self-source skip). Slack ingest resolves subscribers,
`targets := router.Route(...)`, then `dispatcher.DispatchTo(ctx, event, targets)`.
Only the Slack ingest consults the `Router`; other transports keep `Dispatch`.

### 9.5 Single connection owner for Socket Mode (NFR-2)

The Socket Mode runner gates its connection behind a **Postgres advisory lock**
(`pg_try_advisory_lock(<const key>)`):

- The replica that wins the lock opens the single WS connection and runs ingest.
- Losers poll (~10s) to take over on failover (NFR-5).
- Single-replica: always wins.

Encapsulated as a `singleOwner` collaborator. Postgres is already the store; no
new dependency.

## 10. Component inventory

New package `api/pkg/org/infrastructure/transports/slack/` — one job per file:

| File | Responsibility | Implements |
|---|---|---|
| `config.go` | per-org `Config{BotToken,TeamID}` + `Validate`; typed read from configregistry | — |
| `client.go` | thin wrapper over `slack-go/slack` (already a dep, v0.19.0): `PostMessage`, `OAuthV2`, `ConversationsJoin/Info`, `AuthTest` | — |
| `ingest.go` | **the one path**: `Receive(ctx, teamID, ev)` — team→org, self-echo drop, channel→streams, envelope, `router.Route`, `DispatchTo` | — |
| `router.go` | `Router` port + `BroadcastRouter` (default) + `FuzzyRouter` + `map[string]Router` registry, selected per-org by `slack.router` config — §9.4 | — |
| `globalapp.go` | `GlobalApp` port: read the admin-configured `OAuthProvider(type=slack)` (client id/secret, signing secret, app token, ingress mode) | — |
| `events_api.go` | REST source: `http.Handler`, HMAC signature verify, `url_verification` challenge → `ingest.Receive` | — |
| `socketmode.go` | WS source: `Run(ctx)` under `singleOwner`, reconnect/backoff → `ingest.Receive` | — |
| `singleowner.go` | `pg_try_advisory_lock` gate (NFR-2) | — |
| `outbound.go` | `Emit` → `chat.postMessage` w/ persona + `thread_ts` threading (FR-13) | `streaming.Outbound` |
| `provisioner.go` | `Install` = ensure bot is in channel (`conversations.join`); `Status` = membership check | `streaming.Inbound` |
| `oauth.go` | per-org "Add to Slack" start + callback: exchange code → persist `transport.slack` (FR-4, REST mode) | — |
| `persona.go` | `PersonaResolver` type + default | — |
| `credential_provider.go` | optional `mint_credential` "slack" provider (bot token to Worker shells) | `credential.Provider` |

Domain (one file, the established "new Kind = new file + one map entry" rule):

| File | Responsibility |
|---|---|
| `api/pkg/org/domain/transport/slack.go` | `const KindSlack`, `SlackConfig{Channel}`, `Validate`, `slack{}` strategy, `Transport.SlackConfig()` accessor; add `KindSlack` to `kindOrder` + `strategies` in `transport.go` |

Edited (global-app model, composition root, dispatch seam, admin UI):

| File | Edit |
|---|---|
| `api/pkg/types/oauth.go` | extend `OAuthProvider` with `SlackSigningSecret`, `SlackAppToken`, `SlackIngressMode` (nullable; §9.2) |
| `api/pkg/server/oauth.go` | redact + encrypt the two new secret fields on create/update/get (reuse `crypto.EncryptAES256GCM` + the existing `ClientSecret` redaction) |
| `frontend/.../OAuthProvidersTable.tsx` | show the three Slack fields when `type==="slack"` (mode-appropriate — FR-26); regenerate API client |
| `api/pkg/org/application/dispatch/dispatcher.go` | add `DispatchTo(ctx, event, targets)`; `Dispatch` delegates to it with all subscribers (§9.4) |
| `api/pkg/server/helix_org.go` | register `transport.slack` + `slack.router` specs; `RegisterOutbound(KindSlack,…)`; add to `inboundProvisioners`; build `ingest` with `GlobalApp` + `PersonaResolver` + per-org `Router` registry (default broadcast); start the ingress source the global-app row selects |
| `api/pkg/server/server.go` | mount REST `/slack/events` + `/slack/oauth/*` on the **insecure** router (auth is Slack-signature / encrypted-state, not the helix session layer — same as the github webhook); start the socket runner goroutine beside `streamCron.Start` |

## 11. TDD build sequence

Strict test-first, each phase green before the next. Fakes over mocks; gomock
only for the store where a suite already uses it (project convention). Domain
tests are pure table-driven (`transport_test.go` style); infra tests use the
real GORM test DB (`orggorm.GetOrgTestDB`) + a `recordingDispatcher` fake +
`wakebus` over in-memory NATS (`github_test.go` style); a fake Slack API
(`httptest.Server`) stands in for slack.com.

**Phase 1 — Domain: the Kind.** `slack_test.go` first: `Channel` required,
unknown-kind rejection, accessor round-trip, `KindSlack` in `KindValues()` order.
Then `slack.go` to green. *(FR-7)*

**Phase 2 — Ingest core.** `ingest_test.go` first, driving `Receive` with
synthetic events against a seeded DB:
- team id → correct org; unknown team id → dropped, no dispatch *(FR-17)*
- message in a bound channel → one Event appended, dispatched to **all** subscribed Workers *(FR-18)*
- two streams, two orgs, same channel name → strict org isolation, no leakage *(FR-5/FR-17)*
- event whose `bot_id`/`app_id` is our bot → dropped *(FR-20)*
- envelope mapping: `From`=slack user id, `Body`=text, `ThreadID`=`thread_ts`, `MessageID`=`ts`
Then `ingest.go`.

**Phase 3 — REST source.** `events_api_test.go` first: valid signature → 200 +
ingest called once; bad/stale signature → 401, ingest **not** called *(NFR-4)*;
`url_verification` → echoes `challenge`; payload `team_id` threaded to ingest.
Then `events_api.go` (uses `slack.SecretsVerifier`). *(FR-14)*

**Phase 4 — Outbound.** `outbound_test.go` first against fake Slack API:
`chat.postMessage` carries channel from `SlackConfig`, `username`+`icon_url` from
`PersonaResolver`, `thread_ts` from `Message.ThreadID` *(FR-10/11/13)*; missing
per-org token → typed error, no panic; emit identical with either ingress mode
configured *(FR-12)*. Then `outbound.go` + register in dispatcher.

**Phase 4b — Router port.** `router_test.go` first: `BroadcastRouter` returns all
subscribers (FR-25 baseline); `FuzzyRouter` picks the best-matching Worker by
message+thread overlap, returns all on a tie/low-confidence (never drops);
registry resolves `slack.router` value → impl, unknown/unset → broadcast. Then
`router.go` + `Dispatcher.DispatchTo` (with a dispatcher test that `DispatchTo`
restricts fan-out to the named targets and still emits outbound). Ingest test
extended: org with `slack.router=fuzzy` activates only the chosen Worker; an org
left at the default still broadcasts *(§9.4)*.

**Phase 5 — Socket Mode source + single owner.** `singleowner_test.go` first
(two contenders, one acquires); `socketmode_test.go` drives a fake socketmode
event through `Run` → ingest called once; reconnect path covered *(FR-15, NFR-2,
NFR-5)*. Then `socketmode.go` + `singleowner.go`.

**Phase 6 — Provisioner.** `provisioner_test.go` first: `Install` joins the
channel and reports `installed`; `Status` reflects membership / `unknown` on API
failure (degrade, don't error — matches the `Inbound` contract). Then
`provisioner.go`.

**Phase 7 — OAuth install.** `oauth_test.go` first: callback exchanges code
(fake Slack), persists `transport.slack` with bot token + team id for the org in
the encrypted `state`; bad state → rejected *(FR-4, US-1)*. Then `oauth.go`.

**Phase 8 — Wiring + e2e.** Extend `OAuthProvider` + admin handlers/UI; edit
composition root + `server.go`. Verify inert when no enabled `slack` provider
row exists *(FR-3)*. Browser/CLI e2e in the inner Helix: admin configures the
Slack app in the OAuth Providers panel, install an org, bind a channel-stream,
subscribe two Workers, post in Slack → both activate (broadcast default); a
Worker `publish` → appears in Slack under its persona.

## 12. Requirements traceability

| Req | Satisfied by |
|---|---|
| FR-1/2/3, FR-16, FR-26 | admin-editable `OAuthProvider(type=slack)` row; `Enabled`/`SlackIngressMode` fields (§9.2) |
| FR-4/5, US-1 | `oauth.go` per-org install → `transport.slack` row; team-id routing (§10) |
| FR-6 | `provisioner.Status` + per-org config presence on settings page |
| FR-7/8/9 | `KindSlack` + `SlackConfig.Channel`; existing subscription model unchanged |
| FR-10/11/13 | `outbound.go` + `PersonaResolver` + `thread_ts` |
| FR-12 | `Outbound` is ingress-agnostic by construction (§9.1) |
| FR-14, NFR-4 | `events_api.go` + `SecretsVerifier` |
| FR-15, NFR-2, NFR-5 | `socketmode.go` + `singleowner.go` advisory lock |
| FR-17/18/19, NFR-1 | `ingest.Receive` — the single shared path (§9.1) |
| FR-25 | default `BroadcastRouter` (delivers to all; Helix decides nothing); per-org `slack.router` opts into the `Router` seam for code/LLM routing (§9.4) |
| FR-20 | self-echo drop in `ingest` (bot_id/app_id guard) |
| FR-23 | unchanged: Worker↔Worker stays on internal streams; Slack only touches channels |
| NFR-3 | encrypted+redacted secret fields on the provider row; `Secrets:["bot_token"]` redaction in registry |
| NFR-6 | behaviour is stream config + Role text; no per-channel/per-Worker code |
| FR-21/22, FR-24 | **deferred** — depends on Worker-email attribute (§7 dependency); `IconURL` plumbing ships, source deferred |

## 13. Deferred / out of scope for v1

- **FR-21/22/24 human linking** — blocked on the Worker-email attribute (§7).
- **Worker avatar source** — `IconURL` carried but unpopulated until a
  Worker-avatar attribute exists; bot default avatar meanwhile.
- **`LLMRouter` (OQ-1)** — future work; the `Router` port + `BroadcastRouter` +
  `FuzzyRouter` ship now (§9.4).
- **`mint_credential` slack provider** — include only if Workers need to call
  Slack from their shell; the persona-post path does not require it.
