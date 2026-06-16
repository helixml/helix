# Wedged ACP thread → infinite re-send flood (2026-06-15)

## Incident

- Spec task `spt_01kv5q5rz4gstfks14ng1p6qqq`, session `ses_01kv5q5rzz85bwm4c3qaw8pxmb`,
  acp_thread_id `5a01779e-a8be-4839-83fc-f94b1b87f686`, on `meta` (= localhost dev stack).
- User interrupted the agent shortly after it started, then later sent `test`.
- Result: Zed received the same (long) message over and over; the spec task
  appeared "permafucked". Error surfaced to the user:
  `Thread load failed: Failed to send follow-up: Internal error: [ede_diagnostic] result_type=user last_content_type=n/a stop_reason=null`

## Root cause (layered)

1. **Wrapper wedge (origin).** claude-agent-acp's cancel-then-prompt **swallow**
   (claude-agent-acp#551). Interrupting right after start cancels a turn whose
   finalisation never completes; the thread is left in a state where every
   subsequent `prompt` returns either an empty response (bounce) or
   `[ede_diagnostic] … stop_reason=null`. Observed terminal: survived ~17 min and
   ~10 re-sends, never recovered.

2. **Zed retry is bounded (correct).** Priya's PR #60 (`27e8867c9e`, in the pinned
   `ZED_COMMIT`) retries the follow-up 4×750ms (~2.25s) in
   `thread_service.rs::handle_follow_up_message`. Confirmed live in the deployed
   build. Correctly gives up and emits `ThreadLoadError` when the wedge is
   permanent — it cannot fix a thread that never drains.

3. **Helix re-send is unbounded (the bug).** Three independent heads kept hammering
   the wedged thread:
   - **Empty-response bounce** (`websocket_external_agent_sync.go:2687`) →
     `MarkPromptAsBounced` → `MarkPromptAsFailed` → re-queued.
   - **Queue retry**: `GetNextPendingPrompt` / `GetAnyPendingPrompt` /
     `GetNextInterruptPrompt` (`store_prompt_history.go:131/162/193`) re-select
     `status='failed'` once `next_retry_at` passes (backoff capped at 30s).
     **No `retry_count` cap** — the column is incremented but never checked.
   - **Auto-wake self-sustaining loop (the perpetual-motion engine):**
     - auto-wake re-sends a plain `chat_message` to the wedged thread
       (`auto_wake_stuck_interactions.go:549`);
     - the wrapper **echoes the user message back** as `message_added role=user`;
     - `handleMessageAdded` (`websocket_external_agent_sync.go:1450`) **creates a
       NEW `waiting` interaction with no `prompt_id`** for that echo;
     - the turn then fails (`thread_load_error`); the matched interaction is
       errored, but the freshly-minted echo interaction stays `waiting`;
     - the next auto-wake scan finds it and re-fires.
     The per-interaction `autoWakeMaxRetries=2` cap fires correctly
     (`Exhausted retries` at 13:33:28) but is **defeated**: each cycle creates a
     fresh interaction with `auto_wake_count=0`. This is the exact "fired forever"
     bug the comment at `auto_wake_stuck_interactions.go:79-86` claims was fixed.

   From 13:24→13:36 the prompt `retry_count` stayed at **1** (queue quiet) while
   auto-wake alone sustained the flood — proving auto-wake is independently
   self-sustaining and a queue-only cap is insufficient.

## Timeline (interactions)

```
13:22:44 complete  (none)        first turn ok
13:23:59 error     orbritleu     empty-response bounce (interrupt landed)
13:24:01 error     orbritleu     thread_load_error ede_diagnostic
13:24:03 error     (none) x2
13:27:08 AUTO_WAKE attempt=1 on int_…q83 and int_…q85
13:30:18 AUTO_WAKE attempt=2 on both; echo mints int_…km/…kp/…kr (new waiting)
13:33:28 AUTO_WAKE "Exhausted retries" on q83 & q85; immediately attempt=1 on int_…km
13:36:38 AUTO_WAKE attempt=2 on int_…km
13:39:32 error     rz9p7g5o7     user sent "test"
13:41    manual soft-delete of prompts + error the waiting interactions → flood stops
```

## Manual recovery applied (reversible)

```sql
-- stop the queue head
UPDATE prompt_history_entries SET deleted_at=now(), updated_at=now()
WHERE id IN ('1781530772360-rz9p7g5o7','1781529823335-orbritleu');
-- stop auto-wake re-targeting (clear orphaned waiting interactions)
UPDATE interactions SET state='error', completed=now(), updated=now(),
  error='Cleared manually: wedged ACP thread (claude-agent-acp#551 swallow).'
WHERE session_id='ses_01kv5q5rzz85bwm4c3qaw8pxmb' AND state='waiting';
```
The thread itself stays wedged — continuing requires a fresh session/thread.

## Fix direction

- **A. Auto-wake session/thread-scoped circuit breaker** (kills the engine):
  cap auto-wake by session (or acp_thread_id), not per-interaction, so
  echo-minted interactions can't reset the budget. Reset on a genuine non-empty
  completion.
- **B. Queue `retry_count` cap** (defensive, secondary): add `AND retry_count < N`
  to the three selectors.
- **C. (optional) Treat repeated `ede_diagnostic`/thread_load_error on one
  acp_thread_id as terminal** → `MarkPromptAsCrashed` + surface a Restart that
  forces a fresh thread, and stop minting `waiting` interactions from echoes on a
  thread that's actively erroring.

Diagnostics saved under `design/diagnostics/2026-06-15-wedged-session-*.log`.

## Recovery experiment (live, on the preserved wedged container)

Container `ubuntu-external-01kv5q5rzz85bwm4c3qaw8pxmb` was kept alive (auto-sleep
disabled) to test the *minimal* recovery lever, since a full container restart is
destructive to live desktop/Zed UI state (on-disk thread context survives, the
running session does not).

Process topology (the wedge lives in the agent process memory):
```
zed (1889)
  └─ npm exec @agentclientprotocol/claude-agent-acp (2608)
       └─ sh -c claude-agent-acp (2845)
            └─ node claude-agent-acp  (2846)   ← ACP wrapper
                 └─ claude (SDK) (2869)  --session-id 5a01779e… --replay-user-messages
```
gnome-shell and zed are NOT in this subtree, so killing the agent leaves the
desktop up.

**Test 1 — does Zed auto-respawn the agent on death?** Killed the agent subtree
(`pkill -f claude-agent-acp` + the SDK). Result: zed stayed up; **no respawn** after
~15s. The agent is not auto-restarted on death.

**Test 2 — does the next message respawn a clean agent?** Sent a real message via
`POST /sessions/{id}/messages`. Result: **no respawn**; Zed reused its *cached*
(now-dead) `AgentConnection` and returned
`Failed to send follow-up: Internal error: "response channel cancelled — connection may have dropped"`.

**Conclusion.** With today's Zed, killing the agent subprocess is NOT a
self-sufficient recovery lever — Zed holds the cached connection for an existing
thread and never re-establishes it on a follow-up. A non-destructive auto-recovery
(respawn just the agent, reload thread `5a01779e` from disk via the persisted
`--session-id`, preserve desktop) is *possible in principle* but needs a **Zed-side
change**: on a dead/errored agent connection for an existing thread, drop the
cached connection and `load_thread_from_agent` to re-spawn. Not available now.

The only recovery that works with current code is the existing
`restartCrashedAgentThread` (StopDesktop → resume, preserving `ZedThreadID`), which
is **destructive to live desktop/Zed UI state** (conversation context survives on
disk). Hence: surface it as a user-consented **button** for this wedge class now;
pursue the non-destructive Zed-side respawn as a follow-up.

## Implemented (2026-06-15) — verified end-to-end on meta=localhost

1. **Queue retry cap** (`store_prompt_history.go`): `retry_count < N` on all three
   selectors; `HELIX_MAX_PROMPT_QUEUE_RETRIES` (default 20). Runaway guard.
2. **Auto-wake session breaker** (`auto_wake_stuck_interactions.go`): Gate 1c stops
   re-waking once ≥N errored interactions have piled up since the session's last
   completion; `HELIX_AUTO_WAKE_SESSION_WEDGE_THRESHOLD` (default 3). Kills the
   echo-loop engine the per-interaction cap couldn't.
3. **Crash-on-recurrence + Restart** — *not* string-matching. Live testing showed a
   dead agent connection emits varying wordings (`response channel cancelled`, then
   `send failed because receiver is gone`), so:
   - Backend `handleThreadLoadError` crash-marks (`MarkPromptAsCrashed`) on the
     recurrence of *any* `thread_load_error` once `retry_count >= acpWedgeCrashThreshold`
     (2). First occurrence still gets normal retries (transient-drain safety).
   - Frontend `RobustPromptInput.tsx` detects crashed via the far-future
     `next_retry_at` **sentinel** (year 9999) set by `MarkPromptAsCrashed`, robust to
     wording. String markers kept as a first-failure fast-path.
   - Recovery is the existing user-clicked **Restart** (`restart-agent`): StopDesktop
     → resume (preserve `ZedThreadID`) → reset crashed prompts → re-dispatch.

**Verified live:** killed the agent → sent a prompt → it failed (`send failed
because receiver is gone`) → crash-marked (sentinel) → Restart button rendered →
clicked → fresh `claude` agent respawned, crashed prompt reset to `sent` and
answered, desktop/Zed preserved. Queue went quiescent (0 errors, 0 waiting).

Known minor: crash-mark took a few retries to settle (retry_count reached 8 before
sticking) because queue + auto-wake paths raced; bounded by the cap, ended correct.

## Recommended plan

1. **Stop the flood (no state loss, high confidence) — ship now:**
   - Auto-wake **session-scoped breaker**: stop re-waking once consecutive
     wedge-signature errors since the last genuine completion exceed a threshold,
     so echo-minted interactions can't reset the per-interaction budget.
   - Queue **`retry_count < N`** cap on the three selectors (runaway guard).
2. **Recovery — surface existing Restart for this wedge class (user-consented):**
   detect the wedge (repeated `ede_diagnostic`/thread_load_error) →
   `MarkPromptAsCrashed` so the existing Restart UI appears. No destructive
   automation.
3. **Follow-up (cross-repo, non-destructive auto-recovery):** Zed change to
   re-establish the agent connection on a dead-connection follow-up, enabling
   automatic recovery without a desktop restart.
