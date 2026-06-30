# Requirements: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Background

A manager Bot, driving the org-graph MCP surface, tried to create a Bot
(`b-dewey`) and subscribe it to some streams. Two things blocked it:

1. **Array arguments are unusable in practice.** `create_bot` / `update_bot`
   take `tools` (and `create_bot` also takes `topics`) as string arrays, and
   `invite_bots` takes `botIds` as a string array. The server unmarshals into
   `[]string`, but the MCP client/LLM repeatedly sends a *bare string*
   (e.g. `"tools": "subscribe"`), which the server rejects with
   `cannot unmarshal string into …[]string`. Because of this the manager could
   not grant `b-dewey` the `subscribe` (or `dm`) tool — it was stuck on the
   default read baseline.

   Root cause (confirmed empirically): `jsonschema.For` emits a *union* type
   for the slice fields — `"type": ["null","array"]` (a nil Go slice marshals
   to `null`) — whereas scalar fields get a clean `"type":"string"`. Small
   chat models mishandle the union and collapse a one-element list to a bare
   string. This is the **same** failure already solved for the `create_topic`
   tool's `transport` field (whose auto-derived schema also arrived as a
   `["object","null"]` union).

2. **`create_bot`'s `topics` argument is a misleading no-op.** It sets an
   informational manifest field (`Bot.Topics` — "a typed manifest the Bot's
   prompt is expected to subscribe to", stored verbatim) that **never creates
   a subscription**. The manager set it expecting a subscription and got
   nothing. Worse, `subscribe` only ever subscribes the *caller*, so the
   manager could not subscribe `b-dewey` on its behalf that way either.

The fix: make the array arguments accept the shapes models actually produce,
and remove stream/topic handling from bot creation so subscription is an
explicit, separate follow-up call.

Note: the follow-up tool already exists. `invite_bots` subscribes one or more
*other* Bots to a Topic (it validates only that the Topic and Bots exist in
the org — no ownership constraint). No new tool is required.

## User Stories

### US-1: Pass tool/bot lists without the array bug
As a manager Bot, I want `create_bot` / `update_bot` `tools` and `invite_bots`
`botIds` to accept either a single value or a list, so I can grant a Bot the
tools it needs (e.g. `subscribe`, `dm`) and subscribe Bots without the call
being rejected.

**Acceptance criteria**
- `"tools": "subscribe"` (bare string) and `"tools": ["subscribe","dm"]`
  (array) are both accepted and produce the same result — no
  `cannot unmarshal string into …[]string` error.
- The same applies to `invite_bots`'s `botIds`.
- The advertised input schema explicitly permits both shapes (a
  `oneOf: [string, array<string>]`), so strict-schema clients/models accept
  the call before it reaches the server.
- `update_bot` keeps its nil-vs-empty semantics: omitted `tools` preserves the
  existing tools; `[]` clears to the baseline.
- Resulting tool sets (after the base-read-tool union) are correct and
  order-stable.

### US-2: Bot creation does not take streams
As a manager Bot, I want `create_bot` to be about the Bot itself (its content
and tools) and **not** accept any streams/topics, so creation never misleads
me into thinking it subscribed the Bot.

**Acceptance criteria**
- `create_bot` no longer accepts a `topics` argument; its schema and
  description no longer mention topics/streams.
- `create_bot` still accepts `id` (optional), `content` (required), `tools`
  (optional), and `parentId` (optional). A new Bot still receives the
  universal read baseline unioned with any `tools` supplied.
- No vestigial write-nowhere topics/manifest field remains anywhere on the
  create path (MCP, REST, and domain consistent).

### US-3: Subscribe a Bot to streams as a separate step
As a manager Bot, after creating a Bot I want a clear, separate tool call to
subscribe it to the streams of interest.

**Acceptance criteria**
- After `create_bot`, the manager subscribes the new Bot to an existing Topic
  via `invite_bots` (and `create_topic` first if the Topic doesn't exist).
- The `create_bot` description points the model at the explicit follow-up
  (`create_topic` if needed, then `invite_bots`; or the Bot self-subscribes
  via `subscribe` once it holds that tool).
- No new MCP tool is introduced.

## Out of Scope
- Removing `tools` from `create_bot` (tools are part of a Bot's definition;
  US-1 makes setting them at creation work).
- Any new subscription/tool-granting mechanism.
- Changes to subscription authorization rules or topic/stream creation.
- Frontend changes (the chart UI already creates Bots with content/parent
  only and never sent tools/topics).
</content>
