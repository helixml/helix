# Bidirectional Slack Topic delivery

Date: 2026-07-23
Status: implemented, awaiting review

## Summary

Slack Topics support inbound events and synchronous outbound basic text
delivery. Helix sends outbound Topic messages with the workspace credential it
already owns. Agents continue to mint scoped Slack credentials for rich Slack
operations such as reactions, uploads, edits, lookups, and multi-step
workflows.

`ask_human` remains a separate identity-based primitive and is available to
every Bot. It answers "contact this person through their preferred route";
publishing to a Slack Topic answers "send this event to this configured Slack
destination."

A successful Slack `publish` returns a delivery receipt from Slack. A failed
Slack attempt returns an explicit MCP or HTTP error. The internal audit event
is never presented as proof of external delivery.

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
`channel_id` is the one fixed outbound destination. It is optional so an
existing Slack Topic can remain inbound-only. A normal outbound publish to a
Slack Topic without `channel_id` returns an explicit configuration error.

The Topic detail UI exposes `Outbound Slack channel ID (optional)` and
preserves the rest of the Slack transport configuration when it is saved.

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

For those operations, the agent calls `mint_credential` for a scoped Slack
credential and uses the Slack API directly. Tool descriptions, Role
instructions, Bot briefings, and inbound Slack reply hints state this boundary.

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

1. Resolve the Topic and build the canonical event.
2. Append the event to the Topic.
3. Notify long-poll observers.
4. Dispatch the event to internal subscribers.
5. Attempt Slack delivery.
6. Return the Slack receipt or an explicit error.

This ordering preserves the internal audit event and subscriber activation
even when Slack rejects the post. Therefore an MCP or REST publish error can be
returned after the event already exists inside Helix. Callers must not blindly
retry an errored publish because the internal event and downstream activation
have already occurred.

Only `PublishInbound` suppresses outbound Slack delivery. Slack ingestion uses
`PublishInbound`, which prevents an inbound Slack message from being posted
back to Slack. All normal `Publish` and `PublishWithReceipt` callers attempt
outbound delivery when the Topic kind has a registered deliverer.

The Slack helper requires a non-empty destination and a non-empty message
timestamp in Slack's response. Slack API rejection, missing destination, and
missing timestamp are explicit errors.

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

For a Topic without external delivery, the preserved result explicitly says
external delivery is not confirmed and adds:

```json
{
  "delivery": {
    "status": "not_applicable",
    "provider": "helix"
  }
}
```

If Slack delivery fails, the MCP call returns an error. The event ID is not
returned as a success-shaped delivery result, although the audit event already
exists.

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

If Slack delivery fails, the REST endpoint returns an HTTP error after the
audit event has been appended.

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
- "React to the original message and upload the report" uses a minted Slack
  credential and the Slack API.

No generic `message.send` tool is needed. The two existing primitives already
cover event routing and person-oriented contact.

## Implementation

The implementation reuses the existing publishing service, transport registry,
workspace resolver, canonical message, Slack API client, and human delivery
paths.

1. `publishing.Publishing` now registers deliverers by transport kind and
   exposes `PublishWithReceipt` and `PublishInbound`.
2. The shared publish path appends, notifies, and dispatches before invoking a
   registered deliverer.
3. `slackTopicDeliverer` resolves the configured org-owned workspace and calls
   the shared Slack `DeliverText` helper.
4. `DeliverText` posts basic text with an optional thread timestamp and returns
   the actual channel and Slack message timestamp.
5. Slack ingress calls `PublishInbound`; no other boolean or caller convention
   suppresses egress.
6. The MCP `publish` response preserves its original fields and adds a delivery
   receipt.
7. The REST publish response adds the same receipt and accepts `threadId`.
8. `ask_human` is part of the universal Bot baseline and reuses `DeliverText`
   for its Slack route without entering the Topic publishing path.
9. The Topic detail UI configures the optional fixed outbound channel ID.
10. Generated OpenAPI and frontend API artifacts include the new request and
    response fields.

No new generic messaging service, provider workflow abstraction, queue, or
retry subsystem was added.

## Migration

1. Existing Slack Topics remain inbound-only while `channel_id` is empty.
2. An owner enters the fixed Slack channel ID in the Topic detail UI to enable
   outbound basic text.
3. Startup reconciliation adds `ask_human` to existing Bots. New Bots receive
   it through the same universal baseline.
4. Updated prompts direct basic Slack Topic text through `publish`,
   person-oriented contact through `ask_human`, and rich Slack behavior through
   a minted credential.
5. Existing inbound Slack ingestion and thread-follow routes are preserved.

No existing Topic event needs rewriting.

## Testing

### Automated verification

Run in
`/Users/psamuel/helix/helix-worktrees/bidirectional-slack-topic-delivery`:

- `go test ./api/pkg/org/application/publishing ./api/pkg/org/domain/transport ./api/pkg/org/infrastructure/transports/slack ./api/pkg/org/interfaces/mcptools ./api/pkg/org/interfaces/server/api ./api/pkg/server -count=1`: passed all six packages in 0.219 s, 0.376 s, 0.482 s, 0.425 s, 2.327 s, and 4.587 s.
- `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`: passed.
- `cd frontend && yarn build`: passed, 23,409 modules transformed in 22.21 s; command completed in 23.77 s.

The automated tests cover:

- Configured channel and thread values reaching Slack's HTTP API.
- A delivered receipt containing provider, destination, and message ID.
- Missing destinations and Slack API rejection returning explicit errors.
- Publish ordering and an audit event remaining after delivery failure.
- `PublishInbound` suppressing Slack egress while normal publishing delivers.
- MCP contract preservation with the added receipt.
- Cross-org and wrong-service-connection rejection.
- Universal `ask_human` inclusion for new and reconciled Bots.
- Existing human Slack delivery through the shared helper.

### NOT tested

- NOT tested: live Slack workspace delivery.
- NOT tested: Prime deployment and end-to-end merge request reviewer flow.
- NOT tested: browser configuration of the outbound Slack channel ID.
- NOT tested: live inbound Slack thread routing after an outbound Topic post.

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
