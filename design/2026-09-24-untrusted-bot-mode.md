# Untrusted mode for Org Bots

**Date:** 2026-09-24
**Status:** proposal, decisions agreed 2026-09-24 (see "Decisions") —
nothing here is implemented or tested yet.

**Partly superseded (2026-09-24) by `design/2026-09-24-lightweight-tasks.md`:**
end-customer cases run as lightweight tasks, not as Org Bot conversations.

- **Still current:** "Why removing tools is not enough", Part 1 §1 (untrusted
  key) and §2 (tool surface). The trust setting moves from the bot to the
  project.
- **Superseded:** Part 1 §3–4 (bot instructions, hire turn) and all of Part 2.
  The "What exists today" table in Part 2 remains accurate as a record of the
  session chat API.
**Related:** `design/2026-09-24-support-bot-poc-plan.md` (W2.5 "trim the fixed
prompt", W4 cold path), `design/2026-09-24-browser-support-bot-evals.md`
(startup timings), `api/pkg/server/auth_embed_key.go` (the fail-closed key
pattern this reuses).

## Goal

A bot that works for end users on the open web — filling forms, reading
pages — where anything it reads may be hostile. It needs:

- the LLM, a browser, internet;
- its own git repo, which carries the customer-specific skills and scripts;
- the prompt the operator wrote, and nothing else.

It must not be able to read or change anything in Helix beyond its own
session. Starting it should cost only what that list needs.

"Untrusted" describes the bot's inputs, not its operator: we assume the model
will eventually follow instructions planted in a web page.

## Decisions

1. **No `get_secret`, no Helix org MCP at all.** Any details the bot needs
   (for example a form's account data) come through the operator's prompt or
   the agent identity.
2. **The repo is read-only** in untrusted mode.
3. **No model pinning for now.** The key may call any model the owner can.
4. **One sandbox per end customer for the PoC.** Browser state is never shared
   between customers, so a browser reset between conversations is not needed
   yet.
5. **End users reach the bot through a third party.** For example a WhatsApp
   gateway, which calls the existing session chat API (`/api/v1/sessions/chat`).
   Helix gets no WhatsApp integration; the bot's reply is the chat API
   response. See Part 2.

## Example: document-checking bot on WhatsApp

A WhatsApp user chats with the bot. The bot's prompt and skills tell it to
collect documents (passport, business licence), check them against each
other (for example, the licence owner's name matches the passport), go back
and forth with the user until everything is consistent, then submit the
details on the user's behalf through a website. Each user gets their own
sandbox; every sandbox has the same bot identity (prompt, repo, skills,
runtime, model).

## Why removing tools is not enough

Everything below is verified in code on `feat/support-bot-fast-start`.

1. **The sandbox holds a full user key.** `GetOrCreateSessionAPIKey`
   (`api/pkg/services/spec_driven_task_service.go:2050`) mints
   `Type: APIkeytypeAPI` owned by the session owner (the creating user or the
   org owner, `helix_org_inproc.go:306-341`). `SessionID` / `ProjectID` /
   `SpecTaskID` on it are attribution only (`auth_embed_key.go:20-25`); auth
   loads the owner as the caller (`auth_middleware.go:225-240`). The doc
   comments on the minting function ("Non-SpecTask sessions: LLM calls only")
   are wrong.
2. **The key is everywhere in the container:** `USER_API_TOKEN`,
   `ZED_HELIX_TOKEN`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`
   (`types.go:2240-2246`), harness env, MCP `Authorization` headers,
   `~/.git-credentials`.
3. **The whole API is reachable.** `helix-api.internal:18080` is Hydra's
   reverse proxy, which forwards everything except `/debug`
   (`api/pkg/hydra/server.go:140-192`). The `helix` CLI is installed and
   picks the key up automatically (`api/pkg/cli/api/cmd.go:48-49`).
4. **Git ignores key scoping entirely.** `GitHTTPServer.authMiddleware`
   (`api/pkg/services/git_http_server.go:211-278`) resolves the key to the
   bare owner row and never looks at key type, org, session or project. Read
   access is "owner or `ActionGet`" (`:934`); branch restriction applies only to
   keys with a `SpecTaskID` (`:899-932`). A bot key can clone and push every
   repo its owner can.
5. **Keys live as long as the container.** `StopDesktop` revokes the
   session's keys (`hydra_executor.go:992-996`) and a restart mints new ones.
   A running bot's key stays valid for as long as the bot runs.
6. **Desktop containers are privileged** (`hydra_executor.go:1616-1621`);
   headless ones run unprivileged with rootless Podman.

So an injected `curl -H "Authorization: Bearer $USER_API_TOKEN"
helix-api.internal:18080/api/v1/...` acts as the owner. Removing the MCP
servers does not change that.

Side finding: item 4 applies to **embed keys** too, which are handed to
browsers on untrusted pages. As far as I can tell from the code, an embed key
can `git clone` any repo its service account can read. Not tested; it gets
fixed in the same change (step 1 below).

## Part 1 — Untrusted mode

One field on the bot turns on everything below. There are no separate flags,
because the pieces only make sense together.

```
org_bots.trust_mode   text not null default ''   -- '' | 'untrusted'
```

- Exposed on `CreateBotRequest` / `UpdateBotRequest` and on MCP `create_bot`.
- Any caller may create an untrusted bot, since it has strictly less power
  than a normal one. Only org owners and admins may switch a bot from
  untrusted to trusted.
- A change takes effect on the next session start: the key is re-minted and
  the container recreated.
- `trust_mode: untrusted` requires `sandbox_runtime: headless-ubuntu`; the API
  rejects any other combination (item 6).

### 1. Untrusted session key (the core of this)

A new key type, `APIkeytypeUntrusted = "untrusted"`, minted by
`GetOrCreateSessionAPIKey` when the session belongs to an untrusted bot. The
lookup for an existing key must include the type, so a full key already
minted for the session is never reused.

It is enforced in the same places as the embed key, **plus git**:

| Where | File |
|---|---|
| `extractMiddleware` (all of `/api/v1`) | `auth_middleware.go:404` |
| `auth()` (LLM proxy routes on the bare router) | `auth_middleware.go:459` |
| `GitHTTPServer.authMiddleware` | `git_http_server.go:211` — load the full key record, not just the owner |

It fails closed: anything not in the table below is 403. Every row is bound
to the ids stored on the key; the handler's "does this user own it" check is
not enough, because in a replica pool one owner holds every bot.

| Caller | Method | Path | Bound to | Needed for |
|---|---|---|---|---|
| settings-sync-daemon | GET | `/api/v1/sessions/{id}/zed-config` | `id == key.SessionID` | config (required) |
| settings-sync-daemon | POST | `/api/v1/sessions/{id}/agent-startup-error` | session | error reporting |
| settings-sync-daemon | POST | `/api/v1/sessions/{id}/agent-config-applied` | session | runtime switch |
| settings-sync-daemon | GET (WS) | `/api/v1/ws/user?session_id=` | session (query) | config push; polling fallback exists |
| Zed | GET (WS) | `/api/v1/external-agents/sync?session_id=` | session (query) | chat (required) |
| desktop-bridge | GET (WS) | `/api/v1/revdial?runnerid=desktop-{id}` and `?revdial.dialer=` | `runnerid == "desktop-"+key.SessionID` | exec/upload tunnel (required) |
| MCP readiness + harness | OPTIONS/GET/POST | `/api/v1/mcp/external/{name}` | `name` ∈ the session's configured external MCPs | project MCPs, if any |
| harness | POST | `/v1/chat/completions`, `/v1/responses`, `/v1/messages` | — (no model pinning, decision 3) | LLM (required) |
| harness / Zed | GET | `/v1/models` | — | provider discovery |
| git | GET | `/git/{repo}/info/refs?service=git-upload-pack` | `repo` ∈ the bot project's repos | clone |
| git | POST | `/git/{repo}/git-upload-pack` | same | clone/fetch |

**Deliberately absent:**

- `git-receive-pack`: the repo is operator-authored and read-only for the bot
  (decision 2).
- `/api/v1/mcp/helix-org`, `/api/v1/mcp/session`, `helix-desktop`,
  `helix-tasks`, `kodit`, `helix` (native): §2 stops configuring them.
- `/api/v1/sessions/chat`: the bot cannot start or post to other
  conversations.
- `/api/v1/sessions/{id}/claude-credentials` and `codex-credentials`: vendor
  subscription runtimes are out of scope for untrusted bots.
- Projects, orgs, secrets REST, spec tasks, repos REST, and everything the
  `helix` CLI would use.

The path table comes from an inventory of every sandbox → API call (daemon,
Zed, desktop-bridge, MCP configs, harness configs, git in the startup
scripts). The Zed rows were read on the fork's
`perf/agent-ready-without-thread` branch. The inventory is not proof. The
end-to-end test in the plan below is what shows the list is complete.

**Revocation:** already handled. `StopDesktop` revokes every key minted for
the session (`hydra_executor.go:992`), including the new type.

The key stays readable to the agent, and that is fine: the allow-list is
enforced server-side, so a leaked key reaches nothing outside the session.

### 2. Tool surface

`GenerateZedMCPConfig` (`api/pkg/external-agent/zed_config.go:101`) for an
untrusted session emits only:

- `chrome-devtools` (stdio, no Helix calls). A project override with the same
  name still replaces it (for example 1.10.1 with categories trimmed).
- Project MCPs (`project.Skills.MCPs`). This is how customer-specific tools
  get in.

Not emitted: `helix` (org MCP, decision 1), `helix-session`, `helix-desktop`
(already off headless), `helix-tasks`, `kodit`, `helix-native`, and
agent-level MCPs from the legacy App.

`bot.Tools` keeps its baseline in the database (`nodes.go:147`, `:452-467`)
but has no effect: with no `helix` server configured and
`/api/v1/mcp/helix-org` denied by the key, the bot cannot reach the org tools.
No change to the org tool reconcile is needed.

The readiness gate (`start-zed-core.sh:146`) keeps working unchanged: it
probes only what is configured, and the key allows exactly that.

Skills: Hydra sets `HELIX_SKILLS=""` for untrusted sessions, which means no
`helix-*` skills and no GitHub fetch. Project repo skills
(`<repo>/.agents/skills`, `helix-workspace-setup.sh:749-765`) are still
linked; that is where the customer skills live.

### 3. Instructions

`AGENTS.md` / `CLAUDE.md` contain the bot's `content` verbatim. The fixed
preamble and the "Global agent skills" block (`briefing/prompt.go:15-31`) are
dropped. That block tells the agent to `npx skills add` third-party code,
which is exactly what an injected page would ask for. The org-wide
`worker.specs_mandate` does not apply to untrusted bots.

### 4. Startup

Budget, headless bot from start request to "ready for a question" (evals doc,
medians):

| Phase | Today | Untrusted mode |
|---|---|---|
| schedule + create container | 0.4 s | same |
| workspace setup | 3.3 s | shorter: no `helix-specs`, hooks, skills fetch or container engine (not measured) |
| Zed start → connected | 6.0 s | unchanged (separate work) |
| harness + MCP start | 1.6 s | slightly less (fewer servers) |
| activation turn | 9.0 s | **removed** |
| **total** | **~20 s** | **~9–10 s** (estimate), paid on a conversation's first message |

Changes:

1. **No hire turn and no bot session.** `lifecycle.Create` skips
   `DispatchHire` for untrusted bots. An untrusted bot has no durable
   session of its own; sandboxes exist only per conversation (Part 2), and a
   conversation's first turn is the end user's first message. This avoids
   the no-prompt start path that `EnsureAndSend` (`sessions.go:147`) and
   `StartSession` (`helix_org_inproc.go:985`) do not support. Pre-warming
   conversation sandboxes is a later optimisation.
2. **Workspace setup, gated on `HELIX_UNTRUSTED=1`** set by Hydra:
   - Clone the project's repos read-only (the per-bot repo plus any attached
     customer-skills repo). Consider shallow clones.
   - Skip the `helix-specs` orphan branch and worktree, git hooks and branch
     setup. `~/.git-credentials` still gets the untrusted key, since it is
     needed for fetch.
   - Skip `17-start-dockerd.sh` (rootless Podman + BuildKit). This is both
     time and kernel attack surface.

These script gates are for speed only. The agent can run anything inside the
container, so the security boundary is the key (§1) and egress, not the
scripts.

### 5. Git repo

- The per-bot repo is still created (`project.go:567`).
- Customer skills and scripts go in one **shared skills repo** attached to
  every bot's project, so a replica pool reads one source of truth:
  `.agents/skills/<intent>/SKILL.md` + `scripts/`.
- The bot reads it and never writes to it. Operators edit the repo; the next
  session start fetches it.

## Part 2 — Conversations through the session chat API

### What exists today

Verified in code; line numbers are under `api/pkg/`.

| Area | Today |
|---|---|
| New session for a bot | `/sessions/chat` with the bot's app id returns 400: bot apps are `org_agent`, and new sessions require `helix_agent` (`server/session_handlers.go:866`). |
| Bot identity | With only the bot's `project_id`, the session gets the project repos but no identity. `OrgWorkerID`, `RuntimeInstructions` and `SandboxRuntime` are `json:"-"` (`types/types.go:651-666`) and are set only by the internal `StartExternalAgentSession` (`server/session_handlers.go:3219-3229`), which the org spawner calls. |
| Sessions per bot | One. "A worker has one durable session" (`org/infrastructure/runtime/helix/sessions.go:139`, `state.go:47`). |
| Images in chat | Dropped. Only text parts reach the agent (`extractExternalAgentUserMessage`, `controller/controller_external_agent.go:515`); an image-only message fails with "no user message found". |
| Files | `POST /api/v1/external-agents/{id}/upload` writes to `/home/retro/work/incoming/<name>` (`server/external_agent_handlers.go:815`, `desktop/upload.go:19`) and resumes a stopped sandbox first. The UI then mentions the path in the prompt (`frontend/src/components/common/chatAttachments.ts:95-114`). |
| Third-party key | Nothing limits a caller to one bot. App keys cover only `/v1/chat/completions` and `/api/v1/sessions/chat` (`server/auth_middleware.go:29-32`) and are rejected for `org_agent` apps. Embed keys are bound to one spec task. |
| Reply | Blocking: one `chat.completion` with `id` = session id and the final text. `stream:true`: SSE whose first chunk carries the session id. Waits up to 300 s for the sandbox, then 2 h idle, 24 h cap (`controller_external_agent.go:15-24`). |
| Idle | After `HELIX_DESKTOP_IDLE_TIMEOUT` (1 h) the container and its docker volume are deleted and keys revoked (`external-agent/idle_checker.go:44-86`, `hydra/devcontainer.go:1853`). The workspace bind mount (`/home/retro/work`, including `.chrome-state` and `incoming/`) survives. The next message restarts the sandbox (`server/spec_task_design_review_handlers.go:1367`). |
| Concurrency | Default 10 concurrent headless sandboxes per org, configurable as `max_concurrent_headless_sandboxes` (`types/system_settings.go:243`). |
| Conversation id | No field on a session for an external id; the gateway keeps its own number → session id map. |

### Changes

1. **A conversation is a session of an untrusted bot.**
   - `SessionChatRequest` gets a public `bot_id` field (resolved in the
     caller's org). Without `session_id`, it creates a new session through
     the same path the org spawner uses (`StartExternalAgentSession` with
     `OrgWorkerID`, `RuntimeInstructions`, headless `SandboxRuntime`, the
     bot's code-agent config and project). The session is not recorded as the
     bot's durable session.
   - Only for untrusted bots. A trusted bot's org tools assume one session per
     worker, so it keeps the current behaviour.
   - `bot_id`, not the legacy app id: the app link is being migrated away
     (glossary), so the public API should not depend on it.
2. **A conversation key for the third party**, `APIkeytypeBotChat`, bound to
   one (org, bot). It fails closed like the embed key, enforced in the same
   two middleware hooks:

   | Method | Path | Allowed when |
   |---|---|---|
   | POST | `/api/v1/sessions/chat` | new session with `bot_id` == key's bot, or `session_id` whose session's `OrgWorkerID` == key's bot |
   | POST | `/api/v1/external-agents/{id}/upload` | session belongs to the key's bot |
   | POST | `/api/v1/sessions/{id}/cancel` | same |

   Minted by an org owner or admin from the bot's settings.
3. **Files in the chat request.** Image and file parts in `messages` are
   written into the sandbox's `incoming/` using the upload path above, then
   replaced by the same "Attachments available in the agent workspace" note
   the UI uses, before the text reaches the agent. That gives the gateway one
   call per WhatsApp message and fixes the silent image drop for every
   external-agent session. The 10 MB chat body cap limits a base64 file to
   about 7 MB; larger documents go through the upload endpoint once the
   session exists. **Not verified:** that OpenCode passes an image file it
   reads into the model's context. The end-to-end test has to prove the model
   reads a name off a sample passport.
4. **Conversations span days.**
   - After an idle stop the workspace survives, and the next message restarts
     the sandbox. The restart must re-apply the bot identity (headless,
     AGENTS.md). It should, because `applySessionBootstrap` reads it from
     session metadata (`external-agent/hydra_executor.go:841`), but that is
     not verified.
   - Whether the agent's thread (its memory of the conversation) survives a
     restart is not verified either. The bot prompt should therefore keep a
     case file in the workspace (`case.md`: documents received, what was
     checked, what is missing). That is prompt text, not code, and it makes
     restarts safe whatever the harness keeps.
   - Raise `max_concurrent_headless_sandboxes` to the expected number of
     customers active within one idle timeout.

### Gateway contract

What the third party implements, using only the chat API:

1. First message from a number: `POST /api/v1/sessions/chat` with `bot_id`,
   the message and any documents as parts, `stream: true`. Store the session
   id from the first chunk against the number. Use streaming because a
   blocking call that fails to start the sandbox returns a 500 without the
   session id.
2. Later messages: the same call with `session_id`.
3. Send the final assistant text back to WhatsApp.
4. Send one message per session at a time. How `/sessions/chat` treats a
   second message while a turn is running is not verified.
5. Don't rely on `callback_url`: it is stored but never called.

## What this does not cover

- **Details in the prompt or agent identity.** Anything put there is plain
  text to the agent, which has internet access, so a hijacked bot can send it
  anywhere. Only include what the role cannot work without.
- **Browser state across end users.** Out of scope for the PoC because each
  customer gets their own sandbox (decision 4). A pool that serves several
  customers from one bot would share one Chrome profile (cookies, autofill,
  history) and would need a reset between conversations first (PoC plan
  W3.1).
- **Egress.** Public internet is open and private ranges are blocked
  (`sandbox/06-setup-network-policy.sh`). `HELIX_SANDBOX_EGRESS_ALLOW_CIDRS`
  is global per host, not per bot (PoC plan W5.2).
- **LLM spend.** With no model pinning (decision 3), a hijacked bot can call
  any model the owner can reach, bounded only by the org's wallet. Accepted
  for the PoC; model pinning and a per-session token cap are the follow-ups.
- **TLS verification is disabled image-wide** (evals finding 14). That is
  independent of this change, but a customer security review will ask about
  both together.
- **Personal data.** Passport images end up in the sandbox workspace (kept
  after idle stops), in stored interactions, in `llm_calls`, and with the
  model provider. The customer needs a retention and deletion policy before
  their review. Whether deleting a session removes its workspace is not
  verified.
- **Submitting on the user's behalf.** The bot prompt should show the user
  exactly what will be submitted and wait for an explicit yes. That is a
  prompt rule; nothing in Helix enforces it.

## Side findings (independent of this design)

- **App keys are not bound to their app for existing sessions.** With a
  `session_id`, `authorizeUserToSession` ignores the key's app, and the
  handler then takes the app from the session
  (`server/session_handlers.go:837-839`). An app key can chat into any session
  its owner can update.
- **Images sent to external-agent sessions are silently dropped** (Part 2
  table).
- **`callback_url` is accepted and never called.**
- **A blocking `/sessions/chat` that fails to start the sandbox** returns a
  500 without the session id it created.

## Implementation plan

Each step is its own PR and tested end to end in the inner Helix before merge.

1. **Key.**
   - `APIkeytypeUntrusted`, the allow-list with session/repo binding, git
     enforcement (which also closes the embed-key git gap).
   - Table tests in the style of `auth_embed_key_test.go`, covering every
     allowed row plus bound-id mismatches.
2. **Bot field + tool surface + instructions.** `trust_mode` column, API/MCP
   fields, headless-only validation, the `GenerateZedMCPConfig` branch, the
   bare AGENTS.md.
3. **Conversations.** `bot_id` on `/sessions/chat` for untrusted bots, and
   the `APIkeytypeBotChat` key.
4. **Files in chat.** Image and file parts written to `incoming/`.
5. **Startup.** No hire turn, no bot session, `HELIX_UNTRUSTED` gates in the
   workspace and init scripts.
6. **End-to-end verification** in the inner Helix with a headless OpenCode
   bot, driven by a script that plays the gateway (no WhatsApp needed).
   - **Positive:**
     - Two simulated users at once, each with their own conversation.
     - Each sends a sample passport and business licence; the bot spots a name
       mismatch, asks for a correction, then submits to a mock form after an
       explicit yes.
     - One conversation is idle-stopped mid-case; its next message must resume
       with the case intact.
   - **Negative, sandbox key** (from inside the sandbox, each expected to
     fail):
     - `helix api GET /projects` → 403
     - `curl .../api/v1/sessions/chat` → 403
     - `git clone` of a repo outside the project → 403
     - `git push` to its own repo → 403
     - `curl .../api/v1/mcp/helix-org` → 403
     - `revdial` with another session's runner id → 403
     - a page or document carrying a prompt injection that asks the bot to
       call the Helix API
   - **Negative, conversation key:**
     - chat into another bot's session → 403
     - create a session for another bot → 403
     - `GET /api/v1/projects` → 403

## Follow-ups (not in the PoC)

- Model pinning and a per-session token cap for untrusted keys (decision 3).
- Browser reset between conversations, if one bot ever serves more than one
  customer (decision 4).
- A branch-restricted write path to the repo, if a use case needs the bot to
  save output (decision 2).
