# Requirements: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## Background

A manager Bot could not grant tools to a new Bot (`b-dewey`) or subscribe it to
streams. Two problems:

1. **Array-valued MCP args were unusable.** `create_bot.tools`/`.topics`,
   `update_bot.tools`, and `invite_bots.botIds` are advertised with a nullable
   *union* schema (`"type":["null","array"]`) that small models mishandle —
   they send a bare string, which the server rejects (`cannot unmarshal string
   into …[]string`).
2. **`create_bot`'s `topics` silently subscribed nothing.** It wrote a
   `Bot.Topics` "manifest" field that was stored but never created a
   subscription. It *looked* like it subscribed the Bot; it didn't. This is the
   trap that blocked the manager.

Design changes:

- **Tool-granting and subscription become discrete, scalar operations** —
  eliminating the array-schema interop bug and making valid tool values
  discoverable via an `enum`.
- **`create_bot` subscribes immediately.** We are amending the org-package
  principle that subscription must be a separate, prompt-driven step. New
  guiding rule: **complete a user action in as few steps as possible.** Since a
  manager creating a Bot almost always wants it listening right away,
  `create_bot(topics)` now creates real subscriptions at creation. `subscribe`
  remains for subscribing later.
- **`Bot.Topics` (the no-op manifest field) is removed.** Subscriptions are
  their own `(bot, topic)` rows — the single source of truth — so there is no
  denormalized field to drift.

`update_bot` (broken bulk content+tools edit) is removed; the caller-only
`subscribe`/`unsubscribe` and the bulk `invite_bots` are replaced by
Bot-targeted discrete equivalents.

## User Stories

### US-1: Grant or revoke tools (one or many)
As a manager Bot, I want `attach_tool(botId, tools)` and
`detach_tool(botId, tools)`, where `tools` is an array of names chosen from an
enum of valid tool names, so I can give or remove several tools in one call and
see the valid values.

**Acceptance criteria**
- `attach_tool` unions the given `tools` into the Bot's tool set; idempotent per
  name. Takes effect on the Bot's next MCP request.
- `detach_tool` removes the given `tools`; idempotent per name. It refuses if
  any name is a universal read-baseline tool (mandatory; the reconciler would
  re-add it), failing the call before any write.
- `tools` is advertised as a required, non-nullable array whose items are the
  registered-tool-name `enum`; new tools appear automatically. Unknown names
  are rejected.

### US-2: Subscribe or unsubscribe a Bot to/from topics (one or many) later
As a manager Bot, I want `subscribe(botId, topicIds)` and
`unsubscribe(botId, topicIds)`, where `topicIds` is an array, so I can change a
Bot's subscriptions — several at once — after creation (or subscribe myself by
passing my own id).

**Acceptance criteria**
- `subscribe` links the named Bot to every listed Topic (validates the Bot and
  all Topics exist up front); idempotent per topic. `unsubscribe` removes those
  links.
- `topicIds` is advertised as a required, non-nullable array of topic-id strings
  (no enum — topic ids are dynamic). An unknown topic fails the whole call
  before any write.
- The old caller-only `subscribe`/`unsubscribe` and the bulk `invite_bots` are
  removed (superseded).

### US-3: Creating a Bot sets its tools and subscribes it — in one call
As a manager Bot, I want `create_bot` to grant the new Bot its initial tools and
subscribe it to the topics I name, in one call, so it's ready without follow-up
steps.

**Acceptance criteria**
- `create_bot` accepts `id?`, `content`, `tools` (required, non-nullable array;
  each item from the registered-tool-name enum — pass `[]` for baseline only),
  `topics` (required, non-nullable array of existing topic ids — pass `[]` for
  none), `parentId?`.
- `tools` items are advertised as the registered-tool-name `enum` (same source
  as `attach_tool`), so the model sees valid values; the supplied tools are
  unioned with the universal read baseline. `attach_tool`/`detach_tool` change
  a Bot's tools later.
- `topics` is advertised as `{"type":"array","items":{"type":"string"}}`. Both
  arrays are `required` and non-nullable — no `["null","array"]` union.
- On success, the Bot has (supplied tools ∪ baseline) and a real subscription
  `(bot, topic)` row for every listed topic; it receives events immediately.
- Every listed topic must already exist; unknown topics (and unknown tool
  names) fail the call. Topics are validated **before** the Bot row is written,
  so a failed call leaves no partially-created Bot.
- No `Bot.Topics` manifest field remains; subscriptions are the source of truth.

### US-4: Edit a Bot's content after creation
As a manager Bot, I want `set_bot_content(botId, content)` to revise a Bot's
markdown prompt after it exists (kept — confirmed).

**Acceptance criteria**
- `set_bot_content` replaces the Bot's `content`; other fields untouched.

## Out of Scope
- All bot-mutation MCP args are fixed non-nullable arrays:
  `attach_tool`/`detach_tool` `tools` (enum items), `subscribe`/`unsubscribe`
  `topicIds` (strings), and `create_bot`'s `tools` + `topics`. No scalar or
  nullable/union array args remain (that union was the original bug). `botId`
  stays a scalar identifier.
- Auto-*creating* topics that don't exist (the manager calls `create_topic`
  first). Possible future enhancement, not this task.
- Frontend changes (the chart UI already creates Bots with content/parent only).
</content>
