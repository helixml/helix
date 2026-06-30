# Requirements: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Background

A manager bot tried to create a new bot (`b-dewey`) and subscribe it to topics in one
step, and failed on two counts:

1. **`create_bot` conflates creation with subscription.** Its `topics` parameter looks
   like it subscribes the new bot, but `Bot.Topics` is a *no-op manifest* ("a typed
   manifest the Bot's prompt is expected to subscribe to", stored verbatim — it never
   creates an actual subscription). The manager set it expecting subscription and got
   nothing.
2. **Array arguments are unusable in practice.** `create_bot`/`update_bot` declare
   `tools` (and `topics`) as `[]string`, but the MCP client/LLM repeatedly sends a bare
   string (e.g. `"tools": "subscribe"`), which the server rejects with
   `cannot unmarshal string into Go value of type []string`. Because of this the manager
   could not grant `b-dewey` the `subscribe` (or `dm`) tool — it was stuck on the default
   read baseline. And `subscribe` only ever subscribes the *caller*, so the manager could
   not subscribe `b-dewey` on its behalf either.

The fix is to (a) make bot creation do one thing — create the bot — and push stream
subscription to a separate, explicit follow-up call, and (b) make array arguments accept
the shapes models actually produce.

Note: the "separate follow-up tool to subscribe a bot to streams" already exists —
`invite_bots` subscribes one or more *other* bots to a topic, and `subscribe` subscribes
the caller. No new tool is required; the blocker was purely the schema bug preventing the
manager from granting `subscribe`/`dm` and the misleading `topics`-at-create parameter.

## User Stories

### US-1: Bot creation does not pretend to subscribe
As a manager bot, I want `create_bot` to only create the bot (no `topics`/streams
parameter), so I am not misled into thinking creation subscribes the bot, and creation
stays simple and predictable.

**Acceptance Criteria**
- `create_bot` no longer accepts a `topics` parameter; its schema and description make no
  mention of subscribing to topics/streams at creation time.
- The bot is created successfully with `content`, optional `tools`, optional `parentId`.
- The `create_bot` description points the caller to the explicit follow-up:
  `invite_bots` (to subscribe the new bot) and/or `subscribe` (self).

### US-2: Grant tools to a bot without hitting the array bug
As a manager bot, I want to grant tools to a bot via `create_bot`/`update_bot` whether I
pass a single tool as a string or several as an array, so granting `subscribe`/`dm`
actually works.

**Acceptance Criteria**
- `"tools": "subscribe"` (bare string) and `"tools": ["subscribe","dm"]` (array) are both
  accepted and produce the same result; no `cannot unmarshal string into …[]string` error.
- `update_bot` preserves existing tools when `tools` is omitted, and clears them when
  `tools` is `[]` (unchanged nil-vs-empty semantics).
- The advertised input schema explicitly permits both shapes (string or array of strings),
  so strict-schema clients/models accept the call before it reaches the server.
- The granted tool set (after the base-read-tool union) is correct and order-stable.

### US-3: Subscribe a bot to streams as a separate step
As a manager bot, after creating a bot I want to subscribe it to the streams of interest
with a separate, explicit tool call, so subscription is decoupled from creation.

**Acceptance Criteria**
- `invite_bots` subscribes the named bot(s) to a topic and works with both a single
  `botIds` string and an array (same lenient handling as `tools`).
- Documentation/tool descriptions make the create → grant-tools → subscribe flow clear.

## Out of Scope
- Any new subscription mechanism (the `subscribe`/`invite_bots` tools already cover it).
- Changes to topic/stream creation (`create_topic`).
- Frontend changes (this is an MCP/REST + domain change).
</content>
</invoke>
