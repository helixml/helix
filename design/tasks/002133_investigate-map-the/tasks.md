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

## Next: Refactor seams (separate task)

- [ ] Remove `"role": "user"` from `NotifyExternalAgentOfNewInteraction` (sync:1037) — one-line fix for #2642
- [ ] Replace `requestToInteractionMapping` lookup in `getOrCreateStreamingContext` (sync:1492-1497) with DB query for most-recent-waiting interaction keyed on `helixSessionID`
- [ ] Replace `requestToInteractionMapping` lookup in `handleMessageCompleted` Step 1 (sync:2570-2598) with same DB query
- [ ] Stop writing to `requestToSessionMapping` and `requestToInteractionMapping` in `sendQueuedPromptToSession` (sync:3254-3264) once above DB queries are in place
- [ ] Remove consumed-sentinel mechanism (`""` value in `requestToInteractionMapping`) once duplicate-completion dedup is handled by interaction state check
- [ ] Verify auto-wake re-send path (wake:603-607) still functions without `requestToInteractionMapping` (it can use the same DB-query fallback)
