# Incident: ACP agent accepts `session/prompt` then goes silent — turn hangs forever

**Date:** 2026-07-29
**Severity:** High (agent appears to "hang" with no error anywhere in the UI; only a
container restart recovers)
**Status:** Root-caused from live prod logs + DB + container state. Recovered by
restarting the session (confirmed working). Fix implemented in the Zed fork —
see "Fix" below. **NOT yet verified end-to-end against a live wedge** (the wedge is
not reproducible on demand; unit tests cover the watchdog logic).
**Affected:** spec task `spt_01kyq6qd8h4wqq2rfasc4b01e9`
("Fix Critical HelixOS Sales Bot Bugs"), session `ses_01kyq6s3yw610fjmk3r8emw6v2`,
Zed thread `def2f04c-d2df-4d8c-9dfe-5b672bae8c02`
(`code_agent_runtime=claude_code`, `zed_agent_name=claude`).
Prod = meta.helix.ml (node01).
**Related:** `design/2026-07-20-restart-clears-zed-thread-context-loss.md`,
`design/2026-07-21-restart-discards-thread-on-nonclean-turn.md`,
helixml/zed PR #63 (claude-agent-acp wedge recovery).

---

## Summary

The user typed messages into the Zed agent panel and Claude never replied. No error
was shown in Zed, none in the Helix UI, and the Helix interaction sat in
`state=waiting` indefinitely.

The messages **were** delivered. Each one entered `AcpThread::send_inner()` and
dispatched `connection.prompt(...)` to `claude-agent-acp`. The agent process was
alive the whole time (`pid 3373`, `node .../claude-agent-acp`). It simply never
answered. Because `connection.prompt()` never resolved, `run_turn` never emitted
`Stopped` and never emitted an error, so:

- Zed showed a turn that never finished,
- Helix never received `message_completed` or `chat_response_error`,
- the interaction stayed `waiting` forever.

## Evidence

From the container's `Zed.log` (times BST):

```
17:48:56  message_added  message_id=104  role=user  content="hello?"
17:48:56  ERROR acp_thread.rs:2855  failed to get old checkpoint
          …then nothing. No Stopped, no error, no message_completed. 8+ minutes.
```

`acp_thread.rs:2855` sits inside `send_inner()`, immediately before
`this.connection.prompt(message_id, request, cx)` — so reaching it proves the prompt
was dispatched. The same shape appeared for the previous message ("you ok?" at
17:24:02).

DB state confirms the Helix side:

```sql
SELECT id, state, LENGTH(response_message) FROM interactions
 WHERE session_id='ses_01kyq6s3yw610fjmk3r8emw6v2' ORDER BY created DESC LIMIT 1;
-- int_01kyqb1bsj67ymqn6wnz7qq620 | waiting | 0
```

### What put the agent into that state

Earlier in the same session:

```
16:47:29  ERROR Error in run turn: Internal error: There's an issue with the selected
          model (claude-opus-5[1m]). It may not exist or you may not have access to it.
          { "errorKind": "model_not_found" }
16:47:42  ERROR Error in run turn: Session not found
16:47:50  ERROR Error in run turn: Session not found
16:48:07  [ACP_SESSION_LOCK] acquired slot (open_or_create_session id=def2f04c…)
```

A `model_not_found` for `claude-opus-5[1m]`, followed by the agent losing the session
and Zed re-creating it. Turns worked for a while after that, then stopped producing
any events at all.

The `model_not_found` was a **backend model-availability gap** (the 1M-context Opus
was not being served by the GCP-backed endpoint at the time); it has since been fixed
on the serving side and is **not** a malformed-model-id bug — `opus[1m]` is the correct
Helix convention, resolved by `claude-agent-acp`'s `resolveModelPreference()`.

That matters for the framing of this incident rather than diminishing it: a transient
backend model outage is a perfectly ordinary thing to happen, and it must not be able
to leave the agent **permanently silent with no error**. The precise internal cause
inside `claude-agent-acp` is not established — what is established is the externally
observable contract violation: **the agent accepted a prompt and never responded, with
no error**, and stayed that way indefinitely after the upstream cause had passed.

## Why nothing recovered it

Three mechanisms could have caught this and all missed:

1. **Zed had no timeout on `connection.prompt()`.** The await is unbounded. A
   non-responding agent hangs the turn forever.

2. **Helix's existing wedge recovery (`is_claude_acp_drain_race`, PR #63) keys on an
   error string** — a strict triple-match on `ede_diagnostic` + `result_type=user` +
   `stop_reason=null`. A silent wedge emits *no error at all*, so the force-reset +
   reload path could never trigger.

3. **`threadIsWedged()` (api/pkg/server/session_handlers.go) only classifies a wedge
   when the last interaction is `State == error`.** Ours was `waiting`, so a Restart
   would preserve the thread — correct for context, but it means the wedge itself is
   invisible to that classifier.

Helix's auto-wake worker *did* notice the quiescence and deliberately declined to act:

```
[AUTO_WAKE] Connected agent remained quiescent; automatic prompt replay suppressed
  interaction_id=int_01kyqb1bsj67ymqn6wnz7qq620
```

That suppression is intentional (it exists to avoid false positives during long
synchronous tool calls) — see the comment block in
`api/pkg/server/auto_wake_stuck_interactions.go`.

## Recovery (what actually worked)

Restarting the agent session recovered it, with context preserved. That is the
expected behaviour of the PR #2869 gate: `threadIsWedged()` saw `waiting`
(not `error`), returned false, so `resetThread=false` and `ZedThreadID` was kept —
while `StopDesktop`/`StartDesktop` killed the wedged `claude-agent-acp` process. The
fresh process reloaded the thread from the persisted transcript.

## Fix

Implemented in the Zed fork (`crates/external_websocket_sync/src/thread_service.rs`),
deliberately **not** in upstream `acp_thread.rs`, to keep the upstream diff (and
future merge conflict surface) at zero.

**A time-to-first-event watchdog**, not a turn timeout:

- `THREAD_ACTIVITY` — a per-thread counter bumped by the persistent subscription on
  *every* `AcpThreadEvent` (thinking, text, tool call, entry update, Stopped). Any
  event is proof of life.
- `wait_for_first_agent_activity()` — races a freshly dispatched prompt against a
  budget (`HELIX_ACP_FIRST_EVENT_TIMEOUT_SECS`, default **120s**, `0` disables).
- If the agent emits **anything**, the watchdog disarms for the rest of the turn, so
  arbitrarily long tool calls and slow generations are never interrupted. This is the
  key design point: a blanket turn timeout would be wrong (the documented
  long-single-tool-call false positive), but "zero events since dispatch" is
  unambiguous — a healthy agent emits its first event within seconds.
- On expiry the send task is dropped (same rationale as Critical Fix #8: never block
  on a non-responding agent) and an error carrying `helix_silent_prompt_wedge` is
  returned.
- `is_silent_prompt_wedge()` is added to the follow-up recovery trigger alongside
  `is_claude_acp_drain_race()`, so a silent wedge now runs the **existing** PR #63
  force-close → reload → retry machinery. No new recovery mechanism was introduced.

Downstream, the returned error propagates as `chat_response_error`, which marks the
Helix interaction errored and frees the activation lane — turning a permanent silent
hang into a bounded, self-recovering, *visible* failure.

### Tests

`cargo test -p external_websocket_sync` — 55 passed (5 new in
`silent_prompt_wedge_tests`): watchdog fires on total silence; disarms immediately on
first activity; activity counter is per-thread and monotonic; the wedge error is
recognised and is not confused with the drain race; the budget is configurable and
disablable.

### Not done / follow-ups

- **The watchdog only covers Helix-originated follow-ups** (`handle_follow_up_message`).
  Messages typed directly into the Zed panel go through `agent_ui` → `AcpThread::send()`
  and are not yet wrapped. The production trigger here *was* a UI-typed message, so this
  gap matters — covering it needs the watchdog hoisted into the persistent subscription
  (which observes all turns regardless of origin) rather than the send path.
- **`threadIsWedged()` is still blind to silent hangs.** Worth widening to treat a
  `waiting` interaction with no agent events for N minutes on a live connection as a
  wedge, so Restart can reset a genuinely poisoned thread instead of preserving it.
- Not verified end-to-end against a live wedge (not reproducible on demand).
- **The watchdog does not cover "emitted output, then went silent before completing".**
  It is a time-to-FIRST-event watchdog by design (that shape is unambiguous and cannot
  false-positive on a long tool call). A turn that streams some tokens and *then* stalls
  before `Stopped` disarms the watchdog and is still unbounded. The E2E claude round was
  observed failing in exactly that shape on 2026-07-29 (three events including a correct
  assistant answer, then no `message_completed`), so it is a real behaviour, not
  hypothetical. Catching it needs a separate idle-since-last-event budget, sized well
  above the longest plausible tool call.
- **The E2E harness cannot attribute a claude-round failure.** `e2e-test/run_e2e.sh:204`
  reports the agent version with
  `npm view @anthropic-ai/claude-agent-acp version`, but the package Zed actually
  installs is **`@agentclientprotocol/claude-agent-acp`** (confirmed in
  `crates/agent_servers/`, and matching the `ps` output of the wedged production
  container). The wrong scope means the query always returns `unknown`, so the log line
  that exists precisely to distinguish "our regression" from "the agent package changed
  under us" is silently useless. The package is also unpinned in the npm path, so the
  claude round is not reproducible across time. Worth fixing both.
- The `model_not_found` trigger has been fixed on the serving side (model availability
  in GCP), so the specific path into this wedge is closed. The wedge handling still
  matters: any future provider-side error can re-enter it.
