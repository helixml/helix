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

### US-1: Grant or revoke one tool at a time
As a manager Bot, I want `attach_tool(botId, tool)` and
`detach_tool(botId, tool)`, where `tool` is chosen from an enum of valid tool
names, so I can give a Bot exactly the tools it needs without an array bug and
without guessing names.

**Acceptance criteria**
- `attach_tool` adds `tool` to the Bot's tool set; idempotent. Takes effect on
  the Bot's next MCP request.
- `detach_tool` removes `tool`; idempotent. It refuses to remove a universal
  read-baseline tool (mandatory; the reconciler would re-add it) with a clear
  error.
- `tool` is advertised as a required, non-nullable string `enum` of the
  registered tool names; new tools appear automatically. Unknown values are
  rejected.

### US-2: Subscribe or unsubscribe a specific Bot later
As a manager Bot, I want `subscribe(botId, topicId)` and
`unsubscribe(botId, topicId)`, so I can change a Bot's subscriptions after
creation (or subscribe myself by passing my own id).

**Acceptance criteria**
- `subscribe` links the named Bot to the Topic (validates both exist);
  idempotent. `unsubscribe` removes that link.
- The old caller-only `subscribe`/`unsubscribe` and the bulk `invite_bots` are
  removed (superseded).

### US-3: Creating a Bot subscribes it immediately
As a manager Bot, I want `create_bot` to subscribe the new Bot to the topics I
name, in one call, so it starts listening without a follow-up step.

**Acceptance criteria**
- `create_bot` accepts `id?`, `content`, `topics` (required, non-nullable array
  of existing topic ids — pass `[]` for none), `parentId?`. It does **not**
  accept `tools` (use `attach_tool`).
- `topics` is advertised as `{"type":"array","items":{"type":"string"}}` and is
  `required` — no `["null","array"]` union.
- On success, a real subscription `(bot, topic)` row exists for every listed
  topic; the Bot receives events from them immediately.
- Every listed topic must already exist; unknown topics fail the call. Topics
  are validated **before** the Bot row is written, so a failed call leaves no
  partially-created Bot.
- A new Bot still receives the universal read baseline; more tools via
  `attach_tool`.
- No `Bot.Topics` manifest field remains; subscriptions are the source of truth.

### US-4: Edit a Bot's content after creation
As a manager Bot, I want `set_bot_content(botId, content)` to revise a Bot's
markdown prompt after it exists (kept — confirmed).

**Acceptance criteria**
- `set_bot_content` replaces the Bot's `content`; other fields untouched.

## Out of Scope
- Array-valued MCP arguments on the tool-grant/subscription tools (removed by
  design). `create_bot.topics` is the one retained array, with a fixed
  non-nullable schema.
- Auto-*creating* topics that don't exist (the manager calls `create_topic`
  first). Possible future enhancement, not this task.
- Frontend changes (the chart UI already creates Bots with content/parent only).
</content>
