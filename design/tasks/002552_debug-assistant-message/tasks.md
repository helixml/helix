# Implementation Tasks: Fix Live Chat Message Truncation from Stale-Poll Clobber

## Confirm the diagnosis

- [x] Read `design/2026-07-03-spectask-live-message-truncation.md`, treating its
      "trailing-edge lag" conclusion as superseded
- [x] Bring up the inner Helix at `http://localhost:8080`, register `test@helix.ml` /
      `helixtest`, complete onboarding, create a spec task so a live Zed agent streams
- [x] Reproduce: prompt the agent to print a distinctive long sentence then immediately
      run a tool call / `sleep 30`; confirm the long → short flicker
- [x] Instrument `useLiveInteraction` — log `{ src, crId, iiId, guardMatched, msgLen,
      lastKnownLen, entryCount, lastEntryTail }` on every render; capture what causes
      `msgLen`/`entryCount` to go DOWN (poll, patch, or streaming-state clear)
- [x] Instrument the patch layer — server: `(index, patch_offset, total_length, tail)`
      per publish; client: same plus pre/post content length per entry, and assert
      `total_length >= currentContent.length`
- [x] Record which suspect the evidence confirms (A stale-poll clobber / B destructive
      slice / C split sources); adjust the fix emphasis if it is not A

## Server: sequence numbers and snapshots

- [x] Add `Seq uint64` and `Snapshot bool` to `types.WebsocketEvent`
- [x] Increment and stamp `Seq` per interaction in the entry-patch publish path, under
      `sctx.mu`
- [x] Add `response_seq` to the interaction row and stamp it in
      `flushStreamingFieldsToDB` (covers both the leading-edge write and the trailing flush)
- [x] Make `buildFullStatePatchEvent` set `Snapshot: true` and carry the last published
      seq, built under the same lock as the seq read
- [x] Add a client-triggered resync entry point that returns the full-state snapshot

## Client: one versioned source of truth

- [x] Store `{ seq, entries, message }` per interaction id in the patch handler; write
      both entries and message into `currentResponses` and stop discarding fields on the
      `!isSameInteraction` branch
- [x] Rewrite `useLiveInteraction` as a selector that compares `live.seq` against the
      polled row's `response_seq` — same rule for streaming and completed
- [x] Delete `completedMessage || safeResponseMessage || lastKnownMessage` and the
      `isComplete ? (streaming || completed) : (completed || streaming)` selector
- [x] Remove the lossy shrink branch from `applyPatch`; treat `total_length` as a checksum
- [x] Detect divergence: seq gap, `patch_offset > currentContent.length`, or
      `total_length !== result.length`
- [x] On divergence, request a snapshot and replace the store; stop applying deltas to a
      diverged buffer
- [x] Make the reconnect path in `openHandler` resync from a snapshot instead of clearing
      to `[]` and blind-appending

## Force the failure modes

- [x] Skip one server publish deliberately — confirm detection and resync
- [x] Deliver two patches out of order — confirm detection and resync
- [x] Reconnect the WebSocket mid-interaction — confirm the UI recovers correctly
- [x] Record the UI behaviour for each case in the design doc

## Tests

- [x] Frontend test: a poll result racing a live patch — the older value loses
- [x] Frontend test: dropped patch and reordered patch are detected as divergence
- [x] Go test: seq is monotonic per interaction and the snapshot carries the last
      published seq
- [x] `cd frontend && yarn build` passes
- [x] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes

## Verify end-to-end and ship

- [x] Real browser in the inner Helix: full paragraph AND the following tool call stay on
      screen through a tool call and a ≥30s pause, across at least three 3s poll cycles,
      with no flicker
- [x] Check the main chat page and design-review comment streaming (shared
      `patchUtils.ts` / `useLiveInteraction`) for regressions
- [x] Save screenshots to `screenshots/`
- [x] Remove temporary instrumentation; keep only the permanent divergence warning
- [x] Write `design/2026-08-04-chat-message-truncation-clobber.md` with the evidence and
      an explicit correction to the Jul-3 doc's conclusion
- [x] Push the feature branch (the platform opens the PR from the UI)

## Deviations from the plan (evidence-driven)

- **`response_seq` DB column and the `useLiveInteraction` seq-ranked selector were dropped.**
  Instrumentation disproved the stale-poll hypothesis that motivated them: the id-guard held
  on every content-bearing render across 300+ renders and five agent turns, so the polled row
  never won a race. The plumbing was built, measured to be unused, and reverted rather than
  shipped as an unread column plus a risky refactor.
- **`cd frontend && yarn build`** was run as `tsc --noEmit` plus `vitest` inside the
  `helix-frontend-1` container — `node_modules` does not exist on the host in this sandbox.
- **A poll-racing-a-live-patch unit test was not added**; the race it would assert does not
  occur (see above). The dropped/reordered-patch divergence is covered instead, which is the
  failure that actually reproduces.
