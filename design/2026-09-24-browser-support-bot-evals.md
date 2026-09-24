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

## Spec tasks vs Org Bots: where the time goes (2026-09-24)

Top 3 harness × model by the earlier rounds (Goose and Zed excluded): DSH+GLM,
OpenCode+GLM, DSH+Qwen. Same 6 questions (q1, q3, q5, q6, h1, h3), playbook
prompt. Spec tasks: one `just_do_it_mode` task per question, fresh sandbox each
(18 launches per runtime). Bots: one cold launch per variant, then the 6
questions on the warm bot. `evals/browser-support/run_spectasks.py`,
`run_bots_timing.py`, `timing.py`, `report_timing.py`; all 72 answers correct.

### Launch (median seconds from start request)

| phase | bot-desktop | bot-headless | st-desktop | st-headless |
|---|---|---|---|---|
| schedule + create container | 0.2 | 0.4 | 0.1 | 0.1 |
| workspace setup (clone, skills, config) | 5.6 | 3.3 | 6.3 | 3.9 |
| Zed start → first chat delivered | 17.0 | 6.0 | 16.0 | 6.0 |
| harness + MCP start → first LLM call | 1.0 | 1.6 | 1.3 | 1.3 |
| **request → first LLM call** | 23.8 | 12.6 | 24.0 | 10.9 |
| bot activation turn (LLM, bots only) | 11.7 | 9.0 | — | — |
| **request → ready for a question** | 35.4 | 20.3 | 24.0 | 10.9 |
| samples | 3 | 3 | 18 | 18 |

### Question turn split (median seconds per question)

| tag | variant | pass | wall | LLM prefill | LLM decode | tools + harness | dispatch + finish | LLM calls | prompt tok (k) |
|---|---|---|---|---|---|---|---|---|---|
| bot-desktop | sup-dsh-glm | 6/6 | 21.8 | 10.4 | 3.8 | 5.0 | 0.7 | 8.0 | 146.9 |
| bot-desktop | sup-dsh-qwen | 6/6 | 27.6 | 10.1 | 14.1 | 3.5 | 0.7 | 10.0 | 215.5 |
| bot-desktop | sup-opencode-glm | 6/6 | 25.8 | 11.4 | 3.5 | 5.3 | 3.2 | 9.5 | 193.4 |
| bot-headless | sup-dsh-glm | 6/6 | 20.2 | 10.7 | 4.3 | 4.3 | 0.7 | 8.0 | 138.3 |
| bot-headless | sup-dsh-qwen | 6/6 | 29.1 | 8.5 | 14.7 | 3.9 | 0.7 | 8.5 | 174.2 |
| bot-headless | sup-opencode-glm | 6/6 | 25.5 | 12.2 | 3.7 | 5.8 | 3.2 | 9.0 | 167.5 |
| st-desktop | sup-dsh-glm | 6/6 | 55.7 | 18.3 | 23.7 | 8.0 | 0.0 | 13.5 | 210.0 |
| st-desktop | sup-dsh-qwen | 6/6 | 63.7 | 13.1 | 35.1 | 11.4 | 0.0 | 12.0 | 201.0 |
| st-desktop | sup-opencode-glm | 6/6 | 25.2 | 14.1 | 3.4 | 6.2 | 0.1 | 9.5 | 156.7 |
| st-headless | sup-dsh-glm | 6/6 | 73.5 | 17.6 | 43.8 | 12.0 | 0.0 | 12.5 | 207.2 |
| st-headless | sup-dsh-qwen | 6/6 | 68.0 | 13.2 | 35.1 | 10.8 | 0.0 | 12.0 | 185.7 |
| st-headless | sup-opencode-glm | 6/6 | 56.1 | 14.9 | 23.5 | 16.4 | 0.1 | 14.5 | 181.3 |

### Share of total turn time (sum over all questions)

| tag | LLM prefill | LLM decode | tools + harness | dispatch + finish |
|---|---|---|---|---|
| bot-desktop | 39% | 29% | 28% | 5% |
| bot-headless | 38% | 26% | 32% | 5% |
| st-desktop | 28% | 42% | 30% | 0% |
| st-headless | 14% | 36% | 49% | 0% |

### Time to answer one question (median seconds)

| tag | cold (launch + question) | warm (question only) |
|---|---|---|
| bot-desktop | 62.3 | 26.9 |
| bot-headless | 47.1 | 26.8 |
| st-desktop | 73.0 | n/a — every question launches a sandbox |
| st-headless | 75.0 | n/a — every question launches a sandbox |

Reading it:

- **Launch is ~24s desktop / ~11s headless for both.** Container create is
  0.1–0.4s (image cached), workspace setup 3–6s, and the dominant cost is
  Zed + GNOME start to the first delivered chat: 16–17s on desktop vs 6s
  headless. Harness + MCP start to the first LLM call is ~1s.
- **Spec tasks reach the first LLM call no faster than bots**; bots then run a
  9–12s activation turn (the "Re-read AGENTS.md" briefing) before they accept
  a question. Spec tasks skip that turn.
- **But spec-task turns are 2–3× slower** (56–74s vs 20–29s for DSH), so a
  cold spec task answers in ~73–75s against 47–62s for a cold bot and ~27s for
  a warm bot. For a support bot the warm bot wins by ~3×: the sandbox launch
  is paid once, not per question.
- **Why spec-task turns are slow: they hit the headless ACP/MCP startup race**
  (`design/2026-09-15-headless-acp-mcp-startup-race.md`; the pinned Zed does
  not carry that fix). The task prompt is delivered the moment Zed connects,
  before the MCP servers are attached:
  - OpenCode, headless spec task: **10 tools on every call — no MCP servers at
    all**, 0 browser calls, 62 shell calls (it drove Chrome via hand-written
    CDP scripts).
  - DSH, all 12 spec tasks: the first call carries 14–34 tools and no browser;
    DSH plans its own automation (pip-installing Playwright, writing Node CDP
    clients) and keeps that plan after the browser tools appear on call 2 —
    3.5–6.3k completion tokens per question vs ~0.7k on a bot.
  - Bots hit the same race, but only on the activation turn; by the first
    question every harness has the full tool list (83–97 tools).
  Every answer was still correct because the models worked around it — which
  would not survive a JS-heavy SPA or an MFA prompt.
- **Where a warm bot's turn goes:** LLM prefill ~38%, LLM decode ~27%, tools
  + harness ~30%, Helix dispatch/finish ~5%. Prefill dominates because every
  call re-sends a ~17–22k-token prompt (bot AGENTS.md, helix MCP tool list,
  browser tools) — served mostly from prefix cache, but TTFT is still ~1s per
  call × 8–10 calls. The levers are fewer calls (playbook/skills) and a
  smaller fixed prompt (fewer tools).
- **DSH vs OpenCode (GLM):** DSH is faster in all six bot rounds (median
  4–19s per question, 11–43% total): similar call counts, but a smaller
  per-call prompt (~6.6k vs ~10–21k for the first main call) and lower
  per-step harness overhead. In spec tasks the ranking flips because of the
  race above. DSH's tool calls remain invisible in Helix (finding 7).
- **Zed agent did complete.** All 72 Zed turns reached `complete` with no
  errors or timeouts. The failures were fast (8–19s) confident "there is no
  Acme CRM in this workspace" answers with zero browser traffic — finding 2,
  the bot instructions never reach Zed's native agent after a thread reset.

Additional issues found in this pass:

15. **Finished spec tasks hold implementation WIP slots**: `stop-agent`
    leaves the task in `implementation`, so later tasks sit in
    `queued_implementation` until the old ones are archived.
16. **Archiving does not stop a task's sandbox**: two queued tasks started
    when slots freed and kept running after being archived until
    `stop-agent` was called explicitly.

## Toward <10s answers: freeze instead of snapshot (2026-09-24)

`evals/browser-support/pause_test.py`, headless DSH+GLM bot, playbook prompt:

| step | wall | LLM calls |
|---|---|---|
| cold launch → ready (incl. 9s activation turn) | 20s | — |
| q1, first visit to CRM (login) | 19.1s | 10 |
| q3, first visit to billing (3-step login) | 24.7s | 14 |
| q1 again, already logged in | 8.8s | 5 |
| q5, first visit to helpdesk | 14.2s | 7 |
| `docker pause` 90s → `unpause` (0.09s) → q3 | **7.9s** | 4 |
| `docker pause` 600s → `unpause` (0.08s) → q5 | **8.2s** | 3 |

Warm headless bot: 833 MiB RSS, 0.6% CPU idle. The WebSocket, harness, MCP
servers and the logged-in Chrome all survived a 10-minute freeze.

- **CRIU / `docker checkpoint` is not available here** (no `criu` on host or in
  the sandbox; nested dockerd is not experimental) and was not tested. Known
  risk areas: Chrome (sandbox, GPU process, shm), stale TCP to Helix and the
  LLM on restore, the desktop GPU path, and customer tokens + cookies written
  to disk in the image. Its only advantage over a freeze is freeing RAM.
- Startup is not the latency problem for a per-customer warm bot: it is out of
  the critical path. The critical path is **the login dance on a system's
  first visit (10–15s) and the number of LLM round trips** (~1–2s each).
- Helix's existing "paused" state is a *stopped* container
  (`external-agent/idle_checker.go`); there is no in-memory freeze tier.

## Startup after the fixes (2026-09-24, branch `feat/support-bot-fast-start`)

Headless, GLM 5.3 Flash, playbook prompt, image `242f8a`, all answers correct.
Median seconds from the start request (`run/timing.jsonl`, tags `st-headless`
vs `st-headless-fast2` / `bot-headless-fast`):

| phase | spec task before (DSH / OpenCode) | spec task now (DSH / OpenCode) | bot now |
|---|---|---|---|
| container started | 0.1 | 0.2 / 0.3 | 0.2 |
| workspace setup complete | 4.2 | 2.5 / 2.7 | 1.8 |
| first chat reaches Zed | 9.6 | 2.5 / 2.7 | 1.8 |
| first LLM call | 10.9 / 12.8 | 4.7 / 4.8 | 4.3–4.5 |
| answered (spec task) / ready (bot) | 84.5 / 69.0 | 31.0 / 29.0 | 15.2 |

Where the time went and what changed:

- **5.0s — Zed's `agent_ready` timer.** On a connection with no thread to
  reopen, Zed waited 5s for an `open_thread` that never came. Helix now sends
  `no_open_thread`; Zed reports ready at once.
- **~10s — OpenCode installing its plugin SDK** (`@opencode-ai/plugin`) from
  npm into its config home on every fresh sandbox. It used to fail fast on the
  root-owned `~/.npm` (finding 13); after that fix it succeeded, slowly. The
  image now pre-installs it where the daemon points OpenCode, as the agent user.
- **~2s — fixed 1s polls/sleeps**: container-engine readiness, the
  setup-complete signal, and a sleep after launching the setup terminal. Now
  0.1s polls.
- **1.5–2s — Helix skills `git fetch`** from GitHub now runs alongside the
  repo clones.
- **Browser on the first turn** for DSH (0.1.7 upgrade) removed the shell /
  hand-written-CDP detours that made DSH spec-task turns 56–74s.

Still on the cold path:

- **Bot activation turn ~11s** (first LLM call at 4.5s, ready at 15.2s): the
  "Re-read AGENTS.md" briefing. Now the largest cold-start cost.
- **Chrome launch ~2.2s** on the first browser call (0.15s after): measured
  directly against `helix-chrome-devtools-mcp`.
- Model-side variance dominates individual turns: one DSH turn took 64.8s
  because a single call waited 35.5s for its first token with no prefix-cache
  hit on the shared node; every other call took ~1s.

## Fix status (PR from branch `eval/browser-support-bot`)

| Finding | Status |
|---|---|
| 0 ACP thread routing global | **Fixed**: routes keyed by (agent connection, thread); child sessions record their connection (`ExternalAgentID`); DB fallback limited to the connection owner's sessions; `thread_created` may only bind a session of the same owner. Live: two Goose bots used identical thread ids `20260924_1…4` concurrently, 6/6 answers, no cross-delivery. |
| — sync WebSocket unauthorized | **Fixed** (found while fixing 0): any authenticated user could connect as any `session_id`, receive its commands and inject events; a second connection also displaces the live sandbox's registration. Now owner / admin / runner only. Live: non-admin key → 403 for a foreign session; real sandboxes reconnect as owners. |
| 1 Goose never ran | **Fixed** (safe now that 0 is fixed). |
| 2 `zed_agent` bots lose instructions | Open — plan W1 #1. |
| 3 effort profiles | **Fixed**. |
| 4 new org via API | Open — plan W1 #4. |
| 5 `org bots chat` never-started bot | **Fixed**. |
| 6 `llm_calls.interaction_id` n/a | Open — plan W1 #3. |
| 7 DSH tool calls invisible | **Fixed** by the DSH 0.1.7 upgrade: the old `dsh-acp-demo` never emitted ACP `tool_call` updates; `dsh --profile acp` does. Live: 8 / 13 `tool_call` entries per DSH spec task, all `mcp__chrome-devtools__*`. |
| 8 chrome-devtools-mcp 0.25.0 | Per-project override documented; global bump open. |
| 10 headless browser | **Fixed**. |
| 11 MCP telemetry | **Fixed**. |
| 12 project skills for bots | **Fixed**. |
| 13 root-owned `~/.npm` | **Fixed** (build-time installs use a throwaway cache). |
| 14 TLS disabled | Open — plan W1 #7. |
| 15 WIP slots after stop | Open — plan W1 #5. |
| 16 archived task keeps running | **Fixed** in two places: an archived task is never started from the backlog / queues; and `stop-agent` / archive now end the in-flight turn. Before, the turn stayed `waiting`, auto-wake read it as "agent never connected" and rebooted the sandbox ~5s after the stop (reproduced twice; after the fix the sandboxes stayed down). |
| Spec-task first message has no browser | **Fixed in Zed** (`agent_servers`): stdio MCP servers are read from settings for `session/new` instead of the async runtime configuration. Live on image `538c72`: OpenCode spec tasks' first LLM call now carries 48 (headless) / 62 (desktop) tools incl. all 29 browser tools (was 10 / no browser), 0 shell fallbacks, turns 20–27 s (was 56 s headless). DeepSeek Harness is fixed separately, below. |
| DSH first message has no browser | **Fixed** by upgrading DSH to 0.1.7 (`dsh --profile acp` + `desktop/shared/dsh/helix.patch.yml`). MCP servers now arrive in ACP `session/new` like every other harness, and dsh connects and lists them before it answers. Live, headless spec tasks: the first LLM call carries 61 tools incl. all 29 browser tools (was 14, no browser); q1 / q3 answered in 31 / 39 s total, 17 / 25 s turns (was 68–74 s total). |
| Headless sessions configured a dead `helix-desktop` MCP server | **Fixed**: the headless bridge has no `/mcp`, so it 404'd. OpenCode silently dropped it; dsh (which fails `session/new` if any MCP server fails) could not start at all. Now only configured for desktop sessions. |
| ACP `session/new` failure hung the turn until timeout | **Fixed in Zed**: connect / `session/new` failures for ACP agents are reported as `chat_response_error` (only the native agent did before). |
