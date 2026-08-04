# Design: Fix Live Chat Message Truncation from Stale-Poll Clobber

## 1. Diagnosis

### 1.1 What the code actually does today

Render path (re-verified in this repo, unchanged since the Jul-3 trace):

```
SpecTaskDetailPage → SpecTaskDetailContent → EmbeddedSessionView
   └── useListInteractions(..., { refetchInterval: 3000 })      EmbeddedSessionView.tsx:215
   └── InteractionLiveStream                                     InteractionLiveStream.tsx:41
         └── useLiveInteraction(sessionId, initialInteraction)   hooks/useLiveInteraction.ts
               └── useStreaming().currentResponses               contexts/streaming.tsx
```

Two independent producers write the same rendered value:

| Producer | Cadence | Path |
|---|---|---|
| Live entry patches | ~50ms publish, batched to a RAF | NATS → `streaming.tsx:480-542` → `patchEntriesRef` → `currentResponses` |
| DB snapshot | 3s poll of a row written on a 5s throttle | `useListInteractions` → `initialInteraction` |

`useLiveInteraction.ts:57-94` merges them:

```ts
const responseMatchesInteraction = currentResponse?.id === initialInteraction?.id;
if (currentResponse && responseMatchesInteraction) {
  setInteraction(prev => ({ ...prev, ...currentResponse }));   // LIVE
} else {
  if (initialInteraction) setInteraction(initialInteraction);  // DB row, WHOLESALE
}
```

Effect deps are `[sessionId, currentResponses, initialInteraction]`. `initialInteraction`
is a fresh object on **every 3s poll**, so this effect re-runs every 3s for the lifetime
of the turn. There is **no ordering rule** between the two producers — the last writer
wins, and one of the two writers is systematically up to ~8s behind (3s poll interval on
top of a ≤5s DB write throttle).

### 1.2 Which value paints the truncated paragraph

`MessageWithToolCalls` (`InteractionInference.tsx:102`) takes the **structured path**
whenever `responseEntries` is non-empty, and ignores the `text` prop entirely. So the
paragraph on screen comes from `responseEntries`, which for an in-progress interaction
resolves (`useLiveInteraction.ts:142-144`) to `interaction.response_entries` — i.e.
whichever of the two producers wrote `interaction` last.

This answers the brief's §C question: **entries paint it, not `message`.**

### 1.3 Why this is suspect A (poll clobber), not suspect B (destructive slice)

The decisive structural fact, `streaming.tsx:489-493`:

```ts
const currentEntries = patchEntriesRef.current.get(interactionId) || [];
while (currentEntries.length < entryCount) currentEntries.push({...});   // grows only
```

The live entries array **never shrinks**. `applyPatch`'s slice can truncate the *content
of one entry*; it cannot delete the tool-call entry. The screenshot shows the tool-call
entry **absent altogether**. An array with fewer entries can only have come from a source
that had fewer entries — the polled DB snapshot.

The mid-word cut is the natural shape of that snapshot: a throttled write persists the
accumulator's content at an arbitrary instant, which lands mid-token. "the exist" is where
the writer happened to be, not where a `slice()` chose to cut.

And the flicker follows mechanically: a patch RAF fires → live entries → full paragraph +
tool call render; ~3s later the poll resolves → `setInteraction(initialInteraction)` →
back to the stale snapshot. Long → short → long → short, on a 3s period. The screenshot
caught the short phase. This is exactly Luke's report.

**Suspects B and C are real but secondary**, and both are fixed by the same work:

- **B** — `applyPatch`'s `if (totalLength < newContent.length) newContent.slice(0, totalLength)`
  is lossy and irreversible, and nothing detects a dropped best-effort NATS message. A
  dropped patch takes the `patch_offset >= currentContent.length` "pure append" branch, so
  the gap is **silently swallowed** and that entry stays permanently short with no error.
  Latent and unrecoverable; must be fixed even though it is not what the screenshot shows.
- **C** — the patch handler writes only `response_entries` into `currentResponses`
  (`streaming.tsx:528-537`), never `response_message`, and the `!isSameInteraction` branch
  discards every other field. So `message` and `responseEntries` come from different
  producers with different latencies. Structural, and the cause of the confusing
  `completedMessage || safeResponseMessage || lastKnownMessage` ranking chain.

### 1.4 Correction to the Jul-3 doc

`design/2026-07-03-spectask-live-message-truncation.md` concluded "trailing-edge lag" and
shipped a trailing DB flush (`6ff541aa6`). Its *facts* still hold; its *framing* was wrong.
Reducing DB staleness cannot fix a merge that has no ordering rule — it only shrinks the
window. The doc's cause #1 ("the id-guard fails, so it falls back to the polled DB") was
close, but it treated the fallback as a lag problem rather than as a **write of older
state over newer state**, and so the fix targeted the wrong side of the race.

## 2. Design

### 2.1 Principle

Give the live stream and the persisted row a **shared total order**, and make every
consumer obey it. One merge rule, no state-based special cases, no fallbacks.

### 2.2 Server: sequence numbers

- Add `Seq uint64` to `types.WebsocketEvent` (`api/pkg/types/types.go:951`), set on every
  `interaction_patch` publish. Monotonic per interaction, incremented under `sctx.mu` in
  the publish path (`websocket_external_agent_sync.go` ~L4380-4430).
- Add `Snapshot bool` to the same envelope. `buildFullStatePatchEvent`
  (`websocket_external_agent_sync.go:4424`, used by the late-joiner catch-up at
  `websocket_server_user.go:141`) sets it, and carries the seq of the last published patch.
  A snapshot means **replace**, not **patch**.
- Persist the same seq onto the interaction row as `response_seq`, stamped inside
  `flushStreamingFieldsToDB` (the leading-edge write and the trailing flush both go through
  it, `websocket_external_agent_sync.go` ~L1370-1405), so a polled row is directly
  comparable to a live patch. GORM AutoMigrate handles the column.
- The snapshot must be built under `sctx.mu` together with the seq read, or a client can
  apply a delta already baked into its snapshot.

### 2.3 Client: one versioned store

- `patchEntriesRef` becomes a store of `{ seq, entries, message }` per interaction id.
  The patch handler writes **both** entries and message into `currentResponses`, killing
  suspect C, and stops discarding fields on the `!isSameInteraction` branch.
- `useLiveInteraction` becomes a thin selector: take the live value if
  `live.seq >= polled.response_seq`, else the polled row. Same comparator whether or not
  the interaction is complete — the completing `interaction_update` naturally carries the
  highest seq and wins. Delete `completedMessage || safeResponseMessage || lastKnownMessage`
  and the `isComplete ? (streaming || completed) : (completed || streaming)` selector; both
  are compensations for the missing order.

### 2.4 Client: divergence detection and resync

- Track the last applied seq per interaction. `seq !== lastSeq + 1` ⇒ diverged.
- `applyPatch` stops being lossy. With a correct `patch_offset`,
  `slice(0, offset) + patch` **is** the complete new content, so the shrink branch is
  provably unnecessary — delete it. `total_length` becomes a checksum, not a truncator.
  Violations of `patch_offset <= currentContent.length` or
  `total_length === result.length` ⇒ diverged.
- On divergence: request a snapshot from the server, replace the store, reset the seq.
  Reuse `buildFullStatePatchEvent` — the mechanism already exists for late joiners; this
  adds a client-triggered entry point. Do **not** keep applying deltas to a diverged buffer.
- This also fixes the existing reconnect hole: `openHandler` (`streaming.tsx:599-612`)
  clears `patchEntriesRef` to `[]`, after which the next delta with a large `patch_offset`
  hits the pure-append branch and produces entries that are empty or delta-only. With
  snapshot-on-resync that path becomes correct rather than accidentally correct.

### 2.5 Deliberately not doing

- Not tuning `dbWriteInterval` / `publishInterval` — masks the ordering bug.
- Not pausing the 3s poll while streaming — the poll is legitimate; with a seq comparator
  a stale poll simply loses, which is the property we actually want.
- Not comparing string lengths to decide freshness. Length is not a version: a legitimate
  server-driven replacement (tool status change) can shorten content.

## 3. Verification

Per CLAUDE.md, no confidence not earned. A unit test on `applyPatch` is **not** evidence.

1. **Instrument first, fix second.** Log on every `useLiveInteraction` render:
   `{ src: LIVE|DB, crId, iiId, guardMatched, msgLen, lastKnownLen, entryCount, lastEntryTail }`.
   The hunted signal is `msgLen` or `entryCount` going **down**; capture whether the drop
   coincides with a poll resolution, a patch, or a streaming-state clear. Vite HMR and Air
   both hot-reload in the inner Helix. Remove the temporary logs before the PR; keep only a
   permanent warning on detected divergence.
2. **Repro** in the inner Helix: register `test@helix.ml` / `helixtest`, onboard, create a
   spec task so a real Zed agent streams. Prompt it to print a distinctive long sentence
   then immediately run a tool call / `sleep 30`.
3. **Force the failure modes** rather than waiting for them: skip one server publish,
   deliver two out of order, reconnect the WebSocket mid-interaction. Record the UI in each.
4. **End-to-end proof** in a real browser: full paragraph *and* the following tool call
   stay on screen through the tool call and the pause, across at least three poll cycles,
   with no flicker. Screenshots into `screenshots/`.

## 4. Notes for future agents

- `patchEntriesRef` arrays only ever grow. If you see the UI **lose** an entry, the value
  did not come from the live patch stream — look at the poll. This one fact collapses most
  of the search space for this class of bug.
- `MessageWithToolCalls` ignores its `text` prop whenever `responseEntries` is non-empty
  (`InteractionInference.tsx:102`). When debugging "wrong text on screen", establish which
  prop is actually painting before instrumenting anything.
- A mid-word cut points at a **snapshot of a stream taken at an arbitrary instant**
  (throttled DB write), whereas a `slice(0, n)` cut also lands mid-word — the two are
  indistinguishable from the text alone. Use entry *count* to tell them apart.
- Effect deps containing a React Query result object (`initialInteraction`) re-run on
  every poll tick, even when the data is unchanged, because the object identity changes.
  That is what turns a benign fallback into a 3s clobber loop.
- Transport is embedded core NATS: best-effort, drop-on-slow-consumer, **no redelivery**.
  Any client that applies deltas without gap detection will diverge silently and
  permanently. `computePatch` accumulates per *stream*, not per *client*, so the server
  cannot detect it for you.
