# Helix ↔ Zed WebSocket sync: rewrite strategy in light of upstream ACP v2

**Date:** 2026-06-19
**Author:** Luke + Claude
**Supersedes/extends:** `design/2026-04-28-websocket-sync-architecture-review.md`
**Related issues:** #2641, #2642, #2643 (all symptoms of the root cause below)

---

## TL;DR

The Helix WebSocket sync server (`api/pkg/server/websocket_external_agent_sync.go`,
~3982 lines, 5 correlation maps, 130 mutex sections, plus a 412-line auto-wake
worker) is a mess **for one root reason**: it reconstructs turn state by
correlating `request_id`s that the `claude-agent-acp` wrapper hands us stale.
The April-28 review already concluded the fix is "stop routing by `request_id`."

Upstream ACP is now fixing exactly this gap themselves, in their **v2 proposal**
(led by Ben Brandt / @benbrandt — effectively the protocol lead). The v2 "New
Prompt Lifecycle" RFD makes the agent authoritative: `session/prompt` returns on
*accept* (not turn-end), the agent may send `session/update` at any time
(including agent-initiated / background updates), and an explicit `state_update`
notification carries `running` / `idle` (+`stopReason`) / `requires_action`. A
`user_message` ack notification fixes the "message bounced" class.

**Decision: do a targeted refactor along the v2 grain now — NOT a big-bang
rewrite, NOT "wait for v2."**

- Big-bang rewrite now is wasteful: the bulk/complexity is the `request_id`
  correlation machinery, which v2 deletes. Rebuilding an elaborate correlation
  engine = throwaway work.
- "Wait for v2" is a fantasy timeline: v2 is 3 days old, `unstable`, no
  stabilization date, and **implemented by no one** (not upstream Zed, not our
  Zed fork, not the wrapper). Meanwhile #2641/2642/2643 hurt in production now.

---

## The root cause (recap from April-28)

ACP v1 assumes exactly one trigger for agent activity: the user pressing send.
It has `session/prompt` (request) and `session/update` (notifications *during*
the prompt). It has **no first-class verb for "the agent has news that didn't
come from a user prompt"** — background bash, hooks, subagents finishing, MCP
`tools/list_changed`, compaction.

The `claude-agent-acp` wrapper papers over this by **buffering unprompted events
and flushing them on the next `session/prompt`**, carrying the *previous* turn's
`request_id`. Helix routes by that stale id → wrong/already-complete/nonexistent
interaction. Every fallback path we added is one more thing that breaks.

The three live issues are all this:
- **#2641** — stale `api` IP in container `/etc/hosts` after API restart → WS
  reconnect loops forever. (Adjacent infra bug, same incident class.)
- **#2642** — chat path sends prompt as `chat_message` with `role:"user"`; Zed
  drops it as an echo. (Dual delivery paths: chat vs queue.)
- **#2643** — reused long-lived thread after restart: `message_added` carries no
  `request_id`, can't bind to the waiting interaction → reply discarded →
  "empty response." (The core correlation failure.)

---

## What upstream is doing (verified 2026-06-19)

Repo: `github.com/agentclientprotocol/agent-client-protocol`. They have a real
RFD process (`docs/rfds/`, Draft → Preview → Completed).

- **v2 tracking RFD** (`docs/rfds/v2/overview.mdx`, @benbrandt): a batch of
  breaking changes that need core redesigns. Includes the prompt-lifecycle
  change below, whole-message updates, tool-call upserts, capability cleanup.
- **v2 New Prompt Lifecycle RFD** (`docs/rfds/v2/prompt.mdx`, @benbrandt) — the
  one that matters to us:
  - `session/prompt` responds on **accept**, not turn-end.
  - Agent may send `session/update` **at any time**, including initiating an
    interaction before any user prompt ("increasingly important for background
    tasks").
  - New **`state_update`** notification: `running` / `idle` (+ `stopReason`,
    `usage`) / `requires_action`.
  - New **`user_message`** ack notification (agent echoes accepted user message
    with the agent-owned `messageId`) → multi-client consistency; kills the
    #2642 drop class.
  - Status: **merged as `unstable-v2`** (PR #1445, 2026-06-16). Schema-only.
    Can only *stabilize* under protocol v2. No date. @benbrandt's plan: v1
    opt-in "future-flag" capability or a v2 preview/beta flow.
- **#554** (turn-complete signal) — still open; the v2 `state_update: idle` is
  the real answer. **#644** (v1 `turn_complete` RFD) — stalled Draft since March.
- **#533** (Multi-Client Session Attach) — open, active; controller/observer via
  a proxy. Close to our worker-desktop-plus-other-clients topology; track it.

### Does Zed support unstable v2? No (verified against our fork).

- Our fork (`AhegaoBurger/zed`, helixml `main`) pins
  `agent-client-protocol = "=0.14.0"` with `features = ["unstable"]`. v0.14.0
  (tagged 2026-06-18) **does** include #1445, so the v2 types (`src/v2/`,
  `state_update`, etc.) are compiled into our dependency graph.
- But Zed's code speaks **v1 only**: `MINIMUM_SUPPORTED_VERSION =
  ProtocolVersion::V1`, every `InitializeRequest::new(ProtocolVersion::V1)`
  (`crates/agent_servers/src/acp.rs`). Repo-wide grep for any v2 / `state_update`
  ACP usage = empty. The `unstable` flag is on for *other* v1-unstable features
  (elicitation, NES), not the v2 lifecycle.
- Our `external_websocket_sync` crate is v1-only too.

So the v2 lifecycle is **defined in the crate, implemented by no one** — on
either side of the connection or in the wrapper.

---

## Decision: build to the seam

**Rewrite the parts where v1 and v2 agree. Leave thin and swappable the parts
where they differ.**

### v1 and v2 AGREE → safe to rebuild now (also fixes the 3 issues):

1. **Identity = `acp_thread_id`, not `request_id`.** Re-key routing on the
   stable thread id. Kills the #2643 correlation class. Highest leverage.
2. **One delivery path, not two.** Collapse the chat-vs-queue split that causes
   #2642 (`role:"user"` drop). v2 has one path.
3. **Explicit per-interaction turn-state machine** (waiting → running →
   idle/requires_action), derived from the events we currently get. This is the
   internal shape v2's `state_update` feeds directly.

### v1 and v2 DIFFER → keep thin/swappable, do NOT reinvent:

4. **The *source* of state transitions.** Today: inferred from
   `message_added`/`message_completed`. Under v2: arrives as `state_update`.
   Funnel all transitions through **one chokepoint function** so swapping
   inference → notification later is localized, not a re-architecture.
5. **Wire notification types & multi-client attach (#533) semantics.** Don't
   hand-roll our own versions — that's the throwaway work.

### Sequencing
- Delete the auto-wake worker (`auto_wake_stuck_interactions.go`) **only after**
  the turn-state machine is reliable — it's a symptom-masker v2 deletes entirely,
  but it's load-bearing until our state is trustworthy.
- Budget: a focused **2–3 week refactor**, not an open-ended rewrite.

### Strategic lever (back pocket, don't block on it)
We fork Zed and own the wrapper, so we *could* prototype `state_update`
end-to-end on our fork+wrapper ahead of upstream stabilization — the Rust types
are already behind the `unstable` flag we enable. If the refactor lands well, the
jump to real v2 is "wire the chokepoint to the new notification," not a rewrite.

---

## Why this is recorded here and not left in chat
Claude Code transcript retention defaulted to 30 days; the original April-28
design conversation had already been auto-deleted by the time we went looking
(2026-06-19). We found the April thread only because it had been written to a
design doc. `cleanupPeriodDays` is now 3650, but the durable record is *this
file*, not the transcript.
