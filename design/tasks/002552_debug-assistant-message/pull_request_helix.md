# Fix live chat messages truncated mid-word by dropped stream patches

## Summary

While an agent was mid-turn, the assistant message in the chat panel would render **cut off
mid-word**, with the following tool call missing — and the complete text would flicker in
and then revert. The database always held the full text, so this was purely a client-side
reconstruction bug.

`interaction_patch` events carry per-entry string deltas over embedded core NATS, which is
**best-effort: drop-on-slow-consumer, no redelivery**. Nothing detected a dropped message.
When one went missing, the next patch's `patch_offset` pointed past everything the client
held, and `applyPatch` handled that with a "pure append" — **splicing the two sides of the
gap together mid-word**, permanently, with no error anywhere.

Verified by fault injection (skip a publish while still advancing the server's delta
baseline — exactly a lost NATS message). Server truth for one entry began *"**Al**one on a
wave-hammered outcrop… the lighthouse keeper climbs the spiral iron stair"*; the client held
`"Al"` glued onto `"bs the spiral iron stair…"` = **`"Albs the spiral iron stair…"`**, with
131 characters silently gone.

The flicker follows: the DB is the *correct* source and the live patch buffer is the corrupt
one, so whenever the live value momentarily lost, the correct text painted and was then
overwritten again.

## Changes

- **Sequence numbers on patches** (`Seq` on `types.WebsocketEvent`) — monotonic, one per
  published patch; a no-op publish does not consume one. Any step other than `lastSeq + 1`
  is a dropped message.
- **`applyPatch` no longer guesses across a gap.** One uniform reconstruction
  (`slice(0, offset) + patch`) covers first patch, append, and backwards edit. Returns
  `null` on an out-of-range offset or a `total_length` mismatch. `total_length` is now a
  checksum; the lossy shrink branch that could destructively truncate content is removed.
- **Snapshot resync.** `buildFullStatePatchEvent` sets `Snapshot: true` and carries the
  sequence it was taken at, read under the same lock as the entries. On divergence the
  client forces a reconnect, and the server's existing late-joiner catch-up supplies the
  snapshot, which replaces the buffer wholesale. No new protocol surface.
- **Stop blanking state on reconnect.** `openHandler` cleared `currentResponses` and the
  patch buffer, dropping the view to the ≤5s-stale polled row until fresh data arrived — a
  visible long→short flicker in its own right. The snapshot overwrites the buffer anyway,
  so the last-good render stays up.
- Design review comment streaming handles the new `null` return instead of corrupting.

## Testing

End-to-end in the inner Helix with a live Zed agent (long marked paragraph → tool call →
pause, six rounds):

- **12 forced patch drops, post-fix:** every completed entry matched the database exactly;
  every drop detected (`sequence gap 4 -> 6`, …) and resynced; zero shrink events.
- **Pre-fix, same injection:** permanent corruption (448 vs 492 chars, cut mid-word).
- **Clean run:** client entry lengths identical to server, 0 shrinks across 178 renders.
- **Forced mid-stream WebSocket reconnect:** all paragraphs and tool calls stayed complete.
- 8 new `applyPatch` unit tests (including the real `"Albs"` corruption); Go tests for the
  snapshot flag/seq and seq monotonicity. Full `TestWebSocketSyncSuite` passes.

## Notes

Corrects the conclusion of `design/2026-07-03-spectask-live-message-truncation.md`, which
diagnosed this as trailing-edge lag and shipped a trailing DB flush (`6ff541aa6`). That
framing assumed the live stream was always the fresher source; in fact it is the one that
can be corrupt, and freshness tuning cannot address that.

A version/freshness comparison alone would **not** have caught this: after a drop the client
looked perfectly current while holding corrupt text. Gap detection is the load-bearing part.
The approved design's `response_seq` DB column and `useLiveInteraction` selector rewrite were
built, measured to be unused (the id-guard held in 300+ instrumented renders), and reverted.

Full write-up: `design/2026-08-04-chat-message-truncation-clobber.md`

## Screenshots

![Full text and tool calls survive 12 dropped patches](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002552_debug-assistant-message/screenshots/01-after-fix-survives-12-dropped-patches.png)
![Clean run: complete paragraph and tool call](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002552_debug-assistant-message/screenshots/02-clean-run-full-text-and-tool-calls.png)
