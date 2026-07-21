# Live Test Results — Restart preserves the Zed thread on a non-clean last turn

Tested against the **live inner Helix** (`localhost:8080`) with a real connected
Zed, spec task `spt_01ky2asqpcqjz5v0x6kd9wzhpd`, session
`ses_01ky2av2prntmc1v61fv6f0yrv`, Zed thread `69b7bfb6-…`. Fix running via Air
hot-reload (binary built 12:31Z from the committed change).

## Setup
Registered `test@helix.ml`, org `testorg`, project `testproj`, **Claude Code +
Anthropic API Key** agent (claude-opus-4-8) — matching the incident's starting
state. Built two turns of history and taught the agent a codeword **PANGOLIN-42**.

## US4 — restart after a `complete` turn (regression, #2860 must hold)
```
POST /sessions/ses_.../restart-agent
→ {"thread_reset":false}
session_handlers.go:2626  Restart agent session … previous_zed_thread_id=69b7bfb6… thread_reset=false
zed_thread_id after = 69b7bfb6-…  (preserved)
```

## US1 — restart with newest interaction = `waiting` (in-flight) — THE INCIDENT
Newest interaction by id before restart:
```
int_01ky2bbspheadxwhjd5mjn83es | waiting |  (empty error)
```
Restart:
```
POST /sessions/ses_.../restart-agent
→ {"thread_reset":false}
session_handlers.go:2626  Restart agent session … previous_zed_thread_id=69b7bfb6… thread_reset=false
```
Reconnect after restart — thread REATTACHES (not blank), no fork:
```
websocket_external_agent_sync.go:407  [CONNECT] Session loaded for reconnect  zed_thread_id=69b7bfb6-…
websocket_external_agent_sync.go:463  [CONNECT] Sending open_thread directly on new connection … zed_thread_id=69b7bfb6-…
websocket_external_agent_sync.go:499  [CONNECT] ✅ open_thread written directly to WebSocket … zed_thread_id=69b7bfb6-…
```
Follow-up on the same thread recalls prior context:
```
prompt:   "What was the codeword I asked you to remember earlier?"
response: "PANGOLIN-42"   state=complete  last_zed_message_id=22
```
**Contrast with the incident:** there, reconnect showed `zed_thread_id=` (empty),
`open_thread` was skipped → blank thread → a new `thread_created` forked. Here the
pointer survived and the agent kept its full memory.

## US3 — restart alone (no config change) on a non-clean turn
US1 above was a plain Restart with **no** model/provider/credential change and a
non-clean (`waiting`) last turn → thread preserved. Confirms the reviewer's
suspicion is resolved: Restart alone no longer loses context.

## No thread ever forked
Across both restarts + all follow-ups, the ONLY `thread_created` in the logs is
the original `69b7bfb6` (at 12:37:05Z). `last_zed_message_id` climbed
1 → 3 → 5 → 7 → … → 22 on that single thread.

## Old vs new
Under the old `!lastInteractionCompletedCleanly()` gate, US1 (last turn
`waiting`) would have computed `thread_reset=true` and cleared `69b7bfb6` —
reproducing the incident. The new `threadIsWedged()` gate returns false for
`waiting`, so the thread is preserved.

## US5 — genuine wedge still resets (in progress)
Inducing an ACP agent crash so the last interaction errors with
`Session not found` / `Claude Agent process exited`, then restarting → expect
`thread_reset=true` and clean recovery. (Covered by unit test
`agent_crash_error_wedges`; live confirmation pending.)
