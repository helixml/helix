# Implementation Tasks: Fix Spec-Task Detail Live-Message Truncation/Lag

## Setup & repro
- [~] Bring up the inner Helix; register (`test@helix.ml` / `helixtest`), complete onboarding, create a spec task and confirm the Zed agent is live (`config->>'zed_thread_id'` is a non-empty UUID)
- [ ] Open the spec-task detail page and confirm the truncation/lag reproduces with a "print a sentence, then `sleep 30`" prompt

## Instrument
- [ ] Add temporary `[LIVE-RESULT]` `console.log` in `frontend/src/hooks/useLiveInteraction.ts` logging `{ src: LIVE|DB, crId, iiId, match, msgTail, lastTail, entries }` per render (HMR)
- [ ] Add server logging in `api/pkg/server/websocket_external_agent_sync.go` throttle block marking `LEADING` / `FORCE-FLUSH` / `TRAILING-FLUSH` publishes plus the newest text-entry tail (Air hot-reload)

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
