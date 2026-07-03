# Requirements: Fix Spec-Task Detail Live-Message Truncation/Lag

## Background

On the spec-task detail page (`/orgs/:org/projects/:id/tasks/:taskId`), while a Zed
agent is mid-turn, the latest assistant message renders **truncated / stale** — e.g.
`Stack is still co` instead of `Stack is still coming up. Let me poll for the API to
become ready.`. It **self-corrects when the next chunk arrives**, so it is a
*trailing-edge lag*, not a permanent freeze. When the agent pauses (e.g. before a tool
call), the stale view persists until the next chunk is sent.

Already verified on the live outer instance (do **not** re-investigate):
- **DB is correct.** `interactions.response_message` and `response_entries` hold the full
  text; a page reload (snapshot fetch) shows it complete. Not persistence, not Zed↔Helix
  sync (Zed only ever sends full sentences).
- **Server patch logic is correct on inspection** (`computePatch`, `wsprotocol/accumulator.go`)
  and so is the frontend `applyPatch` (`utils/patchUtils.ts`).
- **Transport** is embedded core NATS, best-effort; no `NATS error` logged.

The render path is:
`SpecTaskDetailPage` → `SpecTaskDetailContent` → `EmbeddedSessionView` (base list via
`useListInteractions`, 3s poll) → `InteractionLiveStream` → `useLiveInteraction`, which
overlays the live `currentResponses` entry-patch stream from the streaming context.
`MessageWithToolCalls` renders `responseEntries` (the entry stream) when present, falling
back to the flat `message` text.

There are **two candidate root causes**; the task is to determine which one actually
occurs live, then fix it (ideally hardening both):

1. **id-guard fails.** In `useLiveInteraction.ts` the guard
   `currentResponse?.id === initialInteraction?.id` fails, so the hook falls back to the
   3s-polled DB interaction, whose streaming DB write is throttled to `dbWriteInterval = 5s`
   (leading-edge, no trailing flush) in `websocket_external_agent_sync.go`. That combination
   produces stale-until-next-chunk lag.
2. **id-guard passes (LIVE) but `currentResponses` is behind** — the entry-patch trailing
   flush (`flushTimer`, `publishInterval = 50ms`) isn't delivering the tail, so the last
   chunk isn't published until the next chunk triggers a leading-edge publish.

## User Stories

### US-1: As a user watching a spec-task agent work, I see the assistant's latest words promptly
- **AC1.1:** During streaming, the in-progress assistant message stays current within a few
  hundred ms of what the agent has emitted; it never sits truncated waiting for the next chunk.
- **AC1.2:** When the agent emits a sentence then pauses (e.g. before a tool call or a
  `sleep`), the full sentence is visible during the pause — not a truncated prefix.
- **AC1.3:** Behaviour is verified **live in the inner Helix** (`localhost:8080`) with the
  repro below, not only via unit tests.

### US-2: As a developer, I can tell which root cause occurred from evidence
- **AC2.1:** `useLiveInteraction` is instrumented (temporary `console.log`, HMR-picked-up)
  to log per render: `{ src: LIVE|DB, msgTail, entries, lastTail, crId, iiId, match }`.
- **AC2.2:** The server publish path (`publishEntryPatchesToFrontend` / the throttle block
  in `websocket_external_agent_sync.go`) is instrumented (Air-hot-reloaded) to mark
  leading / force-flush / **trailing** publishes plus the tail of the text-entry content.
- **AC2.3:** The evidence unambiguously identifies cause #1 (`src=DB` during lag) vs cause
  #2 (`src=LIVE` + stale tail, no `TRAILING-FLUSH` after the last chunk), and is recorded in
  the design doc / commit message.

### US-3: The fix does not regress completion or other chat surfaces
- **AC3.1:** Completed interactions still show full final text and entries (no flicker/blank).
- **AC3.2:** The regular Session page live stream is unaffected (it shares `useLiveInteraction`).
- **AC3.3:** No new console errors; frontend `yarn build` and `go build` pass.

### US-4: Regression coverage
- **AC4.1:** Add a regression test where feasible — server publish-timing (a trailing flush
  fires shortly after the last chunk with no follow-up), or a frontend test that a text entry
  followed by a tool_call renders in full.

## Reproduction (inner Helix at localhost:8080)
1. Register (`test@helix.ml` / `helixtest`), complete onboarding, create a spec task so a
   live Zed agent connects (liveness: `config->>'zed_thread_id'` is a non-empty UUID).
2. Add the instrumentation from US-2.
3. Prompt the agent: *print a distinctive sentence, then run `sleep 30`*.
4. Watch during the pause: console `src` = `LIVE` or `DB`? Server logs: does a
   `TRAILING-FLUSH` fire ~50ms after the last text chunk, or only when the tool_call arrives?

## Definition of Done
On the spec-task detail page the in-progress assistant message stays current within ~a few
hundred ms during streaming and never sits truncated waiting for the next chunk — verified
live in the inner Helix with the repro above. Instrumentation removed (or gated) before
final commit; a regression test added where feasible.

## Open Questions
- **Fix both, or only the observed cause?** The source doc says "implement whichever the
  evidence points to; ideally make both robust." Assumption: fix the observed cause and add
  cheap hardening to the other (e.g. a trailing DB flush *and* a correct id-guard) since they
  are low-risk. Confirm if you'd rather keep the change minimal to the single proven cause.
- **Should the 3s `useListInteractions` poll be paused while streaming?** The doc suggests
  "don't let the 3s poll clobber newer live entries — consider pausing it while streaming."
  Assumption: only do this if evidence shows the poll is clobbering live entries; otherwise
  leave polling as-is to avoid affecting the base-list snapshot behaviour.
- **Acceptable staleness target.** Assumption: "a few hundred ms" (≤ ~300–500ms) as stated
  in the doc is the bar. Confirm if a tighter bound is required.
