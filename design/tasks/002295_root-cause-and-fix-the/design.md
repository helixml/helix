# Design: Restart Must Not Discard a Healthy Zed Thread on a Non-Clean Last Turn

## Confirmed root cause (from prod logs in `work/incoming/`)

`restartCrashedAgentThread` → `restartSessionContainer`
(`api/pkg/server/session_handlers.go`) discards the Zed thread whenever the
session's last interaction is not cleanly `complete`:

```go
// session_handlers.go:2470
resetThread := !s.lastInteractionCompletedCleanly(ctx, session)
// ...
// session_handlers.go:2495
func (s *HelixAPIServer) lastInteractionCompletedCleanly(...) bool {
    // returns TRUE only for InteractionStateComplete / InteractionStateInterrupted
    // returns FALSE for waiting / editing / error / none  ← the bug
}
```

At the incident Restart (10:49:12Z) the last turn (`int_01ky24my9`, "summarise
again") was **in-flight (`waiting`)** and failing on the just-applied invalid
subscription token. `lastInteractionCompletedCleanly` returned false →
`resetThread=true` → `session_handlers.go:2581` cleared `ZedThreadID` (log:
`thread_reset=true previous_zed_thread_id=bd5abc10…`). On reconnect (10:49:24Z)
`zed_thread_id` was empty, so the `open_thread` re-attach block
(`websocket_external_agent_sync.go:~439`) was skipped and **Zed came up blank
before any message**. The next message forked `2c1b6724`.

The operator hit Restart **deliberately, to apply the api_key→subscription
change** (the correct way to pick up the new desktop env from
`subscriptionEnvForSession`) — not to recover a hang. The thread jsonl was always
healthy: a later DB-only repoint under subscription mode resumed the full 569 MB
thread (`open_thread` → "Thread already loaded in registry" → `agent_ready` →
`complete`, `message_id=90`, no 401). **Preserving the pointer is sufficient.**

## Principle

The Zed/ACP thread is model-agnostic state on the persistent workspace volume.
Discard the Helix→Zed pointer **only** on positive evidence of a genuine *thread*
wedge — never merely because the last turn isn't `complete`. An in-flight
(`waiting`) turn, an auth 401, a 429, a provider 5xx, a transport drop, and a
user-cancel all leave the thread intact.

Positive wedge evidence already has classifiers in the codebase
(`websocket_external_agent_sync.go`):

```go
isAgentCrashError(err)               // "Claude Agent process exited", "Session not found"
isAuthoritativeMissingThreadError(err) // `no thread found with ID: SessionId("`, "no rollout found for thread id"
```

An auth 401 ("Failed to authenticate. API Error: 401 …") matches **neither** —
correctly, it is not a wedge.

## The fix

**Primary (proposal a): default-preserve on human restart; reset only on positive
wedge evidence.** Rework the reset decision behind
`restartCrashedAgentThread` so that for a human-initiated restart of a session
with existing conversation history, the thread is **preserved** unless there is a
recorded thread wedge:

```go
// Replace `resetThread := !lastInteractionCompletedCleanly(...)` for the
// human-restart entrypoint with something like:
resetThread := s.threadIsWedged(ctx, session)

// threadIsWedged returns true ONLY on positive evidence the Zed thread itself
// can't be driven — the last interaction errored with an agent-crash or
// authoritative-missing-thread marker. A waiting/in-flight turn, or an error
// whose message is auth/rate-limit/provider/transport/cancel, is NOT a wedge.
func (s *HelixAPIServer) threadIsWedged(ctx, session) bool {
    last := <most recent interaction for session/generation>
    if last == nil { return false }              // nothing to wedge
    if last.State != types.InteractionStateError { return false } // waiting/complete/interrupted → healthy
    return isAgentCrashError(last.Error) || isAuthoritativeMissingThreadError(last.Error)
}
```

Key points:
- **`waiting`/in-flight → preserve** (the incident case). The reconnect
  `open_thread` reattaches; if that in-flight turn was genuinely dead, the lazy
  `recoverMissingThread` path cleans it up on the next `thread_load_error` —
  which is strictly better than pre-emptively zeroing a 569 MB thread.
- **`error` (auth/429/provider/transport/cancel) → preserve** — none match the
  wedge classifiers.
- **`error` with an agent-crash / authoritative-missing-thread marker → reset**
  (US5), exactly as autonomous crash recovery already does.
- The autonomous `maybeAutoRestartCrashedAgent` path already passes
  `resetThread=true` and is gated on `isAgentCrashError`; keep it, and make sure
  the human path and it agree on the wedge definition (share `threadIsWedged` /
  the classifiers). Do **not** loosen genuine crash recovery.

**Defence in depth:** WARN-log whenever a restart clears a thread whose last
interaction was `complete` or `waiting` (that combination should now be
impossible on the human path and is a red flag), mirroring #2860.

Preserving the pointer means the existing reconnect path takes over unchanged:
`open_thread(ZedThreadID)` reloads the thread and the next message continues it —
no blank UI, no fork.

## Why not just special-case auth errors?

Because the incident's trigger was a **`waiting`/in-flight** turn, which has no
error string to classify. Narrowing only the `error` branch would still lose the
thread when a user restarts mid-turn. Default-preserve + positive-wedge-only
covers both, and is robust to new provider error wordings (no fragile string
matching on the happy path).

## Other clear-sites (unchanged, for the record)

- `session_switch_agent_handlers.go:237` — unconditional clear on agent switch.
  **Not** this incident, but it should get the same wedge/kind gate for
  consistency (a pure model/provider/credential switch shouldn't clear). Track as
  a follow-up if out of scope; note it in the PR.
- `websocket_external_agent_sync.go:3597` (`recoverMissingThread`) — genuine
  lazy recovery; leave as-is (it's the safety net the primary fix relies on).
- `session_clear.go:90` — explicit `/clear`; leave as-is.

## Testing (mandatory — live, connected Zed, not seeded rows)

Get a live thread via a spec task (`config->>'zed_thread_id'` = non-empty UUID)
and exercise the **next** operation after each restart:

- **US1/US2:** make the last turn non-clean (in-flight, or auth-errored via an
  invalid subscription token), Restart, and assert on reconnect that
  `open_thread(<thread>)` reloads the SAME thread (no blank, no new
  `thread_created`), `thread_reset=false`, then a message climbs
  `last_zed_message_id` on the same thread.
- **US3:** Restart with **no** config change on a non-clean turn → same
  preservation (tests the reviewer's "restart alone" suspicion).
- **US4:** Restart after a `complete` turn → preserved (no #2860 regression).
- **US5:** genuinely wedge the thread (kill the ACP agent so the error carries
  `Session not found` / `Claude Agent process exited`) → Restart resets and
  recovers cleanly.

Capture `thread_reset` from the `session_handlers.go:2609` log line and the
`open_thread` / `thread_created` lines as evidence. Per CLAUDE.md, a unit test
asserting the field value is **not** acceptance evidence — the live thread
survival is.

## Scope

API-side Go only (`api/pkg/server/session_handlers.go`, reusing classifiers from
`websocket_external_agent_sync.go`). Air hot-reloads; no Zed/sandbox rebuild
needed, no `sandbox-versions.txt` bump.

## Implementation notes & live learnings

- **Implemented:** replaced `lastInteractionCompletedCleanly` with
  `threadIsWedged` (`session_handlers.go`); the human Restart entrypoint
  `restartCrashedAgentThread` now sets `resetThread := s.threadIsWedged(...)`.
  Wedge = last interaction `State==error` AND (`isAgentCrashError` OR
  `isAuthoritativeMissingThreadError`). Everything else preserves. Autonomous
  `maybeAutoRestartCrashedAgent` untouched. Unit tests updated
  (`TestThreadIsWedged`, `TestButtonPreservesHealthyThreadResetsWedged`).
- **The WARN "cleared a complete/waiting thread" log was intentionally NOT added:**
  the new gate makes that combination impossible on the human path, so the log
  would be unreachable dead code. Documented in the `threadIsWedged` comment.
- **Live learning (important):** killing the ACP agent does NOT produce the
  `Session not found` / `no thread found with ID` markers — it produces TRANSPORT
  errors (`agent turn aborted: the ACP agent process exited mid-turn or hit max
  tokens`, `send failed because receiver is gone`, `response channel cancelled`).
  My gate preserves all of these, and the Restart RECOVERS full context by
  recreating the container and `claude --resume`-ing the thread (verified: codeword
  recalled after a mid-turn process kill + restart). So "process crash" is a
  preserve-and-recover case, not a reset case — resetting on it (the old behavior)
  was the very data-loss bug. Reset is reserved for a genuinely unloadable thread,
  which also has the unchanged lazy `recoverMissingThread` safety net.
- **Gotcha:** each Restart **rotates the session's API key** (type=`api`) — re-read
  it from `api_keys` before the next authenticated call. The restart endpoint
  rejects `app`-type keys ("path not allowed for app API keys").
- **Gotcha:** the newest interaction must be selected by `ORDER BY id DESC` (ULID),
  NOT `created` — the planning interaction can carry a quirky-recent `created`
  timestamp. `threadIsWedged` uses `id DESC`, matching the code.
