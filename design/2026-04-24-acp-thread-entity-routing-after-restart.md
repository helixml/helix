# ACP Thread Entity Routing After Zed Restart — Empty `message_completed` Bug

**Date:** 2026-04-24
**Status:** Root cause identified, fix not yet implemented
**Spec task:** `spt_01kpmgr0bsd4v9erw5kj4ct8vn` ("Merge latest Zed upstream into Helix fork")
**Helix session:** `ses_01kpmgs44j4qzeg7cem9smpwpd`
**ACP thread UUID:** `58583773-30f1-4ebc-b340-d6ef3b34b87a`
**Container at incident time:** `ubuntu-external-01kpmgs44j4qzeg7cem9smpwpd` (now reaped)

## Symptom

User opens a previously-paused spec task in the Helix UI. The Zed AgentPanel displays
**two tabs for the same logical thread**:

- **Tab A** — populated only with the Helix-queued prompt, repeated 4 times. No agent responses.
- **Tab B** — the actual conversation with all content. The user can keep working in this tab
  by typing directly.

Helix UI shows `Agent returned empty response (message bounced or content lost). The prompt
will be retried.` (`websocket_external_agent_sync.go:2304`) and re-queues the prompt every
~10–80 s in an infinite loop until `prompt_history_entries.retry_count` saturates at 4.

## Evidence (collected live before the container was reaped)

Backend (`docker compose logs api`):

```
18:18:55 📤 [QUEUE] Sending queued prompt via sendCommandToExternalAgent
         acp_thread_id=58583773-30f1-4ebc-b340-d6ef3b34b87a
         interaction_id=int_01kq0bepqc2yvvxathhe1adw88
18:19:04 anthropic_proxy logging LLM call completion_tokens=284  ← Claude DID produce a response
18:19:12 RECEIVED MESSAGE_COMPLETED FROM EXTERNAL AGENT
         data={"acp_thread_id":"58583773-…","message_id":"0","request_id":""}
18:19:12 🔄 Reloaded interaction with latest response content response_length=0
18:19:12 ⚠️ message_completed with EMPTY response — marking as error and re-queuing
```

**In the previous 15 minutes for this thread:**
- 4× `RECEIVED MESSAGE_COMPLETED FROM`
- **0×** `message_added`, `message_chunk`, `entry_updated`, or any other content-bearing event

Zed-side (`/home/retro/.local/share/zed/logs/Zed.log`):

```
19:10:24 🔨 Calling connection.load_session() to load from agent...
19:10:28 ✅ Loaded ACP thread from agent: 58583773-…
19:10:28 📋 Registered thread: 58583773-… → agent session: 58583773-…
19:10:28 🔍 open_existing_thread_sync slow path:
         subscription flag was_set=false,
         load_session entity=EntityId(1418v15) for '58583773-…'
19:18:55 💬 Sending follow-up message: Merge main of our fork in… (simulate_input=false)
19:18:55 🗑️ unregister_thread: removed '58583773-…'
19:19:12 📤 Sending JSON: {"event_type":"message_completed",
                            "data":{"acp_thread_id":"58583773-…","message_id":"0","request_id":""}}
```

Only **one** `acp_thread_id` UUID (`58583773-…`) ever appears in the Zed log, despite the
user seeing two visible tabs.

## Confirmed not the cause

- **Container teardown** — sandbox `ubuntu-external-01kpmgs44j4qzeg7cem9smpwpd` was alive
  and `Up 11 minutes` at incident time; auto-spawned by `spec_task_design_review_handlers.go:978`
  on session reconnect.
- **Claude not responding** — `anthropic_proxy.go` shows
  `completion_tokens=287/258/etc` for the relevant requests; the agent is doing real work.
- **WebSocket dead** — bidirectional traffic is fine; `chat_message` reaches Zed,
  `message_completed` reaches Helix. Only the chunks/entries are missing.
- **Race against a concurrent Zed-typed user message** — none in this incident; the thread
  was idle when Helix re-fed the queued prompt. The earlier
  `design/2026-04-16-lost-responses-race-condition.md` describes a similar symptom from a
  contention race; this is a *different* mechanism.

## Hypothesis: two `Entity<AcpThread>` instances for one `acp_thread_id`

Across a Zed process restart, two separate `Entity<AcpThread>` instances end up holding
the same logical `acp_thread_id`:

- **Entity X** — created by AgentPanel restoration from on-disk panel state. Has the
  visible UI tab the user has been working in. Has *no live agent connection* unless and
  until something rebinds it.
- **Entity Y** — created by `thread_service::load_session_async` when Helix sends
  `[CONNECT]` reconnect → `open_existing_thread_async`. Has the live ACP connection.
  Registered in `THREAD_REGISTRY` under the same UUID. `notify_thread_display(Y)` adds it
  as a *separate* tab.

The `external_websocket_sync` subscription is correctly created on Y
(`ensure_thread_subscription(Y, …)` runs with `was_set=false`), but the **ACP
`SessionUpdate` events arriving from the agent are dispatched to X**, not Y. As a result:

- `cx.emit(AcpThreadEvent::NewEntry)` fires on X → no listener forwards to Helix.
- `cx.emit(AcpThreadEvent::Stopped)` *does* somehow reach Y (or is forwarded via a
  different code path that uses the registry rather than the entity), so Helix sees
  `message_completed` only.
- The user sees content in tab X (where the chunks land) and an empty Helix queue tab Y
  (where they don't).

### Why the `350de991de` / `ba7e97aea6` Zed commits matter

The `AgentConnection` is now deduplicated per `(Project, AgentId)`
(`ba7e97aea6`). One shared connection serves both X and Y. The connection's session-update
dispatch target is set when the connection is first wired up — i.e. by whichever entity
appeared *first*, which is X (panel restoration runs before `load_session` finishes). The
dedup means `load_session` reuses the cached connection without redirecting its dispatch
target to Y.

The companion commit `350de991de` ("revert AgentConnectionStore cache delegation; keep
thread_service caching") is consistent with this — it left thread_service-level caching in
place, which is precisely the layer where Y registers but where X is invisible.

The bug only became user-visible after the new `helix-ubuntu` image (built from
`350de991de`) was pulled by the 19:10 session restart. Earlier restarts at 12:25 and 13:42
used the previous image and went through fine.

## What we already tried (in tree)

- `22f94a8bbb fix: clear stale subscription flag before re-subscribing in open_existing_thread_sync`
- `d470dac687 fix: coordinate panel restoration and open_existing_thread_sync via load lock`

Both targeted the same general failure mode (panel-restoration-vs-load_session race) but
operate at the **subscription** level, not the **dispatch-target** level. With one shared
connection (post-`ba7e97aea6`), the subscription is on Y but the dispatch target is X, so
the "clear stale flag and re-subscribe" fix doesn't reach the underlying problem.

The `// TODO: Still need to notify AgentPanel to display it` comment at
`thread_service.rs:1593` is also live: when `open_existing_thread_sync` finds the thread
already in the registry, it subscribes but never asks the panel to switch to that entity —
leaving X in the foreground.

## Proposed fix (Zed-side, not yet implemented)

Two layers, both probably needed:

1. **In `agent_connection_store` (or wherever `AgentConnection` dispatch is wired):**
   when `load_session` completes and produces a new `Entity<AcpThread>`, **rebind the
   shared connection's session-update dispatch target to that new entity**, replacing any
   prior X. This is the root fix — without it, layer 2 only papers over the symptom.

2. **In `thread_service.rs::load_session_async` and `open_existing_thread_sync`:** after
   `register_thread(Y)` and `notify_thread_display(Y)`, **close any pre-existing
   AgentPanel tab that holds entity X for the same `acp_thread_id`**, so the user sees
   only the live tab. This avoids the "two tabs" UX confusion and prevents the user from
   typing into the dead entity.

Both should be guarded by tests in `crates/external_websocket_sync/e2e-test/` that:
- Start a session, capture a thread UUID.
- Simulate a Zed restart (kill + relaunch the binary, panel state restored from disk).
- Send a queued prompt from Helix and assert that the response chunks (`message_added`)
  arrive at the test server, not just `message_completed`.

This regression isn't caught by the current 9-phase E2E because all phases run inside a
single Zed process — there's no restart in the matrix.

## Workaround for users who hit this now

- Symptom: prompts cycle in `prompt_history_entries.status='failed'` with
  `retry_count` climbing.
- Mitigation 1: close the duplicate empty tab in Zed (the one with only the bouncing
  prompt) and continue working in the live tab. Helix's queue will keep retrying until
  `retry_count=4` then stop.
- Mitigation 2: mark the failed prompt terminal in DB so the queue stops re-feeding:
  `UPDATE prompt_history_entries SET status='cancelled' WHERE id='<id>';`
- Mitigation 3: temporarily pin `ZED_COMMIT` to a commit *before* `ba7e97aea6` (e.g.
  `2f182e64d6`) — gives up the duplicate-ACP-spawn fix but restores chunk delivery.

## Helix-side state at time of writing

- PR #2263 (https://github.com/helixml/helix/pull/2263) merged at 2026-04-24 ~18:24 UTC —
  bumps `ZED_COMMIT` from `350de991de` *backward* to `5000ca904d` (the rebase point,
  before `ba7e97aea6`/`350de991de` landed). This was a deliberate revert per user
  direction; a follow-up Helix PR will re-bump once the Zed-side fix lands.
- Container `ubuntu-external-01kq0crsbz8648tc4p54045d43` is now running the new image.
  Spec task `spt_01kpmgr0bsd4v9erw5kj4ct8vn` is now `status='done'` (user advanced it).

## Files to read first (next session)

- `~/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
  — `load_session_async` (~1500), `open_existing_thread_sync` (~1573),
  `ensure_thread_subscription` (~502).
- `~/pm/zed/crates/agent_ui/src/agent_connection_store.rs`
  — the dedup added in `ba7e97aea6`.
- `~/pm/zed/crates/acp_thread/src/connection.rs`
  — `AgentConnection` trait, where `SessionUpdate` events get dispatched.
- `~/pm/helix/api/pkg/server/websocket_external_agent_sync.go:2244-2310`
  — the empty-response detector and re-queue logic on the Helix side.
