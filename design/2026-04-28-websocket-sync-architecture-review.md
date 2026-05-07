# Zed ↔ Helix sync: architecture review

**Date:** 2026-04-28
**Audience:** staff eng + you, reading on a phone while walking

---

## TL;DR

We built a streaming event router with 5 in-memory maps, 130 mutex sections, a 4000-line file on the Helix side, a 2000-line state machine on the Zed side, and a periodic worker whose job is to apologise for an upstream protocol bug. It does work, but every new feature is one careless write away from a routing bug, and the bugs are subtle: messages go to the wrong interaction, the UI appears done while the agent is still thinking, the queue retries something already in flight.

The honest read: **we are routing events through correlation IDs that were never reliable in the first place**. The wrapper buffers events with stale `request_id`s and we built increasingly clever fallback logic to compensate. Each fallback is one more thing that can go wrong.

The single highest-leverage simplification is to **stop routing by `request_id`**. Two ways to do it: make Zed authoritative and ship snapshots (cheap, mostly Helix-side), or move the ACP wrapper into Helix (deep, but eliminates the bug class).

---

## The shape of the problem

ACP (Agent Client Protocol) assumes one trigger for agent activity: the user pressing send. The protocol has `session/prompt` (request) and `session/update` (notification, scoped *during* the prompt). It has no first-class verb for "the agent has news that didn't come from a user prompt".

Modern Claude Code violates that assumption constantly. Background bash, hooks firing, subagents finishing, MCP servers emitting `tools/list_changed`, compaction completing — none of these are downstream of a user prompt, but they all need to surface to the user.

The `claude-agent-acp` wrapper handles the impedance mismatch by **buffering unprompted events on its outbound channel and flushing them when the next `session/prompt` arrives**. The flushed events carry the *previous* turn's `request_id`. Helix receives them, looks up the request_id, and routes to whatever interaction was bound to it.

Whatever was bound to it might be:

- The right interaction (rare, only true for direct in-turn responses).
- An interaction that's already complete (the request_id was consumed long ago).
- A *different* interaction that grabbed the stale id when our streaming context greedily rebinds (PR #2311, bug 3).
- Nothing at all (API restart cleared the maps).

We have separate code paths for each. They mostly work. They don't always.

The upstream is `agentclientprotocol/agent-client-protocol#554` — a request for a turn-complete signal that would let the wrapper stop buffering. It was filed early March, no patch yet. We should plan for it not landing this quarter.

---

## What we have today

Three layers:

**Zed crate** (`crates/external_websocket_sync`, 2003 lines in `thread_service.rs` alone): six global thread registries, a load-serialisation lock, a 100ms throttle on outbound `message_added`, an "external originated entries" set so Helix-sent messages don't echo back, a "persistent subscriptions" set so we don't re-subscribe twice, an "agent session map" because Zed thread IDs and Claude session IDs live in different namespaces, and a "thread keep-alive" map (separate from the registry) so dropped UI panels don't garbage-collect threads we still need.

**Helix WebSocket handler** (`api/pkg/server/websocket_external_agent_sync.go`, 3982 lines): five concurrent maps under a single shared mutex —

1. `contextMappings`: `acp_thread_id` → `helix_session_id`
2. `requestToSessionMapping`: `request_id` → `helix_session_id`
3. `requestToInteractionMapping`: `request_id` → `interaction_id` (or `""` consumed sentinel)
4. `interactionToPromptMapping`: `interaction_id` → `prompt_id`
5. `requestToCommenterMapping`: `request_id` → `user_id`

Plus per-session `streamingContexts` with throttled DB writes (5s) and frontend publishes (50ms), an `accumulator` carrying `message_id` ↔ content state across calls, and a `dirty` flag with a `flushTimer`.

**Auto-wake worker** (`auto_wake_stuck_interactions.go`, 412 lines): every ~10s, scans for interactions stuck in `state=waiting` for >30s, re-sends the original prompt with a fresh `request_id`, capped at 2 retries. Its purpose statement says the quiet part out loud: *expected lifetime, months, until the upstream protocol gets a turn-complete signal*.

The frontend on top of all this is comparatively simple — a polling prompt-history hook + an optimistic queue UI — but it's still ~1500 lines because it has to model the same routing states.

---

## Complexity inventory

Things that exist *only* because the routing model is fragile:

- Consumed-mapping sentinel (the `""` in `requestToInteractionMapping`) — to drop stale completions.
- "Most recent waiting interaction" fallback — to recover when maps are empty, including normal API restart, recovery from agent crashes, and now-defended against by sentinel checks.
- `interactionToPromptMapping` — added because dispatch failures used to leave queued prompts orphaned (PR #2311 bug 1).
- DB rebuild of `contextMappings` from `session.Metadata.ZedThreadID` — restart recovery for the routing key Zed cares about.
- DB rebuild of `requestToInteractionMapping` from "find latest waiting" — restart recovery for the routing key Helix cares about.
- Auto-wake worker re-sending stuck prompts — wrapper-buffer mitigation.
- `MarkPromptAsCrashed` + Restart button (just added) — circuit-breaker for crashed wrapper retry loops.
- "Already complete" early-return in `handleMessageCompleted` — defends against double-completion from the rebinding bug.
- Streaming-context transition detection — handles concurrent turns racing against each other.
- `RequeueBouncedPrompt` — recovers when `message_completed` arrives empty (turn was bounced).
- `EXTERNAL_ORIGINATED_ENTRIES` on the Zed side — prevents Helix's own messages from echoing back.
- `THREAD_REQUEST_MAP` on the Zed side — tracks "current" request_id per thread because the original might be stale.
- `THREAD_LOAD_IN_PROGRESS` lock — serialises thread loads because race conditions.
- `unregister_thread_if_matches` (vs plain `unregister_thread`) — race-safe variant added after a UI panel transition clobbered a fresh thread.

**Each one was correct at the time it was added.** Each defends against a real failure mode that produced a real user-visible bug. The aggregate is what's failing now.

---

## Why this keeps producing bugs

Three structural reasons:

**1. We've encoded a multi-turn system on top of a single-turn protocol.** ACP says one prompt = one turn. We let users queue, we let Zed initiate, we let comments dispatch from a third path, and all three can be in flight against the same `acp_thread_id`. The protocol has no way to express that, so we tag each one with a `request_id` and hope the wrapper propagates it correctly. It doesn't, and we keep adding mitigations.

**2. The state of "where a turn is" lives in too many places.** A single in-flight turn has presence in: the prompt-history queue (status `sending`), the interactions table (state `waiting`), `requestToInteractionMapping` (`req → int`), `interactionToPromptMapping` (`int → prompt`), `streamingContexts[session]` (live accumulator), and Zed's `THREAD_REQUEST_MAP` (`thread → req`). They drift. Any drift looks like a bug.

**3. We treat events as the source of truth.** If we lose an event, the state is permanently wrong. So we built recovery for every possible miss: WebSocket reconnect → `pickupWaitingInteraction`; map cleared → DB fallback; agent buffers events → auto-wake worker; agent crashes → Restart button. The list grows monotonically because every new failure mode needs its own recovery code.

This is the classic event-sourcing-without-the-log mistake. We have all the cost of caring about every event and none of the benefit of being able to replay.

---

## Five alternatives

### Option 1 — State sync, not event sync

Stop trying to route events through correlation IDs. Treat the agent thread as a state object and sync it.

**How it would work.** Zed (or the wrapper, doesn't matter) computes a content hash of the thread state — list of messages, each with role/content/tool calls/status. On any change, push the new state to Helix as a snapshot, or as a delta from the previous version. Helix stores it. The frontend renders it.

**What goes away.** All five maps. The "most recent waiting" fallback. The consumed sentinel. The auto-wake worker's reason for existing (if the wrapper buffers events, the eventual flush still produces a state change, which still gets snapshotted). `interactionToPromptMapping`. The double-completion guard. `THREAD_REQUEST_MAP`. The streaming-context transition detector.

**What stays.** Some throttling (don't snapshot on every token). A streaming endpoint for low-latency tokens (we want the typing-feel). Reconnect-time "send me the current state".

**Cost.** Medium-high. Helix becomes a passive replica of thread state. Frontend has to render from the snapshot rather than from incrementally-mutated interactions. Token streaming and snapshot-on-completion are two different paths; we'd want a small protocol for "delta of latest message" so streaming feels responsive. Probably 2–3 weeks if we're disciplined.

**Honesty.** This doesn't fix the wrapper buffering events. But it makes the bug invisible, because we're not trying to bind events to interactions — the state just settles to whatever the agent actually did. The user sees the right end-state; they may see slightly delayed streaming during buffer flushes. That's a much smaller pathology than "your message went to the wrong turn".

**My pick if we want a single move.**

---

### Option 2 — Move the ACP wrapper into Helix

Today: Zed → ACP wrapper (in desktop container) → Claude. Helix sits next to Zed talking WebSocket.

Proposed: Helix → ACP wrapper (in Helix process or sidecar) → Claude. Zed becomes a thin presentation client, talking to Helix only.

**What goes away.** The entire WebSocket sync protocol between Zed and Helix. We become the ACP client; we own the request/response loop; we know exactly when each turn ends because we made the call. Wrapper buffer flushes are visible to us at source, not after they've travelled through Zed.

**What stays.** Zed still needs to render thread state, including streaming. So we still need a Helix → Zed channel. But it's pure presentation: "here's the state of thread X, render it". No correlation IDs. No completion events as routing primitives. No `request_id` at all on the wire to Zed.

**Cost.** High. Big architectural change. We have to replicate, in Helix or at least in some Helix-controlled process, whatever the wrapper provides for tool execution (file edits, terminal, MCP). We probably ship `claude-agent-acp` as a binary that Helix execs, which is structurally what the desktop container does today — so really it's "move the same binary to a different host".

**Real benefit.** This is the only option that fixes the bug class at root. We stop being a subscriber to a buffering process; we become the buffering process's parent. We can implement the missing turn-complete semantics ourselves, instead of begging upstream to add `#554`.

**Honesty.** This is a quarter of work, not a sprint. Worth it if Claude Code is going to be a foundational piece of Helix for years.

---

### Option 3 — One thread = one in-flight turn, enforce it

Smallest possible change that kills most of the bugs.

**The rule.** A given `acp_thread_id` can have *at most one* `interaction.state=waiting` at a time. If a new message arrives while one is in flight, it goes to a per-thread queue (we already have one, in `prompt_history_entries`). Helix dispatches the next prompt only after `message_completed` lands.

**What goes away.** All the routing complexity around "which of the in-flight turns is this completion for". `requestToInteractionMapping` collapses to "current pending request for this thread". The "became busy" check becomes the *normal* path, not an error path. Stale completions become trivially detectable: they reference a request_id that's not the current one.

**What stays.** Most of the streaming code, because tokens still need to land somewhere. The Zed-side thread machinery, mostly. The auto-wake worker (still useful as a stuck-detector).

**Cost.** Low. Mostly a discipline change in how we dispatch and a simplified routing layer in Helix. Maybe a week.

**Limitation.** We lose interrupt semantics ("Cmd-Enter while a turn is running"). We could keep an interrupt as a special-case command that doesn't go through the queue, but that's adding back complexity. Honestly, interrupts are rare and clunky today; I'd be okay losing them in v1 and adding back later if users miss them.

**Honesty.** This doesn't solve the wrapper-buffer problem at root, but it does trivialise the routing problem the buffer creates. Combined with the snapshot model from Option 1, it'd be very clean.

---

### Option 4 — Per-thread WebSocket

Today we have one WebSocket per `helix_session_id` and multiplex multiple threads (and request types) over it. That's why we need `acp_thread_id` in every event payload — to demultiplex.

**Alternative.** One WebSocket per `acp_thread_id`. The connection itself *is* the routing key. Every event on it belongs to that thread. No `acp_thread_id` field needed in the payloads. Multiple threads in a session = multiple connections.

**What goes away.** `contextMappings`. The "is this for the right thread?" guards. The "thread not found, scan all threads" recovery paths.

**What stays.** Most of `request_id` logic, because per-thread we still need to bind which interaction. So this is a partial fix.

**Cost.** Medium. Need connection lifecycle per thread; need pool management; need reconnect-per-thread. Probably 1–2 weeks. Possibly worse than the current setup if users have many parallel threads.

**Honesty.** Moves the pain rather than fixes it. Probably not worth doing on its own.

---

### Option 5 — Append-only log per thread, sync by sequence number

Treat each thread as a sequence of immutable events. Both sides agree: `(thread_id, seq) → event`. To sync, you say "send me everything since seq N". To resume after a disconnect, you say "send me everything since the last seq I saw".

**What goes away.** All in-memory routing state. Reconnects become trivial (just resync). Restart recovery becomes trivial (read your last seq from DB, request from there). The whole class of "did we lose this event" becomes "is there a gap in the sequence".

**What stays.** Streaming throttling (events still arrive at agent rate). The accumulator (still need to fold token chunks into a single message for display). Some metadata sync (titles, statuses).

**Cost.** Medium-high. Need a real append-only log on both sides; need agreed-upon ordering; need the wrapper to actually emit events with monotonic sequence numbers (which it doesn't today). This is mostly an upstream ask, similar to `#554`.

**Honesty.** This is the textbook clean answer for distributed event sync. It's also the answer that requires the most cooperation from upstream. If we get to do clean-slate, this. In the world we live in, probably not.

---

## What I'd actually do

Stage this:

**Now (1 week):** Adopt Option 3 — *one thread, one in-flight turn*. Enforce in `sendQueuedPromptToSession` and `sendChatMessageToZedExternalAgent`. This kills the "concurrent turns racing" subclass of bugs immediately and shrinks the surface area we have to defend with maps. Auto-wake worker stays for now. We keep the existing protocol, just stop using its concurrency.

**Next (2–3 weeks):** Move toward Option 1 — *state sync*. Add a `thread_snapshot` event that carries the full thread state at the end of every turn. Frontend prefers the snapshot when it arrives, falling back to the streamed-message view otherwise. This is purely additive: existing routing keeps working while we build confidence. Once snapshots are reliable, **delete the routing maps entirely** and rebuild Helix's view from snapshots. The auto-wake worker can be deleted too — if the wrapper eventually flushes, we get a snapshot, we render it.

**Maybe (next quarter):** Option 2 — *bring the wrapper in-house*. Only worth it if (a) we're going to ship our own Claude Code shell anyway for non-Helix reasons, or (b) `#554` clearly isn't landing and we're permanently stuck mitigating. Don't pre-commit; revisit when state-sync is in production.

---

## What it costs to do nothing

A bug a week, on average, in this area. We've shipped three in PR #2311 alone (no-WS false fail, agent-crash retry loop, stale request_id rebind). Each one is hours to diagnose because it requires reading 4000-line files and tracing five interacting maps. Each fix adds another defensive check.

The WebSocket sync code is now the part of Helix where I'm most afraid to merge. That's a smell. The fix is structural — no amount of additional defensive code in the current model will make the bugs stop. Each fix increases the chance that the *next* edit interacts badly with one of the defences.

Six months from now, if we don't do something, the file will be 6000 lines, there will be seven maps, and someone will write a worker whose job is to apologise for the auto-wake worker.

---

## One thing I want you to push back on

I've been assuming we can't influence the wrapper. We can — we vendor `claude-agent-acp`. If we shipped a wrapper patch that emitted a turn-complete signal (proposed in `#554`), 80% of the routing complexity collapses without any of these architectural changes. It's the smallest possible move that also removes the most bugs. Worth a day of someone reading the wrapper source to see if it's actually feasible.

If the answer is "yes, easy" — do that first, then come back and decide whether the bigger refactor still makes sense.

If the answer is "the wrapper architecture won't allow it" — Option 1.

---

*References: design docs `2026-04-25-zed-claude-async-event-flush-on-user-input.md` (the canonical analysis of the upstream gap), `2026-04-16-lost-responses-race-condition.md` (the most-recent-Waiting fallback bug), `2026-04-28-stale-request-id-rebind-loses-zed-updates.md` (the stale-rebind bug fixed today). Source: `api/pkg/server/websocket_external_agent_sync.go:1-3982`, `api/pkg/server/auto_wake_stuck_interactions.go:1-412`, `crates/external_websocket_sync/src/thread_service.rs:1-2003`.*
