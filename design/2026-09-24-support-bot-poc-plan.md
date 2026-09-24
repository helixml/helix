# Support bot PoC — plan

**Date:** 2026-09-24
**Evidence:** `design/2026-09-24-browser-support-bot-evals.md` (all numbers below
come from it unless marked *estimate*).

## Goal

A support bot that answers questions about one of ~30,000 end customers by
reading several legacy web systems whose APIs are unusable:

- simple asks (balance, payment history, status): **< 10 s**
- complex asks (multi-system, aggregation): allowed to take longer
- nobody knows in advance which customer will ask

## What we know

| Fact | Number |
|---|---|
| Accuracy on the 12-question mock set (OpenCode / DSH + GLM, playbook) | 12/12 |
| Warm, logged-in bot, simple lookup | 8–9 s, 3–5 LLM calls |
| First visit to a system (login dance) | +10–15 s |
| Warm headless bot footprint | 833 MiB RSS, 0.6 % CPU idle |
| `docker pause` → `unpause`, then answer | 0.09 s resume; answers in 7.9–8.2 s after 90 s / 10 min frozen |
| Cold headless bot → ready | ~20 s (11 s to first LLM call + 9 s activation turn) |
| Spec task per question | 73–75 s (launch every time) — not viable |
| Per-call fixed prompt | 17–22k tokens (~1 s TTFT per call even when cached) |
| Playbook / skills | −34…−38 % time and tokens |

Startup is off the critical path once a bot is warm; the critical path is
**login** and **the number of LLM round trips**.

## Architecture decisions

1. **Isolation boundary = the customer company (Helix org), not the end
   customer.** One warm pool of identical support bots per company, sized by
   concurrent conversations (*estimate*: 2–15 at peak for 30k customers at
   1–10 % daily contact), never per end customer.
2. **End-customer identity comes from the front end, not the chat.** The
   conversation carries a trusted `customer_id`; lookups are skill scripts
   that read it from context, so the model chooses *which* lookup, never
   *whose* data. Hard enforcement, per the helix-org rule for high-cost
   violations. Output check: every account id in the answer must belong to
   that customer.
3. **Prefetch on "chat opened".** Fetch the customer's summary (balance, last
   N payments, open tickets) while they type; simple asks answer from it in
   one LLM call (*estimate* 2–4 s).
4. **Scripted skills for top intents** (balance, payment history, ticket
   status): one tool call against the logged-in browser (`evaluate_script` +
   same-origin `fetch`) instead of 4–10 navigation steps (*estimate* 3–5 s).
   Free browsing remains the fallback for anything else.
5. **Warm tiers per bot**: active → frozen (`docker pause`) after N idle
   minutes → stopped overnight; pre-warm before business hours. Built into the
   existing idle checker (`external-agent/idle_checker.go`), no new service.
6. **Credentials** as bot secrets fetched with `get_secret` (free: +2 %
   time); service accounts per system; login during warm-up + keepalive.
7. **Runtime**: headless sandbox, OpenCode + GLM (tool calls visible for the
   audit trail; DSH is ~10 % faster but its tool calls are invisible until
   fixed), project-level `chrome-devtools` override on 1.10.1 with irrelevant
   tool categories off (−23 % time, −25 % tokens).

## Workstreams

### W0 — Correctness and safety fixes (this PR)

Done here, see the PR: ACP thread routing scoped per connection + sync
WebSocket authorization; Goose actually runs Goose; stdio MCP servers (the
browser) reach the first ACP session (Zed); headless Chrome; MCP telemetry
off; project skills linked for bots; effort profiles for the self-hosted
models; archived tasks never start from the queue; `org bots chat` starts a
never-started bot; npm cache ownership.

### W1 — Remaining bugs (small, independent)

| # | Bug | Fix direction |
|---|---|---|
| 1 | `zed_agent` bots lose their instructions on every new thread | Zed fork: treat `$ZED_WORK_DIR/AGENTS.md` as a rules file for the native agent, or Helix prefixes the re-read nudge when a turn starts a new thread. Needs a decision. |
| 2 | DeepSeek Harness tool calls invisible in Helix | Trace the ACP `tool_call` updates DSH emits vs what `external_websocket_sync` forwards. |
| 3 | `llm_calls.interaction_id` is `n/a` for bot sessions | Thread the interaction id through the proxy request context. |
| 4 | New org via API: wallet not found → no subscription → 5-min "agent not ready" | Create the wallet with the org; surface billing refusal as an immediate error. |
| 5 | Finished spec tasks keep implementation WIP slots after `stop-agent` | Decide whether stop should release the slot. |
| 6 | Browser hang → agent spends the full turn writing its own CDP clients | Skill rule (added); plus a per-turn wall-clock budget in Helix. |
| 7 | TLS verification disabled image-wide | Trust customer CAs (`NODE_EXTRA_CA_CERTS`, system store) instead. |

### W2 — Fast path (the < 10 s goal)

1. Trusted conversation context: pass `customer_id` (and conversation id)
   from the caller through `/sessions/chat` into the bot's environment/tools.
2. Scripted lookup skill format: `SKILL.md` + `scripts/<intent>.js` executed
   via `evaluate_script` in the logged-in tab; the script reads `customer_id`
   from context, not from the model.
3. Prefetch hook: on conversation start, run the summary scripts and put the
   result in the first turn's context.
4. Warm-up turn that logs into every system; keepalive that re-logs on
   redirect-to-login.
5. Trim the fixed prompt for support bots: no Helix org tools they don't use,
   the 1.10.1 browser tool set, short core prompt.
6. **Exit criterion:** on the mock systems, p90 < 10 s for balance / payment
   history / ticket status on a warm bot, 100 % correct, zero cross-customer
   reads under a prompt-injection test set.

### W3 — Pool and warm tiers

1. Bot replicas: N identical bots per support role; dispatcher in the Helix
   API assigns a conversation to an idle replica (clear thread, wipe scratch,
   keep service-account login) and queues when all are busy.
2. Freeze tier in the idle checker (`docker pause`/`unpause` via Hydra), with
   pre-warm schedule and scale-to-zero overnight.
3. Measure: freezes of hours, host memory ceiling (bots per host), whether a
   frozen sandbox is still metered.
4. **Exit criterion:** 20 concurrent conversations on one host, p90 first
   answer < 10 s with the pool warm, < 30 s when a replica must cold-start.

### W4 — Cold path (only hit on bursts / first use)

1. Skip or shorten the 9 s activation turn for support bots.
2. Spawn the harness + MCP servers at container start, not on the first chat.
3. Workspace setup (3–6 s): skip the skills `git fetch` when offline-pinned.
4. **Exit criterion:** cold headless bot → first answer < 20 s.

### W5 — Customer PoC readiness

1. Replace the mock with the customer's real systems (read-only service
   accounts), write one skill per system from a recorded session.
2. Per-customer network path (VPN / egress IP) — the operator egress
   allow-list is global today and must not be shared across companies.
3. Security review pack: authz fix, secrets model, TLS (W1 #7), audit trail
   (tool calls visible), data retention of transcripts.
4. Eval harness (`evals/browser-support/`) on the real systems: golden
   questions per intent, run nightly.

## Open questions for the customer

1. Who talks to the bot: end customers directly, or support staff on their
   behalf?
2. Do the systems allow concurrent sessions for one service account, and what
   are their session timeouts and rate limits?
3. Are the systems internet-reachable or behind VPN / IP allow-lists?
4. Top 10 question types and their current handling time — defines the
   scripted intents and the success metric.
