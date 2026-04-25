# Zed ↔ Claude ACP: events accumulate while idle, only flush on next user input

**Date:** 2026-04-25
**Status:** Symptom understood, root cause is upstream protocol/agent behaviour
**Spec task that triggered investigation:** `spt_01kq1gc8rcahwx66kk3k24f1yc`
**Helix session:** `ses_01kq1gcbabjs4q8w8c6z384094`
**ACP thread UUID:** `08211da1-2856-4d49-972f-82afa393d5e3`
**Sandbox image:** `cb2694` (post-rebase ZED_COMMIT 9f0475c6c2 — *one short of* d7be64fad1 which contains the registry-clobber fix from 2026-04-24)

## Symptom (user-visible)

> The Zed thread seems to be dead. I send a message, I get nothing back. Until I do
> it a few times. Then it seems to "get through" (as in a queue) of notifications
> that had async been sent from e.g. background processes finishing in claude
> agent, notifying the agent. Only after I send enough messages, things sync up
> and responses start streaming again.

Concretely on this session:

- Several minutes of apparent silence after a long-running turn finishes.
- User sends a short message ("what are you doing"). Nothing visibly happens.
- User sends another ("??"). Suddenly a brief response appears.
- User sends another ("fucking fix it"). Now the agent is actually responsive again.

## Evidence (Helix-side)

The Helix-side `/api/v1/sessions/.../interactions` snapshot for this session:

```
int_01kq1m92z5h62hpmgddpy6trkj  06:12:22  state=waiting  resp_len=0       prompt="what are you doing"
int_01kq1m9vpyqmhmt1dx6vzy6q83  06:12:47  state=complete resp_len=1234    prompt="??"
int_01kq1mabddcwacm11y5s33xz1r  06:13:04  state=complete resp_len=14710   prompt="fucking fix it"
```

The first interaction *never gets a `response_message`* and stays in `waiting`. The
two follow-up messages get responses fine.

What the API server actually saw on the WebSocket from Zed during the same window:

```
06:12:22  External agent added message  message_id=292  role=user        ← user msg from Zed
06:12:22  Created interaction for user message from Zed
          interaction_id=int_01kq1m92...

06:12:23  RECEIVED MESSAGE_COMPLETED
          data={"acp_thread_id":"08211da1-…","message_id":"0",
                "request_id":"req_fd0d65c7-34fb-42e3-91f4-c1bf40f5b12f"}
          ↑ STALE — req_fd0d65c7 was Helix's request_id for the spec-implementation
            chat_message at 05:11:41 and was already consumed by a message_completed
            at 05:23:38.

06:12:47  Created interaction for user message from Zed
          interaction_id=int_01kq1m9vpyqmhmt1dx6vzy6q83
06:12:48  External agent added message  message_id=294  role=assistant   ← FIRST chunks arrive
06:12:48  RECEIVED MESSAGE_COMPLETED
          data={"…","request_id":"int_01kq1hfw8dg3kjdhpym4wkd22d"}
          ↑ ALSO STALE — int_01kq1hfw8 is the request_id of a turn that already
            received message_completed at 05:27:13.
```

Two different stale `message_completed` events arrive in the same ~30 s window —
both in response to the user's new prompts, both naming `request_id`s that were
already settled hours earlier. After 06:13 the session catches up and responds
normally.

The same pattern repeats around 06:28–06:29 with the same stale
`request_id=int_01kq1hfw8…`, and again at 06:29:04 — Zed appears to be replaying
queued events whenever a fresh prompt nudges the loop.

## Why we think this is upstream

There is a confirmed protocol-level gap with several active upstream issues
describing the same shape of bug across different ACP clients:

### `agentclientprotocol/agent-client-protocol` #554 — "Add turn-complete signal for session_update notifications"

<https://github.com/agentclientprotocol/agent-client-protocol/issues/554>

> After a `prompt()` call returns, there is no protocol-level signal indicating
> that all `session_update` notifications for that turn have been delivered.
> Clients that accumulate state from these notifications must resort to
> heuristic delays (`asyncio.sleep`) to wait for in-flight notifications to
> arrive before reading the accumulated result.

Filed by `simonrosenberg` (OpenHands) on 2026-03-06, still open. **This is the
canonical upstream issue.** ACP does not guarantee that all `session_update`
events for a turn have been flushed by the time the prompt response returns.
A client (Zed) can correctly observe `stopReason=end_turn` while the server
still has pending updates buffered behind it.

### `agentclientprotocol/claude-agent-acp` #551 — "Multiple stop reasons from cancelled turns"

<https://github.com/agentclientprotocol/claude-agent-acp/issues/551>

After `session/cancel` returns `stopReason=cancelled`, the *next* `session/prompt`
returns `stopReason=end_turn` immediately with 0 input/output tokens — Claude
isn't actually processing the new prompt. A *third* `session/prompt` is needed to
get a real response. From CSRessel's wire trace:

```
→ session/cancel
← stopReason=cancelled
→ session/prompt "what have you finished?"        ← gets bounced
← stopReason=end_turn (inputTokens=0, outputTokens=0)
→ session/prompt "."                               ← finally lands
← (real chunks)
```

Different mechanism (Claude Code wrapper specifically), same user-facing
experience: messages "lost" until enough send to wake the loop.

### `zed-industries/zed` #54767 — "Agent UI doesn't recognize Claude Code stopped"

<https://github.com/zed-industries/zed/issues/54767>

Filed yesterday (2026-04-24) by `pepyakin`:

> Current behaviour:
> 1. The model finishes on the remote, but the Zed UI stays in the "thinking"
>    state and no "done" sound plays.
> 2. Press Stop and submit a new prompt.
> 3. The "done" sound for the previous turn plays the instant the model gets
>    you the first reply.

Exact same symptom in the OSS Zed app (no Helix involved): the previous turn's
completion event is delivered only when the next user input pokes the
connection. State `state:needs triage`.

### `zed-industries/zed` #51098 — "Thinking freezes, just stops.."

<https://github.com/zed-industries/zed/issues/51098>

Older (filed 2026-03-05 by `techker`) — same shape, with a Zed maintainer
confirming on the issue:

> "interesting: the problem you're describing seems similar to what users are
> reporting with the GitHub Copilot ACP integration."

i.e. the Zed team already suspects this is an ACP-layer pattern, not a
provider-specific bug. State `state:needs repro`, P2.

### `zed-industries/codex-acp` #186 — "session/cancel does not interrupt an active prompt after session/load on the same session"

<https://github.com/zed-industries/codex-acp/issues/186>

A different ACP server with a related session-lifecycle bug — `session/load`
on a session with an in-flight prompt leaves the prompt running and a later
`session/cancel` doesn't reach it. Same family of "the connection's view of
turn state and the actual turn state diverge" issues.

## Architectural root cause: ACP assumes one trigger; Claude has many

Pull back from the specific issues above and the shape of the problem becomes
hard to miss. **ACP's mental model has exactly one trigger for agent
activity: the user pressing send.** The protocol's verbs encode this
directly:

- Client → Agent: `session/prompt` (request)
- Agent → Client: `session/update` (notification, scoped *during* the prompt)
- Agent → Client: `session/prompt` response with `stopReason` (terminates the turn)

Everything the agent emits — text chunks, tool calls, plan updates, mode
changes, usage info — is conceptually downstream of the most recent
`session/prompt`. The notification channel inherits that lineage. There is
no first-class verb for "the agent has news that didn't arise from a user
prompt": no subscription, no long-poll, no always-on event channel.
Compare to LSP, which has `$/progress`, `window/workDoneProgress`,
`window/showMessage`, `telemetry/event` — explicit fire-and-forget channels
for unprompted server→client traffic. ACP's `session/update` is positioned as
a turn-scoped stream, and the spec doesn't carve out a "between turns" mode
for it.

That assumption breaks the moment the agent gains **non-user-initiated
triggers**, and Claude Code now has many:

- a backgrounded shell command (`run_in_background: true` on the Bash tool)
  finishes — output to deliver,
- a file change on disk fires a `PostToolUse`/`UserPromptSubmit`/`Stop` hook —
  reaction to surface,
- a subagent spawned via the `Task` tool finishes its work — result to flow
  back to the parent,
- compaction completes autonomously — available context just changed
  (`anthropics/claude-code` #52685 is literally this freezing),
- an MCP server emits `tools/list_changed` / `resources/updated` — agent's
  view of the world just changed,
- a Cowork / Ultraplan session running in the background hits a checkpoint,
- a skill loads — capabilities should now be visible.

Each of these is **the agent having something to say without the user having
said anything**. The wrapper has events the user needs to see and **no
protocol-legal place to put them**. So it's left with three bad options:

1. Buffer them on the outbound channel and hope the next `session/prompt`
   flushes the queue. *This is what we see in our wire trace* — stale
   `request_id`s arriving hours later, immediately after a fresh user
   message.
2. Send them anyway, tagged with the most recently completed turn's
   `request_id` — the client either drops them (data lost) or routes them
   to a freshly-created interaction (data corruption, which is the
   `request_id` desync cascade we keep fixing).
3. Mint a new `request_id` for late events with no corresponding prompt —
   the client has no entry in its `request_id → turn` map for it and the
   events fall on the floor.

The Claude wrapper picks roughly (1) + (2). claude-agent-acp #551
("multiple stop reasons from cancelled turns") and ACP-protocol #554
("add turn-complete signal") both exist because the wrapper authors are
dancing around the fact that the protocol doesn't give them a place to put
async state.

ACP does have hints of async-ish events — `usage_update`,
`current_mode_update`, `session_info_update` — but they're all bundled into
the same `session/update` notification, which inherits turn-scoped delivery
semantics. So there's a partial, un-formalised acknowledgement that some
events aren't really turn-bound, but no protocol mechanism to signal *which*
are which or *when* they should be delivered.

### Why this gets worse, not better

The dynamic across the layers is asymmetric:

| Layer                                          | Owner                  | Direction      |
| ---------------------------------------------- | ---------------------- | -------------- |
| `anthropics/claude-code`                       | Anthropic              | adding async features fast |
| `agentclientprotocol/claude-agent-acp`         | Zed Industries         | retrofit       |
| `agentclientprotocol/agent-client-protocol`    | cross-vendor governance | catch-up       |
| `zed-industries/zed`                           | Zed Industries         | consumer       |

**Anthropic evolves the agent's contract independently of the protocol it
happens to be wrapped in.** Each new async feature widens the gap. Background
bash, hooks, skills, Cowork are already in production; more are coming. The
wrapper and protocol layers can't fix it at their layer without retroactively
constraining what the agent is allowed to do — and they have no leverage to
do that.

Helix gets caught in the middle, because we're the ones whose users notice
when chat panels go quiet for minutes. None of the work in
`websocket_external_agent_sync.go` can fix it at the source — the events
have already been mangled by the time they reach our WebSocket. The most we
can do is detect and route around the breakage.

### What "fixing it properly" would actually require

A real fix needs an unparented event channel in the protocol — something the
agent can speak on without claiming the user authored a turn:

- a separate notification (`session/event` or `agent/notification`) that's
  not turn-scoped, with its own delivery semantics,
- *or* a turn lifecycle that explicitly distinguishes "stopReason emitted"
  from "turn fully closed and all updates flushed" (#554 asks for this
  weaker version),
- *or* explicit binding of every `session/update` to either a turn id or
  `null` (out-of-band) — so the wire format stops forcing the wrapper to
  pick a misleading `request_id`.

Once the protocol admits the agent can speak unprompted, all five upstream
issues (#554, #551, zed#54767, zed#51098, codex-acp#186) collapse into "use
the unprompted channel" instead of "find creative ways to attach unprompted
events to expired requests".

Until then, **every ACP server with non-trivial async behaviour will ship
the same family of bugs**, and every ACP client will rediscover the same
workarounds — heuristic timers, periodic kicks, stale-event filters. Which
is exactly what the upstream issue tracker shows (OpenHands, Zed, the
Copilot-via-ACP integration, JetBrains).

### Where the ACP team is on this (state of upstream RFDs)

The protocol team is **aware of the symptoms but has not yet articulated
the structural fix**. Snapshot as of 2026-04-25:

- **#554 ("Add turn-complete signal")** has movement.
  [`@benbrandt`](https://github.com/benbrandt) (agentclientprotocol member)
  responded: *"yes I think I would like to move the entire prompt lifecycle
  to be notification based entirely, which I think will help with this."*
  That's the right architectural direction — the prompt lifecycle as
  notifications rather than a request/response — but no concrete spec PR
  yet.
- **PR #644 ("docs(rfd): draft turn_complete signal for session/update
  sync")** by `stablegenius49`, opened in response to #554, proposes a
  `sessionUpdate: "turn_complete"` notification with `promptRequestId` and
  `stopReason`, capability-gated via `sessionCapabilities.turnComplete`.
  Still in `Draft` RFD status. **This is the only concrete protocol PR
  addressing the family of bugs in this doc.**
- **PR #392 ("RFD: Agent-to-Client Logging")** by `chazcb` is the closest
  existing proposal to a real unparented channel. Its motivation reads like
  it could have been written for our bug:
  > *Today, agents have limited ways to inform clients about status that
  > might impact their experience […] Neither [JSON-RPC errors nor
  > `session/update`] works when: there's no active JSON RPC request to
  > attach an error response to […] there's no session yet […] we don't
  > want to put diagnostics in chat history.*
  But the RFD scopes itself explicitly to **diagnostic logs**, not to
  user-visible content. So even if it lands it doesn't give the wrapper a
  legal home for "background bash output" or "subagent finished".
- **PR #484 ("docs(rfd): prompt queueing RFD")** by `SteffenDE` is
  circumstantial evidence the team knows the agent SDK has timing it
  doesn't expose: *"I tried to find a way to actually get the Claude Agent
  SDK to tell me when the queued message is inserted into the context, but
  it looks like there is no way."* Same shape of problem as ours — the
  wrapper doesn't get the signals it needs to give the client a clean view.
- **PR #865 ("RFD: Agent Status Update")** by `anaslimem` adds an
  `AgentStatusUpdate` variant for "thinking, reading, writing, waiting,
  idle". Helps the *thinking-vs-stuck* UX gap but is still a turn-scoped
  `session/update`.
- **#533 (RFD: Multi-Client Session Attach)** is adjacent — about letting
  multiple clients attach to a live session — but isn't structured as an
  unparented event channel.
- **#419 (Session Ready Signal RFD)** addresses session lifecycle, not
  per-event lifecycle.

The full list of currently-open Draft RFDs in `agentclientprotocol/agent-client-protocol`
spans: forking sessions, request cancellation, meta-field propagation, agent
telemetry export, agent extensions via ACP proxies, MCP-over-ACP, session
usage/context status, authentication methods, Rust SDK based on SACP,
logout, session delete, message id, deleted-file diff representation,
boolean config option, elicitation, next-edit-suggestions, additional
workspace roots, configurable LLM providers, streamable HTTP/WebSocket
transport. **None of these directly proposes "unparented agent→client
notifications for non-user-initiated triggers"** as a first-class concept.

### Net assessment

The protocol team is aware of the *symptoms* (#554's existence and `benbrandt`'s
comment confirm this) and has one in-flight PR (#644) for the weakest
version of a fix (a barrier signalling end-of-turn, not an unparented
channel). They are *not* yet treating "the agent has things to say without a
prompt" as a first-class protocol concept — every open RFD that touches the
agent→client direction is still scoped to either:

  - a single turn (`session/update`, AgentStatusUpdate, turn_complete), or
  - lifecycle metadata (Session Ready, attach/detach), or
  - a narrow domain like diagnostics (#392 Logging).

The conceptually-correct fix — admit unprompted notifications as a peer of
turn-scoped ones — does not yet exist as an RFD. Realistic timeline: even
the narrow turn_complete fix (#644) has to clear RFD review, ship in a
protocol release, ship in `claude-agent-acp`, and ship in Zed before users
see relief. **None of that helps with intentionally-unprompted events like
background bash output**, which is the harder half of the architectural
problem and is currently un-RFD'd.

The honest message for Helix users for the foreseeable future: the agent
will sometimes go quiet and need to be poked, and we can't fix it from the
Helix side alone.

## Working hypothesis

The agent (`@zed-industries/claude-agent-acp` 0.23.x) buffers
`session/update` notifications on its outbound JSON-RPC channel during a turn.
Because the protocol has no turn-complete signal (#554), there is no fence
that says "all updates for turn T are now flushed". Multiple things can leave
the buffer non-empty by the time the turn's response is sent:

- async tool completions emitted after the agent has already decided the turn
  is done,
- usage updates issued from background bookkeeping in the wrapper,
- replayed events on `session/load` that pre-date the new client's
  subscription.

These accumulate. Zed's outbound flush of these is gated on its own event
loop running, which it does eagerly while a prompt is active and lazily once
the turn has resolved. The next inbound `session/prompt` from the user kicks
the loop hard enough to drain the backlog, which is what we observe as a
"queue of stale notifications" landing all at once when the user sends a new
message.

This is also consistent with what we see Helix-side: stale `request_id`s
that were already consumed by a previous `message_completed` showing up again
in `RECEIVED MESSAGE_COMPLETED` events hours later, immediately after a fresh
user message.

We believe the post-rebase increase in severity is because the wider rebase
brought in the `AgentConnectionCache` dedup
(`ba7e97aea6` → `350de991de`), which makes a *single* underlying ACP
connection serve multiple `Entity<AcpThread>` instances. Buffered events on
that shared connection now have more places to be misrouted, so the user
hits the underlying ACP-layer issue more visibly.

## What this is *not*

- Not the registry-clobber bug fixed yesterday in
  `design/2026-04-24-acp-thread-entity-routing-after-restart.md` /
  helix#2278. That fix is in `d7be64fad1` and addresses a different
  failure (panel rebind clobbering Y's `THREAD_REGISTRY` entry on old-view
  drop). The session here was started from `9f0475c6c2`, *one commit
  earlier*, but even on `d7be64fad1` we'd expect this symptom to persist
  because the ACP-layer event buffering happens before the Helix-side
  bookkeeping ever sees it.

- Not the duplicate spec-approval race fixed by helix#2260. That race
  produced two real interactions; this one strands a single interaction
  in `waiting` until later activity flushes the agent's outbound buffer.

- Not Helix's prompt queue refusing to dispatch — `Created interaction for
  user message from Zed` is the user-typed-in-Zed path, not the queue path.
  The queue's busy/idle gating doesn't apply here.

## A worse failure mode: Zed-side silent drop of user messages

While reviewing this session with the user, a *third* failure pattern
surfaced that we hadn't accounted for. The user has a screenshot of Zed's
chat history showing a "carry on" prompt they typed during the stuck period.
Zed's UI displayed it inside the thread. **Helix never saw it.**

The complete list of `Created interaction for user message from Zed` events
for `ses_01kq1gcbabjs4q8w8c6z384094` over the four-hour window is:

```
05:23:39  i stopped the other build
06:12:22  what are you doing       ← stuck (waiting, never completed)
06:12:47  ??
06:13:04  fucking fix it
06:28:32  ?
06:29:04  i pushed a fix to main of our fork...
```

There is no "carry on" anywhere in the `interactions` table, in the
`prompt_history_entries` queue, or in the API logs for this session.
The keystroke reached Zed (we have the screenshot showing it in the local
thread) and never crossed the WebSocket sync boundary.

This is qualitatively worse than what the rest of this doc covers:

| Failure mode                    | What Helix sees                | Detectable? |
| ------------------------------- | ------------------------------ | ----------- |
| Stale events flushed late       | Stale `request_id`s replayed   | Yes (logs)  |
| Stuck `waiting` interaction     | `state=waiting`, `resp_len=0`  | Yes (DB)    |
| **Zed-side silent drop**        | **Nothing**                    | **No**      |

The other two failures land in the DB or the wire log eventually; we can
react to them. The silent-drop case has no Helix-side fingerprint at all —
the message never crosses the WebSocket, so there is no event to subscribe
to and no DB row to scan.

### Plausible Zed-side causes (in decreasing order of likelihood)

1. **The `unregister_thread` race we fixed in `d7be64fad1` is still biting**
   on this older binary. The session runs Zed `9f0475c6c2`, which is *one
   commit before* the fix. If the panel rebound to entity Y and the OLD
   ConversationView's `on_release` clobbered Y's registry entry plus the
   persistent-subscription flag, then `ensure_thread_subscription` was never
   re-armed on the live entity. `AcpThreadEvent::NewEntry` fires when the
   user types — but no subscription is listening, so no `MessageAdded` is
   sent over the WebSocket. This is the leading hypothesis precisely because
   the fix in `d7be64fad1` was for the symmetric outbound failure mode
   (assistant chunks not delivered) and the same race orphans the user-input
   listener too.
2. **Zed's outbound event loop is starved.** If the wrapper is unresponsive
   (`session/prompt` blocked, MCP child stuck), the outbound channel that
   carries `MessageAdded` events to Helix may queue events without flushing
   them. The local UI update happens in-process and is unaffected, so the
   user sees the message in Zed even though it's never sent.
3. **Subscription on the wrong entity** — variant of (1). Subscription
   wired up on entity X, user types in displayed entity Y, X never sees
   the `NewEntry`. Same effect as (1) without requiring an explicit unregister.

### What Helix can do about it

Effectively nothing from inside Helix's process. We have no signal to
detect on, so:

- The auto-wake mitigation proposed below is **partial coverage** — it
  helps the cases where Zed *did* relay the user message but the response
  never came (stuck `waiting`). It does *not* help the case where Zed
  silently dropped the user message in the first place.
- We *cannot* show a "your last keystroke may have been dropped" UI hint,
  because the only way Helix knows the user typed is via the same WebSocket
  event that's missing. There is no out-of-band channel by which the
  Zed UI tells Helix "I rendered a user message".

### Path forward

1. **Get the user onto a Zed binary that contains `d7be64fad1` (or later).**
   helix#2278 lands the `ZED_COMMIT` bump. New sessions started against the
   bumped image will exercise the registration fix. If the silent-drop
   pattern disappears for sessions on the new binary, that's strong
   evidence (1) was the cause and we're in much better shape.
2. **If it persists post-bump,** the failure is something else
   inside Zed's outbound dispatch — most likely a starvation issue on
   the WebSocket sender goroutine when the wrapper is wedged. That needs
   instrumentation inside `external_websocket_sync` to confirm (e.g.
   timestamp every `MessageAdded` send attempt and every channel-full
   delay). Out of scope for this doc until we have a reproducer on the
   newer binary.
3. **Open an upstream Zed issue *only* if it persists post-bump** — and
   only with a full repro (Zed log + Helix log + screenshot of the dropped
   message). The leading hypothesis (#1) is something we already fixed
   on our fork; a vanilla Zed issue would be premature.

## Investigation directions (not implemented)

In rough order of effort vs. likely payoff:

1. **Subscribe and reproduce on `agentclientprotocol/agent-client-protocol`
   #554.** That's the upstream fix that would actually close the class of
   bug. If a `turn_complete` notification is added to the protocol, both
   the Zed client and the claude-agent-acp wrapper will need releases that
   honour it; we should be ready to bump as soon as those land.

2. **Add a periodic "kick" on the Helix side when the agent is idle.**
   Send a no-op `session/cancel` (which the Claude wrapper handles
   gracefully even on idle sessions) every N seconds while the most recent
   interaction is in `state=waiting`. If the cancel reliably drains the
   buffer, we can close the user-visible gap without waiting for upstream.
   Risk: duplicate interrupt tokens and the
   "double-stop-reason" misbehaviour described in claude-agent-acp #551.

3. **Detect stale `request_id`s on the Helix side and treat them as
   no-ops.** When `RECEIVED MESSAGE_COMPLETED` arrives with a `request_id`
   we previously consumed, the current code falls back to "match the most
   recent waiting interaction" which is exactly the wrong thing — it
   marks a brand-new user message as already-completed. A literal
   "consumed → drop" behaviour (the existing `mappingConsumed` path at
   `websocket_external_agent_sync.go:2186-2210`) should be tightened so
   *any* re-arrival of a settled `request_id` is logged and dropped, even
   when fallback would otherwise match.

4. **Move every `request_id` into a per-turn nonce that includes both the
   Helix interaction id and a monotonic counter.** Any stale event from
   Zed's buffer would then carry an unambiguously-old id and be trivially
   discardable. This is a wire-format change that needs cooperation from
   the test server and the e2e harness.

5. **Open a Zed-side issue cross-linking #54767, #51098, ACP-protocol
   #554, and claude-agent-acp #551** with our wire trace and the
   helix-in-helix repro, so the Zed team has a single canonical reference
   the next time they triage.

## Mitigation users can apply now

- **Don't trust an apparently-idle thread after a long turn.** If the
  thread "feels dead" send a very small follow-up (`?`, `.`, `ok`) — that
  reliably flushes the buffer. The reply to your follow-up will land
  shortly after.

- **If a new user message has been sitting in `waiting` for more than a
  few seconds without any chunks arriving, send another short message.**
  Per claude-agent-acp #551 the *next* prompt is the one that's actually
  processed.

- **Avoid `session/cancel` mid-turn unless necessary.** The wrapper's
  cancel-then-prompt path is exactly where claude-agent-acp #551 bites,
  and it makes the next 1–2 messages effectively no-ops.

## Proposed Helix-side mitigation: auto-wake stuck waiting interactions

**Status: deferred — re-evaluate after the next Zed rebase lands and we can
test whether `d7be64fad1` reduces incidence on its own.**

The mitigation below targets the *detectable* failure mode (interaction
in `state=waiting`, no chunks ever published). It deliberately does *not*
attempt to address the silent-drop case described above — that one has
no Helix-side signal to act on.

### Behaviour

A goroutine in the API process scans, every ~10 s, for interactions
matching:

- `state = 'waiting'`
- `LENGTH(COALESCE(response_message, '')) = 0`
- `created < now() - interval '30 seconds'`
- `auto_wake_count < 2`
- `response_entries IS NULL` (no streaming entries published either)

For each match, send `"continue"` as a fresh chat message via
`sendChatMessageToExternalAgent`, with a brand-new `request_id`. The new
auto-sent interaction carries `auto_wake_count = N + 1` so the loop
retries at most twice per stuck interaction. After two retries the
interaction is marked as `state=error` with
`Error="Agent unresponsive after 2 auto-wake attempts; upstream ACP
buffering"`, and the queue stops re-feeding it.

### Why "continue" specifically

We considered four wake-up payloads:

| Payload     | Effect                                                                  |
| ----------- | ----------------------------------------------------------------------- |
| `"continue"`| Verb the model knows. Rough no-op when there's nothing to continue.     |
| `"."`       | Minimal context pollution but the model still says *something* back.    |
| Resend prompt | Idempotent in intent but agent may redo work (extra tools / API spend).|
| `session/cancel` | Triggers claude-agent-acp #551 — next 1-2 prompts get bounced.    |

`"continue"` wins on grossness vs. effectiveness. It is the same wake-up
the user's habit already produces (`"??"`, `"fix it"`, `"ok"`) — we are
automating a pattern users already do manually.

### UI surfacing

Auto-sent interactions render with a small inline badge — **"↻ Helix
auto-sent · upstream ACP buffering"** — and a tooltip linking to this
design doc. The user always sees what Helix did and why. The badge is
read off `auto_wake_count > 0` on the interaction record.

### Why this is a justified workaround

- It addresses the dominant *detectable* manifestation of the upstream
  bug, which is the one users complain about most.
- The implementation surface is entirely Helix-side: one DB column, one
  goroutine, one frontend badge. No Zed work, no wrapper work, no
  protocol work.
- The empirical observation (Helix user, 2026-04-25) that "sending enough
  messages always consumes the queue and re-syncs properly" rules out the
  perpetual-chase risk that worried us in earlier iterations of this
  proposal.
- The fix is honest: badged in UI, capped at two retries, gives up cleanly
  rather than looping. When the upstream protocol gap closes (#554, #644
  land), the workaround can be feature-flagged off without any data
  migration.

### Why this is *not* a real fix

- It does nothing for the silent-drop case (Zed never sends the user
  message). We are blind to that failure.
- It does nothing for intentionally-asynchronous events (background bash
  output, hook-triggered actions) — those need a real unparented channel
  in the protocol.
- It treats a structural protocol mismatch with a polling-and-retry loop.
  Every ACP server with non-trivial async behaviour will end up shipping
  some version of this same workaround until ACP grows the missing
  primitives.

### Critical: bypass the prompt queue, do *not* interrupt

The `prompt_history_handlers.go:203` busy check (*"is there a waiting
interaction? if so, defer"*) would deadlock the auto-wake against the very
interaction we're trying to wake. There are two ways to send around it,
and they are not equivalent:

| Approach                          | What happens                                                                            |
| --------------------------------- | --------------------------------------------------------------------------------------- |
| Queue path with `interrupt=true`  | Zed fires `session/cancel`, triggering claude-agent-acp #551 → "continue" itself gets bounced (`stopReason=end_turn`, 0 tokens). Both retries burn for nothing. **Worse than not auto-waking at all.** |
| **Direct `sendChatMessageToExternalAgent`, no interrupt** | Auto-wake skips the queue check entirely. The wrapper is in fact idle in our stuck scenario (it emitted `stopReason` for the prior turn — that's *why* Zed UI shows "done") and processes the "continue" normally. |

Use the second. Helix's `state=waiting` is bookkeeping that the wrapper
never sent a `message_completed`; it is *not* a claim that the wrapper
is mid-prompt. The busy check is reasoning correctly for the normal
user-prompt path and incorrectly for the auto-wake path; we sidestep
rather than relax the check.

Side effect: there will momentarily be *two* `state=waiting` interactions
for one session — the original (stuck) and the auto-wake (in flight). That
is acceptable. Whichever the wrapper's next `message_completed` matches
via `request_id` mapping is the one that resolves; the other either drains
on the same event tick (the wake worked, so the original turn's buffered
response flushes too) or hits the auto-wake retry cap and is marked
`state=error` after the second attempt.

### Implementation sketch

Single source file (`api/pkg/server/auto_wake_stuck_interactions.go`)
with one exported start-on-server-init function. Heavy comment at the
top of the file explaining:

1. Why this exists (the ACP turn-locked-events architectural mismatch).
2. The upstream issues being papered over (#554, #551, zed#54767).
3. The expected lifetime of the workaround (until upstream lands and
   downstream rolls out, then can be feature-flagged off).
4. The known coverage gap (silent-drop case, see "A worse failure mode"
   section of this doc).
5. **Why the auto-wake bypasses the queue and never sets `interrupt=true`**
   (see table above) — this is non-obvious from the surrounding code and
   future maintainers will be tempted to "fix" it by routing through the
   queue.

DB change: one new column, `interactions.auto_wake_count INTEGER NOT NULL
DEFAULT 0`. Frontend reads the column and renders the badge when
`> 0`. No migration needed beyond `AutoMigrate`.

Goroutine started from the same place as other periodic API tasks (e.g.
the spec-task orchestrator). Single instance per process — multi-replica
deployments need a Postgres advisory lock to avoid the same interaction
being woken by multiple replicas simultaneously, but that's a problem
for later (Helix is single-replica today).

### Implementation status

**Not implemented as of 2026-04-25.** Holding pending the next Zed
rebase test. If `d7be64fad1` reduces the incidence enough that the
remaining cases are tolerable, we can punt the workaround indefinitely.
If the symptom recurs at similar frequency, this is the proposal to
build first.

## Files to read first (next session)

- `helix/api/pkg/server/websocket_external_agent_sync.go:2120-2260`
  — `handleMessageCompleted`, including the `mappingConsumed` and DB-fallback
  logic. Direction (3) above lives here.
- `zed/crates/external_websocket_sync/src/thread_service.rs:454-489`
  — `unregister_thread`, `unregister_thread_if_matches` (added 2026-04-24
  in `d7be64fad1`).
- `zed/crates/agent_servers/src/acp.rs:3145-3247`
  — `handle_session_notification`. Where ACP `SessionUpdate` events get
  dispatched onto an `Entity<AcpThread>` via the `WeakEntity` stored in
  `AcpSession.thread`.
- `claude-agent-acp` source (out of tree) — for direction (1) and to
  reproduce #554/#551 against a known-good test client.
