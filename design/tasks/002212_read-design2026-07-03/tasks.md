# Implementation Tasks: Fix Spec-Task Detail Live-Message Truncation/Lag

## Setup & repro
- [x] Bring up the inner Helix; registered, onboarded (testorg/testproj), created spec task, started Planning (live Zed agent provisioning)
- [x] Ran repro on the detail page (text turn, text→tool_call+25s sleep, long prose). Reported *text* truncation did NOT reproduce on current main — live view stays current

## Instrument
- [x] Add temporary `[LIVE-RESULT]` console.log in useLiveInteraction.ts
- [x] Add server LEADING/FORCE-FLUSH/TRAILING-FLUSH publish logging + entry tail (lastEntryTail helper)

## Diagnose
- [x] Captured console `[LIVE-RESULT]` + server `📤 [PUBLISH]` logs during the pause
- [x] Determined: id-guard always `src=LIVE` (not #1 symptom); server TRAILING-FLUSH fires fine (not #2). Real gap = DB write throttled to 5s with no trailing flush (cause-#1 mechanism)
- [x] Recorded evidence + conclusion in design.md (Implementation Notes)

## Fix (apply the branch the evidence points to; harden the other cheaply)
- [x] **Fix (cause-#1 mechanism):** added trailing-edge DB flush (`dbFlushTimer`, 500ms) via `flushStreamingFieldsToDB` + `UpdateInteractionStreamingFields`, bounding DB staleness to ~500ms
- [x] **Cause #2:** verified already working on main (server TRAILING-FLUSH + frontend LIVE track fully); no change needed. Frontend LIVE path left untouched

## Verify
- [ ] Live in inner Helix: the sentence stays complete during the `sleep 30` pause; message stays current within a few hundred ms
- [ ] Confirm no regression on completion (full final text/entries, no flicker) and on the regular Session page live stream
- [ ] `cd frontend && yarn build` passes; `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes

## Regression test & cleanup
- [ ] Add a regression test: server publish-timing (trailing flush fires after last chunk, no follow-up) and/or frontend test that text-entry-then-tool_call renders in full
- [ ] Remove (or gate) the temporary instrumentation
- [ ] Commit with conventional-commit message; open PR against `helixml/helix`; check Drone CI green
