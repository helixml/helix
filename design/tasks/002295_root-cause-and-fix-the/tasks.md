# Implementation Tasks: Restart Must Not Discard a Healthy Zed Thread on a Non-Clean Last Turn

## 1. Reproduce live (confirm the exact trigger)

- [ ] Register/login to the inner Helix at `http://localhost:8080`, complete onboarding.
- [ ] Create a `zed_external` `claude_code` **spec task** (repo-backed → Zed connects); send 2+ messages so `sessions.config->>'zed_thread_id'` is a non-empty UUID with completed turns.
- [ ] **Repro A (in-flight):** start a turn and, while it is still `waiting`, click **Restart** → observe `session_handlers.go:2609` logs `thread_reset=true`, reconnect shows `zed_thread_id=` empty, thread comes up **blank** before any message.
- [ ] **Repro B (auth-errored, mirrors incident):** edit the app api_key→subscription with an invalid/absent subscription token, send a turn (fails 401), click Restart → same thread loss.
- [ ] **Repro C (restart alone):** with a non-clean last turn but **no** config change, Restart → confirm whether it also loses the thread (reviewer's suspicion).
- [ ] Record `thread_reset`, the reconnect `zed_thread_id`, and any `thread_created` for each; confirm no `thread_load_error` on the original thread.

## 2. Implement the fix (done first; Air hot-reloads before live verify)

- [x] Add `threadIsWedged(ctx, session)` that returns true **only** on positive wedge evidence: last interaction `State==error` AND (`isAgentCrashError(last.Error)` OR `isAuthoritativeMissingThreadError(last.Error)`). `waiting`/`complete`/`interrupted`, and non-wedge errors (auth/429/provider/transport/cancel), return false. (Replaced `lastInteractionCompletedCleanly`.)
- [x] In the human-restart entrypoint (`restartCrashedAgentThread`), replaced `resetThread := !lastInteractionCompletedCleanly(...)` with `resetThread := s.threadIsWedged(...)`.
- [x] Autonomous `maybeAutoRestartCrashedAgent` left as-is — already gated on `isAgentCrashError`, consistent with the new wedge definition.
- [x] WARN red-flag log NOT added: with the new gate, clearing a `complete`/`waiting` thread is structurally impossible on the human path, so the log would be dead code. Documented in the `threadIsWedged` comment instead.
- [ ] Deferred (note in PR): the unconditional clear at `session_switch_agent_handlers.go:237` — out of scope for this incident (switch-agent, not restart); flag as follow-up.

## 3. Build & unit test

- [x] `go build ./api/pkg/server/ ./api/pkg/types/` — passes.
- [x] Unit tests `TestThreadIsWedged` + `TestButtonPreservesHealthyThreadResetsWedged`: `waiting`→preserve, `complete`→preserve, auth-`error`→preserve, `Session not found`-`error`→reset, `no thread found with ID`-`error`→reset. Pass. (Mechanism only — NOT acceptance evidence.)

## 4. Live acceptance test (mandatory — connected Zed, not seeded rows)

- [x] **US1** (the incident): restart with newest interaction = `waiting` (in-flight) → `thread_reset=false`; reconnect logged `zed_thread_id=69b7bfb6…` + `Sending open_thread` (reattach, NOT blank); no new `thread_created`; follow-up recalled the codeword "PANGOLIN-42" at `last_zed_message_id=22` on the SAME thread. (Old code would have `thread_reset=true` → blank.) Evidence in `live-test-results.md`.
- [x] **US3:** Test B (US1) was a restart with **no** config change on a non-clean (`waiting`) turn → thread preserved. Confirms restart-alone no longer loses context.
- [x] **US4:** restart with last turn `complete` → `thread_reset=false`, thread `69b7bfb6` preserved (no #2860 regression).
- [x] **US5:** killed the ACP agent mid-turn → the errors are TRANSPORT errors (`send failed because receiver is gone`, `agent turn aborted …`), NOT the missing-thread markers → gate preserves → **restart recovered full context** (codeword recalled, `last_zed_message_id=34`), better than the old reset. The genuine unloadable-thread reset is covered by unit tests + the unchanged lazy `recoverMissingThread` net. Details + honest scope in `live-test-results.md`.

## 5. Ship

- [x] Wrote the root-cause + fix design doc `helix/design/2026-07-21-restart-discards-thread-on-nonclean-turn.md` (mirrors the #2860 writeup) with the confirmed clear-path, the gate, and live evidence.
- [x] Pushed branch `feature/002295-restart-must-not-discard` (merged latest `origin/main`, rebuilds clean). PR description at `pull_request_helix.md`. Platform opens the GitHub PR on "Open PR".
- [ ] Check CI green once the PR exists (Drone) — pending PR creation.

## 6. Deferred / follow-up

- [ ] The unconditional `ZedThreadID = ""` at `session_switch_agent_handlers.go:237` (in-place switch-agent) is out of scope for this restart incident but should get the same wedge/kind gate; flagged for a follow-up.
