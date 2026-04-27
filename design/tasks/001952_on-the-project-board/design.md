# Design

## Summary

Compute `agent_work_state` server-side in the existing `listTasks` enrichment loop, and use it on the frontend to drive a more honest "In Progress vs Idle" indicator on each task card. No new database tables, no new websocket events, no column changes.

## Key Decisions

### Derive, don't store

The unimplemented `2025-12-22-external-agent-state-reconciliation` design proposed a new `external_agent_activity` table with a stored `agent_work_state` column, plus writes in `handleMessageCompleted`, `NotifyExternalAgentOfNewInteraction`, and `MarkTaskDone`. That's the right long-term shape for the reconciliation/continue-prompt feature it was scoped for, but it is overkill for "show the right indicator on the card."

We already have everything we need:

- `Session.Updated` is touched on every `message_completed` (`api/pkg/server/websocket_external_agent_sync.go:2427`).
- `Interaction.State` is set to `InteractionStateComplete` when the agent finishes a turn (line 2397). Until then it sits in `InteractionStateWaiting`.
- `SandboxState` (`absent` / `starting` / `running`) is already computed in the same `listTasks` enrichment loop (`api/pkg/server/spec_driven_task_handlers.go:300-313`).

Compute `AgentWorkState` from those three. It costs one extra batch lookup of the latest interaction per session in the `listTasks` loop that already runs. If/when the larger reconciliation system lands, it can replace the derivation with a stored value without changing the API contract.

### Where to derive it

Put the derivation in the same enrichment block at `api/pkg/server/spec_driven_task_handlers.go:258-318` that already populates `SessionUpdatedAt` and `SandboxState`. Add a single extra batched call to fetch the latest interaction per planning session ID, then set `task.AgentWorkState` per task.

We need a new (or existing) batched store method like `GetLatestInteractionsForSessions(ctx, sessionIDs []string) (map[string]*types.Interaction, error)`. Check `api/pkg/store/store_interactions.go` first — there may already be one. If not, add it; the implementation is a single SQL query: `SELECT DISTINCT ON (session_id) * FROM interactions WHERE session_id = ANY(?) ORDER BY session_id, created_at DESC`.

### State machine

```
SandboxState   Latest interaction      → AgentWorkState
absent         (any)                   → "" (empty; UI shows sandbox hint)
starting       (any)                   → "" (empty; UI shows "Starting…")
running        Waiting / streaming     → "working"
running        Complete / Error / nil  → "idle"
(any)          task.Status in {implementation_review, pull_request, done}
                                       → "done"
```

The `done` branch is mostly a safety net — those phases already render their own dot/label and don't show the timer at all. We set it for API consistency.

### Frontend

Change `TaskCard.tsx`:

1. Replace the `task.status === "implementation"` predicate at `frontend/src/components/tasks/TaskCard.tsx:602-605` with `task.agent_work_state === "working"`. The hook already returns `null` when `enabled` is false, so the timer disappears the moment the backend reports the agent has gone idle.
2. In the status row at `frontend/src/components/tasks/TaskCard.tsx:949-997`, when `task.phase === "implementation"`, choose the label based on `agent_work_state`:
   - `"working"` → `In Progress` (today's label)
   - `"idle"` → `Idle`
   - empty + `SandboxState === "absent"` → `Sandbox stopped`
   - empty + `SandboxState === "starting"` → `Starting…` (use existing `SandboxStatusMessage` if present)
   - everything else → fall back to `In Progress` so we never show worse than today during the rollout.
3. Keep the dot color logic exactly as it is (still green during `implementation`). Only the label and the timer change. We can later add a subtle pulse on the dot when working — out of scope for v1.

The kanban already polls every 30s (`frontend/src/components/tasks/AgentKanbanBoard.tsx`), which is the cadence at which `agent_work_state` will refresh. That is enough; users perceive "agent stopped working" as a soft signal, not an emergency.

### API client regeneration

The Go field already exists (`api/pkg/types/simple_spec_task.go:151`) and is in swagger. No type changes are needed — but the field is currently never set, so the frontend type is correct. Run `./stack update_openapi` only if the swagger output changes (it shouldn't).

## Trade-offs

- **Polling vs push.** Push (websocket "agent_work_state_changed" events) would be snappier but requires plumbing through the existing `WebsocketEventType` system and changes to the frontend `useTaskUpdates` hook. The 30s polling latency is acceptable for a status indicator and matches the rest of the board's cadence. Defer push to a follow-up if users complain.
- **Per-task DB query vs batched.** Don't do per-task — there can be tens of in-flight tasks. The single batched `latest interaction per session` query keeps `listTasks` performance flat.
- **`Idle` vs `Awaiting review` label.** The agent doesn't actually know whether the human is reviewing or whether the agent is between turns. `Idle` is the honest description of what we can detect; `Awaiting review` would imply more semantic certainty than the data supports. If the user later wants `Awaiting review` specifically, we'd need to detect "agent has explicitly emitted a final/done signal" — currently no such signal exists.

## Files Touched

Backend:
- `api/pkg/server/spec_driven_task_handlers.go` — add `AgentWorkState` derivation in the enrichment loop (~30 lines).
- `api/pkg/store/store_interactions.go` (or wherever the interaction queries live) — add a batched "latest interaction per session" lookup if one doesn't exist.
- Tests: a small unit test of the derivation function with the four state combos.

Frontend:
- `frontend/src/components/tasks/TaskCard.tsx` — change the timer-enabled predicate (line ~604) and the label/state rendering in the status row (lines ~949-997).

No migrations. No new tables. No new endpoints.

## Notes for Future Implementers

- The existing `AgentWorkState` Go type and the swagger entry already exist — they were left over from the unimplemented 2025-12-22 design. Reuse them. Do not invent new constants.
- Don't add a write path in `handleMessageCompleted` to update a stored field. The whole point of this design is to avoid the database churn from the larger reconciliation system. Derivation in `listTasks` is enough for v1.
- If you find yourself wanting a separate websocket event for "agent went idle", stop and check whether 30s polling is actually painful in practice first.
- The CLAUDE.md rule "ALWAYS use generated API client" applies — `agent_work_state` is already in the generated types (`frontend/src/api/api.ts:1538`), no client regen needed.
