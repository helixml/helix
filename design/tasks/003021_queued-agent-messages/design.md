# Design: Make API-Queued Agent Messages Visible in the Prompt Queue

## Codebase notes (verified 2026-09-01, for future agents)

Everything below was read in `/home/retro/work/helix/` at the time of writing.

- **The single send path.** All server-side agent sends funnel through
  `enqueueAgentMessage` (`api/pkg/server/prompt_history_handlers.go:408`) →
  `persistQueuedPrompt` (:423) → `nudgeSessionQueue` (:465). The old
  immediate-dispatch path (`sendChatMessageToExternalAgent`) was removed because it
  had no busy check. Do not reintroduce a bypass.
- **Three callers**, only one of which is broken:
  | Caller | `specTaskID` passed |
  |---|---|
  | `session_handlers.go:2571` (generic session-messages API) | `""` ← **the bug** |
  | `spec_task_design_review_handlers.go:1753` | `task.ID` |
  | `spec_tasks_org_wiring.go:41` | `task.ID` |
- **Reverse lookup already exists.** `ListSpecTasks` supports a `PlanningSessionID`
  filter (`api/pkg/store/store_spec_tasks.go:469` → `planning_session_id = ?`). No new
  store method or index is needed for the resolution.
- **Queue drains are serialised per session** by `lockPromptDrain`
  (`websocket_external_agent_sync.go:3540`). Held across claim → cancel → send →
  CreateInteraction. Three drain entry points: `processPromptQueue` (:3561, has a busy
  pre-check), `processInterruptPrompt` (:3610), `processAnyPendingPrompt` (:3651, has
  **no** busy pre-check).
- **Claim is atomic inside the selector.** `GetNextPendingPrompt` /
  `GetAnyPendingPrompt` / `GetNextInterruptPrompt` set `status='sending'` as part of
  selection. Callers must NOT then call `ClaimPromptForSending` (there is a comment
  warning about this — it silently drops every prompt).
- **`sendQueuedPromptToSession`'s busy re-check** lives at
  `websocket_external_agent_sync.go:~3771` inside a large comment block (:3745-3800)
  that encodes several past incidents: the interrupt exemption, the ASC/DESC
  `ListInteractions` ordering bug (fixed in 853492e14), and the boot-race exception
  where an interrupt dispatched before `ZedThreadID` is populated makes Zed fork a
  second divorced thread. **Read that block before touching anything there.**
- **The busy-defer is signalled as a plain error string** today:
  `fmt.Errorf("session %s became busy (interaction %s is Waiting), deferring queue prompt", …)`.
  Callers cannot distinguish it from a real failure — that is the whole of secondary bug 2.
- **`MarkPromptAsFailed`** (`store_prompt_history.go:353`) reads the row, increments
  `retry_count`, and sets exponential-backoff `next_retry_at` (2,4,8,16, capped 30s).
- **Retry cap** `defaultMaxPromptQueueRetries = 20` (`store_prompt_history.go:24`),
  overridable via `HELIX_MAX_PROMPT_QUEUE_RETRIES`. The selectors apply
  `retry_count < ?`, so hitting the cap makes the row permanently unselectable.
- **Frontend is correct and needs no change.** `usePromptHistory.ts:260` calls
  `listPromptHistory(apiClient, specTaskId, { projectId })`; it was only ever being
  handed an empty result set.

## Decision 1 — Stamp `spec_task_id` at write time, inside `persistQueuedPrompt`

Add an unexported helper in `prompt_history_handlers.go`:

```go
// resolveSpecTaskIDForSession returns the id of the spec task that owns sessionID
// via its planning_session_id, or "" when the session is a plain (non-spec-task)
// session. Never returns an error: a failed lookup degrades to "" so that a plain
// org-chat send is never broken by a spec-task query problem.
func (apiServer *HelixAPIServer) resolveSpecTaskIDForSession(ctx context.Context, sessionID string) string
```

It calls `Store.ListSpecTasks` with `PlanningSessionID: sessionID`, `Limit: 2`, and
`IncludeArchived: true` (an archived task's queue must still render). Zero results → `""`.
More than one → take the first and `log.Warn()` the ambiguity.

`persistQueuedPrompt` calls it **only when the caller passed an empty `specTaskID`**, so
the two callers that already know their task pay nothing and keep their explicit value:

```go
if specTaskID == "" {
    specTaskID = apiServer.resolveSpecTaskIDForSession(ctx, sessionID)
}
```

**Why here rather than in `session_handlers.go:2571`.** One fix point covers the broken
handler and any future caller that forgets the argument, and it keeps the handler thin.
The cost is one indexed query per API-path enqueue, on a path that already does
`GetSession` + an insert.

**Why not loosen the query filter.** Explicitly rejected in the brief and correct: an
`OR spec_task_id = ''` predicate would leak unrelated sessions' queues onto a task page.
The row genuinely belongs to that spec task; `ListPromptHistoryBySpecTask` and the
design-review linkage all benefit from the column being right.

## Decision 2 — Backfill via an idempotent SQL migration

`api/pkg/store/migrations/0010_backfill_prompt_history_spec_task_id.up.sql`:

```sql
UPDATE prompt_history_entries p
SET spec_task_id = t.id
FROM spec_tasks t
WHERE p.session_id = t.planning_session_id
  AND (p.spec_task_id IS NULL OR p.spec_task_id = '')
  AND p.deleted_at IS NULL;
```

Idempotent by construction (the `WHERE` no longer matches after the first run) and it
never overwrites a non-empty value. Follows the existing `0006_backfill_spec_task_assignees`
precedent in the same directory. The `.down.sql` is a documented no-op — the pre-update
empty strings cannot be reconstructed, and reverting them would only restore the bug.

This makes Luke's 53 rows visible on the next deploy without re-sending anything. Confirm
the row count before/after on meta and state it in the PR.

## Decision 3 — Soft-delete predicate on `ListPromptHistory`

`api/pkg/store/store_prompt_history.go:509` — add `.Where("deleted_at IS NULL")` to the
base query, before the `Count`, so both the returned entries and `total` exclude deleted
rows. This matches `ListPromptHistoryBySpecTask` (:256) and `ListPromptHistoryBySession`
(:274). One line; no other call-site changes.

## Decision 4 — Busy-defer is not a failure: option (b), a typed sentinel error

**Chosen: (b) distinguish the busy-defer from a real error.** Rejected (a) adding a busy
pre-check to `processAnyPendingPrompt`.

Why (b):

- `processAnyPendingPrompt` claims **any** prompt including interrupts, and interrupts
  are deliberately exempt from the busy check. A blanket pre-check there would break the
  interrupt path; a conditional one would have to peek at the prompt's `interrupt` flag
  *before* the atomic claim, which the selector API does not allow without restructuring
  the claim.
- A pre-check only fixes the one caller. The sentinel fixes **every** defer path with one
  change — including the boot-race case where an *interrupt* is deferred because
  `ZedThreadID` is still empty, which today also burns retry budget.
- It requires no change to the delicate ordering logic in the :3745-3800 comment block —
  only the error value returned from the two `return fmt.Errorf("... became busy ...")`
  sites and how callers classify it.

Implementation:

```go
// errPromptBusyDeferred signals that a queued prompt could not be dispatched because
// the session is mid-turn. This is the system working as designed, not a failure:
// callers must return the prompt to 'pending' WITHOUT incrementing retry_count, which
// exists to bound genuine failures. Burning it on defers silently drops the message at
// the cap (see design/tasks/003021).
var errPromptBusyDeferred = errors.New("prompt dispatch deferred: session busy")
```

- `sendQueuedPromptToSession` wraps its busy return with `%w` so the detail message and
  logs are unchanged: `fmt.Errorf("session %s became busy (interaction %s is Waiting), deferring queue prompt: %w", …, errPromptBusyDeferred)`.
- All three drain callers (`processPromptQueue`, `processInterruptPrompt`,
  `processAnyPendingPrompt`) branch on `errors.Is(err, errPromptBusyDeferred)` and call a
  new store method instead of `MarkPromptAsFailed`, logging at Info (a defer is normal),
  not Error.

New store method:

```go
// RevertPromptToPending returns a claimed prompt to the queue after a non-failure
// defer: status='pending', next_retry_at=NULL, error_message=''. retry_count is
// deliberately left untouched — a defer neither charges nor forgives the budget.
func (s *PostgresStore) RevertPromptToPending(ctx context.Context, promptID string) error
```

It must clear `next_retry_at` (a stale backoff sentinel from an earlier genuine failure
would otherwise gate re-selection) and `error_message` (so the UI stops showing
"Failed - retrying" for a prompt that is merely waiting). `MarkPromptAsPending` is not
reused because it only sets `status` and would leave both fields stale.

**No busy-wait risk:** drains are event-driven (`nudgeSessionQueue`, `message_completed`,
cancel-ack) plus the existing idle poller — nothing spins on a `pending` row, and
`processPromptQueue`'s pre-check defers cheaply before claiming. Verify during
implementation that `GetNextPendingPrompt`'s selector treats a `pending` row with
`next_retry_at IS NULL` as immediately eligible (it should — that is the fresh-enqueue case).

## Files touched

| File | Change |
|---|---|
| `api/pkg/server/prompt_history_handlers.go` | `resolveSpecTaskIDForSession` helper; call it from `persistQueuedPrompt` when `specTaskID == ""` |
| `api/pkg/store/store_prompt_history.go` | `deleted_at IS NULL` on `ListPromptHistory`; new `RevertPromptToPending` |
| `api/pkg/store/store.go` (or the store interface file) | Add `RevertPromptToPending` to the `Store` interface + regenerate mocks |
| `api/pkg/server/websocket_external_agent_sync.go` | `errPromptBusyDeferred` sentinel; wrap the two busy returns; branch in the three drain callers |
| `api/pkg/store/migrations/0010_backfill_prompt_history_spec_task_id.{up,down}.sql` | Idempotent backfill |
| Frontend | **None** |

## Risks

- **Mock regeneration.** Adding a `Store` interface method requires regenerating gomock
  (`api/pkg/store/mocks`) or the build breaks across many test packages.
- **Interrupt path regression.** Any change near :3745-3800 risks the boot-race incident.
  The sentinel change is deliberately confined to the error *value*, not the control flow.
- **Backfill scope on meta.** The UPDATE touches every historical row with an empty
  `spec_task_id` that has a matching planning session, not just Luke's 53. That is the
  intent (those rows were all mis-stamped by the same bug), but count them first.
