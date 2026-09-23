# Browser-use support bot evals (self-hosted GLM / Qwen)

**Date:** 2026-09-24
**Goal:** prepare a PoC for a customer who wants a support bot that logs into
several internal web systems (whose APIs are unusable) and answers staff
questions. Measure which Helix harness × self-hosted model combination does
this reliably, how fast, and at what token cost, then optimise with skills.

Harness: `evals/browser-support/`.

## Setup

- **Targets.** `mock_systems.py` serves three deliberately awkward legacy apps
  with deterministic data (seeded), so every answer has ground truth:
  - *Acme CRM* — CSRF-protected login with non-obvious field names, 6-page
    customer list, search, detail pages using in-house labels ("Relationship
    Owner" = account manager, "Service Level" = tier, "Contract End").
  - *BillPro* — two-step login plus security question, then a mandatory
    notice checkbox; no account list (lookup by `BA-#####` only); the
    outstanding balance renders `calculating…` and is filled by JS 2s later.
  - *HelpDesk Classic* — login fails unless the Domain dropdown is changed
    from `LOCAL` to `CORP`; status history lives in an iframe.
  - Plus one real public site (saucedemo.com).
  The mock is exposed through an ephemeral Cloudflare quick tunnel because
  sandbox egress deliberately rejects host/private networks
  (`design/2026-08-30-sandbox-egress-hardening.md`); this also matches the
  customer shape (remote web apps). Every request is logged with its
  User-Agent so we can tell browser traffic from `curl`.
- **Questions** (`questions.json`): 8 — two single-screen CRM lookups, two
  billing (async value, counting/summing overdue invoices), one iframe
  history lookup, two cross-system (CRM → billing, helpdesk → CRM), one public
  site. Graded by required-substring groups on the final answer text.
- **Runner** (`run_eval.py`): each variant is a helix-org Bot in
  `unmanned-org` (the new-org path needs a wallet *and* an active
  subscription on this dev stack — see Findings). Per variant: restart the bot
  (fresh session/container), then per question `POST /sessions/{id}/clear`
  and `POST /sessions/chat`. Chrome's profile persists across questions (as a
  real bot's would), so each system's login happens on its first question.
  Metrics come from `interactions.response_entries` (tool calls, final text),
  `llm_calls` (calls, prompt/cached tokens — attributed by session + time
  window because `llm_calls.interaction_id` is `n/a` for these sessions) and
  the mock access log.
- **Models:** `glm-5.3-flash` and `qwen3.8-flash-next` on the
  `ds4-flash-node06` endpoint (both accept image input; verified with a
  data-URL PNG).
- **Harnesses:** `opencode` 1.18.18, `qwen_code` 0.22.0, `zed_agent`,
  `goose_code`, `deepseek_harness`. `claude_code` and `codex_cli` are
  hard-gated to vendor models (`task_management.go` validation), so they can
  only serve as a reference baseline.
- Browser: `chrome-devtools-mcp` 0.25.0 (pinned in `Dockerfile.ubuntu-helix`;
  latest is 1.10.1), headful Chrome on the desktop session.

## Findings (product issues hit while setting this up)

0. **BLOCKER for shipping finding 1 — ACP thread IDs are routed through one
   global map.** `message_added` / `message_completed` resolve the Helix
   session with `contextMappings[acp_thread_id]`
   (`websocket_external_agent_sync.go:1263`, `:3032`), a server-wide map, not
   scoped to the WebSocket connection that sent the event. Goose's ACP session
   ids are date-sequential (`20260923_1`), so two Goose sandboxes started the
   same day both produce `20260923_1`. Observed live: `sup-goose-glm`
   (`ses_…6x5gp77`) and `sup-goose-qwen` (`ses_…dzdn5p`) ran concurrently and
   117 of the events for thread `20260923_1` were delivered to the Qwen
   session — the GLM bot's activation never completed (runner timed out at
   600s) while the Qwen bot's turns completed in 3s carrying the GLM bot's
   tool calls. Every other harness uses random/UUID ids, and before finding 1
   `goose_code` silently ran Zed's agent (UUID threads), so the collision was
   unreachable. **Fixing Goose routing alone would make cross-session — and
   cross-user — message delivery reachable.** The goose fix therefore stays
   off the PR until the mapping is keyed by (agent connection, thread id).

1. **`goose_code` never ran Goose** — fixed in this branch. `buildCodeAgentConfig`
   and `CodeAgentRuntime.ZedAgentName()` had no Goose case, so `AgentName`
   fell through to `zed-agent`: settings-sync-daemon wrote an
   `agent_servers.goose` entry that nothing used, and every "Goose" chat was
   Zed's native agent. Visible as `"agent_name":"zed-agent"` in Zed.log and no
   `goose acp` process. After the fix: `goose acp` runs and receives
   `agent_name:"goose"`. Goose's main calls are ~16k prompt tokens with 102
   tools offered (OpenCode ~19k, Qwen Code ~34k); it also fires a tiny
   "summarize this tool call" side request per tool call and a title request
   per thread, which is why its LLM-call count is ~2.5× its tool-call count.
2. **`zed_agent` Org Bots lose their instructions on every new thread.** Bot
   content is materialised only as `/home/retro/work/AGENTS.md`
   (`hydra_executor.go:863`). Zed's native agent loads rules files only from
   worktree roots (`RULES_FILE_NAMES`, `crates/agent/src/agent.rs`), and the
   worktrees are the repo, `helix-specs` and `incoming` — never their parent.
   Activation turns work because the briefing says "Re-read AGENTS.md"; any
   later thread (clear, new chat) gets the stock "I'm the Zed coding agent"
   prompt. OpenCode / Qwen Code / Goose / DSH walk up to the cwd and pick it
   up. Not fixed here — needs a decision on where Zed should read bot rules
   (open `/home/retro/work` as a worktree, write a worktree-root rules file,
   or have Helix prefix the re-read nudge on thread creation).
3. **`qwen3.8-flash-next` rejects `reasoning_effort: high`** (400 lists
   "xhigh (default), medium, and low") — same trap as `qwen3.8-27b`, but it
   had no profile, so the UI offered `high`. **`glm-5.3-flash` ignores the
   parameter** (identical prompt-token counts for every value, including
   garbage). Both profiles added to `reasoning_efforts.go`; verified on
   `/provider-endpoints?with_models=true`.
4. **A new org created through the API cannot run bots on a billing-enabled
   stack**: first `get org wallet: not found` (wallets are created lazily on
   `GET /wallet`), then `insufficient credits`, then `organization … does not
   have an active subscription` from the LLM proxy. The failure surfaces to
   the chat caller only as a 5-minute "external agent not ready" timeout.
5. **`helix org bots chat` refuses a bot that has never been started**
   ("no project yet — try start") even though its help says it starts the
   bot.
6. **`llm_calls.interaction_id` is `n/a`** for org-bot sessions, so per-turn
   cost must be reconstructed from session + time window.
7. **DeepSeek Harness tool calls are invisible**: its turns contain only
   `text` entries (the browser was used — the mock logged Chrome hits), so a
   support operator cannot see what the bot did.
8. **`chrome-devtools-mcp` is pinned at 0.25.0; latest is 1.10.1.** Default
   tool schemas are ~22 KB (29 tools) on 0.25.0 and ~26 KB (30) on 1.10.1;
   1.10.1 with memory/performance/network/emulation categories off is
   ~18.6 KB (22 tools). That is ~2k tokens/call — small next to harness
   overhead, but it also removes irrelevant tools a small model can wander
   into.
9. OpenRouter retired `stealth/ox-alpha` (it was GLM-5.3 Flash); bots on this
   stack still configured with it (WHR Marketing Bot, a Chief of Staff) now
   get 404s.
