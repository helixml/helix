# Incident: "Restart" discards a healthy Zed thread when the last turn isn't cleanly complete

**Date:** 2026-07-21
**Severity:** High (silent, total context loss on a routine "restart to apply settings")
**Status:** Root-caused from prod logs; fix implemented.
**Affected:** spec task `spt_01kvtnrkgp5t2a7n4pwcv2cb8j` ("LinkedIn Outreach"),
session `ses_01kvwezmrtnftx2mqxa13zxtbf`, Zed thread
`bd5abc10-7155-420a-a87b-6658da6b88fd` (569 MB jsonl, ~869 messages). Prod =
meta.helix.ml.
**Related:** `design/2026-07-20-restart-clears-zed-thread-context-loss.md` (#2860,
same path, earlier partial fix).

## Summary

A user switched the agent's model (Opus → 4.8) and flipped the app's credential
type **api_key → subscription**, then clicked **Restart** to apply it (restarting
is the correct way to pick up the new desktop env from `subscriptionEnvForSession`).
The Restart cleared `config.zed_thread_id`; on reconnect the thread came up
**blank before any message**, and the next message forked a new empty thread —
total, silent loss of the agent's context.

## Root cause (ground truth from prod logs)

`restartCrashedAgentThread` computed `resetThread = !lastInteractionCompletedCleanly(session)`.
`lastInteractionCompletedCleanly` returned true only for `complete`/`interrupted`
and false for everything else. At the Restart (10:49:12Z) the last turn
(`int_01ky24my9`, "summarise again") was still **in-flight (`waiting`)** — its
`message_completed` flush landed at 10:49:24Z, *after* the restart — and it was
failing on the just-applied invalid subscription token (`401 OAuth access token
is invalid`, surfaced downstream on the forked thread). So `resetThread=true` →
`ZedThreadID` cleared (`session_handlers.go` logged `thread_reset=true
previous_zed_thread_id=bd5abc10…`). Reconnect showed `zed_thread_id=` empty →
`open_thread` skipped → blank thread. Next message forked `2c1b6724`.

The thread was never invalid: a later **DB-only** repoint to `bd5abc10`, desktop
**still in subscription mode**, resumed the full 569 MB thread (`open_thread` →
"Thread already loaded in registry" → `agent_ready` → `complete`,
`last_zed_message_id=90`, no 401). Preserving the pointer is sufficient.

This was **not** switch-agent, **not** `recoverMissingThread`, **not** a
`thread_load_error` on `bd5abc10` (all such log lines are for other sessions).

## The defect

`lastInteractionCompletedCleanly` equated "last turn not cleanly complete" with
"thread wedged". But a turn is non-`complete` for many reasons that leave the
thread intact — an **in-flight `waiting`** turn (the incident, and the state a
user is most likely in when they reach for Restart), or an `error` from auth
(401) / rate-limit (429) / provider outage / transport drop / user-cancel.
Resetting on those is catastrophic.

## Fix

Replace the reset predicate with `threadIsWedged` (`session_handlers.go`): the
human Restart discards the thread **only** when the last interaction errored with
positive wedge evidence — `isAgentCrashError` (`Claude Agent process exited`,
`Session not found`) or `isAuthoritativeMissingThreadError` (`no thread found with
ID: SessionId("…"`, `no rollout found for thread id`). `complete`, `interrupted`,
`waiting`/in-flight, and non-wedge errors all **preserve** the thread; the
reconnect `open_thread` reattaches, and a genuinely dead in-flight turn is cleaned
up lazily by `recoverMissingThread` on the next `thread_load_error`.

Autonomous crash recovery (`maybeAutoRestartCrashedAgent` →
`restartSessionContainer(..., true)`) is unchanged: it is already gated on
`isAgentCrashError`, so it stays consistent with the new wedge definition.

## Tested

Unit: `TestThreadIsWedged`, `TestButtonPreservesHealthyThreadResetsWedged`
(waiting/auth-error preserve; agent-crash/missing-thread reset). Live: see the
spec-task verification in the PR (restart on a non-clean turn preserves the
thread; `last_zed_message_id` keeps climbing on the same `zed_thread_id`).
