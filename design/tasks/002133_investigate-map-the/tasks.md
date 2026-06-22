# Implementation Tasks: Forensic Map of request_id Routing in WebSocket Sync

These tasks cover the investigation (done) and the fix for the three restart-surfaced bugs (#2641, #2642, #2643), including the full `acp_thread_id` re-key — all shipping in a single PR (per Luke's review).

## Investigation (completed)

- [x] Read `api/pkg/server/server.go` — confirm struct fields for all correlation maps
- [x] Read `api/pkg/server/websocket_external_agent_sync.go` (4415 lines) — trace all map write/read/delete sites, identify chokepoints
- [x] Read `api/pkg/server/auto_wake_stuck_interactions.go` (792 lines) — document trigger, re-send logic, retry cap, state transitions
- [x] Read `api/pkg/server/external_agent_handlers.go` — confirm `RegisterRequestToSessionMapping` and chat-path divergence
- [x] Read `api/pkg/server/spec_task_design_review_handlers.go` — confirm commenter map guards and write sites
- [x] Read `zed/crates/external_websocket_sync/src/websocket_sync.rs` — locate `role:"user"` drop at line 421 (#2642)
- [x] Read `zed/crates/external_websocket_sync/src/thread_service.rs` — confirm Zed-side global registries
- [x] Cross-reference `design/2026-04-28-websocket-sync-architecture-review.md` — flag `interactionToPromptMapping` deletion discrepancy
- [x] Cross-reference `design/2026-06-19-acp-v2-and-websocket-sync-rewrite-strategy.md` — confirm alignment

## Deliverable

- [x] Write forensic map (`design.md`) answering all 8 questions with file:line citations
- [x] Confirm restart-survival matrix (Q8): `requestToSessionMapping` and `requestToInteractionMapping` are in-memory-only, lost on restart
- [x] Identify dual-delivery drop point: `NotifyExternalAgentOfNewInteraction` adds `role:"user"`, Zed drops at `websocket_sync.rs:421`

## Fix the three restart-surfaced bugs (#2641, #2642, #2643)

See `design/2026-06-19-fix-restart-surfaced-websocket-bugs.md` for the full fix design, rationale, and test plans. Implementation is the next phase (these are NOT done yet).

**Single PR (per Luke's review).** All three fixes plus the full `acp_thread_id` re-key land in one PR. The re-key is folded into #2643 (no longer a "later" task) because it is the structural fix for the common cause — the restart-survival matrix shows `acp_thread_id`/`ZedThreadID` is the only DB-persisted correlation state; everything else is in-memory and dies on restart. Build internally in the order below (smallest → core → independent subsystem) so each layer is verifiable before the next stacks.

### #2642 — chat path `role:"user"` drop + N-notify storm (build first)
- [x] Remove `"role": "user"` from `NotifyExternalAgentOfNewInteraction` command data (`websocket_external_agent_sync.go:1033-1041`)
- [x] Confirm nothing on the Helix→Zed `chat_message` path requires `role` — Zed `IncomingChatMessage.role` is `Option<String>` ("can be ignored", `types.rs:355`), only read by the echo-drop check (`websocket_sync.rs:421`); unused after. No Zed change needed.
- [x] Fix the history-storm: notify only the last (newly appended) interaction (`session_handlers.go:661-679`). Root cause confirmed: `appendOrOverwrite` always appends exactly one new interaction as the last element; the generation-boundary scan fails when all interactions share the current generation.
- [ ] Live test: chat-path prompt runs an ACP turn; long-lived session fires exactly one `Notify`; queue path still works — **BLOCKED:** inner dev env cannot provision a live Zed desktop (startup `build-sandbox`/`build-ubuntu` failed on an unrelated qwen-code `npm run bundle` error → no `helix-ubuntu` image in inner dockerd → desktop never launches → Zed never connects). Verified offline only (build + Zed-side code read). Needs a working env before merge.

> **ENV NOTE (2026-06-22):** While bringing up the stack I also observed #2641 live: the sandbox's hydra failed to reach `api` at a stale IP (`10.214.1.10:8080`, connection refused) after the restart before recovering RevDial — exactly the stale-pin failure class. Recorded as supporting evidence for the #2641 fix.

### #2643 + full `acp_thread_id` re-key — the core change (build second)
Make `acp_thread_id` the **primary routing key** behind one swappable chokepoint; demote `request_id` to an in-thread disambiguator + dedup. This fixes #2643 and removes the in-memory correlation that dies on restart.
- [ ] Chokepoint: `getOrCreateStreamingContext` (`:1491`) resolves `acp_thread_id`→session (via `contextMappings`/`findSessionByZedThreadID`, persisted `ZedThreadID`)→most-recent-`waiting` interaction (`:1636-1643`) as the **primary** path — no in-memory map dependence
- [ ] `handleMessageAdded` nil-interaction branch (`:1391-1396`) resolves via the chokepoint instead of logging-and-dropping
- [ ] Replace `requestToInteractionMapping` lookup in `handleMessageCompleted` Step 1 (`:2570-2598`) with the chokepoint resolution
- [ ] Stop writing `requestToSessionMapping` / `requestToInteractionMapping` in `sendQueuedPromptToSession` (`:3254-3264`)
- [ ] Replace consumed-sentinel (`""`) dedup with an interaction-state check (a `completed` interaction rejects a second completion) — don't rip out dedup blindly
- [ ] Verify auto-wake re-send (`auto_wake_stuck_interactions.go:603-607`) routes through the chokepoint
- [ ] Keep turn-state (running/done/waiting) behind the chokepoint so ACP v2 `state_update` can swap the source later
- [ ] If any routing-relevant inbound event lacks `acp_thread_id`, fall back to its request_id binding for that event only and log loudly (confirm during impl)
- [ ] Live test ACROSS an `api` restart on a reused thread; dedup (duplicate completion rejected); concurrent/sequential turns on one thread; queue + auto-wake; `TestWebSocketSyncSuite` + `run_docker_e2e.sh` (both agents)

### #2641 — stale `api` IP pinned in desktop `/etc/hosts` (build third; independent subsystem)
Root cause is the **frozen IP**, not the lack of a route: `/etc/hosts` pin shadows the live DNS the sandbox already provides. Resolve `api` dynamically instead of freezing it (NOT a static IP — that doubles down on the snapshot, per Luke's review).
- [ ] Confirm whether desktop containers already point their resolver at the sandbox dns-proxy gateway (no `HostConfig.DNS` in `devcontainer.go`; `daemon.json` at `04-start-dockerd.sh:66-93` sets no default `dns`) — wire it if not (`HostConfig.DNS = <SANDBOX_GATEWAY>` or inner `daemon.json` `dns`)
- [ ] Have the sandbox dns-proxy (`sandbox/dns-proxy/main.go`) answer `api`/`outer-api` by live-resolving the real outer `api`
- [ ] Remove the frozen `api`/`outer-api` lines from `buildExtraHosts()` (`devcontainer.go:1100-1126`) / drop `ExtraHosts` (`:877`) so DNS wins
- [ ] Defense-in-depth (optional, bounded): self-heal recreates a desktop that still can't connect after threshold, capped by `AutoWakeCount` (`auto_wake_stuck_interactions.go:425-435`) — backstop only
- [ ] Live test: full `stop`/`start` (api gets a new IP) → surviving desktop re-resolves and reconnects WITHOUT recreation; verify no pinned IP in `/etc/hosts`
- [ ] H-in-H regression: nested desktop's `outer-api` still resolves to the real outer api
