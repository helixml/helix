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

## US5 — crash recovery (live) + genuine-wedge reset (unit + lazy net)

I killed the ACP agent (`claude-agent-acp` wrapper + `claude` binary) inside the
desktop container mid-turn and drove the **human Restart**. Key findings:

**A killed ACP agent surfaces as TRANSPORT errors, not missing-thread markers.**
The interaction errors I could induce were:
```
agent turn aborted: the ACP agent process exited mid-turn or hit max tokens …
Thread load failed: … Internal error: "send failed because receiver is gone"
Thread load failed: … Internal error: "response channel cancelled — connection …"
```
None match `agentCrashErrorMarkers` (`Claude Agent process exited`,
`Session not found`) or `isAuthoritativeMissingThreadError` (`no thread found
with ID`). So `threadIsWedged` returns **false** → `thread_reset=false` →
**preserve**. And that is CORRECT: on Restart the container is recreated, `claude
--resume=69b7bfb6` reloads the thread, and the agent recovers with FULL context —
proven:
```
kill claude-agent-acp + claude  →  turn errors "agent turn aborted …"
POST restart-agent  →  {"thread_reset":false}   (thread 69b7bfb6 preserved)
reconnect  →  [CONNECT] ✅ open_thread written … zed_thread_id=69b7bfb6-…
follow-up "what was the codeword?"  →  "PANGOLIN-42"   last_zed_message_id=34
```
Under the OLD gate this errored turn would have been `thread_reset=true` → context
lost. The new gate keeps it and the conversation survives a process crash.

**The genuine "thread truly unloadable" case** (Zed emits `no thread found with
ID: SessionId(...)`): `threadIsWedged` returns true → `thread_reset=true` (reset),
covered by unit tests `agent_crash_error_wedges` / `missing_thread_error_wedges`.
I could not induce that exact string live by killing the process (it produces the
transport errors above, which correctly preserve+recover), and it is additionally
handled by the **unchanged** lazy `recoverMissingThread` path
(`websocket_external_agent_sync.go`): if a preserved thread genuinely fails to
reload on the next attempt, Zed's authoritative `thread_load_error` clears it and
replays the message. So a genuinely-dead thread is never a permanent stuck state.

**Net:** in practice the human Restart now preserves the thread for every
process-crash / transport / auth / in-flight case and recovers via resume; it
resets only on a positive authoritative missing-thread signal, with lazy recovery
as the safety net.
