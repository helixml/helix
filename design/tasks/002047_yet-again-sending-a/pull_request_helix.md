# fix(spectask): stop spinner flicker when chatting an idle desktop

Closes the race that made the "Starting Desktop..." spinner
flicker off when sending a chat message to an idle spec-task
session on the detail page. Third attempt at this UX bug — full
root-cause analysis in spec
`design/tasks/002047_yet-again-sending-a/`.

## Why the previous attempts didn't stick

Three prior commits each closed *part* of this gap:
- `e43acefdb` (Apr 2026): made `useSandboxState` poll
  unconditionally. Reduced the worst case from "never" to
  "within 3 s" — but didn't make the transition immediate.
- `bea5d6ae1` (May 2026): added
  `optimisticallyMarkSessionStarting` + `onWillSend` to push
  the spinner up synchronously. Same helper also called
  `queryClient.invalidateQueries(...)` "as belt-and-braces".
  That invalidate triggered an immediate refetch.
- spec `001995_…` (May 2026): generalised
  `autoStartDevContainerForSession` so the backend *eventually*
  wakes the desktop on `/messages`. The wake is async.

The bug: the immediate refetch (from `invalidateQueries`) wins
against the async backend wake. The refetch returns the still
`stopped` row and overwrites the optimistic "starting" in the
React Query cache. The spinner flickers off, then ~3 s later the
regular poll catches the goroutine's eventual DB write and the
spinner returns. Users perceive this as "the spinner never
showed" and click send a second time.

## The fix (two prongs)

**Frontend** — `frontend/src/utils/optimisticSessionStarting.ts`:
remove the `invalidateQueries` call. The 3 s poll reconciles
naturally, and the backend now writes "starting" synchronously
(see backend prong) so even an unrelated refetch returns the
right value.

**Backend** —
`api/pkg/server/prompt_history_handlers.go`: before firing the
async wake goroutine, `syncPromptHistory` now calls a new
helper `markCanonicalSessionStartingForSync` that
column-updates the canonical planning session's
`external_agent_status` to `"starting"` and `status_message` to
`"Starting Desktop..."`, gated on the session having no live
WebSocket and not already being in `starting`/`running`.

**Failure-mode UX** —
`api/pkg/server/auto_wake_stuck_interactions.go`: on cold-start
retry exhaustion, the worker now also reverts the sync-time
"starting" mark so the spinner returns to "Desktop Paused"
instead of sitting on a perpetual spinner.

## Changes

- `frontend/src/utils/optimisticSessionStarting.ts` — delete
  `queryClient.invalidateQueries` at line 53; update the
  comment block to describe the new contract.
- `frontend/src/utils/optimisticSessionStarting.test.ts` —
  replace the "fires invalidateQueries" assertion with two new
  tests that assert (a) `invalidateQueries` is NOT called and
  (b) the cache entry's `isInvalidated` stays false. Existing
  11 tests still pass.
- `api/pkg/store/store_sessions.go` — two new helpers:
  `MarkSessionStartingIfIdle` (atomic JSONB merge gated on
  `status NOT IN ('starting','running')`) and
  `ClearSessionStartingStatus` (atomic JSONB merge gated on
  `status = 'starting'`). Both follow the existing
  `ClearStaleStartingSessions` pattern.
- `api/pkg/store/store.go` — interface additions for the two
  helpers, plus prose comments.
- `api/pkg/store/store_mocks.go` — generated-style mock entries
  for both new helpers.
- `api/pkg/server/prompt_history_handlers.go` — new
  `markCanonicalSessionStartingForSync` helper; called from
  `syncPromptHistory` before the wake goroutine fires.
- `api/pkg/server/auto_wake_stuck_interactions.go` — in the
  cold-start exhaustion branch, call
  `ClearSessionStartingStatus` to revert the sync-time mark.
- `api/pkg/server/prompt_history_handlers_test.go` — four new
  tests for the sync-time mark (no-WS marks, live-WS skips,
  no planning session no-ops, already-starting no-update).
- `api/pkg/server/auto_wake_stuck_interactions_test.go` — extend
  `TestMarksAsErrorAfterMaxRetries` to assert the clear call;
  new `TestClearsStartingStatusOnExhaustion` for the
  "something was cleared" branch.

## Test plan

- [x] `cd api && CGO_ENABLED=1 go test ./pkg/server/ -count=1`
      — full server suite green (9.9 s).
- [x] `cd api && CGO_ENABLED=1 go test ./pkg/server/ -run
      'PromptHistory|AutoWake' -count=1` — focused suites green.
- [x] `cd api && CGO_ENABLED=1 go build ./pkg/server/
      ./pkg/store/ ./pkg/store/memorystore/ ./pkg/external-agent/`
      — clean.
- [x] `cd frontend && yarn test --run optimisticSessionStarting`
      — 11/11 pass.
- [x] `cd frontend && yarn tsc` — clean. `yarn build` blocked
      by a pre-existing root-owned `frontend/dist` bind mount,
      not by these changes (CLAUDE.md warns "NEVER `rm -rf
      frontend/dist`").
- [ ] Manual E2E in inner Helix (deferred — no inner Helix
      reachable in this sandbox; sufficient unit coverage above).
- [ ] CI (Drone) — to be checked after push.

## Out of scope

- Refactor of `resumeSession` to share code with
  `startDevContainerForSession` (deferred in spec #001995,
  still deferred here).
- Splitting `prompt-history/sync` into multiple endpoints.
- Frontend WebSocket session_update handling — already preserves
  config per `streaming.tsx:307-327`.
