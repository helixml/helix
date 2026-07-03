# Task: root-cause & fix the spec-task detail live-message truncation / lag

## The bug
On the **spec-task detail page** (`/orgs/:org/projects/:id/tasks/:taskId`), while
an agent is mid-turn, the latest assistant message is shown **truncated / stale**
(e.g. `Stack is still co` instead of `Stack is still coming up. Let me poll for
the API to become ready.`). It **self-corrects when the next chunk arrives** — so
it is a *trailing-edge lag*, not a permanent freeze. If the agent pauses (e.g.
about to make a tool call), you see a stale view until the next chunk is sent.

## Confirmed (do not re-investigate — verified on the live outer instance)
- **The DB is correct.** `interactions.response_message` and `response_entries`
  both hold the FULL text; the truncated text is never the latest entry. A page
  reload (snapshot fetch) shows the full text. So this is **not** persistence and
  **not** the Zed↔Helix sync — Zed's `Zed.log` shows it only ever sends the FULL
  sentence, never a partial.
- **Server patch logic is fine.** `computePatch` (append fast-path + rune-diff)
  and the accumulator (`wsprotocol/accumulator.go`, map-keyed, fresh-copy
  `Entries()`) are correct on inspection. The frontend `applyPatch`
  (`utils/patchUtils.ts`) is also correct.
- **Transport is embedded core NATS** (`nats://127.0.0.1:4222`), best-effort
  (drop-on-slow-consumer, no redelivery), but no `NATS error` was logged.

## Render path (traced — this is what actually renders the message)
`SpecTaskDetailPage` → `SpecTaskDetailContent` → **`EmbeddedSessionView`**
(interactions from `useListInteractions`, **3s poll**) → for the last `Waiting`
interaction it renders **`InteractionLiveStream`** → **`useLiveInteraction`**,
which reads the live **`currentResponses`** entry-patch stream from the
streaming context. So the view IS wired to the live stream; the DB poll is only
the base list.

## The two candidate root causes — the task is to determine WHICH, then fix it
`frontend/src/hooks/useLiveInteraction.ts:55-92`:
```ts
const currentResponse = currentResponses.get(sessionId);
const responseMatchesInteraction = currentResponse?.id === initialInteraction?.id;
if (currentResponse && responseMatchesInteraction) { /* LIVE (50ms) */ }
else { /* falls back to initialInteraction = the 3s-polled DB */ }
```
1. **The id-guard fails** (`currentResponse.id !== initialInteraction.id`) so it
   falls back to the 3s-polled DB — and the server's streaming DB write is
   throttled to **5s, leading-edge only, no trailing flush**
   (`websocket_external_agent_sync.go`, `dbWriteInterval = 5s`, ~L1334). That
   combination = the stale-until-next-chunk lag.
2. **The id-guard passes (LIVE) but `currentResponses` itself is behind** — i.e.
   the entry-patch **trailing flush** (`flushTimer`, `publishInterval = 50ms`,
   ~L1386) isn't delivering the tail, so the last chunk isn't published until the
   next chunk triggers a leading-edge publish.

## How to reproduce & test (WORKS inside this helix-in-helix sandbox)
The outer instance couldn't be instrumented (Air won't hot-reload across a ZFS
bind-mount; headless Chrome/auth blocked). **In this inner Helix at
`localhost:8080` those work** — that's the whole point of running this as a spec
task. Suggested method:
1. Register/onboard, create a spec task so a live Zed agent is connected.
2. Instrument `useLiveInteraction` with a `console.log` of `{ src: LIVE|DB,
   msgTail, entries, lastTail, crId, iiId, match }` on each render (Vite HMR
   picks it up). Add server logging in `publishEntryPatchesToFrontend` to mark
   leading/forceflush/**trailing** publishes + the text-entry content tail (Air
   hot-reloads here).
3. Prompt the agent: *print a distinctive sentence, then run `sleep 30`* (text
   entry followed by a pause) and watch:
   - Console `[LIVE-RESULT]`: is `src` `LIVE` or `DB` during the lag?
     `DB` ⇒ cause #1 (id-guard). `LIVE` + stale tail ⇒ cause #2 (trailing flush).
   - Server logs: does a `TRAILING-FLUSH` publish fire ~50ms after the last text
     chunk, or only when the tool_call arrives?

## Fix (implement whichever the evidence points to; ideally make both robust)
- If cause #1: make the streaming context set `currentResponse.id` to the active
  interaction id so the guard matches (or fix how `useLiveInteraction` matches),
  and/or add a **trailing-edge DB flush** mirroring the frontend `flushTimer` so
  the DB fallback is never >~300-500ms stale.
- If cause #2: fix the entry-patch trailing flush so the tail is always published
  shortly after the last chunk (and don't let the 3s `useListInteractions` poll
  clobber newer live entries — consider pausing it while streaming).
- Add a regression test where possible (server publish timing; or a frontend
  test that a text entry followed by a tool_call renders in full).

## Definition of done
On the spec-task detail page, the in-progress assistant message stays current
within ~a few hundred ms during streaming and never sits truncated waiting for
the next chunk — verified live in the inner Helix with the repro above.
