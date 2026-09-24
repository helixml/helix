# Org Bot instances

**Date:** 2026-09-24
**Status:** in progress (branch `feat/org-bot-instances`)
**Related:** `design/2026-09-24-untrusted-bot-mode.md`,
`design/2026-09-24-lightweight-tasks.md` (earlier explorations; this
supersedes their end-customer parts).

## Goal

An Org Bot can run many **instances**: extra sessions that share the bot's
identity but each get their own sandbox. Examples:

- one per WhatsApp customer;
- one per broker filling in the Dubai Properties portal.

Instances are minimal by default. Tools and MCP servers are switched on per
bot only when a use case needs them. Both desktop and headless sandboxes are
supported.

## What an instance is

| Property | Value |
|---|---|
| Storage | a `sessions` row, `session_role = org_bot_instance`, `org_worker_id` = bot, `parent_app` = the bot's app, `project_id` = the bot's project |
| Identity | the bot's instructions **verbatim** in `AGENTS.md` / `CLAUDE.md`: no helix-org preamble, no "install skills" block. Plus the bot's code-agent config (runtime, model) and the bot's repo |
| Sandbox | its own container and workspace; `headless-ubuntu` or `ubuntu-desktop`, chosen per instance, defaulting to the bot's instance profile |
| First turn | none on creation. The first turn is the first user message; there is no hire/activation turn |
| Profile | a snapshot of the bot's instance profile, kept in the session metadata (`bot_instance`) and refreshed when the bot's profile changes |

Instances never become the bot's main session. The spawner, triggers and
transcript mirror only follow the `exploratory` session, so instance chats
stay out of the bot's transcript.

## Instance profile (per bot, `org_bots.instance_profile`)

```json
{
  "sandbox_runtime": "headless-ubuntu",
  "mcp_servers": ["chrome-devtools"],
  "tools": [],
  "helix_skills": false
}
```

- `sandbox_runtime`: the default for new instances. Empty means the bot's
  own runtime.
- `mcp_servers`: the context servers kept in the instance's Zed config.
  - Built-in: `chrome-devtools` (browser), `helix-session`, `helix-desktop`
    (desktop only), `kodit`.
  - Plus any of the bot project's own MCP servers, by name.
  - Everything else is removed. **Default: `["chrome-devtools"]`.**
- `tools`: helix-org tools the instance may call. Empty means no `helix`
  MCP server at all. Served tools = `tools ∩ bot.tools`, checked live on
  every MCP request.
- `helix_skills`: link the `helix-*` agent skills. Default off
  (`HELIX_SKILLS=none`). The project repo's own `.agents/skills` are always
  linked; that is where customer-specific skills live.

Changing tools needs the same org owner/admin rights as changing the bot's
tools.

## API

Org-scoped, under `/api/v1/orgs/{org}`:

| Method | Path | |
|---|---|---|
| GET | `/bots/{id}/instances` | list (id, name, runtime, sandbox status, created, updated, owner) |
| POST | `/bots/{id}/instances` | `{name?, sandbox_runtime?, message?}` → create and start the sandbox asynchronously; `message` goes through the durable prompt queue |
| DELETE | `/bots/{id}/instances/{session_id}` | stop the container, delete the workspace directory, delete the session. The bot is untouched |
| PATCH | `/bots/{id}` | `instance_profile` |

Deleting a bot also deletes all its instances.

MCP tools `create_bot_instance`, `list_bot_instances` and
`delete_bot_instance` let an owner bot such as chief-of-staff manage
instances through the same use case.

## UI

- **Chat sidebar:** instances are listed under their bot, like spec tasks,
  with a sandbox status dot. Clicking one opens the instance chat.
- **Row actions:** delete with confirmation, which also deletes the sandbox
  and workspace. Start/stop sandbox.
- **Bot row menu:** "New instance".
- **Bot settings, "Instances" section:** default runtime, MCP server
  toggles, tools picker (from the bot's tools), Helix skills switch.

## Phases

1. **Instances (this PR):**
   - profile, API, sandbox/MCP/skills/`AGENTS.md` shaping;
   - delete with workspace wipe (new Hydra route);
   - bot delete cascade;
   - UI;
   - MCP tools for owner bots.
2. **Untrusted key for instances:** a fail-closed allow-list, see
   `untrusted-bot-mode.md` §1. Until then an instance's key is the usual
   full session key, so instances are *minimal*, not yet *untrusted*.
3. **Gateway entry:**
   - `POST /api/v1/sessions/chat` with the bot's app id creates an instance;
   - image/file parts go into `incoming/`;
   - `callback_url` is delivered through Standard Webhooks;
   - an app key is bound to the bot.

## Verification

- Unit tests for the profile filter, the org MCP allow-list, the service and
  the handlers.
- End to end in `unmanned-org` on the dev stack:
  - **`b-dubai-properties-broker`:** a desktop and a headless instance;
    `AGENTS.md` is the bare prompt; only the browser MCP is present; the
    browser opens the portal login page (read-only). Then enable
    `chat` + `read_events` and confirm exactly those tools appear.
  - **`chief-of-staff`:** create, list and delete an instance of the broker
    through its MCP tools.
  - **Delete:** the container and workspace directory are gone and the bot
    still works.
