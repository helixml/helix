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
- **The frontend queue is localStorage-first.** `usePromptHistory.ts` keeps a local
  history keyed by spec task (`helix_prompt_history`, cap 100) and *merges* backend rows
  into it; `refreshStatusesFromBackend` (:255) pulls backend-owned status. There is a
  reconcile rule: a locally-known entry with `syncedToBackend` that is **absent** from the
  authoritative list is surfaced as `failed`. Consequence for Decision 5: the list and the
  sync response must be scoped identically, or widening one alone makes live prompts flap
  to failed.
- **Authorization on the prompt-history endpoints is `user_id` scoping and nothing else.**
  `listPromptHistory` (`prompt_history_handlers.go:752`) does no spec-task or project
  authz — it just passes `user.ID` to the store. So `user_id` cannot simply be dropped;
  it has to be *replaced* by a real check. `deletePromptHistoryEntry` (:824) does have an
  explicit `prompt.UserID != user.ID → 403`.
- **The project authz helper** is `authorizeUserToProject(ctx, user, project, action)`
  (`api/pkg/server/authz.go:181`); the spec-task pattern is GetSpecTask → GetProject →
  authorize (see `spec_task_workflow_handlers.go:43-58`). For sessions there is
  `authorizeUserToSession` (used at `session_handlers.go:2556`).
- **`PromptHistoryEntry.UserID` is already serialised as `user_id`** in JSON
  (`api/pkg/types/prompt_history.go:14`), so the identity indicator needs no API shape
  change — only a frontend mapping.
- **`OrganizationUserAvatar`** (`frontend/src/components/widgets/OrganizationUserAvatar.tsx`)
  is the existing avatar widget; `frontend/src/utils/user.ts` has display-name helpers.
  The queue renders in `frontend/src/components/session/SessionPromptQueue.tsx` (which
  has a test file alongside it).

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

## Decision 5 — Scope the queue by task/session and authorize properly, not by `user_id`

The queue belongs to the agent, so it must show every prompt waiting for that agent. The
`user_id` predicate is removed from the *read* path — but only together with a real
authorization check, because today that predicate **is** the authorization.

**Store.** Change the signature to drop the out-of-band user argument and make user
filtering an explicit, optional part of the request:

```go
ListPromptHistory(ctx context.Context, req *types.PromptHistoryListRequest) (*types.PromptHistoryListResponse, error)
```

with `UserID string` added to `PromptHistoryListRequest` (json `-`; server-set only, never
parsed from the query string). Applied only when non-empty. There is exactly one non-test
caller, so the churn is the signature, the interface entry, and the mocks.

**Refuse to run unscoped.** Add a store-level guard: if `SpecTaskID`, `SessionID` and
`UserID` are all empty, return an error rather than every row in the table. The handler
already requires `spec_task_id` or `session_id` (:765); this is defence in depth for
future callers.

**Handler.** `listPromptHistory` gains the authz the `user_id` predicate was implicitly
providing:

- `spec_task_id` given → `GetSpecTask` → `GetProject(specTask.ProjectID)` →
  `authorizeUserToProject(ctx, user, project, types.ActionGet)`. 403 on failure. Then call
  the store with **no** `UserID` filter.
- `session_id` given (no spec task) → `GetSession` → `authorizeUserToSession(ctx, user,
  session, types.ActionGet)`, then likewise no `UserID` filter.
- Belt-and-braces: if the task/session lookup fails or returns nothing, fall back to the
  old owner-scoped behaviour rather than returning a wider set. Failure must never widen.

**Sync response must match the list.** `SyncPromptHistory`'s trailing "return all entries"
query (`store_prompt_history.go:~104`) is also `user_id`-scoped. Drop `user_id` from *that
query only*, keeping the task/session predicate, so list and sync agree — otherwise the
`syncedToBackend && !inBackendList → failed` rule in `refreshStatusesFromBackend` flips
other people's live prompts to failed in the local cache. The sync handler needs the same
authz check as the list handler, for the same reason.

**Writes stay owner-attributed.** Two guards, because the frontend merges foreign entries
into its localStorage cache and would otherwise push them back on the next sync:

1. Backend (load-bearing): in `SyncPromptHistory`'s "entry exists" branch, skip the update
   when `existingEntry.UserID != userID`. Foreign rows become read-only to other clients.
   The create branch already stamps `UserID: userID`, and the update branch never touches
   `user_id` — keep both properties.
2. Frontend: exclude entries whose owner is not the current user from the sync push
   payload, so the no-op round trips don't happen at all.

`deletePromptHistoryEntry`'s existing owner-only 403 is left exactly as is.

## Decision 6 — Identity indicator on queue entries

`user_id` is already on the wire. Add `userId?: string` to the frontend
`PromptHistoryEntry` interface and map it in `backendToLocal`
(`frontend/src/services/promptHistoryService.ts`).

In `SessionPromptQueue.tsx`, render `OrganizationUserAvatar` beside an entry when the
queue contains **more than one distinct owner**. Rationale: the single-user case is by far
the most common and an avatar on every row there is pure noise; the moment a second
participant appears, attribution becomes load-bearing and every row gets a marker
(including your own, so the set reads consistently). Name on hover via the existing
`utils/user.ts` display-name helper.

Entries not owned by the current user render without the delete affordance — the backend
would 403 anyway, so offering it would only produce a dead button.

## Files touched

| File | Change |
|---|---|
| `api/pkg/server/prompt_history_handlers.go` | `resolveSpecTaskIDForSession` helper; call it from `persistQueuedPrompt` when `specTaskID == ""` |
| `api/pkg/store/store_prompt_history.go` | `deleted_at IS NULL` on `ListPromptHistory`; drop `user_id` from the read path + unscoped-query guard; drop `user_id` from `SyncPromptHistory`'s return query; skip-foreign-row guard in its update branch; new `RevertPromptToPending` |
| `api/pkg/store/store.go` (:877, :919) | New `ListPromptHistory` signature + `RevertPromptToPending`; regenerate `store_mocks.go` and update `memorystore` |
| `api/pkg/types/prompt_history.go` | `UserID` on `PromptHistoryListRequest` (server-set, json `-`) |
| `api/pkg/server/prompt_history_handlers.go` | `resolveSpecTaskIDForSession`; project/session authz on the list **and** sync handlers |
| `api/pkg/server/websocket_external_agent_sync.go` | `errPromptBusyDeferred` sentinel; wrap the two busy returns; branch in the three drain callers |
| `api/pkg/store/migrations/0010_backfill_prompt_history_spec_task_id.{up,down}.sql` | Idempotent backfill |
| `frontend/src/services/promptHistoryService.ts` | `userId` on the entry type + `backendToLocal` mapping |
| `frontend/src/hooks/usePromptHistory.ts` | Exclude foreign-owned entries from the sync push payload |
| `frontend/src/components/session/SessionPromptQueue.tsx` (+ `.test.tsx`) | Owner avatar when the queue has >1 distinct owner; hide delete on foreign entries |

## Implementation notes (written during implementation — read these first)

Things that turned out differently from the plan. Future agents: these are the
discoveries that cost time.

1. **The spec-task queue is NOT `SessionPromptQueue.tsx`.** The design named the wrong
   component. `SessionPromptQueue.tsx` renders the queue for sessions with **no** spec
   task (org-chat / bot sessions). The spec-task page's queue is rendered by
   `RobustPromptInput.tsx` (1900 lines) via its `SortableQueueItem` subcomponent, fed by
   `usePromptHistory`. All the owner-attribution work landed there. Its file header says
   so explicitly — read it before assuming.
2. **There is only ONE busy-defer return site**, not two as the design claimed:
   `websocket_external_agent_sync.go`, inside `sendQueuedPromptToSession`. The other
   `MarkPromptAsFailed` calls nearby are genuine failures (paused-session drop, real
   dispatch failure) and were correctly left alone.
3. **The three drain callers now share one classifier**, `requeueUndispatchedPrompt`,
   rather than each doing its own `errors.Is` branch. This was not in the design but is
   strictly better: the defer-vs-failure decision cannot drift between the three paths.
   Note `processInterruptPrompt` lives in `prompt_history_handlers.go:555`, not in
   `websocket_external_agent_sync.go` — easy to miss.
4. **The planned "exclude foreign entries from the sync push payload" is unnecessary.**
   `mergeWithBackend` already stamps backend-originating entries with
   `syncedToBackend: true`, and `syncToBackend` only pushes `!h.syncedToBackend` entries,
   so foreign rows are never in the payload. The one path that could push one was the
   interrupt-toggle at `usePromptHistory.ts:659` — closed by hiding that affordance on
   foreign entries. The backend `existingEntry.UserID != userID` guard remains the
   load-bearing protection; the frontend just doesn't offer the action.
5. **Fail-fast over silent fallback, per CLAUDE.md.** Both the design's "degrade to empty
   spec_task_id on lookup error" and its "fall back to owner-scoped on authz lookup
   failure" were dropped. `resolveSpecTaskIDForSession` propagates store errors, and
   `authorizeUserToPromptQueue` returns 404/403. CLAUDE.md's "NO FALLBACKS — one
   approach, fix properly, no dead code paths" applies, and a spec-task query failing
   while the very next statement writes to the same DB is not a case worth papering over.
6. **`resolveOrganizationUser` already exists** in `OrganizationUserAvatar.tsx` and is
   exported — use it for the hover label rather than re-resolving members by hand.
7. **Frontend deps are not installed on this box** (`frontend/node_modules` absent) and
   `npm` fails on a root-owned cache. Typecheck by mounting the source into the
   `helix-frontend` image, which carries its own `node_modules` with `tsc`:
   `docker run --rm -v "$PWD":/src -w /src --entrypoint sh helix-frontend -c 'ln -sfn /app/node_modules /src/node_modules; ./node_modules/.bin/tsc --noEmit -p tsconfig.json'`
   **Delete the `node_modules` symlink afterwards** — the bind mount writes it to the host.

## CI failure and fix (2026-09-01)

The first push went red. Cause: `persistQueuedPrompt` now calls `ListSpecTasks`, and the
existing gomock tests in `session_messages_handler_test.go` had no expectation for it —
`TestEnqueuesOntoQueue` and `TestNotifyUserIDCarriedOnRow` aborted with
"there are no expected calls of the method \"ListSpecTasks\"".

**This was avoidable and is the direct cost of pushing without running the tests.**
`go build ./...` does not compile test files, so a green build said nothing about them.
Lesson for anyone extending an existing handler with a new store call: every gomock suite
that exercises that handler needs the new `EXPECT()`, and only `go test` reveals it.

Running the suite locally needs `sudo apt-get install -y gcc libc6-dev` (tree-sitter needs
CGo). Note that once gcc is present, plain `go build ./...` starts failing on the go-gst
GStreamer bindings for want of `pkg-config` — that is environmental, not a code problem;
build with `CGO_ENABLED=0`.

The fix also added the regression tests that should have been there from the start:
`TestStampsSpecTaskIDFromPlanningSession`, and `TestSpecTaskLookupErrorFailsEnqueue`
pinning the fail-fast decision from note 5.

## Environment blocker (2026-09-01)

The live browser verification in `tasks.md` was **not performed**. The session's startup
script failed before the stack came up: it builds Zed from source and `rustc` was
OOM-killed (`signal: 9, SIGKILL`) compiling the `project` crate, so the script exited 1
(`❌ Error: Failed to build Zed IDE`) and no containers were started.

`postgres`, `api` and `frontend` were subsequently brought up by hand, but a LIVE Zed
session — which the test plan requires, and which needs the desktop image that depends on
that Zed binary — was not reachable. The user then directed that the change be merged
without testing. Everything unverified is listed explicitly in `pull_request_helix.md`;
nothing has been reported as verified that was not.

## Risks

- **Mock regeneration.** Adding a `Store` interface method requires regenerating gomock
  (`api/pkg/store/mocks`) or the build breaks across many test packages.
- **Interrupt path regression.** Any change near :3745-3800 risks the boot-race incident.
  The sentinel change is deliberately confined to the error *value*, not the control flow.
- **Backfill scope on meta.** The UPDATE touches every historical row with an empty
  `spec_task_id` that has a matching planning session, not just Luke's 53. That is the
  intent (those rows were all mis-stamped by the same bug), but count them first.
- **Read hole (Decision 5) — the highest-risk item in this task.** Dropping `user_id`
  without landing the authz check in the same commit exposes any queue to anyone who
  knows a `spec_task_id`. Do the two changes together, and make the fallback on a failed
  task/project lookup *narrow* (owner-scoped), never wide. The two-account browser check
  (authorized sees all, unauthorized sees none) is the acceptance gate.
- **List/sync scoping drift.** Widening the list but not the sync response makes the
  frontend mark other users' live prompts as `failed` via the
  `syncedToBackend && !inBackendList` rule. Change both or neither.
- **`ListPromptHistory` signature change** ripples through the `Store` interface, the
  gomock mocks, and `memorystore`.
