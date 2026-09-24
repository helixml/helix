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
10. **Headless sandboxes had no working browser** — fixed on this branch.
    chrome-devtools-mcp was always started with `--ozone-platform=wayland`;
    headless containers run no compositor, so every call returned
    `Protocol error (Target.setDiscoverTargets): Target closed` and the agent
    fell back to `curl` (answered q1 correctly, but in 150s with 15 curl hits
    and zero browser traffic — useless for JS-rendered apps). The wrapper
    (`desktop/shared/helix-chrome-devtools-mcp.sh`) now starts Chrome with
    `--headless` when there is no Wayland socket. Verified on image `f63c19`:
    headless bot passes q1 and q3 (JS-loaded balance) over Chrome; desktop
    bots on the same image still run `--ozone-platform=wayland` (headful).
11. **chrome-devtools-mcp phoned home by default** (usage statistics to
    Google, performance-trace URLs to the CrUX API) — now disabled with
    `--no-usage-statistics --no-performance-crux` in `zed_config.go`. Matters
    for air-gapped / regulated customers.
12. **Project skills never reached Org Bots** — fixed on this branch.
    Harnesses scan `.agents/skills` relative to their cwd; org-bot sessions
    run with cwd `~/work`, not inside the repo, so `<repo>/.agents/skills` was
    missing from every harness's catalog (OpenCode's `<available_skills>`
    listed only the global `helix-*` skills). `helix-workspace-setup.sh` now
    links the primary repo's skills into `~/.agents/skills` and
    `~/.claude/skills`. Verified on `f63c19`: both skills in the OpenCode
    catalog and loaded (`Loaded skill: browser-lookup`) on the first turn.
    The first "skills" round ran before this fix; it is kept as
    `baseline-rep`, a replicate used to measure noise.
13. **`~/.npm` is root-owned in the desktop image**, so any user
    `npm install` / `npx` fails with `EACCES` ("Your cache folder contains
    root-owned files"). That breaks the "Global agent skills" instruction
    every bot prompt carries (`npx --yes skills add …`) and any MCP launched via
    `npx`. Workaround used here: `--cache ~/work/.npm-cache`. Fix: `chown` the
    cache in the image, or set `npm_config_cache` to a user-owned path.
14. **TLS verification is disabled image-wide**:
    `NODE_TLS_REJECT_UNAUTHORIZED=0`, `ZED_HTTP_INSECURE_TLS=1` and
    `git config --system http.sslVerify false` (`Dockerfile.ubuntu-helix`
    ~1204). Chrome still validates certificates, so passwords typed into
    websites are protected, but every Node process (harnesses, MCP servers,
    npm) and git in the sandbox accepts any certificate. For a PoC that holds
    customer-system credentials this is a finding the customer's security
    review will raise; the fix is to trust the customer CA
    (`NODE_EXTRA_CA_CERTS`, system trust store) instead of disabling checks.

## Results

12 questions per variant (8 core + 4 hard: 72-record aggregation, 8-account
fan-out, near-miss name, filter-and-compare). Times are wall clock per
question as the support user experiences it; tokens are prompt tokens summed
over every LLM call (~95% served from the provider's prefix cache).

**Noise.** A partial replicate of the baseline (31 question pairs) gave
p10–p90 per-question time ratios of 0.75–1.59 and token ratios of 0.82–1.46,
with identical accuracy (31/31). Per-question differences mean little;
per-variant totals over 12 questions and totals over all variants do.

### Baseline (prompt lists systems + credentials only)

| harness | model | pass | median s | total s | prompt tok (M) |
|---|---|---|---|---|---|
| deepseek_harness | glm-5.3-flash | 12/12 | 25 | 460 | 3.9 |
| opencode | glm-5.3-flash | 12/12 | 37 | 514 | 3.9 |
| zed_agent | qwen3.8-flash-next | 12/12 | 46 | 677 | 6.2 |
| deepseek_harness | qwen3.8-flash-next | 12/12 | 59 | 758 | 3.8 |
| opencode | qwen3.8-flash-next | 12/12 | 50 | 985 | 4.7 |
| qwen_code | glm-5.3-flash | 12/12 | 51 | 1228 | 13.0 |
| qwen_code | qwen3.8-flash-next | 12/12 | 110 | 1352 | 13.8 |
| zed_agent | glm-5.3-flash | 7/12 | 20 | 334 | 3.3 |

Accuracy is not the differentiator: every harness except `zed_agent`
(finding 2) answered everything, including the 72-record aggregation — the
models reach for `evaluate_script` + same-origin `fetch()` loops on their own
(Qwen Code wrote an 8-worker parallel fetch pool unprompted). Speed and cost
are: GLM is faster than Qwen on every harness (Qwen's 60s+ reasoning bursts
dominate its tail), and Qwen Code costs ~3× the tokens of OpenCode / DSH
(34k-token harness prompt per call).

### Optimisation: a per-system playbook

Same text, delivered two ways: inlined in the bot prompt (`playbook`) or as two
repo skills, `browser-lookup` (technique) and `support-systems` (site map,
login sequences, URL patterns, field-name glossary) (`skills`).

| comparison (96 question pairs) | pass | total time | prompt tokens |
|---|---|---|---|
| baseline → playbook | 91 → 93 | −38% | −38% |
| baseline → skills (after finding 12 fix) | 91 → 94 | −34% | −34% |
| playbook → skills | 93 → 94 | +6% (noise) | +7% (noise) |

Largest gains on the chattiest harness (Qwen Code: −54% time, −56–61%
tokens); smallest on OpenCode+GLM, which was already efficient. Inlined and
skill delivery perform the same; skills cost two `Loaded skill` calls per
thread but scale to many systems (progressive disclosure), so the PoC should
use one skill per customer system plus a short core prompt.

Goose (run one bot at a time so finding 0 cannot trigger, playbook prompt):
`goose_code` + Qwen 12/12 in 1295s, + GLM 11/12 in 1981s (1200s of that one
hung saucedemo question, see recommendations) — 4–5× OpenCode/DSH on the same
prompt, ~510 LLM calls per 12 questions from its per-tool summary side calls.

### Browser MCP upgrade

`chrome-devtools-mcp` 1.10.1 with memory/performance/network/emulation
categories off, via a project-level `chrome-devtools` override, on the two
best variants against a same-conditions replicate on 0.25.0:

| variant | pass | total s | prompt tok (M) |
|---|---|---|---|
| deepseek_harness + glm | 12 → 12 | 289 → 246 (−15%) | 2.03 → 1.53 (−24%) |
| opencode + glm | 12 → 12 | 399 → 283 (−29%) | 2.81 → 2.11 (−25%) |

Part of the gain is browser-session persistence rather than the smaller tool
list: login submissions fell from 10–14 to 4–6 per 12 questions, i.e. 1.10.1
kept the logged-in browser across cleared threads more often. Not bumped
globally here — spec-task coding agents use the network/performance tools
for frontend debugging, so the category trim belongs per use case (the
project override is exactly that mechanism). The version bump itself needs
its own PR, tested against spec tasks.

### Credentials as secrets

Credentials removed from the prompt, stored as project secrets, bound to the
bot, fetched with `get_secret` right before each login: 24/24, +2% time,
+4% tokens vs the same prompt with inline credentials — i.e. free. No
credential value appeared in any of the 24 answers. (DSH's `get_secret` use
was confirmed from `llm_calls.response`; its tool calls are invisible in the
UI, finding 7.)

### Headless runtime

After finding 10's fix a headless (`headless-ubuntu`) OpenCode+GLM bot
answers over real (headless) Chrome — q1 in 42s, q3 (JS-loaded balance) in
30s. Headless bots skip GNOME + streaming, so this is the cheaper runtime
for a support bot nobody needs to watch.

## PoC recommendation

- **Harness × model:** OpenCode + `glm-5.3-flash` (fast, cheapest per
  answer, tool calls visible to operators). DeepSeek Harness + GLM is
  marginally faster but its tool calls are invisible (finding 7) — a support
  PoC needs an audit trail. Avoid `zed_agent` for bots until finding 2 is
  fixed, and Goose until finding 0 is fixed.
- **Prompt shape:** short core prompt (role, answer format, "never guess,
  never substitute a near-miss record") + one skill per customer system with
  login sequence, URL patterns for direct record access, and a glossary
  mapping the UI's labels to what staff call things. That glossary was the
  single highest-value content: "Relationship Owner" ≠ "account manager" is
  exactly the kind of legacy-UI trap the customer's systems will have.
- **Credentials:** bot secrets + `get_secret`, never in the prompt.
- **Runtime:** headless sandbox (with the finding 10 fix) and a project
  `chrome-devtools` override running 1.10.1 with irrelevant categories off.
- **Browser hangs:** in the Goose playbook run a saucedemo page hung (CDP
  commands timed out after a successful navigate) and the agent spent the
  full 20-minute budget writing its own CDP clients and killing Chrome. The
  `browser-lookup` skill now says: two failed browser calls → fresh tab,
  retry once, then report (added after the measured runs). The PoC should
  also cap each question's wall-clock time.
- **Before the customer security review:** findings 0, 11 and 14.
