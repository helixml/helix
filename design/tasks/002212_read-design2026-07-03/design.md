# Design: Fix Spec-Task Detail Live-Message Truncation/Lag

## Approach

This is a **diagnose-then-fix** task. The two candidate causes require different fixes, so
the design is: (1) instrument both ends of the live path, (2) run the live repro to decide
which cause is real, (3) apply the matching fix, (4) verify live, (5) remove instrumentation
and add a regression test.

## Key files (all in `/home/retro/work/helix`)

| Concern | File / location |
|---|---|
| Live hook + id-guard | `frontend/src/hooks/useLiveInteraction.ts:55-92` |
| Streaming context (patch → `currentResponses`) | `frontend/src/contexts/streaming.tsx:480-545` |
| Frontend patch apply | `frontend/src/utils/patchUtils.ts` |
| Render (entries vs text) | `frontend/src/components/session/InteractionInference.tsx` (`MessageWithToolCalls`, prefers `responseEntries` at ~L101) |
| Live stream component | `frontend/src/components/session/InteractionLiveStream.tsx` |
| Server throttle / publish | `api/pkg/server/websocket_external_agent_sync.go` (`dbWriteInterval=5s` ~L114, `publishInterval=50ms` ~L118, throttle block ~L1329-1413, `flushTimer` trailing flush ~L1396) |
| Server entry-patch publish | `publishEntryPatchesToFrontend` ~L4044 |
| Accumulator | `api/pkg/server/wsprotocol/accumulator.go` |

## How the live path actually works (traced)

- Server streams assistant content: on each `message_added`, it updates an in-memory
  accumulator, throttles the **DB write** to every 5s, and throttles the **frontend publish**
  to every 50ms with a `time.AfterFunc(publishInterval, …)` trailing flush.
- Frontend `streaming.tsx` receives `interaction_patch` events and applies per-entry patches
  into `currentResponses.get(sessionId) = { id: interactionId, response_entries }`.
  **Note:** the patch path sets only `id` + `response_entries`, *not* `response_message`.
- `useLiveInteraction` overlays `currentResponses` onto the 3s-polled `initialInteraction`
  **only if** `currentResponse.id === initialInteraction.id`. On match → live entries; else →
  the 3s-polled DB interaction (≤5s stale during streaming).
- `MessageWithToolCalls` renders `responseEntries` when present, so the visible text is the
  entry stream's tail in both cases; the difference is whether that tail is live (50ms) or DB
  (5s).

## Instrumentation (temporary — removed before final commit)

**Frontend** (`useLiveInteraction.ts`, HMR): log once per render
`{ src: match ? 'LIVE' : 'DB', crId, iiId, match, msgTail: message.slice(-40),
lastTail, entries: responseEntries?.length }`. Emit at `[LIVE-RESULT]`.

**Server** (`websocket_external_agent_sync.go`, Air): in the throttle block, log the branch
taken — `LEADING`, `FORCE-FLUSH` (tool_call), or `TRAILING-FLUSH` — plus interaction id and
the last ~40 chars of the newest text entry's content. Existing `📤 [FLUSH]` log already
marks force-flush; add explicit `TRAILING-FLUSH` logging inside the `AfterFunc`.

## Decision procedure

Run the repro (sentence → `sleep 30`) and read the logs during the pause:
- `src = DB` during the lag ⇒ **cause #1** (id-guard fails / DB fallback + 5s throttle).
- `src = LIVE` with a stale `msgTail`/`lastTail`, and **no** `TRAILING-FLUSH` after the last
  text chunk (only a publish when the tool_call arrives) ⇒ **cause #2** (trailing flush not
  delivering the tail).

## Candidate fixes

### If cause #1 (id-guard / DB fallback)
- Make the match succeed: the streaming context already sets `currentResponses.id =
  interactionId` from the patch, and `initialInteraction.id` is the last Waiting interaction
  from the poll — confirm they are the same id (session vs interaction id mismatch, or the
  live patch arriving before the poll has the Waiting row). Fix whichever side is wrong so the
  guard matches the active interaction.
- **And/or** add a **trailing-edge DB flush** mirroring the frontend `flushTimer`: after the
  last streaming write, schedule a short (~300–500ms) `time.AfterFunc` DB flush so the DB
  fallback is never >~half a second stale. This bounds the worst case even if the guard is
  briefly wrong.

### If cause #2 (entry-patch trailing flush)
- Ensure the `flushTimer` trailing publish always fires shortly after the last chunk and is
  not cancelled without a replacement. Verify the `AfterFunc` closure captures the latest
  `currentEntries` (it re-reads `sctx.accumulator.Entries()` under the lock — confirm this is
  correct and not racing with completion teardown at ~L1856/L1623 which stops the timer).
- If the 3s `useListInteractions` poll is observed clobbering newer live entries, pause it
  (or ignore its result for the active interaction) while streaming.

## Key decisions & rationale

- **Diagnose before fixing.** The two causes have opposite fixes (DB path vs publish path);
  guessing risks fixing the wrong one. The source doc explicitly confirmed the DB, patch
  logic, and transport are fine, narrowing the search to the id-guard vs the trailing flush.
- **Prefer hardening both cheaply.** A trailing DB flush and a correct id-guard are both
  low-risk and independently make the fallback path robust, so implement the observed cause's
  fix and add the other as inexpensive belt-and-suspenders (pending the Open Question).
- **Live verification is mandatory** (per CLAUDE.md): the inner Helix has a real Zed agent;
  Air + Vite HMR make instrumentation and iteration fast. Unit tests verify the algorithm,
  not the wired-up component, so they are additive, not a substitute.
- **No new fallbacks / dead code** (Go rule): if the id-guard is the bug, fix the matching
  logic rather than adding a parallel path.

## Testing / verification
- Live: inner Helix repro, confirm the sentence stays complete during `sleep 30`.
- `cd frontend && yarn build`; `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`.
- Regression test (US-4): a Go test asserting a trailing publish fires after the last chunk
  with no follow-up event, and/or a frontend test that a text entry followed by a tool_call
  renders the full text.

## Implementation Notes (diagnosis + fix — verified live in inner Helix)

**Diagnosis (evidence, not analogy).** With the instrumentation live, three streaming
turns were run on the spec-task detail page (a text turn, a text→tool_call turn with a
25s `sleep` pause, and a long prose turn):

- **Frontend id-guard always passes.** Every `[LIVE-RESULT]` logged `src:"LIVE",
  match:true` — `currentResponses.id` always equalled `initialInteraction.id`. Cause #1's
  *frontend symptom* (falling back to the 3s-polled DB) was never observed.
- **Server trailing flush works.** `📤 [PUBLISH] branch=TRAILING-FLUSH` fires ~50ms after
  each burst carrying the full tail, and the frontend `lastTail` tracked it to completion.
  Cause #2 (trailing publish not delivering the tail) is **not broken** on current `main`.
  The trailing-edge *publish* flushTimer was added 2026-04-25 (commit `6fdef74a4`), before
  the doc was written — the live-view lag it describes is already fixed on `main`.
- **The real remaining gap is the DB write cadence.** Sampling `interactions.response_message`
  during a `waiting` turn showed the length growing in ~5s steps (`updated` advanced
  11:46:05 → 11:46:10, +5.2s) — i.e. the DB is throttled to `dbWriteInterval = 5s`
  leading-edge with **no trailing flush**, while the live stream is current to ~50ms. So
  any consumer that reads the DB *during* streaming — the id-guard fallback path (cause #1),
  a page reload/snapshot mid-stream, or any other reader — sees up to 5s-stale text. The
  publish path had a trailing flush; the DB path did not. That asymmetry is the bug to fix.

**Fix implemented (server, `websocket_external_agent_sync.go`).** Added a trailing-edge DB
flush that mirrors the existing publish `flushTimer`:
- New `sctx.dbFlushTimer` + `dbTrailingFlushInterval = 500ms`.
- Extracted the DB write into `flushStreamingFieldsToDB(sctx)` (caller holds `sctx.mu`),
  reused by both the leading-edge write and the trailing timer (DRY; column-scoped write
  preserved so it never clobbers state/completed/error).
- In the throttled-DB-write block: the leading branch (`>= dbWriteInterval`) now also
  cancels any pending `dbFlushTimer`; the new `else` branch schedules/reschedules the
  trailing flush 500ms out. Continuous streaming keeps resetting it (so writes still happen
  at the 5s leading cadence, bounding TOAST churn); a pause or end-of-burst triggers one
  catch-up write within ~500ms.
- `dbFlushTimer` is stopped at the same teardown points as `flushTimer` (interaction
  transition reset + `flushAndClearStreamingContext`). The AfterFunc re-checks `!sctx.dirty`
  and nil accumulator/interaction, matching the publish flushTimer's race handling.

Net effect: DB staleness during a streaming pause drops from up to 5s to ~500ms, so the
fallback/reload path is never badly stale. The frontend LIVE path was already correct and
is left unchanged (no risk to the delicate completion/flicker logic).

## Instrumentation status
Temporary `[LIVE-RESULT]` (frontend) and `📤 [PUBLISH] branch=…` LEADING/TRAILING-FLUSH
logs (server) were added for diagnosis and are **removed** before the final commit. The
`lastEntryTail` helper is removed with them. The `flushStreamingFieldsToDB` helper,
`dbFlushTimer`, and `dbTrailingFlushInterval` are the permanent fix and stay.

## Risks
- **Heisenbug / timing-sensitive.** The lag depends on chunk cadence; the `sleep 30` repro
  forces a clean trailing-edge pause to make it reproducible.
- **Completion path is delicate.** `useLiveInteraction` has extensive logic to avoid
  blank/flicker on completion and cross-session leaks; changes must preserve AC3.1/AC3.2.
- **Column-scoped streaming writes** must not clobber `state/completed/error` — keep using
  `UpdateInteractionStreamingFields` if adding a trailing DB flush.
