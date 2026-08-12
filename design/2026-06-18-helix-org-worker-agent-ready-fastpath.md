# helix-org Worker flakiness: `open_thread` reconnect skips `agent_ready` (registry fast-path)

**Status:** **Fixed** (2026-06-18). Root cause confirmed in production; red test landed & verified;
fix implemented and **merged to `helixml/zed` main** as `4ae2094b54a23c1254c13fd045d3a976de446863`
(PR `https://github.com/helixml/zed/pull/64`) and pinned here via `sandbox-versions.txt`
(`ZED_COMMIT`). Verified end-to-end on a locally-rebuilt desktop image — see "Manual verification".
**Severity:** High — long-lived org Workers degrade to majority-failure over days.
**Component:** Zed `external_websocket_sync` (agent side) × Helix external-agent WS sync (server side).

## TL;DR

A long-lived helix-org Worker reconnects its WebSocket many times over its life. On each
reconnect Helix sends `open_thread`, which **suppresses Zed's fallback `agent_ready`** and
delegates the ready signal to the thread service. But the thread service's *"thread already
loaded in registry"* fast-path returns `Ok(())` **without calling `send_agent_ready`** — the
only one of its four load paths that omits it. So `agent_ready` never arrives, Helix's
readiness gate burns a **60s timeout on every reconnect**, and the late message flush collides
with the empty-`interaction_id` streaming race to produce bounced/dropped turns. The bug is
**worker-specific** because only a process that reconnects to an *already-registered* thread
trips the fast-path; short-lived spec-tasks never do.

The fix is one block in Zed: the registry fast-path must call `send_agent_ready` before
returning, matching the other three load paths.

## Symptom (production)

Worker `w-docs-engineer` (org project `prj_01kv7pth8bncpjg41dagw1dt0p`, session
`ses_01kv7ptj44bsy43g2kvymt56w5`, Zed thread `27f551fa-4f16-45ae-bee7-375f0b12e428`), live
since 2026-06-16 — **not** a spec task.

- Lifetime: 989 complete / 1,038 error / 22 waiting (~51% failure).
- Degrading: last 24h 541 complete / 1,027 error (~65% failure).
- "Host down" cannot explain a worker that works half the time. It is intermittent — flaky.

Errored-interaction breakdown:

| Count | Error message | Maps to |
|---|---|---|
| 545 | `Agent returned empty response (message bounced or content lost)` | empty-`interaction_id` content drop (downstream) |
| 239 | `Agent thread wedged (claude-agent-acp cancel/prompt swallow)` | auto-wake wedge breaker |
| 144 | `Agent unresponsive after auto-wake retries (upstream ACP buffering)` | ACP buffering |
| 91 | `Agent never connected after auto-wake cold-start retries` | cold-start backstop exhausted |
| 19 | `Interrupted` | benign (user cancel) |

## Root cause

`open_existing_thread_sync` (Zed repo: `crates/external_websocket_sync/src/thread_service.rs`)
has four code paths; three send `agent_ready`, one does not:

Line numbers below are as of `helixml/zed@main` on 2026-06-18 (re-synced after a
main pull; an earlier draft cited pre-pull numbers). The `send_agent_ready` call
sites:

| Path | `send_agent_ready` line | Sends `agent_ready`? |
|---|---|---|
| Fresh async load | `:1789` | ✅ |
| Lock-wait recheck | `:2324` | ✅ |
| Slow-path sync load | `:2523` | ✅ |
| **Registry fast-path ("already loaded")** | **`:2257-2265`** | ❌ |

```rust
// thread_service.rs:2257 (inside open_existing_thread_sync, fn starts :2243)
if let Some(thread_weak) = get_thread(&request.acp_thread_id) {
    eprintln!("✅ [THREAD_SERVICE] Thread already loaded in registry: {}", request.acp_thread_id);
    log::info!("✅ [THREAD_SERVICE] Thread already loaded in registry: {}", request.acp_thread_id);
    if let Some(thread_entity) = thread_weak.upgrade() {
        ensure_thread_subscription(&thread_entity, &request.acp_thread_id, cx);
    }
    // TODO: Still need to notify AgentPanel to display it
    return Ok(());   // ← returns WITHOUT crate::send_agent_ready(...)
}
```

The `// TODO` already flags the path as incomplete.

## Handshake mechanics (why the fast-path is fatal, not just incomplete)

1. On WS connect, Helix sends `open_thread` **before** the readiness gate
   (`api/pkg/server/websocket_external_agent_sync.go:425-479`).
2. Zed's connection loop sees `open_thread` and **suppresses its own 5s fallback `agent_ready`**
   (`crates/external_websocket_sync/src/websocket_sync.rs:332-335`, sets `agent_ready_sent=true`),
   on the contract that the thread service will send `agent_ready` after loading.
3. The thread service hits the registry fast-path → `return Ok(())` → no `agent_ready`.
4. Helix's readiness gate waits, then times out at **60s** and flushes queued messages anyway
   (`api/pkg/server/websocket_external_agent_sync.go:2199-2203`). Every reconnect = 60s dead time.

Because step 2 disarmed the only fallback, step 3's omission means `agent_ready` is *never*
emitted for the rest of the connection's life.

## Why it's worker-specific (answers "why is helix-org flakier than spec-tasks?")

The fast-path only fires on **reconnect to a thread still in the in-process registry** — i.e. a
Zed process that has been alive long enough to reconnect against a surviving `AcpThread` entity.
A *fresh* load (new worker, or any spec-task's first/only load) takes the slow path and sends
`agent_ready` correctly. Spec-tasks are short-lived (one task → done) and never reconnect to an
already-registered thread; the org Worker is the **only** workload that lives for days and trips
this path on every WS cycle. So helix-org is not flakier because its orchestration layer is
worse — it is flakier because it is the one workload that repeatedly hits a reconnect path that
everything else outgrows before it matters.

## Downstream cascade

The 60s-late flush collides with the streaming-context interaction-resolution race:

- `getOrCreateStreamingContext` (`websocket_external_agent_sync.go:1491`) yields
  `interactionID=""` when the `request_id` maps to the consumed sentinel `""` **and** there is no
  `Waiting` interaction left **and** the last interaction isn't `Error=="Interrupted"` (the
  restart-recovery at `:1649` only revives `Interrupted`).
- Streamed assistant content then hits the `else` at `:1391` → **"No interaction found to update
  with assistant response"** and is dropped (the ~2,640 such lines).
- At `message_completed`, empty response + empty entries → bounce at `:2700` (the 545 errors).
- The homeless-content branch **never bumps `sctx.lastPublish`**, so auto-wake's quiescence gate
  (`auto_wake_stuck_interactions.go:512`) thinks the agent is idle and errors/re-sends the turn →
  more reconnects → more fast-path-no-`agent_ready` → **degradation spiral** (51%→65% over the
  worker's life).

The empty-`interaction_id` transition (`websocket_external_agent_sync.go:1735`) was observed
landing mid-loop at 10:21:02, confirming the collision.

## Production confirmation (2026-06-18)

All four fingerprint elements observed on both sides of the wire for thread `27f551fa`:

1. **`open_thread` every reconnect** (Helix `…sync.go:444/477`):
   `[CONNECT] Sending open_thread directly on new connection before agent_ready gate` →
   `[CONNECT] ✅ open_thread written directly to WebSocket (zed_thread_id=27f551fa…)`.
   Zed receives it: `[WEBSOCKET] open_thread received — thread service will send agent_ready after loading`.
2. **Registry fast-path** (Zed): `[THREAD_SERVICE] ✅ Thread already loaded in registry: 27f551fa-…`.
3. **No `agent_ready`** — decisive count across both Zed logs: `already loaded in registry: 1`,
   `send_agent_ready / agent_ready sent: 0`. The only `agent_ready` string anywhere is the
   handler's promise ("…will send agent_ready after loading"); it is never fulfilled.
4. **Deterministic 60s timeout** (Helix `…sync.go:2203`), every cycle:
   ```
   open_thread 10:13:17 → READINESS timeout 10:14:17  (60s)
   open_thread 10:14:18 → READINESS timeout 10:15:18  (60s)
   open_thread 10:15:19 → READINESS timeout 10:16:19  (60s)
   open_thread 10:17:22 → READINESS timeout 10:18:22  (60s)
   open_thread 10:21:15 → READINESS timeout 10:22:15  (60s)
   ```

Every reconnect re-opens the same `zed_thread_id=27f551fa…` (reconnect-to-already-registered-thread —
the precise fast-path trigger), and the empty-`interaction_id` transition lands inside the loop
(10:21:02).

## Fix (landed)

In `open_existing_thread_sync`'s registry fast-path
(`crates/external_websocket_sync/src/thread_service.rs`), call `send_agent_ready`
before `return Ok(())`, mirroring the other three load paths. The merged version also
calls `set_thread_agent_name` first (as the fresh-load, slow-path, and lock-wait paths
all do) so wedge-recovery can route to the correct `AgentConnection`:

```rust
if let Some(thread_weak) = get_thread(&request.acp_thread_id) {
    if let Some(thread_entity) = thread_weak.upgrade() {
        ensure_thread_subscription(&thread_entity, &request.acp_thread_id, cx);
    }
    // TODO: Still need to notify AgentPanel to display it
    let agent_name_for_ready = request
        .agent_name
        .clone()
        .unwrap_or_else(|| "zed-agent".to_string());
    set_thread_agent_name(&request.acp_thread_id, agent_name_for_ready.clone());
    crate::send_agent_ready(agent_name_for_ready, Some(request.acp_thread_id.clone()));
    return Ok(());
}
```

### Build / deploy — DONE

Shipped following the CLAUDE.md ordering: Zed PR `https://github.com/helixml/zed/pull/64`
(test commit + fix commit) squash-merged to `helixml/zed` main as
`4ae2094b54a23c1254c13fd045d3a976de446863`; this repo's `sandbox-versions.txt`
`ZED_COMMIT` bumped to that SHA so CI/prod sandbox builds pick up the fix.

## Test plan (TDD red-first)

- **Red test (Zed/Rust) — LANDED & VERIFIED 2026-06-18.**
  `crates/external_websocket_sync/src/thread_service.rs`, module
  `agent_ready_on_reconnect_tests`, test
  `open_thread_on_already_registered_thread_emits_agent_ready` (a `#[gpui::test]`). It:
  1. builds a real `AcpThread` via `StubAgentConnection::new_session` and `register_thread`s it
     (so `open_existing_thread_sync` hits the registry fast-path — the precondition is asserted);
  2. installs a recording `WEBSOCKET_SERVICE` via a new `#[cfg(test)]`
     `WebSocketSync::new_test() -> (Arc<Self>, UnboundedReceiver<SyncEvent>)` seam in
     `websocket_sync.rs` so `send_agent_ready` is observable (it otherwise errors on an
     uninitialised service);
  3. calls `open_existing_thread_sync` with an `open_thread` request and asserts **exactly one**
     `AgentReady` for that thread id;
  4. tears down the recording service + the registry/keep-alive strong refs before asserting, so
     the gpui harness doesn't flag the entity as a leaked handle.

  Verified discriminating: **RED** on current `main` (`agent_ready_count == 0`, `left: 0 right: 1`);
  **GREEN** when the design's one-block fix is applied to the fast-path; reverting the fix returns
  it cleanly to RED with no leak panic. The other 43 crate lib tests still pass (the global-state
  mutation is cleaned up and doesn't contaminate the parallel suite).

  The only non-test code added is the `#[cfg(test)] WebSocketSync::new_test` seam (15 lines) plus an
  `acp_thread = { features = ["test-support"] }` dev-dependency — the buggy fast-path itself is
  untouched, so the test is a true pre-fix red.

- **Regression guard (Helix, green):** `SessionMessagesHandlerSuite` already proves a
  `terminated_idle` session is cold-started on send; the readiness path is Zed-side, so the
  authoritative test lives in the crate.

## Manual verification (2026-06-18)

Built the merged branch into the desktop image locally (`./stack build-zed dev` →
`./stack build-ubuntu` → `helix-ubuntu:516f84`; the in-image `/zed-build/zed` checksum
matched the freshly-built binary) and exercised the real reconnect path against a live
exploratory desktop (`ses_01kv7tgayjp0…`, thread `b052d08e…`):

1. Initial boot took the **slow path** → `agent_ready event sent successfully`; the thread
   registered in the long-running Zed process.
2. Restarted `helix-api-1` to drop the agent WebSocket while the Zed process (and its
   registry) stayed alive.
3. On reconnect the API re-sent `open_thread` (`…sync.go:444/477`) and Zed hit the
   **`✅ Thread already loaded in registry`** fast-path — then immediately
   **`🚀 Sending agent_ready event` → `✅ agent_ready event sent successfully`**.
4. The API logged **zero** `Timeout waiting for agent_ready` past the 60s deadline.

That is the production fingerprint inverted: the reconnect-to-already-loaded-thread path that
previously burned 60s now completes the readiness handshake instantly.

## Related / still-open

- Downstream correctness gap (separate, lower priority): the empty-`interaction_id` content drop.
  Note the consumed-sentinel and `Interrupted`-only recovery are **deliberate** guards against the
  stale-rebind regression (`design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md`) —
  do **not** naively "attach late content anyway".
- Adjacent prior work: `design/2026-04-24-acp-thread-entity-routing-after-restart.md`,
  `design/2026-06-15-wedged-acp-thread-autowake-flood.md`,
  `design/2026-04-25-zed-claude-async-event-flush-on-user-input.md`.
