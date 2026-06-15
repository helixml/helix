# Design: Show Booting State Immediately When Chatting to an Idle Spec-Task Session

## Summary

The current "immediate spinner" mechanism is racing itself. The
frontend writes `external_agent_status="starting"` into the React
Query cache, then *in the same tick* invalidates the query, which
triggers a refetch against a backend that has not yet been told to
boot anything. The refetch returns the stale `stopped` row and
overwrites the optimistic write.

Two complementary fixes are proposed. Either one alone resolves the
visible regression; doing both removes the race in both directions and
keeps the cache, the backend, and the next poll mutually consistent.

- **Fix A (frontend, required):** stop self-invalidating in
  `optimisticallyMarkSessionStarting`. The 3s poll catches up
  naturally; the optimistic write survives until then.
- **Fix B (backend, required for consistency):** make
  `syncPromptHistory` synchronously mark the canonical session's
  `external_agent_status="starting"` (with a
  `status_message="Starting Desktop..."`) *before* returning, when
  there is no live WebSocket. The async wake goroutine that already
  exists takes over from there.

With both fixes in place, even a refetch that happens to land just
after the send (e.g. triggered by a different consumer's
`invalidateQueries`, a window-focus refetch, or a manual reload)
returns a session whose status agrees with the optimistic write.

## Architectural slice (current state, verified 2026-05-22)

```
SpecTaskDetailContent.tsx:1984           SpecTaskDetailContent.tsx:2783
  ┌─ RobustPromptInput ──────┐               ┌─ RobustPromptInput ──┐
  │  onWillSend = handleWillSend            │  onWillSend = handleWillSend
  └─────────────┬────────────┘               └────────────┬─────────┘
                ▼                                          ▼
       handleWillSend (line 576)                handleWillSend (line 576)
                │
                ▼
    optimisticallyMarkSessionStarting(queryClient, activeSessionId)
       ├─ setQueryData(['session', id, 'full'], …'starting'…)
       ├─ setQueryData(['session', id, 'skip'], …'starting'…)
       └─ queryClient.invalidateQueries(['session', id])    ◀── kicks refetch
                                                                 │
                                                                 ▼
                                                       GET /api/v1/sessions/{id}
                                                                 │
                                                                 ▼
                                                    returns external_agent_status="stopped"
                                                                 │
                                                                 ▼
                                                    React Query replaces cache
                                                                 │
                                                                 ▼
                                                  useSandboxState → isStarting=false
                                                                 │
                                                                 ▼
                                                    "Desktop Paused" snaps back
```

Meanwhile, in parallel:

```
RobustPromptInput.handleSend (line 694-714)
  └─ saveToHistory()
       └─ syncEntryImmediately()
            └─ POST /api/v1/prompt-history/sync
                  └─ syncPromptHistory (handler)
                       ├─ Store.SyncPromptHistory   ← persists prompt
                       └─ go processPendingPromptsForIdleSessions(specTaskID)
                            └─ (eventually) processPromptQueue
                                 └─ sendChatMessageToExternalAgent
                                      └─ sendCommandToExternalAgent
                                           └─ go autoStartDevContainerForSession
                                                └─ StartDesktop
                                                     └─ writes external_agent_status="starting" to DB
```

The DB write of `starting` does not happen until several goroutine
hops and (typically) a Docker/exec round-trip after the request has
returned. The refetch wins; the spinner flickers off.

## Fix A — Remove the self-invalidate

`frontend/src/utils/optimisticSessionStarting.ts:50-53` ends with:

```ts
// Belt-and-braces: a prefix-matching invalidate kicks the next poll a bit
// earlier than waiting for the 3s tick.
queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sessionId) })
```

This is the line that actively dismantles the optimistic write. The
3s poll from `useSandboxState` (`refetchInterval: 3000`) catches up
on its own — well within human "spinner is doing something" patience
— and the optimistic write survives until then.

**Change:** delete the `invalidateQueries` call. Keep the two
`setQueryData` writes.

The comment must be updated to reflect the new contract: "We do not
invalidate — the next 3s poll will reconcile, by which time the
backend's wake goroutine has had time to write `starting` to the DB."

### Why this alone is not enough

Other code paths can still trigger a refetch within the race window
(window focus, manual reload, an unrelated component invalidating the
session). On any such refetch, the cached "starting" is overwritten by
the still-stale backend row. Hence Fix B.

## Fix B — Mark the session `starting` synchronously in `syncPromptHistory`

`api/pkg/server/prompt_history_handlers.go:29-68` already knows the
spec_task_id of the incoming sync and already fires
`processPendingPromptsForIdleSessions` in a goroutine. It does NOT
write anything to the session's `external_agent_status` before
returning.

**Change:** before firing the goroutine, do a cheap synchronous check:

1. Resolve the canonical session for the spec task (we already do
   this inside `processPendingPromptsForIdleSessions` via
   `Store.GetSpecTask(...).PlanningSessionID`). Lift that lookup up
   so the handler has the session ID.
2. Look up the live WS state for that session via
   `apiServer.externalAgentWSManager.getConnection(sessionID)`.
3. If `!connected`, also fetch the session row. If its
   `external_agent_status` is anything other than `"running"` or
   `"starting"`, update it (column-level update only — not a full
   `Save`, to avoid clobbering streaming-path writes per the existing
   `auto_wake_stuck_interactions.go:75-86` comment) to:
   - `external_agent_status = "starting"`
   - `status_message = "Starting Desktop..."`
4. Then fire the goroutine exactly as today.

Net effect: by the time the frontend's first refetch arrives, the row
already says `starting`. The optimistic write and the backend row
agree; React Query happily replaces the cache with an equivalent
value. No flicker.

### Why this is the right level to put the synchronous mark

- It is the boundary between "user pressed send" and "the long async
  wake-up chain runs". The handler holds the request context, has DB
  access, and runs before any goroutine.
- Placing it deeper (inside `autoStartDevContainerForSession`) leaves
  the race in place because that helper still runs in a goroutine.
- Placing it shallower (inside the frontend) is exactly what Fix A
  already covers — necessary but not sufficient when *other* refetches
  fire in the window.
- The CLAUDE.md rule "root-cause the issue" applies: the absence of a
  synchronous "wake-intent registered" backend signal is the root
  cause; this fix adds it.

### Idempotency and safety

- The "no-op if already running/starting" check in the handler
  mirrors the existing guard in
  `optimisticallyMarkSessionStarting:30-34`. They agree on the
  invariant.
- The column update touches only `external_agent_status` and
  `status_message`. It will not clobber `desired_state`,
  `container_name`, or anything the streaming path writes.
- If the wake goroutine subsequently fails (e.g. no project
  context), the existing `auto_wake_stuck_interactions.go` retry
  bounded by `autoWakeMaxRetries` already marks the interaction
  `state=error`. We will additionally need the worker to *reset*
  `external_agent_status` from `"starting"` to `"stopped"` after the
  retries are exhausted, so the spinner doesn't sit forever on a
  permanently-broken session. See "Failure mode" below.

## Fix B' — Failure mode (paired with B)

If a session is marked `starting` by Fix B but the goroutine chain
fails or never delivers, today the only correction is a successful
`StartDesktop` writing `running` or the existing cold-start sweep.
After Fix B, we additionally need:

- In `auto_wake_stuck_interactions.go`, when
  `autoWakeMaxRetries` is hit and the interaction is marked
  `state=error`, also column-update the session's
  `external_agent_status` back to `"stopped"` (and
  `status_message=""`) if it is currently `"starting"`. Same
  targeted update pattern as the other column writes in that file.

Without this, a permanently-broken session shows a perpetual
"Starting Desktop..." spinner instead of "Desktop Paused" — a
regression in the failure UX.

## Decisions and trade-offs

**Why not just delete the `invalidateQueries` call and stop there?**
Because other consumers (`useGetSession` in different components,
window-focus refetch, manual `invalidateQueries` from unrelated
flows) can still trigger a refetch that overwrites the optimistic
write in the race window. Pre-marking the row in the backend closes
that hole.

**Why not move the wake-up out of a goroutine and make
`syncPromptHistory` synchronous end-to-end?** That would couple the
HTTP response time to a Docker/exec round-trip (often ≥ 5s) and
change the semantics of an endpoint many other callers depend on.
The synchronous "mark intent" + async "do the work" split is the
established pattern in this codebase
(`autoStartDevContainerForSession` itself runs in a goroutine from
`sendCommandToExternalAgent`).

**Why not use a WebSocket push to tell the frontend "I have
acknowledged the send"?** The frontend doesn't reliably have a
session WebSocket open in the "paused" state (that's the whole
point — the desktop is not running). The HTTP response is the
synchronous channel we already have; using it is cheaper than
introducing a new push.

**Why both fixes A and B instead of just B?** The
`invalidateQueries` in the helper still triggers a needless refetch
even after B (which just makes the refetch return the right value).
Removing it is a small simplification that also makes the helper's
behaviour match its comment ("synchronously flip the cached session
config"). Belt-and-braces was always going to be brittle; the comment
admits as much.

**Why not retire the optimistic helper entirely once B lands?**
Because Fix B writes to the DB, then the frontend still needs a
refetch to *read* the new value. The optimistic helper covers the
~0–3000ms gap between the send and the next poll without an extra
HTTP round-trip. It's exactly the layered approach React Query is
designed for.

## Files touched

| File | Change |
|---|---|
| `frontend/src/utils/optimisticSessionStarting.ts` | Remove the `invalidateQueries` call at lines 50-53. Update the comment block at lines 11-18 to describe the new contract. |
| `frontend/src/utils/optimisticSessionStarting.test.ts` (or `__tests__/`) | New test: after `optimisticallyMarkSessionStarting`, a simulated refetch that returns `external_agent_status="stopped"` must not flip `useSandboxState`'s derived `isStarting` to false within the cache-hold window. Existing tests (commit `279b2128b`) should still pass. |
| `api/pkg/server/prompt_history_handlers.go` | In `syncPromptHistory`, resolve the canonical session, check WS connectivity, and column-update `external_agent_status="starting"` + `status_message="Starting Desktop..."` before firing the wake goroutine. No-op if already running/starting. |
| `api/pkg/store/store_sessions.go` (or wherever column updates live) | If no existing helper does targeted column updates for `external_agent_status`, add one — pattern-match the one used in `auto_wake_stuck_interactions.go`. |
| `api/pkg/server/auto_wake_stuck_interactions.go` | When `autoWakeMaxRetries` is exhausted and the interaction is marked `state=error`, also reset `external_agent_status` from `"starting"` to `"stopped"` and clear `status_message`. Targeted column update only. |
| `api/pkg/server/prompt_history_handlers_test.go` (new or extend) | Unit test: `syncPromptHistory` with a session that has no live WS and `external_agent_status="stopped"` writes `"starting"` before returning. Already-running session is left alone. |
| `api/pkg/server/auto_wake_stuck_interactions_test.go` | Extend the existing exhaustion test to assert `external_agent_status` is reset to `"stopped"`. |

No changes to:

- `RobustPromptInput.tsx` — the `onWillSend` wiring stays as is.
- `SpecTaskDetailContent.tsx` / `ExternalAgentDesktopViewer.tsx` —
  their `handleWillSend` wiring stays as is.
- `streaming.tsx` — already preserves config on session_update.
- The cold-start helper `startDevContainerForSession` from spec
  #001995 — orthogonal.

## Verification plan

1. **Unit tests pass locally.** `cd api && CGO_ENABLED=1 go test
   ./pkg/server/ -run 'PromptHistory|AutoWake' -count=1`. Frontend:
   `cd frontend && yarn test optimisticSessionStarting`.
2. **Manual reproduction of the regression with no fix.** Confirm the
   flicker is reproducible by reverting the proposed changes locally.
3. **Manual confirmation after fix.** With both fixes applied:
   - Register `test@helix.ml` / `helixtest` in inner Helix,
     create org + project + spec task.
   - Run the task to first stop / "Desktop Paused".
   - Open browser devtools Network tab; filter to
     `/sessions/{id}`.
   - Send a chat message.
   - Watch: spinner appears within one frame, the immediate refetch
     returns `external_agent_status="starting"`, the spinner stays.
4. **Regression on running session.** Send a chat to an
   already-running session and watch the network tab: no
   `external_agent_status` flips, no spinner flicker.
5. **Failure-mode UX.** Force a session with no project context (or
   stub `StartDesktop` to error) and confirm that after retry
   exhaustion the spinner returns to "Desktop Paused" rather than
   sitting on "Starting Desktop..." forever.
6. **Spec #001995 regression check.** Run the issue #2397 reproducer
   on an exploratory `zed_external` session and confirm the
   end-to-end wake still works.

## Implementation notes (added during build-out)

- **Single-statement gated UPDATE beats SELECT-then-UPDATE.** The
  original sketch was: fetch the session row, branch on
  `external_agent_status`, conditionally column-update. Replaced
  with a single `UPDATE … WHERE COALESCE(config->>'external_agent_status','') NOT IN ('starting','running')`.
  Atomic at the DB level, one round-trip, no race with hydra's own
  status writes. Returns `RowsAffected==0` when the guard skipped the
  row — caller logs no-op.
- **No `prior_status` in the INFO log.** The guarded UPDATE doesn't
  return the prior value, and a separate SELECT before/after would race
  the UPDATE. `session_id` + `spec_task_id` is enough to correlate with
  the hydra-side `setExternalAgentStatus` logs.
- **Helper extraction skipped.** Original sketch said "extract the
  canonical-session lookup so it runs in the handler before the
  goroutine dispatch". The existing `processPendingPromptsForIdleSessions`
  goroutine keeps its own `GetSpecTask` call — they're independent
  concerns and pulling out a shared lookup would couple unrelated code
  paths with no benefit. `markCanonicalSessionStartingForSync` calls
  `GetSpecTask` independently; both are cheap reads.
- **Empty `PlanningSessionID` is DEBUG, not WARN.** Fresh spec tasks
  exist in a brief window before the planning session is wired up; a
  WARN log spams noise during normal operation. The empty case is
  expected and silent.
- **`MockStore.ClearSessionStartingStatus` must be expected even when
  nothing is cleared.** The exhaustion path in `maybeKickColdStart`
  always calls the clear helper; the *DB* gates whether anything
  actually changes. Existing `TestMarksAsErrorAfterMaxRetries` needed a
  `Return(false, nil)` expectation to satisfy gomock — the new
  `TestClearsStartingStatusOnExhaustion` covers the `Return(true, nil)`
  branch.
- **`yarn build` blocked by `frontend/dist` bind mount.** The dist dir
  is owned by root because it's bind-mounted into the production
  frontend container. CLAUDE.md: "NEVER `rm -rf frontend/dist` —
  breaks the bind mount". Used `yarn tsc` instead — meaningful check
  for our TypeScript edits.
- **Memorystore is partial.** `api/pkg/store/memorystore/memorystore.go`
  doesn't implement the full `Store` interface (e.g. no
  `ClearStaleStartingSessions`) — it's a test fixture, not a real
  store. New helpers were not added there. If a future e2e test needs
  them, add stubs at that point.
- **`getConnection` defensive nil-check.** Other call sites in
  `websocket_external_agent_sync.go` use the pattern
  `if exists && conn != nil`, so the new helper matches that to avoid
  a nil-pointer hazard if the map ever ends up with a nil entry.

## Notes for future-me / cloned tasks

- This is the third visible attempt to fix this. The pattern keeps
  recurring because the fix has been targeted at "make the spinner
  appear" rather than "stop the cache and the backend disagreeing
  during the wake window". The disagreement is the root cause; if a
  future symptom looks similar, *check whether anything between send
  and the next poll could be silently overwriting cache with a stale
  backend row*.
- The synchronous-mark step in `syncPromptHistory` is small but
  cross-cutting — it touches the DB on what used to be a near-pure
  store-and-dispatch handler. Reviewers may push back on adding I/O
  to a fast path. The cost is one row update on a path that already
  does `Store.SyncPromptHistory` (a write) and fires a goroutine; the
  marginal latency is negligible and the UX win is large.
- Do NOT replace `setQueryData` with `queryClient.setQueryData(...)`
  on the bare `["session", id]` key. `useGetSession` uses suffixed
  keys (`'full'` / `'skip'`); setting the bare key is a silent no-op
  (this was the bug `bea5d6ae1`'s commit message called out).
- If a follow-up changes `useGetSession`'s key shape again, update
  `QUERY_VARIANTS` in `optimisticSessionStarting.ts` to match.
