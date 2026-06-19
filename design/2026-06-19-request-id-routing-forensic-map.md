# Design: Forensic Map of request_id Routing in WebSocket Sync

> **Read-only investigation.** No code was modified. Every claim is backed by
> a `file:line` from the current checkout.  
> Primary files:
> - `api/pkg/server/websocket_external_agent_sync.go` (4415 lines, "sync")
> - `api/pkg/server/auto_wake_stuck_interactions.go` (792 lines, "wake")
> - `api/pkg/server/external_agent_handlers.go` (1792 lines, "handlers")
> - `api/pkg/server/server.go` (struct definitions)
> - `api/pkg/server/spec_task_design_review_handlers.go` ("review")
> - `zed/crates/external_websocket_sync/src/websocket_sync.rs` ("zed-ws")
> - `zed/crates/external_websocket_sync/src/thread_service.rs` ("zed-ts")

---

## Q1 — Correlation Maps

### Struct declarations (`api/pkg/server/server.go`)

```
119: contextMappings             map[string]string  // acp_thread_id  → helix_session_id
120: contextMappingsMutex        sync.RWMutex       // guards ALL FIVE maps below
121: requestToSessionMapping     map[string]string  // request_id → helix_session_id
122: requestToInteractionMapping map[string]string  // request_id → interaction_id | "" sentinel
123: // interactionToPromptMapping: DELETED 2026-04-30. Replaced by Interaction.PromptID column.
126: externalAgentSessionMapping map[string]string  // agent_session_id → helix_session_id
127: externalAgentUserMapping    map[string]string  // agent_session_id → user_id
132: requestToCommenterMapping   map[string]string  // request_id → commenter_user_id
133: sessionToCommenterMapping   map[string]string  // session_id  → commenter_user_id
156: streamingContexts           map[string]*streamingContext  // helix_session_id → *sctx
157: streamingContextsMu         sync.RWMutex       // separate, guards only streamingContexts
```

There are **five maps under `contextMappingsMutex`** and one map under `streamingContextsMu`. A sixth (`externalAgentSessionMapping`/`externalAgentUserMapping`) is also under `contextMappingsMutex` but not part of the turn-routing surface.

---

### Map 1: `contextMappings` — `acp_thread_id → helix_session_id`

**Guard:** `contextMappingsMutex` (RWMutex; reads use `RLock`, writes use `Lock`)

**Write sites:**
| Line | Function | Trigger |
|------|----------|---------|
| sync:401-404 | `handleExternalAgentSync` (WS connect handler) | WS reconnect: restores from `session.Metadata.ZedThreadID` |
| sync:799 | `handleThreadCreated` Priority 1 | thread bound to existing Helix session via request_id |
| sync:826-828 | `handleThreadCreated` Priority 3 | existing session found by ZedThreadID DB scan |
| sync:891-895 | `handleThreadCreated` fallback | brand-new session created for user-initiated Zed context |
| sync:1161-1163 | `handleMessageAdded` | on-the-fly session created for user message race |
| sync:1176-1178 | `handleMessageAdded` fallback | DB fallback found session, restore |
| sync:2543-2544 | `handleMessageCompleted` fallback | DB fallback found session, restore |

**Read sites:**
| Line | Function |
|------|----------|
| sync:1107 | `handleMessageAdded` — primary session lookup for all message_added events |
| sync:2523-2525 | `handleMessageCompleted` — primary session lookup |
| sync:3431 | `handleThreadLoadError` — session lookup for error routing |

**Not-found branch:**  
`handleMessageAdded` (sync:1110-1183): falls back to `findSessionByZedThreadID` DB scan. If DB also misses and `role != "assistant"`: creates a session on-the-fly and re-populates the map. If DB also misses and `role == "assistant"`: returns an error and drops the token. **This is where buffered background events with no matching session vanish silently.**

`handleMessageCompleted` (sync:2527-2548): falls back to `findSessionByZedThreadID`. If DB misses: logs warning and `return nil` — **reply discarded, #2643**.

---

### Map 2: `requestToSessionMapping` — `request_id → helix_session_id`

**Guard:** `contextMappingsMutex`

**Write sites:**
| Line | Function | Notes |
|------|----------|-------|
| handlers:188-198 | `RegisterRequestToSessionMapping` | called from external ingress |
| sync:523-531 | `pickupWaitingInteraction` | fallback: uses interactionID as requestID |
| sync:3255-3264 | `sendQueuedPromptToSession` | queue path; only on FIRST message (no ZedThreadID) |
| sync:1966-1971 | `sendChatMessageToExternalAgent` | only when `acpThreadID == nil` (new thread) |
| wake:603-607 | `maybeAutoWake` | auto-wake re-sends get their own fresh request_id |

**Read sites:**
| Line | Function |
|------|----------|
| sync:742-744 | `handleThreadCreated` PRIORITY 1 — maps thread back to existing session |
| sync:514-520 | `pickupWaitingInteraction` — reverse scan: iterates all entries, matches by sessionID |
| sync:1408-1415 | `handleMessageAdded` user-role branch — checks for pre-created interaction to avoid echo-duplicate |

**Delete sites:**  
sync:756-758 `handleThreadCreated` after Priority 1 match: deletes the request_id→session entry. **Note: `requestToCommenterMapping` is explicitly NOT deleted here** (sync:754-759 comment).

**Not-found branch:**  
`handleThreadCreated` PRIORITY 1: if no mapping found, falls to PRIORITY 2 (helixSessionID from syncMsg) or PRIORITY 3 (DB scan by ZedThreadID). If all three miss: **creates a new orphan Helix session** for the thread (sync:837-896).

---

### Map 3: `requestToInteractionMapping` — `request_id → interaction_id | ""`

The `""` empty-string is an explicit **consumed sentinel**: once `handleMessageCompleted` processes a request_id, it writes `""` so that a duplicate `message_completed` (common on interrupt races) is dropped.

**Guard:** `contextMappingsMutex`

**Write sites:**
| Line | Function | Value written |
|------|----------|---------------|
| sync:533-536 | `pickupWaitingInteraction` | interaction_id |
| sync:3261-3264 | `sendQueuedPromptToSession` | interaction_id |
| sync:1953-1957 | `sendChatMessageToExternalAgent` | interaction_id |
| sync:944-948 | `handleThreadCreated` new-session path | interaction_id |
| sync:1691-1717 | `getOrCreateStreamingContext` | interaction_id (only if not already consumed `""`) |
| wake:603-607 | `maybeAutoWake` | interaction_id |
| sync:2586 | `handleMessageCompleted` | `""` (consumed sentinel) |

**Read sites:**
| Line | Function |
|------|----------|
| sync:1495-1497 | `getOrCreateStreamingContext` — resolve which interaction request_id targets |
| sync:2574-2587 | `handleMessageCompleted` — consumed-sentinel check; sets `""` on success |
| sync:3578-3580 | `handleChatResponseError` — error routing |
| sync:2114-2115 | `handleTurnCancelled` — mark interaction as interrupted |

**Not-found/else branch — this is where the bug lives:**

In `getOrCreateStreamingContext` (sync:1695-1717):
- `alreadySeen && existing == ""` (consumed sentinel): stale wrapper replay detected. **Sentinel preserved, streaming tokens still flow to the most-recent-waiting fallback**. The subsequent `message_completed` with the stale request_id will also hit the `""` sentinel and be dropped.
- `existing != newInteractionID`: rebinds to the current waiting interaction (normal flow).

In `handleMessageCompleted` (sync:2577-2587):
- `mappedID == ""`: consumed sentinel — logs "Duplicate message_completed" and **returns nil, discarding the completion**.
- Key not present at all: falls through to Step 2 DB scan of most-recent-waiting interaction. **This is the stale-id fallback that misroutes buffered events from the previous turn to the current turn's interaction.**

---

### Map 4: `requestToCommenterMapping` — `request_id → commenter_user_id`

Used only for design-review comment streaming. Not on the main turn-routing critical path.

**Guard:** `contextMappingsMutex`

**Write sites:** review:1623-1627 (write), review:1650-1651 (cleanup on error)

**Read sites:** sync:3779-3780, sync:3838-3839 (lookup during streaming publish)

**Delete sites:** sync:2884-2885, sync:2913-2915 (`handleMessageCompleted` cleanup)

**Not-found branch:** read sites silently skip commenter routing if mapping absent.

---

### Map 5: `sessionToCommenterMapping` — `session_id → commenter_user_id`

Fallback for cases where `request_id` is unavailable during streaming.

**Guard:** `contextMappingsMutex` (write at review:1628-1631; read at sync:1202-1203)  
**Note:** `sessionCommentMutex` (a *separate* `sync.RWMutex` at server.go:131) guards only the `sessionCommentTimeout` map, NOT `sessionToCommenterMapping`. Both mutexes coexist; care needed to avoid confusion.

**Write sites:** review:1628-1631  
**Delete sites:** review:1375-1378 (cleanup after response complete)  
**Read sites:** sync:1202-1203 inside `handleMessageAdded`

---

### Map 6: `streamingContexts` — `helix_session_id → *streamingContext`

**Guard:** `streamingContextsMu` (separate RWMutex — **different lock from the five maps above**)

`streamingContext` (sync:72-101) caches: session, interaction, interactionID, commenterID, lastDBWrite, dirty flag, lastPublish, flushTimer, previousEntries, accumulator, priorEntries, and its own `mu sync.Mutex`.

**Write sites:**
| Line | Function | Action |
|------|----------|--------|
| sync:1748-1755 | `getOrCreateStreamingContext` | creates new entry |
| sync:1805-1808 | `flushAndClearStreamingContext` | deletes entry |

**Read sites:**
| Line | Function |
|------|----------|
| wake:505-515 | `maybeAutoWake` — quiescence gate: skips wake if lastPublish < threshold |
| sync:1500-1502 | `getOrCreateStreamingContext` — check for existing context before DB query |

**Not-found branch:**  
`getOrCreateStreamingContext` (sync:1607-1763): if context missing (first token or after transition), performs GetSession + ListInteractions DB queries. Falls back through: (1) `expectedInteractionID` match, (2) most-recent-waiting scan, (3) recover interrupted interaction from API restart.

---

### Lock ordering / contention picture

Two distinct locks govern this surface:

1. `contextMappingsMutex` — coarse shared `sync.RWMutex` that guards five maps (contextMappings, requestToSessionMapping, requestToInteractionMapping, requestToCommenterMapping, sessionToCommenterMapping). Every event handler that touches any of these acquires this lock. There are approximately 130 lock/unlock pairs across the sync, handlers, and review files.

2. `streamingContextsMu` — separate `sync.RWMutex` guarding `streamingContexts`. Never held simultaneously with `contextMappingsMutex` in the same goroutine.

3. `sctx.mu` — per-context `sync.Mutex` on each `streamingContext`. Always acquired *after* `streamingContextsMu` is released (no nested locking between them).

**Contention risk:** `contextMappingsMutex.Lock()` (write lock) is acquired inside hot streaming paths (`getOrCreateStreamingContext`, `handleMessageAdded`) for map reads that should be RLock. Some callsites at sync:2574 and sync:2574 hold the write lock while iterating; these serialize all concurrent streaming events.

---

## Q2 — request_id Lifecycle

### Where minted

| Site | Format | Function |
|------|--------|----------|
| sync:3247 | `createdInteraction.ID` | `sendQueuedPromptToSession` — queue path |
| sync:1037 | `interaction.ID` | `NotifyExternalAgentOfNewInteraction` — chat path |
| review:1620 | `"req_" + system.GenerateUUID()` | design-review comment dispatch |
| wake:595 | `"autowake_" + system.GenerateUUID()` | auto-wake re-send |
| Zed-generated | opaque string | Zed-initiated user messages (no Helix involvement) |

### Where bound to interaction

`requestToInteractionMapping[requestID] = interactionID` — five write sites listed in Q1 Map 3.

### One prompt end-to-end (queue path)

1. `sendQueuedPromptToSession` (sync:3215-3247) creates `Interaction{State:Waiting, PromptID:prompt.ID}`, stores `requestID = createdInteraction.ID`.
2. `requestToInteractionMapping[requestID] = createdInteraction.ID` (sync:3263) + `requestToSessionMapping[requestID] = sessionID` (sync:3258) registered.
3. `chat_message` command sent to Zed with `request_id=requestID`, `acp_thread_id=session.Metadata.ZedThreadID`.
4. Zed dispatches to ACP; `claude-agent-acp` starts streaming. Emits `message_added(role=assistant, acp_thread_id=..., request_id=..., content=...)`.
5. `handleMessageAdded` (sync:1061) looks up `contextMappings[acp_thread_id]` → `helixSessionID`. Calls `getOrCreateStreamingContext(helixSessionID, requestID)`.
6. `getOrCreateStreamingContext` (sync:1495) reads `requestToInteractionMapping[requestID]` → `expectedInteractionID`. Queries DB if no existing context. Finds `targetInteraction`. Tokens accumulated in `sctx.accumulator`.
7. When token stream ends, Zed emits `message_completed(acp_thread_id=..., request_id=...)`.
8. `handleMessageCompleted` (sync:2574): reads `requestToInteractionMapping[requestID]` → `targetInteractionID`. Sets mapping to `""` (consumed). Calls `flushAndClearStreamingContext`. Marks interaction `State=Complete`.
9. Queue is advanced: `processPromptQueue` checks for next pending prompt.

### Consumed-sentinel logic

`handleMessageCompleted` (sync:2586): `apiServer.requestToInteractionMapping[messageRequestID] = ""` — key retained but value cleared. Subsequent `message_completed` for the same `request_id` finds `mappedID == ""`, logs warning, and returns nil (drop).

**The stale-id window:** between step 3 (command sent) and step 8 (sentinel written), if the `claude-agent-acp` wrapper buffers any background events and flushes them on the *next* user prompt, those events arrive with this same `request_id`. When the next turn is in flight, `getOrCreateStreamingContext` rebinds the stale `request_id` to the new interaction (sync:1708-1710) — unless the sentinel was already written, in which case it refuses to rebind (sync:1697-1707). The sentinel is the defence; the window is the vulnerability.

---

## Q3 — Dual Delivery Paths (#2642)

### Queue path (`/sessions/{id}/messages` or via `sendQueuedPromptToSession`)

1. `processPromptQueue` (sync:2993) → `sendQueuedPromptToSession` (sync:3134)
2. Creates `Interaction{State:Waiting}` first. Sets `requestID = createdInteraction.ID`.
3. Sends `chat_message` command: `{message, request_id, acp_thread_id, agent_name, interrupt}`. **No `role` field.**
4. Zed receives, `handle_chat_message` (zed-ws:415): `chat_msg.role` is `None` → passes the guard at line 421, processed normally.

### Chat path (`POST /sessions/chat` → `startChatSessionHandler`)

1. `startChatSessionHandler` (session_handlers.go ~L680) creates interaction via `Controller.WriteInteractions`.
2. Calls `NotifyExternalAgentOfNewInteraction` (sync:1000).
3. `NotifyExternalAgentOfNewInteraction` (sync:1034-1038) builds command:
   ```go
   commandData := map[string]interface{}{
       "message":    interaction.PromptMessage,
       "role":       "user",          // <-- added here
       "request_id": interaction.ID,
   }
   ```
4. Sends `chat_message` command with `role:"user"`.

### Where #2642 drops it

`zed/crates/external_websocket_sync/src/websocket_sync.rs`, line 421:

```rust
if chat_msg.role.as_deref() == Some("user") {
    eprintln!("🔄 [WEBSOCKET-IN] Ignoring echoed user message (role=user)...");
    return Ok(());
}
```

Zed treats any `chat_message` with `role:"user"` as an echo of the user's own input (to avoid double-processing). `NotifyExternalAgentOfNewInteraction` inserts `role:"user"` (sync:1037) to distinguish its payload semantically, but this label is the exact discriminant Zed uses to drop the message. **The prompt is silently discarded. No interaction is updated. No error is returned.** The waiting interaction sits in `state=waiting` until the auto-wake worker fires (≥180s later).

### Divergence point

The paths diverge at `sendQueuedPromptToSession` vs `NotifyExternalAgentOfNewInteraction`. Both call `sendCommandToExternalAgent`, but only `NotifyExternalAgentOfNewInteraction` adds `role:"user"` to the payload.

---

## Q4 — Turn-State Inference

There is no explicit turn-state machine. "Turn running / done / waiting" is inferred from multiple independent sites:

| Site | File:Line | What it infers from | Decision |
|------|-----------|---------------------|----------|
| `getOrCreateStreamingContext` transition detection | sync:1507 | `sctx.interactionID != expectedInteractionID` | Old turn ended; new turn started |
| `handleMessageCompleted` | sync:2748 | receipt of `message_completed` event | Turn → Complete |
| `handleTurnCancelled` | sync:2113 | receipt of `turn_cancelled` event | Turn → Interrupted |
| `handleMessageCompleted` empty-response branch | sync:2711-2714 | `ResponseMessage == ""` | Turn → Error (bounced) |
| `autoWakeStuckInteractions` | wake:383 | age of `state=waiting` interaction > threshold | Turn → stuck, re-send |
| `sessionWedgedByConsecutiveErrors` | wake:319 | N consecutive error interactions | Session → wedged |
| `getOrCreateStreamingContext` "interrupted" recovery | sync:1649-1666 | `State == Error && Error == "Interrupted"` | Recover → Waiting |
| `handleThreadLoadError` | sync:3484-3489 | receipt of `thread_load_error` | Turn → Error |

These are the call sites an explicit transition chokepoint would absorb. The absence of a single FSM is why state can be "in-flight" in `requestToInteractionMapping` but `state=complete` in the DB at the same time.

---

## Q5 — Auto-Wake Worker

**File:** `api/pkg/server/auto_wake_stuck_interactions.go`

**Trigger (wake:357-380):** `time.NewTicker(10s)` calls `scanAndAutoWakeStuckInteractions`. SQL filter (`ListStuckWaitingInteractions`): `state=waiting AND response_message='' AND response_entries IS NULL AND created < now()-threshold`. Default threshold: 180s (env: `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS`).

**Gates before re-sending (wake:399-525):**
1. WebSocket connection present; if not → `maybeKickColdStart` (dev container restart).
2. Activity anchor check: `max(conn.ConnectedAt, session.Updated)` must be > threshold old.
3. Streaming-context quiescence: skip if `sctx.lastPublish` < threshold (agent actively streaming).
4. Interaction age recheck against threshold (defense in depth).
5. Session wedge breaker: `sessionWedgedByConsecutiveErrors` — counts consecutive error interactions since last complete; trips at ≥ 3 (`HELIX_AUTO_WAKE_SESSION_WEDGE_THRESHOLD`).
6. Per-interaction retry cap: `stuck.AutoWakeCount >= 2` → mark `state=error`.

**What it re-sends (wake:615-623):**
```go
requestID := "autowake_" + system.GenerateUUID()
apiServer.requestToInteractionMapping[requestID] = stuck.ID  // map new id → stuck interaction
command := types.ExternalAgentCommand{Type: "chat_message", Data: {
    "message":       stuck.PromptMessage,
    "request_id":    requestID,
    "acp_thread_id": session.Metadata.ZedThreadID,  // nil if no thread yet
}}
```

**State transitions:**
- Retry cap exhausted: → `state=error`, `Error="Agent unresponsive after auto-wake retries"` (wake:556-571)
- Session wedge tripped: → `state=error`, `Error="Agent thread wedged"` (wake:537-550)
- Cold-start cap exhausted: → `state=error`, `Error="Agent never connected..."` (wake:712-769)
- Normal re-send: `auto_wake_count` incremented via targeted SQL `IncrementInteractionAutoWakeCount` (avoids GORM Save race).

---

## Q6 — acp_thread_id Availability

### Present on inbound events

| Event | Field | Struct field | File:Line |
|-------|-------|-------------|----------|
| `thread_created` | `acp_thread_id` (primary) or `context_id` (fallback) | — | sync:708-711 |
| `message_added` | `acp_thread_id` (primary) or `context_id` (fallback) | — | sync:1063-1066 |
| `message_completed` | `acp_thread_id` | required | sync:2517-2519 |
| `thread_load_error` | `acp_thread_id` | optional | sync:3417 |

**Not present in:** `message_added` events that originated from background/hook/subagent activity buffered by `claude-agent-acp`. In that case the event carries only a `request_id` (the stale one from the previous turn). `acp_thread_id` IS still sent in the `message_completed` that terminates the buffered flush — but that completion event references the old interaction via the stale `request_id`.

### Persisted on session/interaction

- `session.Metadata.ZedThreadID` (types.go:433): written in `handleThreadCreated` (sync:785) when a thread is first bound to a Helix session. **Survives API restart.** Used to rebuild `contextMappings` on WS reconnect (sync:401-404).
- `types.SpecTaskZedThread.ZedThreadID` (task_management.go:78): a separate audit record, indexed. Used by `findLatestZedThreadForSpecTask` to find the current active thread for a spec task on reconnect (sync:433-437).

**How close to routing on it:** The DB persistence is already complete. `contextMappings` is rebuilt from `session.Metadata.ZedThreadID` on every WS reconnect (sync:401-404). The only missing piece is routing `message_added`/`message_completed` lookup through `acp_thread_id → helixSessionID` directly without passing through `request_id` for the interaction binding.

---

## Q7 — Chokepoint Candidates

### 1. `getOrCreateStreamingContext` (sync:1491)

Every `message_added(role=assistant)` event flows through this function before any token is routed to an interaction. It already:
- Resolves `request_id → expectedInteractionID` (sync:1495-1497)
- Falls back to most-recent-waiting scan (sync:1636-1643)
- Detects interaction transitions (sync:1507-1598)
- Rebinds `requestToInteractionMapping` for Zed-initiated messages (sync:1690-1718)

This is the **natural home for an explicit transition chokepoint**. Replacing the `requestID → interactionID` lookup here with an `acp_thread_id → current_interaction` lookup is the minimal surface that would fix the stale-id routing. The function already takes `requestID` as a parameter; swapping in `acp_thread_id` as the primary key requires changing only this function's lookup logic and the `streamingContexts` map key (currently `helix_session_id`, which could be derived from `acp_thread_id` via `contextMappings`).

### 2. `handleMessageCompleted` (sync:2509)

All turn completions funnel here. It already performs the `acp_thread_id → helixSessionID` lookup via `contextMappings` (sync:2523-2525), then falls back to DB. The additional `request_id → interactionID` lookup (Step 1, sync:2574-2587) is what needs to be replaced by a stable `acp_thread_id → current_waiting_interaction` query.

### 3. `processExternalAgentSyncMessage` (sync:651)

The dispatch table for all inbound events. Too coarse for a transition chokepoint, but the right place to inject an `acp_thread_id` normalization pass (extract thread_id, resolve to session_id, check liveness) before any per-event handler runs. Would let all eight handlers share a single "thread is known and live" gate.

---

## Q8 — Restart-Survival Matrix

| State piece | Storage | Lost on API restart? | Notes |
|-------------|---------|---------------------|-------|
| `contextMappings` (thread→session) | In-memory | Rebuilt on WS reconnect from `session.Metadata.ZedThreadID` | Survives if WS reconnects; lost if restart happens with no reconnect |
| `requestToSessionMapping` | In-memory | **Yes — completely lost** | No DB counterpart; after restart, `handleThreadCreated` Priority 1 never fires |
| `requestToInteractionMapping` | In-memory | **Yes — completely lost** | After restart, all completions fall through to most-recent-waiting DB scan |
| `requestToInteractionMapping` sentinel (`""`) | In-memory | **Yes — lost** | Stale duplicates that were previously guarded can now re-complete interactions |
| `sessionToCommenterMapping` | In-memory | **Yes** | Comment streaming loses commenter routing |
| `requestToCommenterMapping` | In-memory | **Yes** | Comment streaming loses commenter routing |
| `streamingContexts` (session→sctx) | In-memory | **Yes** | But content is flushed to DB every 5s; at most 5s of tokens lost |
| `streamingContexts.accumulator` | In-memory | **Yes** | message_id→content map lost; Zed replay needed |
| `Interaction.PromptID` | DB column | No — survives | Key link: prompt_history → interaction; enables `MarkPromptAsSent` recovery |
| `Interaction.State` | DB column | No — survives | Source of truth for "waiting/complete/error" |
| `Interaction.ResponseMessage` | DB column (throttled) | Partial — last 5s may be missing | Rebuilt from Zed replay on reconnect |
| `session.Metadata.ZedThreadID` | DB JSONB | No — survives | The anchor for `contextMappings` rebuild |
| `auto_wake_count` (retry counter) | DB column | No — survives | `IncrementInteractionAutoWakeCount` targeted UPDATE |
| `pendingCancelChannels` | In-memory | **Yes** | Outstanding cancel requests dropped; Zed may still cancel, Helix won't hear it |

**The crux:** A refactor keyed on `acp_thread_id` (persisted in `session.Metadata.ZedThreadID`) would preserve what matters (`contextMappings` rebuild) while eliminating what is lost and fragile (`requestToSessionMapping`, `requestToInteractionMapping`, sentinels). The DB fallback via `findSessionByZedThreadID` is already in place; the refactor demotes it from "fallback" to "primary".

---

## Discrepancies vs Prior Docs

### vs `design/2026-04-28-websocket-sync-architecture-review.md`

| Prior doc claim | Current code |
|-----------------|-------------|
| 5 maps: lists `interactionToPromptMapping` (interaction_id → prompt_id) | **DELETED.** server.go:123-125 comments: "in-memory map was deleted in the 2026-04-30 stuck-queue fix." Replaced by `Interaction.PromptID` DB column. |
| "5 concurrent maps" | Current: 5 maps (interactionToPromptMapping gone, sessionToCommenterMapping present but same total count). The *names* differ from what the prior doc listed. |
| "~130 mutex sections" | Still approximately accurate. |
| "412 lines" for auto_wake | Current: 792 lines (nearly doubled; wedge-breaker, cold-start grace, setup-sentinel logic added). |
| "3982 lines" for main sync file | Current: 4415 lines (+433 lines since April-28 review). |
| No mention of `sendChatMessageToExternalAgent` vs `NotifyExternalAgentOfNewInteraction` dual-path | Divergence documented in current code; `role:"user"` is the discriminant Zed drops on. |

### vs `design/2026-06-19-acp-v2-and-websocket-sync-rewrite-strategy.md`

This document was written same day as this investigation. Its root-cause analysis matches the code exactly. No discrepancies found. It references #2642 (chat-path drop) as known but does not cite the exact Zed drop point (zed-ws:421).

---

## Smallest-First Refactor Seams

These are the concrete edit points for the `acp_thread_id` re-keying refactor. No implementation here — just the seams.

1. **`NotifyExternalAgentOfNewInteraction` (sync:1034-1038):** Remove `"role": "user"` from the `commandData` map. This is the single-line fix for #2642. The chat path would then behave identically to the queue path and not be silently dropped by Zed.

2. **`getOrCreateStreamingContext` (sync:1492-1497):** Replace `requestToInteractionMapping[requestID]` lookup with a `acp_thread_id → current_waiting_interaction` query. The function already has `helixSessionID` in scope (from `contextMappings`); a DB query for `ListInteractions(sessionID=helixSessionID, state=waiting, order=newest)` returns the interaction without needing the in-memory map. This is the chokepoint that handles the stale-id rebind bug.

3. **`handleMessageCompleted` Step 1 (sync:2570-2598):** Replace `requestToInteractionMapping[messageRequestID]` lookup with the same `acp_thread_id → current_waiting_interaction` DB query. Eliminates the consumed-sentinel mechanism (which also disappears after restart).

4. **`sendQueuedPromptToSession` (sync:3254-3264):** Stop writing to `requestToSessionMapping` and `requestToInteractionMapping`. Once Steps 2 and 3 above derive the interaction from DB, these writes become unnecessary.

5. **`streamingContexts` map key (sync:1754):** Currently keyed by `helixSessionID`. No change needed — `helixSessionID` remains the routing key for the context cache; only the *interaction lookup inside* the context changes.
