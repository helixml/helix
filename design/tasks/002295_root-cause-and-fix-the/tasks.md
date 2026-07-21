# Implementation Tasks: Preserve Zed Thread When Editing an Agent's Model/Provider/Credential

## 1. Reproduce & root-cause (live, connected Zed)

- [ ] Register/login to the inner Helix at `http://localhost:8080`, complete onboarding.
- [ ] Create a `zed_external` `claude_code` **spec task** (repo-backed → Zed connects), send 2+ messages so `sessions.config->>'zed_thread_id'` is a non-empty UUID.
- [ ] Add a distinctive log line at all four `ZedThreadID = ""` sites (`session_handlers.go:2581`, `session_switch_agent_handlers.go:237`, `websocket_external_agent_sync.go:3597`, `session_clear.go:90`) logging `session_id`, `prev_thread_id`, caller/reason.
- [ ] Reproduce the **exact** operator sequence: (a) change the model in the **Zed desktop UI** (native default_model), (b) edit the **Helix app** config — model + flip credential **api_key → subscription** (save), (c) go to the spec task and click **Restart**, (d) send one message.
- [ ] Capture whether any `thread_load_error` appears on reconnect, its exact `error` string, and whether `isAuthoritativeMissingThreadError` matched (distinguishes site 3a from a direct DB clear at site 1/2).
- [ ] Read `helix-api-1` logs + DB to confirm the **exact** clear-path that fired; record it in the root-cause report. (Recovery evidence already rules out an unloadable thread — preserving the pointer is sufficient.)

## 2. Implement the gate

- [ ] Add `agentKindChanged(prev, target, prevAgentName, targetAgentName)` helper.
- [ ] In `switchAgentInPlaceForNextTurn`, replace the unconditional clear at line 237 with `resetThread = agentKindChanged(...) || !lastInteractionCompletedCleanly(...)`; only clear `ZedThreadID` / set `AgentSwitchedAt` when `resetThread`.
- [ ] When not resetting: skip the `fork_seed` reseed + synthetic handoff, keep `publishAgentConfigChange`, and arm the restart fallback **only** when a reset actually happened.
- [ ] If a desktop recreate is needed for new subscription env, ensure it preserves the thread pointer (mirror `restartSessionContainer(resetThread=false)`).
- [ ] Add the WARN "cleared a thread whose last interaction was `complete`" red-flag log to sites 1 and 2.
- [ ] If the repro implicated site 1 (restart) via a non-`complete` last interaction (e.g. a lingering `Waiting` handoff), fix that root cause rather than loosening the gate.
- [ ] If the repro implicated **site 3a** (a one-shot `no thread found` at the subscription-mode recreate boot): don't treat the first post-recreate authoritative error as terminal — wait for `agent_ready` and/or retry `open_thread` once before `recoverMissingThread` zeroes the pointer, so a transient boot miss can't orphan a healthy thread. (Do NOT weaken genuine stale-thread recovery.)

## 3. Build & unit test

- [ ] `go build ./api/pkg/server/ ./api/pkg/types/`.
- [ ] Add a unit test asserting the gate: same-runtime model/credential change keeps `ZedThreadID`; runtime/agent-name change or wedged last-interaction clears it. (Mechanism only — NOT acceptance evidence.)

## 4. Live acceptance test (mandatory — connected Zed, not seeded rows)

- [ ] **US1 model:** change opus→sonnet, send a message → `zed_thread_id` unchanged, `last_zed_message_id` climbs on the SAME thread, no new `thread_created`, reply shows prior context. Capture log/DB output.
- [ ] **US2 credential:** api_key ⇄ subscription → context preserved across any desktop recreate; `open_thread` re-attaches. Capture output.
- [ ] **US3 provider:** same-runtime provider change → context preserved.
- [ ] **US4 agent-kind:** claude_code ⇄ zed-agent → thread may reset; new agent comes up cleanly and takes a message.
- [ ] **US5 wedged:** kill the ACP agent mid-turn → restart/next-turn resets and recovers.

## 5. Ship

- [ ] Write/refresh the root-cause + fix design doc under `helix/design/2026-07-21-*.md` (mirror the #2860 writeup); include the found clear-path, the gate, and pasted live evidence.
- [ ] Open a PR against `helixml/helix` with full URL; report the exact clear-path, the gate added, and the live test output (do NOT claim "covered by unit tests").
- [ ] Check CI (Drone / `gh pr checks`) green; fix and re-check if red.
