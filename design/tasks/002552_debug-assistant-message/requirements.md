# Requirements: Fix Live Chat Message Truncation from Stale-Poll Clobber

## Background

On the spec-task detail page Chat tab, while an agent is mid-turn, the latest assistant
message renders **cut off mid-word** and the tool call that follows it is **not rendered
at all**. The attached screenshot (2026-08-04 12:59:58 BST, session `ses_01kz5zgx69c2r68vy1bbeqpbr8`,
interaction `int_01kz69snpbkz2ch66ngeer3vbx`) shows the Helix panel stopping at
"…gate the nav behind the **exist**" while the Zed window in the *same frame* shows
"…the **existing flag — pages stay reachable by direct URL for demos.**" followed by
**Write find-ai/lib/features.ts**.

Luke's key observation: *"I always see a flicker of the complete text that then flickers
back to the truncated version."* The transition is **long → short**, repeatedly. That is a
**clobber** — newer state being overwritten by older state — not trailing-edge lag.

Already established (do not re-derive):
- The DB holds the complete text (`response_message` 159044 chars, 118 `response_entries`)
  including the following tool call. Purely a render bug.
- The server patch publisher ran continuously and sub-second, `entry_count` monotonically
  increasing, with zero publish/flush failures in the window.
- The Jul-3 investigation (`design/2026-07-03-spectask-live-message-truncation.md`) shipped
  `6ff541aa6` (trailing-edge DB flush). That fix is live; the bug persists. Its
  "trailing-edge lag" framing is **superseded**.

## User Stories

### US-1 — Live message stays complete (primary)
**As a** user watching an agent work on the spec-task detail page,
**I want** the assistant's in-progress message to remain complete and to include every
tool call already issued,
**so that** I can follow what the agent is doing without reloading the page.

Acceptance criteria:
- [ ] With a live Zed agent streaming, the rendered assistant text is never a strict
      prefix of text the server has already published — no mid-word truncation.
- [ ] A tool call that has appeared in the stream never disappears from the chat panel.
- [ ] No long → short flicker: rendered text length and entry count for a given
      interaction are **monotonically non-decreasing** for the duration of that
      interaction, except for legitimate server-driven content replacement.
- [ ] Verified end-to-end in a real browser against the inner Helix at
      `http://localhost:8080` — a text entry followed by a tool call and a ≥30s pause,
      watched for at least three 3s poll cycles, with the full paragraph and the tool
      call on screen throughout.

### US-2 — A dropped or reordered patch is recoverable, not permanent
**As a** user on a flaky connection,
**I want** the chat to recover automatically when a best-effort NATS patch is dropped or
arrives out of order,
**so that** a single lost packet doesn't leave the message permanently wrong.

Acceptance criteria:
- [ ] Patch events carry a per-interaction monotonic sequence number.
- [ ] The client detects a sequence gap, an inconsistent `patch_offset`, or a
      `total_length` mismatch and treats it as divergence.
- [ ] On divergence the client resyncs from a server snapshot rather than continuing to
      apply deltas to a diverged buffer.
- [ ] Deliberately dropping one server publish, delivering two out of order, and
      reconnecting the WebSocket mid-interaction each leave the UI correct within one
      resync — demonstrated, not reasoned about.

### US-3 — One source of truth for the live interaction
**As a** developer maintaining this code,
**I want** the rendered message text and the rendered entries to come from the same
versioned source,
**so that** the two can't disagree and a stale value can't outrank a fresh one.

Acceptance criteria:
- [ ] `useLiveInteraction` selects its source by comparing a real version, never by
      string length, arrival order, or an incidental `isComplete` gate.
- [ ] `message` and `responseEntries` are derived from the same object.
- [ ] The accidental-correctness selector
      `isComplete ? (streaming || completed) : (completed || streaming)` is gone.
- [ ] No dual code paths or fallbacks (repo rule) — one merge rule, applied to the
      streaming and completed cases alike.

### US-4 — Regression coverage
Acceptance criteria:
- [ ] Test: a poll result racing a live patch — the older value must lose.
- [ ] Test: a dropped patch and a reordered patch — divergence is detected.
- [ ] Test: server-side sequence numbers are monotonic per interaction and the snapshot
      carries the seq of the last published patch.
- [ ] `cd frontend && yarn build` and `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
      both pass; CI green on the PR.

### US-5 — Findings written up
Acceptance criteria:
- [ ] `design/2026-08-04-chat-message-truncation-clobber.md` records the confirmed root
      cause with the instrumentation evidence, and explicitly corrects the Jul-3 doc's
      conclusion.
- [ ] PR opened against `helixml/helix`, full URL given in the summary (repo rule).

## Out of Scope
- Changing `dbWriteInterval` (5s) or `publishInterval` (50ms). Tuning throttles hides the
  ordering bug instead of fixing it.
- Pausing the 3s `useListInteractions` poll. The poll is legitimate; the merge rule is
  what's broken.
- The Zed↔Helix sync protocol and persistence — both confirmed correct.
- Any change to the production/meta instance. All work happens in the sandbox's inner
  Helix at `localhost:8080`.

## Open Questions

1. **Confirmation before commitment.** The design below names the **stale-poll clobber**
   (suspect A) as the root cause, on the strength of one structural fact: the client's
   patch buffer arrays only ever grow (`while (currentEntries.length < entryCount) push`
   in `streaming.tsx:491`), so live entries *cannot* lose the tool-call entry — yet the
   screenshot shows it gone. That is a property of a DB snapshot with fewer entries, not
   of `applyPatch`'s slice. Assumption: instrumentation will confirm this. If the logs
   instead show `entryCount` dropping on a *patch*, suspect B owns it and the fix
   emphasis shifts. The plan instruments first and does not skip that step.
2. **New DB column.** The design adds `response_seq` to the interactions row so a polled
   snapshot can be version-compared against the live stream. Is a schema addition
   acceptable here (GORM AutoMigrate), or would you rather the rule be "the live stream
   is authoritative for a non-terminal interaction, the poll only for terminal ones" —
   which needs no column but is a state-based rule rather than a true total order?
   Assumed: add the column.
3. **Version compatibility.** Since `seq` becomes required for the merge rule, a new
   frontend against an old API would see no seq. Repo rules forbid fallback paths, so the
   assumption is API and frontend ship together in one PR and no compatibility shim is
   added. Confirm that's acceptable for rolling deploys.
4. **Blast radius.** `patchUtils.ts` is shared with design-review comment streaming, and
   the same `useLiveInteraction` hook is used by the main chat page, not just the
   spec-task detail page. Assumption: fix the shared code once and verify both surfaces.
   Confirm the main chat page is in scope for verification.
5. **Snapshot atomicity.** The resync snapshot must be built under the same lock as the
   publish path, or a client can apply a delta that's already baked into its snapshot.
   Assumption: `sctx.mu` is the right lock and the snapshot can be taken under it without
   adding contention on the streaming hot path. To be verified while implementing.
