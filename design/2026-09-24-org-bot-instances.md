# Org Bot instances

**Date:** 2026-09-24
**Status:** in progress (branches `feat/org-bot-instances`, `feat/org-bot-instances-untrusted`)
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

1. **Instances:**
   - profile, API, sandbox/MCP/skills/`AGENTS.md` shaping;
   - delete with workspace wipe (new Hydra route);
   - bot delete cascade;
   - UI;
   - MCP tools for owner bots.
2. **Untrusted key for instances** (done, see "Restricted key" below).
3. **Gateway entry** (done, see "Gateway" below).

## Restricted key

An instance's sandbox gets an API key of type `bot_instance` instead of the
owner's full session key (`GetOrCreateSessionAPIKey` picks the type from the
session role). The allow-list is fail-closed and every denial is logged as
"Bot instance key denied" with its method and path
(`api/pkg/server/auth_bot_instance_key.go`):

| Allowed | Condition |
|---|---|
| `/v1/chat/completions`, `/v1/responses`, `/v1/messages` (POST), `/v1/models` (GET) | — |
| `/api/v1/sessions/{own}/zed-config` (GET), `zed-config/user`, `agent-startup-error`, `agent-config-applied` (POST) | own session only |
| `/api/v1/ws/user`, `/api/v1/external-agents/sync` | `session_id` = own session |
| `/api/v1/revdial` | `runnerid=desktop-{own}`, or a data connection's dialer token |
| `/api/v1/mcp/helix-org` | the backend serves only the profile's tools |
| `/api/v1/mcp/{session,desktop,kodit}`, `/api/v1/mcp/external/{name}` | only if the profile keeps that server |
| git smart-HTTP | fetch/clone only, only the bot project's repositories |

Everything else is denied: project/session/app/secret inventory, chatting
into any session, git push. Embed keys are now also refused by the git
server.

## Gateway

A third party (e.g. a WhatsApp gateway) drives instances through the
existing chat API with an **app key bound to the bot's app**
(`POST /api/v1/api_keys {"type":"app","app_id":"<bot app>"}`):

1. `POST /api/v1/sessions/chat` without `session_id` creates a new instance
   of that bot (owned by the key's owner, who must be an org member) and
   runs the message as its first turn. The response carries the instance's
   session id as `id`.
2. Later messages pass `session_id`. An app key may only chat in sessions of
   its own app.
3. Inline attachments in the message are written to the instance's
   `~/work/incoming/` and replaced by the same manifest the chat composer
   sends after an upload, so the chat shows them as attachments:
   - `{"type":"image_url","image_url":{"url":"data:image/png;base64,…"}}`
   - `{"type":"file","file":{"filename":"contract.pdf","file_data":"data:application/pdf;base64,…"}}`

   Only base64 `data:` URLs are accepted; Helix never fetches a URL for the
   caller, and an unsupported part is a 400 rather than silently dropped.
   The chat request body limit (10 MB) applies.
4. Each completed instance turn emits a Standard Webhooks event
   `bot_instance.turn_completed` to the org's endpoints (optionally scoped to
   the bot's project). Unlike the other events it carries the reply text,
   because a finished turn never changes and an app key cannot read
   sessions back. Failed turns are retried by the prompt queue and do not
   emit.

The request's `callback_url` field is not used: delivery goes through the
org's signed webhook endpoints instead of an unsigned per-request URL.

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

### Browser stall: cause and fix

- **Cause:** the `h4` outlier was a Chrome profile lock, not a hung page.
  DeepSeek Harness (and Goose) start a new `chrome-devtools-mcp` for every
  ACP session (so every cleared thread) and never stop the old ones. Each
  launched its own Chrome on `/home/retro/work/.chrome-state`; the surviving
  Chrome held the profile lock, so the new server failed with "The browser is
  already running". The agent then killed Chrome by hand (10 `bash` calls,
  ~190 s).
- **How often across all eval rounds:** DeepSeek Harness 2/145 questions,
  Goose 7/50, OpenCode / Qwen Code / Zed 0.
- **Fix:** `helix-chrome-devtools-mcp` now starts one detached Chrome per
  sandbox with a loopback debugging port (under `flock`) and every MCP server
  attaches with `--browserUrl`.
- **Reproduced before/after in a sandbox:** with two overlapping MCP
  servers, the old wrapper fails the second navigate; the new one serves
  three from one Chrome.
- **End to end on image `7a6706`, DeepSeek Harness instance, 12 questions:**
  12/12, 0 lock errors, 0 `bash` fallbacks, 262 s total and 106 LLM calls
  (vs 822 s / 178 in the stalled run). `h4` took 11 s.
- **Not established:** why the calls also dropped. It is one run, and the
  mock access log that would show logins surviving across threads was not
  recording.

**The leak itself:** Zed never told the harness that a thread Helix cleared
was gone, so DeepSeek Harness and Goose kept one idle MCP server per cleared
thread. On clear, Helix now sends `close_thread` for the discarded thread
when the agent is connected, and Zed closes that ACP session
(`session/close`), which stops its MCP servers.

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

Restricted key and gateway, same stack, bot `sup-opencode-glm-headless`
(OpenCode, GLM 5.3 Flash):

| Check | Result |
|---|---|
| `/sessions/chat` with only an app key for the bot's app | new `org_bot_instance` session, key type `bot_instance`; reply in 8.7 s including sandbox cold start |
| Follow-up with `session_id` | answered from the thread's context |
| Same app key into another app's session | 403 |
| Inline PNG + `file` part (text document) | both written to `~/work/incoming/` (`image-1.png`, `contract.txt`); prompt carries the manifest; rent read from the document. The image was refused by the agent because the dev stack's older `model_info.json` lacks `glm-5.3-flash` image input (this branch has it) |
| Org webhook endpoint for `bot_instance.turn_completed`, project-scoped | delivered with the reply; Standard Webhooks signature verified against the endpoint secret |
| Inside the instance sandbox, with its own key | `/projects`, `/sessions`, `/apps`, `/secrets` → 403; own `zed-config` → 200; LLM proxy passes auth; `git fetch` OK; `git push` → 403 "this API key may not access this repository" |
| Headless and desktop instance turns (browser on the desktop one) | completed; the only "Bot instance key denied" lines were the deliberate probes above |

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
