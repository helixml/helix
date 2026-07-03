# Implementation Tasks: Fix Spec-Task Detail Live-Message Truncation/Lag

## Setup & repro
- [x] Bring up the inner Helix; registered, onboarded (testorg/testproj), created spec task, started Planning (live Zed agent provisioning)
- [ ] Open the spec-task detail page and confirm the truncation/lag reproduces with a "print a sentence, then `sleep 30`" prompt

## Instrument
- [x] Add temporary `[LIVE-RESULT]` console.log in useLiveInteraction.ts
- [x] Add server LEADING/FORCE-FLUSH/TRAILING-FLUSH publish logging + entry tail (lastEntryTail helper)

## Diagnose
- [ ] Run the repro; capture console + server logs during the pause
- [ ] Determine the cause: `src=DB` ⇒ cause #1 (id-guard/DB fallback); `src=LIVE` + stale tail with no `TRAILING-FLUSH` ⇒ cause #2 (entry-patch trailing flush)
- [ ] Record the evidence and conclusion in the design doc / commit message

## Fix (apply the branch the evidence points to; harden the other cheaply)
- [ ] **Cause #1:** correct the id match so `currentResponse.id === initialInteraction.id` holds for the active interaction, and/or add a trailing-edge DB flush (~300–500ms `time.AfterFunc`) via `UpdateInteractionStreamingFields` so the DB fallback is never >~half a second stale
- [ ] **Cause #2:** ensure the `flushTimer` trailing publish always fires shortly after the last chunk (capturing the latest entries, no race with completion teardown); pause/ignore the 3s poll for the active interaction while streaming only if it is observed clobbering live entries

## Verify
- [ ] Live in inner Helix: the sentence stays complete during the `sleep 30` pause; message stays current within a few hundred ms
- [ ] Confirm no regression on completion (full final text/entries, no flicker) and on the regular Session page live stream
- [ ] `cd frontend && yarn build` passes; `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes

## Regression test & cleanup
- [ ] Add a regression test: server publish-timing (trailing flush fires after last chunk, no follow-up) and/or frontend test that text-entry-then-tool_call renders in full
- [ ] Remove (or gate) the temporary instrumentation
- [ ] Commit with conventional-commit message; open PR against `helixml/helix`; check Drone CI green
