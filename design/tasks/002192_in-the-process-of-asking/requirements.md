# Requirements: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## Background

A manager Bot could not grant tools to a new Bot (`b-dewey`) or subscribe it
to streams. The root cause was that the bulk, array-valued MCP arguments
(`create_bot.tools`/`.topics`, `update_bot.tools`, `invite_bots.botIds`) are
advertised with a nullable *union* schema (`"type":["null","array"]`) that
small models mishandle — they send a bare string, which the server rejects
(`cannot unmarshal string into …[]string`).

Rather than patch the array schema, we **remove the arrays entirely** and make
each mutation a discrete, scalar operation. This both fixes the interop failure
(no array params left to misrepresent) and makes valid values discoverable: the
tool argument on `attach_tool`/`detach_tool` is a JSON-Schema `enum` of the
registered tool names, so the model sees exactly what it may pass.

`update_bot` (bulk content+tools edit) does not work and is removed. The
caller-only `subscribe`/`unsubscribe` and the bulk `invite_bots` are replaced by
Bot-targeted discrete equivalents.

## User Stories

### US-1: Grant or revoke one tool at a time
As a manager Bot, I want `attach_tool(botId, tool)` and
`detach_tool(botId, tool)`, where `tool` is chosen from an enum of valid tool
names, so I can give a Bot exactly the tools it needs without hitting an array
bug and without guessing tool names.

**Acceptance criteria**
- `attach_tool` adds `tool` to the target Bot's tool set; idempotent (already
  present → no-op). Takes effect on the Bot's next MCP request.
- `detach_tool` removes `tool` from the Bot's tool set; idempotent (absent →
  no-op). It refuses to remove a universal read-baseline tool (those are
  mandatory and the reconciler would re-add them anyway) and returns a clear
  error.
- The `tool` argument is advertised as a required, non-nullable string `enum`
  of the registered tool names; new tools appear in the enum automatically
  without editing these tools.
- An unknown `tool` value is rejected with a clear error.

### US-2: Subscribe or unsubscribe a specific Bot
As a manager Bot, I want `subscribe(botId, topicId)` and
`unsubscribe(botId, topicId)`, so I can subscribe the Bot I just created (not
only myself) to the streams of interest as a separate step.

**Acceptance criteria**
- `subscribe` links the named Bot to the Topic (validates both exist);
  idempotent.
- `unsubscribe` removes that link; a missing link is a clear error/no-op.
- A Bot may subscribe itself by passing its own id.
- The old caller-only `subscribe`/`unsubscribe` and the bulk `invite_bots` are
  removed (superseded by these).

### US-3: Bot creation is bare
As a manager Bot, I want `create_bot` to take only `id` (optional), `content`
(required), and `parentId` (optional), so creation does one thing and there are
no array/stream arguments to misuse.

**Acceptance criteria**
- `create_bot` no longer accepts `tools` or `topics`.
- A new Bot automatically receives the universal read baseline, so it has a
  usable MCP surface immediately; further tools are added via `attach_tool`.
- No vestigial `topics`/manifest field remains anywhere on the bot
  (`Bot.Topics` removed end-to-end — it was a stored no-op that never
  subscribed anything).

### US-4: Edit a Bot's content after creation *(assumption — confirm)*
As a manager Bot, I want `set_bot_content(botId, content)` so I can revise a
Bot's markdown prompt after it exists (the capability `update_bot` used to
provide besides tools).

**Acceptance criteria**
- `set_bot_content` replaces the Bot's `content`; other fields untouched.
- If content should instead be immutable after creation, this tool is dropped.

## Out of Scope
- Any array-valued MCP argument on these tools (removed by design).
- Changes to subscription authorization rules beyond the signature change
  (`subscribe`/`unsubscribe` become Bot-targeted owner mutations, granted via
  `OwnerBotTools`, not the read baseline).
- Frontend changes (the chart UI already creates Bots with content/parent only).
</content>
