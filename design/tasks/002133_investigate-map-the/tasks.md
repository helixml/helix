# Implementation Tasks: Forensic Map of request_id Routing in WebSocket Sync

These tasks cover the investigation (done) and the minimal refactor seams identified in the forensic map. Implementation of the refactor is a separate task.

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

### #2642 — chat path `role:"user"` drop + N-notify storm (ship first)
- [ ] Remove `"role": "user"` from `NotifyExternalAgentOfNewInteraction` command data (`websocket_external_agent_sync.go:1034-1038`)
- [ ] Confirm nothing on the Helix→Zed `chat_message` path requires `role` (grep Zed `IncomingChatMessage`)
- [ ] Fix the history-storm: notify only the newly appended interaction(s), not the broken generation-boundary scan (`session_handlers.go:664-674`, `:708`); root cause is the generation rewrite at `:1018-1026`
- [ ] Live test: chat-path prompt runs an ACP turn; long-lived session fires exactly one `Notify`; queue path still works

### #2643 — reused-thread response dropped after API restart
- [ ] In `getOrCreateStreamingContext` (`:1491`), when `request_id` is empty/unmapped, make `acp_thread_id`→session→most-recent-waiting-interaction the primary resolution (session already recoverable via `contextMappings`/`findSessionByZedThreadID`)
- [ ] Make `handleMessageAdded` nil-interaction branch (`:1391-1396`) retry the `acp_thread_id` resolution instead of logging-and-dropping
- [ ] Scope guard: do NOT delete `requestToInteractionMapping` in this PR — additive fallback only
- [ ] Live test ACROSS an actual `api` restart on a reused long-lived thread: streamed content lands, interaction completes with real content, no orphan "New Conversation" session

### #2641 — stale `api` IP pinned in desktop `/etc/hosts`
Root cause is the **frozen IP**, not the lack of a route: `/etc/hosts` pin shadows the live DNS the sandbox already provides. Resolve `api` dynamically instead of freezing it (NOT a static IP — that doubles down on the snapshot, per Luke's review).
- [ ] Confirm whether desktop containers already point their resolver at the sandbox dns-proxy gateway (no `HostConfig.DNS` in `devcontainer.go`; `daemon.json` at `04-start-dockerd.sh:66-93` sets no default `dns`) — wire it if not (`HostConfig.DNS = <SANDBOX_GATEWAY>` or inner `daemon.json` `dns`)
- [ ] Have the sandbox dns-proxy (`sandbox/dns-proxy/main.go`) answer `api`/`outer-api` by live-resolving the real outer `api`
- [ ] Remove the frozen `api`/`outer-api` lines from `buildExtraHosts()` (`devcontainer.go:1100-1126`) / drop `ExtraHosts` (`:877`) so DNS wins
- [ ] Defense-in-depth (optional, bounded): self-heal recreates a desktop that still can't connect after threshold, capped by `AutoWakeCount` (`auto_wake_stuck_interactions.go:425-435`) — backstop only
- [ ] Live test: full `stop`/`start` (api gets a new IP) → surviving desktop re-resolves and reconnects WITHOUT recreation; verify no pinned IP in `/etc/hosts`
- [ ] H-in-H regression: nested desktop's `outer-api` still resolves to the real outer api

## Later: full `acp_thread_id` re-keying refactor (separate task)
- [ ] Replace `requestToInteractionMapping` lookup in `handleMessageCompleted` Step 1 (sync:2570-2598) with the `acp_thread_id` DB query
- [ ] Stop writing `requestToSessionMapping` / `requestToInteractionMapping` in `sendQueuedPromptToSession` (sync:3254-3264)
- [ ] Remove the consumed-sentinel (`""`) mechanism once duplicate-completion dedup is handled by interaction state
- [ ] Verify auto-wake re-send path (wake:603-607) works without `requestToInteractionMapping`
