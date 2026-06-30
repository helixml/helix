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

The fix is to make bot creation do exactly one thing — create the bot from its markdown —
and push **both** tool assignment and stream subscription to separate, explicit follow-up
calls, then make those array arguments accept the shapes models actually produce.

Two things to note:
- The "separate follow-up tools" already exist. `update_bot` grants/edits tools;
  `invite_bots` subscribes one or more *other* bots to a topic; `subscribe` subscribes the
  caller. No new tool is required.
- The chart UI already follows this pattern — `NewBotDialog` sends only `id`, `content`,
  and `parent_id`; it has no tools or topics picker. The create path merely carries unused
  `tools`/`topics` fields today.

## User Stories

### US-1: Bot creation does exactly one thing
As a manager bot, I want `create_bot` to only create the bot from its markdown (with an
optional manager), with no `tools` and no `topics`/streams parameters, so creation is
simple and never misleads me into thinking it grants tools or subscriptions.

**Acceptance Criteria**
- `create_bot` accepts only `id` (optional), `content` (required), and `parentId`
  (optional). It no longer accepts `tools` or `topics`.
- A newly created bot automatically receives the universal read baseline
  (`BaseReadTools`), so it always has a usable MCP surface.
- The `create_bot` description points the caller to the explicit follow-ups: `update_bot`
  (to grant tools) and `invite_bots`/`subscribe` (to subscribe to streams).

### US-2: Grant tools after creation without hitting the array bug
As a manager bot, I want to grant tools to an existing bot via `update_bot` whether I pass
a single tool as a string or several as an array, so granting `subscribe`/`dm` actually
works.

**Acceptance Criteria**
- In `update_bot`, `"tools": "subscribe"` (bare string) and `"tools": ["subscribe","dm"]`
  (array) are both accepted and produce the same result; no
  `cannot unmarshal string into …[]string` error.
- `update_bot` preserves existing tools when `tools` is omitted, and clears them when
  `tools` is `[]` (unchanged nil-vs-empty semantics).
- The advertised input schema explicitly permits both shapes (string or array of strings),
  so strict-schema clients/models accept the call before it reaches the server.
- The resulting tool set (after the base-read-tool union) is correct and order-stable.

### US-3: Subscribe a bot to streams as a separate step
As a manager bot, after creating a bot I want to subscribe it to the streams of interest
with a separate, explicit tool call, so subscription is decoupled from creation.

**Acceptance Criteria**
- `invite_bots` subscribes the named bot(s) to a topic and accepts both a single `botIds`
  string and an array (same lenient handling as `update_bot`'s `tools`).
- Documentation/tool descriptions make the create → grant-tools → subscribe flow clear.

## Out of Scope
- Any new subscription/tool-granting mechanism (`update_bot`, `subscribe`, `invite_bots`
  already cover the follow-ups).
- Changes to topic/stream creation (`create_topic`).
- Frontend changes (the UI already creates bots with content/parent only).
</content>
