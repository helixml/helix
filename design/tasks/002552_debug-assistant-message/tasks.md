# Implementation Tasks: Fix Live Chat Message Truncation from Stale-Poll Clobber

## Confirm the diagnosis

- [~] Read `design/2026-07-03-spectask-live-message-truncation.md`, treating its
      "trailing-edge lag" conclusion as superseded
- [~] Bring up the inner Helix at `http://localhost:8080`, register `test@helix.ml` /
      `helixtest`, complete onboarding, create a spec task so a live Zed agent streams
- [ ] Reproduce: prompt the agent to print a distinctive long sentence then immediately
      run a tool call / `sleep 30`; confirm the long → short flicker
- [ ] Instrument `useLiveInteraction` — log `{ src, crId, iiId, guardMatched, msgLen,
      lastKnownLen, entryCount, lastEntryTail }` on every render; capture what causes
      `msgLen`/`entryCount` to go DOWN (poll, patch, or streaming-state clear)
- [ ] Instrument the patch layer — server: `(index, patch_offset, total_length, tail)`
      per publish; client: same plus pre/post content length per entry, and assert
      `total_length >= currentContent.length`
- [ ] Record which suspect the evidence confirms (A stale-poll clobber / B destructive
      slice / C split sources); adjust the fix emphasis if it is not A

## Server: sequence numbers and snapshots

- [ ] Add `Seq uint64` and `Snapshot bool` to `types.WebsocketEvent`
- [ ] Increment and stamp `Seq` per interaction in the entry-patch publish path, under
      `sctx.mu`
- [ ] Add `response_seq` to the interaction row and stamp it in
      `flushStreamingFieldsToDB` (covers both the leading-edge write and the trailing flush)
- [ ] Make `buildFullStatePatchEvent` set `Snapshot: true` and carry the last published
      seq, built under the same lock as the seq read
- [ ] Add a client-triggered resync entry point that returns the full-state snapshot

## Client: one versioned source of truth

- [ ] Store `{ seq, entries, message }` per interaction id in the patch handler; write
      both entries and message into `currentResponses` and stop discarding fields on the
      `!isSameInteraction` branch
- [ ] Rewrite `useLiveInteraction` as a selector that compares `live.seq` against the
      polled row's `response_seq` — same rule for streaming and completed
- [ ] Delete `completedMessage || safeResponseMessage || lastKnownMessage` and the
      `isComplete ? (streaming || completed) : (completed || streaming)` selector
- [ ] Remove the lossy shrink branch from `applyPatch`; treat `total_length` as a checksum
- [ ] Detect divergence: seq gap, `patch_offset > currentContent.length`, or
      `total_length !== result.length`
- [ ] On divergence, request a snapshot and replace the store; stop applying deltas to a
      diverged buffer
- [ ] Make the reconnect path in `openHandler` resync from a snapshot instead of clearing
      to `[]` and blind-appending

## Force the failure modes

- [ ] Skip one server publish deliberately — confirm detection and resync
- [ ] Deliver two patches out of order — confirm detection and resync
- [ ] Reconnect the WebSocket mid-interaction — confirm the UI recovers correctly
- [ ] Record the UI behaviour for each case in the design doc

## Tests

- [ ] Frontend test: a poll result racing a live patch — the older value loses
- [ ] Frontend test: dropped patch and reordered patch are detected as divergence
- [ ] Go test: seq is monotonic per interaction and the snapshot carries the last
      published seq
- [ ] `cd frontend && yarn build` passes
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes

## Verify end-to-end and ship

- [ ] Real browser in the inner Helix: full paragraph AND the following tool call stay on
      screen through a tool call and a ≥30s pause, across at least three 3s poll cycles,
      with no flicker
- [ ] Check the main chat page and design-review comment streaming (shared
      `patchUtils.ts` / `useLiveInteraction`) for regressions
- [ ] Save screenshots to `screenshots/`
- [ ] Remove temporary instrumentation; keep only the permanent divergence warning
- [ ] Write `design/2026-08-04-chat-message-truncation-clobber.md` with the evidence and
      an explicit correction to the Jul-3 doc's conclusion
- [ ] Open a PR against `helixml/helix`, check CI yourself, and give the full PR URL
