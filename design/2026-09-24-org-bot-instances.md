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

## Context from the browser-support evals

The evals (`design/2026-09-24-browser-support-bot-evals.md`) and the PoC plan
(`design/2026-09-24-support-bot-poc-plan.md`) established:

- **Where the time goes.** A warm bot's simple answer (~20 s) is 9–11 LLM
  calls. Each call re-sends a ~20k-token prompt, costing ~1–1.3 s even from
  cache. Prefill is ~38% of a turn.
- **What that thread proposed next:**
  1. scripted lookups, one tool call per common question;
  2. a smaller fixed prompt: drop the helix-org tools a support bot never
     uses, make the trimmed browser tool set the default, shorten the base
     prompt;
  3. model-server tail latency;
  4. keep logins warm;
  5. cold path: skip the ~11 s activation turn.

Instances are the vehicle for (2) and (5):

- **(2):** the default profile removes every Helix MCP server (only the
  browser stays) and replaces the helix-org preamble with the bare prompt.
- **(5):** an instance has no activation turn.
- **Per-customer isolation:** "one sandbox per customer" from the plan is one
  instance per customer.
- **Browser upgrade still applies:** the trimmed chrome-devtools-mcp 1.10.1
  is a project MCP override named `chrome-devtools`, so the default profile
  already keeps it.

The before/after on the same 12-question mock set is under "Eval: instance vs
bot session" below.

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

## Eval: instance vs bot session (2026-09-24)

Same 12 questions (`questions_all.json`), playbook prompt, default
chrome-devtools-mcp, GLM 5.3 Flash on `ds4-flash-node06`, both arms run back
to back (`run_eval.py --tag cmp2-bot`, then `--tag cmp2-instance --instance`).

**Ready for the first question** (fresh sandbox):

| | bot session (restart + activation turn) | instance (create + one-line warm-up) |
|---|---|---|
| OpenCode | 18 s | 9 s |
| DeepSeek Harness | 20 s | 8 s |

A real instance skips even the warm-up, so it is ready sooner still.

**Answering:**

| | | pass | total | median | LLM calls | prompt tok/call |
|---|---|---|---|---|---|---|
| OpenCode | bot | 12/12 | 325 s | 25.3 s | 123 | 19.4k |
| OpenCode | instance | 12/12 | 370 s | 28.0 s | 132 | 14.4k (−26%) |
| DeepSeek Harness | bot | 12/12 | 546 s | 36.9 s | 167 | 24.5k |
| DeepSeek Harness | instance | 12/12 | 822 s | 42.4 s | 178 | 19.4k (−21%) |

- **One outlier in DeepSeek Harness's instance arm:** `h4_ticket_compare`
  took 276 s. Only 85 s of that was LLM time; the rest was tool time
  (browser), matching the browser-hang case in the evals doc. Without it:
  505 s (bot) vs 546 s (instance).
- **Reading:**
  - The instance cuts start-up roughly in half.
  - It trims the per-call prompt by ~5k tokens (21–26%).
  - It changes neither accuracy nor per-question time, which stays within
    the evals doc's 0.75–1.6× run-to-run noise.
- **Why the prompt cut doesn't speed turns up:** the prompt is served from
  the prefix cache (per-call time 1.93 → 1.94 s for OpenCode). Turn time is
  set by the number of round trips (~10 per question).
- **The lever for the < 10 s goal:** scripted lookups (evals thread item #1),
  not further prompt trimming.
- DeepSeek Harness tool calls are now visible in Helix (155–166 counted per
  arm), where the evals doc's finding 7 had them invisible.

## Verification results (2026-09-24, dev stack, `unmanned-org`)

Phase 1 was applied temporarily to the dev API and Hydra, then reverted.

| Check | Result |
|---|---|
| Headless instance of `b-dubai-properties-broker` via REST | headless container; `AGENTS.md` = the bot prompt verbatim; `HELIX_SKILLS=none`, `~/.agents/skills` empty; context servers `[chrome-devtools]`; the instance's own key gets **403** on `/api/v1/mcp/helix-org` |
| First turn on it (read-only: open the portal login page) | page title "Login" in 24.5 s including cold start; tools = browser + OpenCode built-ins, no Helix tools |
| Profile `tools: [chat, read_events]`, desktop default | the existing instance's profile copy updated; a new instance started as a desktop; context servers `[chrome-devtools, helix]`; org `tools/list` = exactly `chat`, `read_events` |
| chief-of-staff with `create/list/delete_bot_instance`, driven by chat | created, listed and deleted an instance in 12 s; container, workspace directory and session gone; other instances untouched |
| UI | instances listed under the bot with a status dot; bot menu "New desktop / headless instance"; instance opens as its own chat; a headless instance has no Desktop tab; row and context-menu delete with confirmation; Instances settings section saves immediately |
| Bot delete | deleting a bot with a running instance removed the instance's container, workspace and session |

Not tested live: a refused delete by a plain member (every member of the test
org is an owner or platform admin). Covered by `TestBotInstancesDeleteSuite`.

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
