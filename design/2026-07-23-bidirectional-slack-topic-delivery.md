# Bidirectional Slack Topic delivery

Date: 2026-07-23
Status: implemented, awaiting review

## Summary

Slack Topics support inbound events and synchronous outbound basic text
delivery. Helix sends outbound Topic messages with the workspace credential it
already owns. Agents continue to call `mint_credential` to receive the
workspace's long-lived Slack bot token for rich Slack operations such as
reactions, uploads, edits, lookups, and multi-step workflows.

`ask_human` remains a separate identity-based primitive and is available to
every Bot. It answers "contact this person through their preferred route";
publishing to a Slack Topic answers "send this event to this configured Slack
destination."

A successful Slack `publish` returns a delivery receipt from Slack. A failed
Slack attempt after the event append returns the event ID, a failed receipt,
and an explicit `do not retry publish` status. The internal audit event is
never presented as proof of external delivery.

## Context

An earlier design position made Topics inbound-only and left outbound API calls
to agents:

https://docs.google.com/document/d/1FR4Q3BpRDEnEYpWn1PcRr5zpmeVnnjviqIAiOjAEqNo/edit?tab=t.0#heading=h.f8p2e3y9dof

That keeps pipes simple and avoids turning Helix transports into provider
workflow engines. In practice, agents repeatedly called `publish` for Slack
notifications, received an internal event ID, and claimed that Slack delivery
succeeded. Nothing reached Slack. Requiring each Bot to remember the separate
credential minting and Slack API sequence for a basic text post proved
unreliable.

The code already supported outbound Topic transports through
`streaming.Outbound` and the HTTP webhook emitter. Slack had inbound ingestion
and credential support but deliberately had no outbound emitter. The generic
`publish` MCP tool returned the appended event ID and Topic ID, so its result
did not distinguish an internal append from external delivery.

A later design position proposed bidirectional Slack Topics so agents and
automated producers could publish into Slack without activating an agent. The
implementation adopts that direction while preserving the original
minimal-pipe principle: the Slack pipe performs one narrow text-delivery
operation, not arbitrary Slack behavior.

The existing human delivery design is documented in
`design/2026-07-15-org-human-slack-delivery.md`. It gives `ask_human` a
different responsibility: resolve a person and use that person's configured
Helix or Slack contact route.

## Problem

There are three distinct intents:

1. Publish an event to a Topic.
2. Contact a known human through their preferred route.
3. Perform an arbitrary Slack action.

Before this implementation, the second and third intents could reach Slack but
the first could not. Agents inferred that a Slack Topic was an outbound
destination, called `publish`, and mistook persistence inside Helix for
external delivery. Prompting alone did not make this reliable.

Automated producers had the same gap. A processor or scheduled workflow should
be able to route a basic event to a Slack Topic without an agent being present
solely to call `chat.postMessage`.

## Decisions

### 1. Slack Topics are bidirectional

Publishing basic text to a Slack Topic invokes a Slack deliverer. The deliverer
uses the workspace and channel configured for the Topic.

Inbound Slack events continue to enter through the same Slack Topic transport.
Inbound and outbound messages remain canonical `streaming.Message` values.

### 2. Each Slack Topic has one outbound destination

`transport.SlackConfig` contains:

```json
{
  "service_connection_id": "conn_123",
  "channel_id": "C123"
}
```

`service_connection_id` selects the org-scoped Slack workspace connection.
`channel_id` controls both directions:

- Non-empty `channel_id` receives inbound messages from that exact Slack
  channel and is the one fixed outbound destination.
- Empty `channel_id` makes the Topic an inbound-only, workspace-wide fallback.
  It receives an inbound channel only when no exact channel Topic exists.

Inbound ownership is the tuple of `service_connection_id` and `channel_id`.
The service connection identifies the installed workspace and owns its
credential; the channel ID is interpreted only inside that workspace. When one
or more Topics match both values, every exact Topic receives the message and
workspace-wide fallback Topics do not. When no exact Topic exists, every
fallback with the same `service_connection_id` and an empty `channel_id`
receives it. A DM cannot enter a Topic bound to a different channel or
workspace.

Normal outbound `Publish` and `PublishWithReceipt` calls to a Slack Topic
without `channel_id` are rejected before event creation, notification, or
dispatch. MCP returns an error and REST returns HTTP 409 Conflict.
`PublishInbound` remains allowed so the empty-channel workspace fallback can
ingest unmatched Slack channels.

The Topic detail UI exposes `Slack channel ID (optional)`, explains both
directions, and preserves the rest of the Slack transport configuration when
it is saved.

The basic path does not accept arbitrary per-message channel selection. An
agent that needs to discover or select another channel mints a credential and
calls Slack directly.

### 3. Helix owns credentials for basic Topic delivery

The Slack deliverer resolves the exact service connection from the Topic,
checks that it belongs to the publishing org and is a Slack workspace
connection, decrypts its bot token, and posts with that token. The publishing
agent does not mint or receive the credential.

This is the same credential boundary as other Helix-owned transport delivery.
It does not grant the agent broader Slack access.

### 4. Agents own rich Slack operations

The Slack deliverer supports only basic text and an optional existing thread
timestamp. It does not model Slack reactions, file uploads, message edits,
lookups, interactive blocks, or multi-step workflows.

For those operations, the agent calls `mint_credential` and uses the Slack API
directly. For Slack, this does not mint a scoped or short-lived credential. It
returns the same long-lived workspace `xoxb-` bot token stored in the Slack
service connection. The `resource` team ID selects a workspace when the org has
more than one; it does not narrow the token's OAuth scopes.

`publish` and `ask_human` use that workspace token entirely server-side and do
not expose it to the agent. Tool descriptions, Role instructions, Bot
briefings, and inbound Slack reply hints state this boundary.

### 5. `ask_human` is available to every Bot

Every Bot receives `ask_human` through the universal baseline tool set.
Startup reconciliation backfills it onto existing Bots, and Bot creation paths
include it for new Bots.

This remains necessary after Slack Topics become bidirectional because the two
tools select destinations differently:

| Intent | Tool | Destination selection |
|---|---|---|
| Send an event to a configured Slack route | `publish` | Slack Topic configuration |
| Contact a known person | `ask_human` | Human ID and preferred contact route |
| Perform a rich Slack action | `mint_credential` and Slack API | Agent-selected Slack resource |

`ask_human` is not an owner-only org mutation. Its implementation scopes the
lookup to the caller's org, requires a human node, and delivers through the
human's configured route. Any Bot may legitimately need a decision, approval,
or notification from a person. Restricting the tool to the Chief of Staff
would force ordinary Bots to delegate basic human contact or rediscover Slack
credentials.

Slack delivery for `ask_human` and Slack Topic publishing shares the small
`DeliverText` helper. `ask_human` does not call `publish`, does not require a
Slack Topic, and does not append a Topic audit event. Its existing
person-oriented route selection and reply-participant recording remain intact.

### 6. Delivery is synchronous and explicit

Slack outbound delivery makes one synchronous `chat.postMessage` call. There
is no delivery queue and no automatic retry.

The operation order is:

1. Resolve the Topic and validate outbound publishability.
2. Build the canonical event.
3. Append the event to the Topic.
4. Notify long-poll observers.
5. Dispatch the event to internal subscribers.
6. Attempt Slack delivery.
7. Return the Slack receipt or an explicit error.

For normal outbound Slack publishing, validation rejects an empty
`channel_id` before step 2. `PublishInbound` bypasses this outbound-only check.

This ordering preserves the internal audit event and subscriber activation
even when Slack rejects the post. When delivery fails after append, the
publishing service returns both its error and a partial result containing the
event and failed receipt. MCP and REST convert that partial result into a
response with the event ID, `delivery.status=failed`, the provider error, and
an explicit `do not retry publish` status. Callers must not retry because the
internal event and downstream activation have already occurred. Errors before
append still return as ordinary MCP or HTTP errors without an event ID.

Only `PublishInbound` marks a publish as externally inbound. It stores that
provenance on the publish context. Slack ingestion uses `PublishInbound`, and
the dispatcher and processor runner carry the same context through nested
publishes. Every Topic event produced by that inbound processor chain
therefore remains internal and cannot call an external deliverer. A Bot or
automated producer using normal `Publish` or `PublishWithReceipt` has no
inbound provenance and attempts outbound delivery for every configured Slack
Topic reached through its processor chain.

The Slack helper requires a non-empty destination and a non-empty message
timestamp in Slack's response. Slack API rejection, missing destination, and
missing timestamp are explicit errors.

### 7. Inbound reply guidance names the usable route

Each inbound Slack message carries a reply hint built for the Topic that
received it:

- An exact channel Topic names that Topic ID and tells the Bot to call
  `publish` with the correct thread ID for a basic reply when the Bot has the
  `publish` capability. Otherwise it directs the Bot to `mint_credential` and
  `chat.postMessage`.
- A workspace-wide fallback names itself as inbound-only, tells the Bot to use
  `list_topics` to find a Slack Topic whose `service_connection_id` matches the
  fallback Topic and whose `channel_id` matches the incoming channel. It uses
  that Topic only when `publish` is available; otherwise, or when no matching
  Topic exists, it directs the Bot to `mint_credential` and
  `chat.postMessage`.

The hint continues to direct reactions, uploads, edits, lookups, and other rich
operations to `mint_credential` and the Slack API.

### 8. MCP Topic reads expose only typed Slack configuration

`list_topics` and `get_topic` no longer expose arbitrary raw transport
configuration. Their MCP view includes `transportConfig` only for Slack, parsed
as the typed `SlackConfig` containing `service_connection_id` and `channel_id`.
Other transport configurations are omitted.

## Contracts

### Basic Slack Topic publish

The MCP request continues to use the canonical message envelope:

```json
{
  "topicId": "s-slack-workspace",
  "body": "Merge request review complete.",
  "threadId": "optional Slack root message timestamp",
  "subject": "optional internal subject"
}
```

The Topic selects the fixed workspace and channel. `body` is the Slack message
text. `threadId`, when present, posts into an existing Slack thread. `subject`
remains message metadata and is not mapped to Slack UI.

The REST `POST /topics/{id}/publish` request also accepts `threadId`.

### MCP delivered result

The MCP response preserves the existing `id`, `topicId`, `scope`, and `status`
fields and adds `delivery`:

```json
{
  "id": "evt_123",
  "topicId": "s-slack-workspace",
  "scope": "helix",
  "status": "appended inside Helix and delivered to slack",
  "delivery": {
    "status": "delivered",
    "provider": "slack",
    "destination": "C123",
    "messageId": "1753286400.123456"
  }
}
```

For a non-Slack Topic without external delivery, the preserved result
explicitly says external delivery is not confirmed and adds:

```json
{
  "delivery": {
    "status": "not_applicable",
    "provider": "helix"
  }
}
```

An outbound publish to a Slack Topic without `channel_id` never returns this
result. It fails before event creation with an MCP error. Inbound ingestion of
that Topic remains valid through `PublishInbound`.

### MCP failed delivery result

When Slack rejects the post after the audit append, MCP returns the partial
result instead of discarding the event identity:

```json
{
  "id": "evt_123",
  "topicId": "s-slack-workspace",
  "scope": "helix",
  "status": "appended inside Helix; external delivery failed; do not retry publish",
  "delivery": {
    "status": "failed",
    "provider": "slack",
    "destination": "C123",
    "error": "do not retry publish: post Slack message: not_in_channel"
  }
}
```

### REST delivered result

The REST success response preserves `event_id` and adds the delivery receipt:

```json
{
  "event_id": "evt_123",
  "delivery": {
    "status": "delivered",
    "provider": "slack",
    "destination": "C123",
    "messageId": "1753286400.123456"
  }
}
```

For a post-append Slack failure, REST returns HTTP 201 with `event_id` and the
same failed delivery receipt. This is partial success: Helix appended and
dispatched the event, while Slack delivery failed. Pre-append failures remain
HTTP errors. In particular, an outbound Slack Topic without `channel_id`
returns HTTP 409 Conflict and creates no event.

### `ask_human`

```json
{
  "personId": "h-owner",
  "message": "The merge request review is complete.",
  "expectsReply": false
}
```

`ask_human` keeps its existing person-oriented result, including the route
actually used. It does not accept raw Slack channel IDs and does not expose
Slack credentials.

## Why both `publish` and `ask_human`

A Slack Topic is an integration route. A human node is an identity.

Conflating them would require every Bot to know the person's current workspace,
channel, user ID, and contact preference. That is the failure mode
`ask_human` removes. Conversely, routing every automated Slack event through a
human identity would prevent channel-oriented automation and require an agent
or person to be in the loop.

The intent boundary is:

- "Post the review result to the engineering Slack Topic" uses `publish`.
- "Ask the org owner whether this should merge" uses `ask_human`.
- "React to the original message and upload the report" uses the workspace
  token returned by `mint_credential` and the Slack API.

No generic `message.send` tool is needed. The two existing primitives already
cover event routing and person-oriented contact.

## Implementation

The implementation reuses the existing publishing service, transport registry,
workspace resolver, canonical message, Slack API client, and human delivery
paths.

1. `publishing.Publishing` now registers deliverers by transport kind and
   exposes `PublishWithReceipt` and `PublishInbound`.
2. The shared publish path rejects outbound Slack Topics without `channel_id`
   before event creation. `PublishInbound` bypasses that check.
3. For accepted publishes, the shared path appends, notifies, and dispatches
   before invoking a registered deliverer.
4. `slackTopicDeliverer` resolves the configured org-owned workspace and calls
   the shared Slack `DeliverText` helper.
5. `DeliverText` posts basic text with an optional thread timestamp and returns
   the actual channel and Slack message timestamp.
6. Slack ingress matches the workspace service connection and channel
   together, routes exact channel Topics ahead of same-workspace fallback
   Topics, and creates a route-specific reply hint for each event.
7. Slack ingress calls `PublishInbound`. Its context provenance survives
   processor chains and is the only mechanism that suppresses egress.
8. The MCP `publish` response preserves its original fields and adds a
   delivered or failed receipt.
9. The REST publish response adds the same receipt, returns post-append
   failures as HTTP 201 partial success, and accepts `threadId`.
10. MCP Topic reads expose only parsed `SlackConfig`, never arbitrary raw
   transport configuration.
11. `ask_human` is part of the universal Bot baseline and reuses `DeliverText`
   for its Slack route without entering the Topic publishing path.
12. The Topic detail UI configures the optional channel ID for inbound
    filtering and outbound delivery.
13. Generated OpenAPI and frontend API artifacts include the new request and
    response fields.

No new generic messaging service, provider workflow abstraction, queue, or
retry subsystem was added.

## Migration

1. Existing Slack Topics with empty `channel_id` remain inbound-only
   workspace-wide fallbacks.
2. An owner enters a fixed Slack channel ID in the Topic detail UI to make the
   Topic exact-channel inbound and enable outbound basic text.
3. Startup reconciliation adds `ask_human` to existing Bots. New Bots receive
   it through the same universal baseline.
4. Updated prompts direct basic Slack Topic text through `publish`,
   person-oriented contact through `ask_human`, and rich Slack behavior through
   the workspace token returned by `mint_credential`.
5. Existing inbound Slack ingestion and thread-follow routes are preserved.

No existing Topic event needs rewriting.

## Testing

### Automated verification

Run in
`/Users/psamuel/helix/helix-worktrees/bidirectional-slack-topic-delivery`:

- `go test ./api/pkg/org/... -count=1`: passed.
- `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`: passed.
- `cd frontend && yarn build`: passed, 23,410 modules transformed; only the existing chunk-size warning was reported.
- `git diff --check`: passed.

The automated tests cover:

- Configured channel and thread values reaching Slack's HTTP API.
- Exact-channel inbound routing overriding workspace-wide fallback routing.
- DMs not entering Topics bound to another channel.
- Exact and fallback reply hints naming the correct Topic behavior.
- A delivered receipt containing provider, destination, and message ID.
- Missing destinations and Slack API rejection returning failed receipts.
- Outbound empty-channel Slack Topics returning MCP errors and REST HTTP 409
  before any event append, while `PublishInbound` remains accepted.
- Publish ordering and an audit event remaining after delivery failure.
- Post-append MCP and REST failure responses preserving the event ID and
  warning callers not to retry.
- Inbound provenance suppressing Slack egress through nested processor
  publishes while normal automated publishing delivers.
- MCP contract preservation with the added receipt.
- MCP Topic reads exposing only typed Slack transport configuration.
- Cross-org and wrong-service-connection rejection.
- Universal `ask_human` inclusion for new and reconciled Bots.
- Existing human Slack delivery through the shared helper.

### Live Prime findings

Prime at commit `45d98c1e` successfully delivered a basic Slack Topic publish
and a threaded reply. Both returned Slack delivery receipts.

A later threaded human reply was ingested but did not activate the Bot because
the Bot creator had omitted the custom-Topic subscription. After the
subscriptions were added manually, a Slack DM was incorrectly fanned into
channel and workspace Topics. That produced duplicate Bot activations and an
unintended reply in the configured channel. These findings drove the final
exact-channel precedence, channel filtering, route-specific reply hints, and
inbound-provenance fixes.

### NOT tested

- NOT tested: the final exact-channel, DM, and threaded-reply flow on Prime
  after the routing and provenance fixes.
- NOT tested: browser configuration of the revised bidirectional Slack channel
  ID field after the final UI wording change.

## Non-goals

- Wrapping the full Slack API in Helix MCP tools.
- Supporting reactions, uploads, edits, lookups, blocks, or multi-step Slack
  workflows through `publish`.
- Replacing `ask_human` with Slack Topic publishing.
- Routing arbitrary provider actions through Topics.
- Making every transport bidirectional.
- Adding a generic `message.send` abstraction.
- Adding an outbound queue or automatic retry.
- Treating an internal Topic event as proof of external delivery.
