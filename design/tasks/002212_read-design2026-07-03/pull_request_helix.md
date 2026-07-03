# fix(api): add trailing-edge DB flush for streamed interaction writes

## Summary
Fixes the spec-task detail live-message truncation/lag investigation
(`design/2026-07-03-spectask-live-message-truncation.md`).

The task named two candidate root causes. Instrumenting both ends of the live
path in the inner Helix and running the repro (a text turn, a text→tool_call turn
with a 25s pause, and a long prose turn) showed:

- **Frontend id-guard always passes** — every render used the live
  `currentResponses` (`src=LIVE`), never the DB fallback. Cause #1's frontend
  symptom did not occur.
- **Server trailing publish flush already works** — `TRAILING-FLUSH` publishes
  fire ~50ms after each burst and the frontend tracked them to completion. The
  live-view lag the doc describes was already fixed by the publish `flushTimer`
  (commit `6fdef74a4`, 2026-04-25), so the reported *text* truncation no longer
  reproduces on `main`.
- **The real remaining gap is the DB write cadence.** Sampling
  `interactions.response_message` during a streaming turn showed it advancing in
  ~5s steps: the streaming DB write is throttled to `dbWriteInterval` (5s)
  leading-edge with **no trailing flush**, while the live stream is current to
  ~50ms. The publish path had a trailing flush; the DB path did not. So any
  consumer that reads the DB mid-stream — the `useLiveInteraction` 3s-poll
  fallback (cause #1's mechanism), a page-reload snapshot, or any other reader —
  saw up to 5s-stale/truncated text.

This PR closes that asymmetry by mirroring the publish `flushTimer` with a
**trailing-edge DB flush**.

## Changes
- Add `streamingContext.dbFlushTimer` and `dbTrailingFlushInterval = 500ms`.
- Extract the streaming DB write into `flushStreamingFieldsToDB(sctx)` (caller
  holds `sctx.mu`; column-scoped so it never clobbers state/completed/error),
  shared by the leading-edge write and the new trailing flush.
- In the throttled-DB-write block: the leading branch now also cancels any
  pending `dbFlushTimer`; the new `else` branch schedules/reschedules a trailing
  flush 500ms after the last chunk. Continuous streaming keeps resetting the
  timer, so writes still happen at the 5s leading cadence (bounding TOAST churn);
  only pauses and burst tails trigger a catch-up write.
- Stop `dbFlushTimer` at the same teardown points as `flushTimer` (interaction
  transition reset + `flushAndClearStreamingContext`); the AfterFunc re-checks
  `!dirty` and nil accumulator/interaction, matching the publish flushTimer's
  race handling.
- Add regression test `TestMessageAdded_TrailingDBFlush`.

## Verification
- Live in the inner Helix (`localhost:8080`): DB catch-up on a mid-turn pause
  drops from up to 5s to ~500ms; continuous streaming keeps the 5s cadence
  (no extra churn); the live view was current every turn and is unchanged.
- `go build ./pkg/server/` passes; `TestWebSocketSyncSuite` (incl. the new test)
  green.
- Frontend was only instrumented temporarily for diagnosis; that instrumentation
  is fully removed (no net frontend change).

## Notes
The frontend LIVE path is intentionally left untouched — it already renders the
in-progress message correctly, and its completion/flicker/cross-session logic is
delicate. This change hardens only the DB/fallback path.
