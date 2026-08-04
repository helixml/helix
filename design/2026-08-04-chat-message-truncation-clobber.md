# Live chat message truncation: a dropped patch, silently spliced

**Task:** 002552 · **Branch:** `feature/002552-fix-live-chat-message`
**Supersedes the conclusion of:** `design/2026-07-03-spectask-live-message-truncation.md`

## The bug

On the spec-task detail page Chat tab, while an agent is mid-turn, the latest assistant
message renders **cut off mid-word** and the tool call that follows it is missing. Reported
with the crucial detail that the complete text **flickers in and then reverts to the
truncated version** — long → short, repeatedly, not trailing-edge lag.

The database always held the full text. This was purely a client-side reconstruction bug.

## Root cause

`interaction_patch` events carry per-entry string deltas (`patch_offset`, `patch`,
`total_length`). The transport is embedded core NATS: **best-effort, drop-on-slow-consumer,
no redelivery**. Nothing detected a dropped message.

When a patch went missing, the next patch's `patch_offset` pointed past everything the
client held. The old `applyPatch` handled that case with a "pure append":

```ts
} else if (patchOffset >= currentContent.length) {
  newContent = currentContent + patch;   // silently splices across the hole
}
```

So the two sides of the gap were **glued together mid-word** and the missing bytes were
gone forever. No error, client or server. A later `if (totalLength < newContent.length)
newContent.slice(0, totalLength)` could then destructively shorten content on top of that.

Because `computePatch` accumulates per *stream* and not per *client*, the server had no way
to notice the client had diverged, and kept sending deltas against a baseline the client no
longer shared.

## Evidence

Reproduced in the inner Helix with deliberate fault injection: skip the publish while still
advancing the server's `previousEntries`, i.e. exactly a lost NATS message. Twelve drops
during a real Zed agent turn, then client buffer vs database:

| entry | server (truth) | client | symptom |
|---|---|---|---|
| 8 | 492 chars | **448** | tail `"…settle once more. ENDOFSENTEN"` — cut mid-word |
| 10 | 467 chars | **336** | head `"Albs the spiral iron stair…"` |

Entry 10 is the clearest. The server's text begins:

> "**Al**one on a wave-hammered outcrop miles from the mainland, the lighthouse keeper
> climbs the spiral iron stair…"

The client held `"Al"` (received before the drop) spliced straight onto `"bs the spiral
iron stair…"` (received after), producing **`"Albs the spiral iron stair…"`**. 131
characters vanished mid-word, silently.

That is the reported symptom exactly: a mid-word cut that persists.

### Why it looked like a flicker

The DB is correct; the live patch buffer is the corrupt, short one. Whenever the live value
momentarily lost — a completion, a guard miss, a reconnect that blanked streaming state —
the correct database text painted, then the corrupted live entries won again. Long → short,
repeatedly.

### What was ruled out

Both alternative hypotheses were instrumented and **disproved**:

- **The 3s poll clobbering live entries.** Across 300+ instrumented renders spanning five
  agent turns, the id-guard in `useLiveInteraction` held on every render that had content
  (`src: LIVE`), and `entryCount`/`msgLen` never decreased. The poll does lag the live
  stream (observed 3 entries vs 5), but `setInteraction({...prev, ...currentResponse})`
  keeps the live entries, so the polled row never wins.
- **`interaction_update` clearing the patch buffer mid-stream.** Exactly one such event
  arrives per turn, at `state: "complete"`. The `patchEntriesRef.current.delete()` at
  `streaming.tsx` is therefore not firing mid-stream.

## Correction to the 2026-07-03 doc

That investigation concluded "trailing-edge lag" and shipped `6ff541aa6` (a trailing-edge DB
flush). Its *facts* hold — DB correct, Zed correct, sync correct — but the framing was
wrong, and so was mine at the start of this task. Both assumed the **live stream was always
the fresher source** and the persisted row the staler one, making the fix a question of
reducing DB staleness or ranking the two.

The truth is the opposite: the live stream is the one that can be **corrupt**, and no amount
of DB freshness tuning addresses that. Reducing lag shrank the window in which the correct
DB text was visible, which if anything made the corruption *more* persistent on screen.

## The fix

**Detect divergence, then rebuild from a snapshot. Never guess across a gap.**

1. **Sequence numbers** (`Seq` on `types.WebsocketEvent`) — a monotonic counter incremented
   once per published patch, per streaming context. A publish that changes nothing does not
   consume a sequence number. The client treats any step other than `lastSeq + 1` as a
   dropped message.
2. **`applyPatch` no longer guesses.** One uniform reconstruction —
   `currentContent.slice(0, offset) + patch` — covers first patch, pure append, and
   backwards edit alike. It returns `null` when `patch_offset` is beyond what we hold, or
   when `total_length` disagrees with the result. `total_length` is now a **checksum**, not
   a truncator; the lossy shrink branch is gone.
3. **Snapshot resync.** `buildFullStatePatchEvent` now sets `Snapshot: true` and carries the
   sequence it was taken at, read under the same lock as the entries so the snapshot and its
   version cannot disagree. On divergence the client forces a WebSocket reconnect, which
   makes the server send that snapshot via its existing late-joiner catch-up; the snapshot
   replaces the buffer wholesale and re-establishes the sequence. No new protocol surface.
4. **Stop blanking on reconnect.** `openHandler` used to clear `currentResponses` and the
   patch buffer, which dropped the view back to the ≤5s-stale polled row until fresh data
   arrived — a visible long→short flicker, and one this change would otherwise have caused
   on every resync. The snapshot overwrites the buffer anyway, so the last-good render now
   stays on screen throughout.

The diverged interaction is flagged so further deltas are ignored until the snapshot lands,
which bounds the work to one reconnect per divergence.

## Verification

All in the inner Helix at `localhost:8080` with a live Zed agent, prompted to emit a long
marked paragraph then immediately run `sleep`, six rounds — text entry followed by a tool
call and a pause, the pattern that shows the bug.

- **With 12 forced patch drops, post-fix:** every completed entry matched the database
  byte-for-byte (465/496/534/494/477 vs server 465/496/534/494/477). Every drop was detected
  (`sequence gap 4 -> 6`, `9 -> 11`, …) and resynced. Zero shrink events.
- **Pre-fix, same injection:** permanent corruption, as tabulated above.
- **Clean run, no injection:** client `[489,69,492,69,451,69,438,69,461,69,476,67]` identical
  to server; 0 shrinks across 178 renders; all markers intact.
- **Forced mid-stream reconnect** (`offline`/`online`): all six paragraphs and all six tool
  calls remained complete, no truncation, no console errors.
- Screenshots in `design/tasks/002552_debug-assistant-message/screenshots/`.

Tests: 8 `applyPatch` cases including the real `"Albs"` corruption; Go tests for snapshot
flag/seq and for seq monotonicity including the no-op-must-not-consume-a-seq rule. Full
`TestWebSocketSyncSuite` passes.

## Deviation from the approved design

The approved design also proposed persisting `response_seq` on the interaction row so the
polled snapshot could be version-ranked against the live stream, and rewriting
`useLiveInteraction` as a seq-ranked selector. **Both were dropped**, because the evidence
disproved the hypothesis that motivated them: the poll never won a race in 300+ renders, so
the column would have been written and never read, and the selector rewrite would have been
a risky refactor addressing a race that does not occur. The plumbing was built, measured to
be unused, and reverted.

Note also that a version comparison alone would **not** have caught this bug: after a drop
the client had applied seq 46 and the row was stamped 46, so the client looked perfectly
current while holding corrupt text. **Gap detection is the load-bearing part.**

## Notes for whoever hits this next

- `patchEntriesRef` arrays only ever grow. If the UI *loses* an entry, that value did not
  come from the live patch stream — look at the poll.
- `MessageWithToolCalls` ignores its `text` prop whenever `responseEntries` is non-empty
  (`InteractionInference.tsx`). While streaming, `response_message` is empty and entries
  paint everything. Establish which prop is painting before instrumenting.
- A mid-word cut means a *byte-level* reconstruction fault, not a stale snapshot. A stale
  snapshot loses whole entries; a bad splice loses characters. Entry **count** tells them
  apart.
- Reproducing transport faults by waiting is hopeless. Inject them:
  skip a publish while advancing the server's delta baseline and the bug appears on demand.
- Go `omitempty` means zero-valued `patch_offset` / `total_length` arrive as `undefined` in
  JS. Normalise before comparing or every empty patch looks like divergence.
- Docker Compose lets the invoking shell's environment override `.env`. The agent
  environment exports an empty `ANTHROPIC_API_KEY`, which silently blanked the key in the
  api container and broke model listing. Export the values from `.env` explicitly when
  recreating the container.
