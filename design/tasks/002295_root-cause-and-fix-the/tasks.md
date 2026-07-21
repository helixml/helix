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

- [~] **US1/US2:** non-clean last turn (in-flight, then auth-errored) → Restart → reconnect sends `open_thread(<thread>)`, Zed reloads it (no blank, no new `thread_created`), `thread_reset=false`; a follow-up message climbs `last_zed_message_id` on the SAME thread with prior context. Paste log/DB output.
- [ ] **US3:** Restart alone (no config change) on a non-clean turn → thread preserved. Paste output.
- [ ] **US4:** Restart after a `complete` turn → preserved (no #2860 regression).
- [ ] **US5:** kill the ACP agent so the last interaction errors with `Session not found` / `Claude Agent process exited` → Restart resets and recovers cleanly.

## 5. Ship

- [ ] Write the root-cause + fix design doc under `helix/design/2026-07-21-restart-discards-thread-on-nonclean-turn.md` (mirror the #2860 writeup); include the confirmed clear-path, the gate, and pasted live evidence.
- [ ] Open a PR against `helixml/helix` (full URL); report the exact clear-path, the gate added, and the live test output (do NOT claim "covered by unit tests").
- [ ] Check CI (Drone / `gh pr checks`) green; fix and re-check if red.
