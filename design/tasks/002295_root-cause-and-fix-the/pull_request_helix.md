fix(api): restart only resets the Zed thread when it is genuinely wedged

## Summary

A routine "edit the agent settings, then click **Restart** to apply them" flow
was silently discarding a healthy Zed conversation thread and all of its context.
On meta prod (2026-07-21) this wiped a 569 MB, ~869-message Claude Code thread
(`bd5abc10-…`): the operator flipped an app to subscription mode and hit Restart;
the thread pointer was cleared, the thread came up blank, and the next message
forked a new empty thread.

**Root cause (confirmed from prod logs).** `restartCrashedAgentThread` computed
`resetThread = !lastInteractionCompletedCleanly(session)`, which treated *any*
last interaction that was not `complete`/`interrupted` as a wedged thread. At the
Restart the last turn was still **in-flight (`waiting`)** (and failing on the
just-applied invalid subscription token), so the thread was reset and the
conversation lost. The thread itself was always healthy — a later DB-only repoint
resumed the full thread under subscription mode with no change to the jsonl.

An in-flight (`waiting`) turn, or an `error` from auth (401) / rate-limit (429) /
provider outage / transport drop / user-cancel, leaves the conversation thread
completely intact. Resetting on those is silent, total context loss.

## Changes

- Replace `lastInteractionCompletedCleanly` with **`threadIsWedged`**
  (`api/pkg/server/session_handlers.go`). The human Restart now discards the Zed
  thread pointer **only** when the last interaction errored with positive wedge
  evidence — `isAgentCrashError` (`Claude Agent process exited`, `Session not
  found`) or `isAuthoritativeMissingThreadError` (`no thread found with ID …`,
  `no rollout found for thread id`). `complete`, `interrupted`, in-flight
  `waiting`, and non-wedge errors all **preserve** the thread; the reconnect
  `open_thread` reattaches, and a genuinely unloadable thread is cleaned up lazily
  by the existing `recoverMissingThread` path.
- Autonomous crash recovery (`maybeAutoRestartCrashedAgent`) is unchanged — it is
  already gated on `isAgentCrashError`, consistent with the new definition.
- Unit tests updated: `TestThreadIsWedged`,
  `TestButtonPreservesHealthyThreadResetsWedged` (waiting/auth-error preserve;
  agent-crash / missing-thread reset).

## Testing

Unit tests pass. **Live-verified against a connected Zed** in the inner Helix
(spec task, real thread `69b7bfb6`):

- **Restart with an in-flight `waiting` last turn** (the incident) →
  `thread_reset=false`; reconnect logged `zed_thread_id=69b7bfb6…` + `open_thread`
  reattach (not blank); no new `thread_created`; a follow-up recalled the codeword
  taught before the restart, `last_zed_message_id` climbing on the same thread.
- **Restart with a `complete` last turn** → preserved (no #2860 regression).
- **Restart with no config change** on a non-clean turn → preserved.
- **Killed the ACP agent mid-turn + Restart** → preserved and **recovered full
  context** via `claude --resume` (codeword recalled). A process crash is now
  non-destructive; reset is reserved for a genuinely unloadable thread (unit
  test + lazy `recoverMissingThread`).

Full evidence: `design/tasks/002295_root-cause-and-fix-the/live-test-results.md`
in the helix-specs branch. Incident writeup:
`design/2026-07-21-restart-discards-thread-on-nonclean-turn.md`.
