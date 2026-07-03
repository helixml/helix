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
- [x] Live in inner Helix: live view stayed fully current every turn; measured DB catch-up now ~500ms on pauses (was up to 5s)
- [x] No regression: completion writes full final content; frontend LIVE path untouched
- [x] `go build ./pkg/server/` passes; frontend unchanged (instrumentation reverted, empty diff)

## Regression test & cleanup
- [x] Added Go regression test `TestMessageAdded_TrailingDBFlush` (throttled update → no immediate write → trailing flush persists within ~500ms); full WebSocketSyncSuite green
- [x] Removed all temporary instrumentation (frontend [LIVE-RESULT], server [PUBLISH] logs, lastEntryTail helper)
- [x] Committed + pushed feature branch `feature/002212-fix-spec-task-detail` (merged latest main; platform opens the GitHub PR → Drone CI runs then)
