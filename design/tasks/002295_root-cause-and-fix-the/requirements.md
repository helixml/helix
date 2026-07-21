# Requirements: Restart Must Not Discard a Healthy Zed Thread on a Non-Clean Last Turn

## Background

On meta prod (2026-07-21), a long-running spec-task session
(`ses_01kvwezmrtnftx2mqxa13zxtbf`, spec task `spt_01kvtnrkgp5t2a7n4pwcv2cb8j`,
"LinkedIn Outreach", owner Chris) had a healthy Claude Code (ACP) conversation
thread `bd5abc10-…` — ~869 messages, a 569 MB jsonl on the workspace volume.
Clicking **Restart** cleared `config.zed_thread_id` to empty; on reconnect the
thread came up **blank** and the next message forked a new empty thread
(`2c1b6724`, then `1151c086`) — silent, total loss of the agent's working
context. Recovery required manually re-pointing `config.zed_thread_id` back to
`bd5abc10` in Postgres.

### Confirmed root cause (ground truth from prod logs — not a hypothesis)

Verified against `work/incoming/` (`FINDINGS-host-analysis.md`, the raw API/Zed
logs, and `interactions.txt`):

1. The operator changed the model in the **Zed desktop UI** (the dropdown label
   is just "Opus", which resolves to Opus 4.8), then edited the **Helix app**
   config: model + flipped credential type **api_key → subscription**.
2. The operator sent a turn ("just switched you to opus 4.8, summarise again",
   `int_01ky24my9`) and then clicked **Restart deliberately to apply the
   api_key → subscription change** — restarting is the correct, expected way to
   pick up the new credential env (`subscriptionEnvForSession` is injected at
   desktop-start). At the moment of Restart that turn was still **in-flight**
   (`waiting`) — its `message_completed` flush landed at 10:49:24, *after* the
   10:49:12 Restart — and it was failing on the now-invalid subscription token
   (the `401 OAuth access token is invalid` surfaced downstream on the
   already-forked thread). So the last interaction was **not `complete`** when
   Restart ran.
3. **Restart** ran `restartSessionContainer`, which computes
   `resetThread = !lastInteractionCompletedCleanly(session)`. `lastInteraction
   CompletedCleanly` treats **only `complete`/`interrupted`** as healthy; the last
   turn was **not** `complete` (in-flight `waiting`, then `error`) → it returned
   **false** → `resetThread=true` → it **cleared `bd5abc10`**
   (`session_handlers.go:2609` logged `thread_reset=true
   previous_zed_thread_id=bd5abc10…`).
4. On reconnect (10:49:24) `zed_thread_id` was **empty**, so the `open_thread`
   re-attach block was skipped — **the Zed thread came up blank before any
   message was sent.** The next message then forked `2c1b6724`
   (`title="New Conversation"`).

This is the **`restartSessionContainer` path that PR #2860 already touched**. It
is **NOT** switch-agent, **NOT** `recoverMissingThread`, **NOT** a
`thread_load_error` on `bd5abc10` (every `THREAD_LOAD_ERROR` line in the raw log
is for other sessions).

### The thread was never invalid — only the pointer was zeroed

A **DB-only** repoint (`zed_thread_id → bd5abc10`, no jsonl change), with the
desktop **still in subscription mode**, resumed the full 569 MB thread:
`open_thread` → "Thread already loaded in registry" → `agent_ready` → interaction
`complete`, `last_zed_message_id=90`, **no 401**. So the api_key-era jsonl resumes
fine under subscription, and **preserving the pointer is sufficient** — no
thread-store portability work is needed.

## The core defect

**`lastInteractionCompletedCleanly` equates "last turn is not cleanly complete"
with "the thread is wedged" — and Restart discards the thread on that basis.**
But a turn can be non-`complete` for reasons that leave the conversation thread
completely intact:

- **In-flight / `waiting`** — the turn is still running (or hung on a slow/failing
  provider). This is the incident's trigger, and the state a user is *most likely*
  in when they reach for Restart.
- **`error` from auth (401), rate-limit (429), provider outage, transport drop,
  or user-cancel** — the LLM call failed, the thread did not.

Resetting the Zed thread in any of these cases is catastrophic, silent data loss.
Only a genuine *thread* wedge (Zed cannot load/drive the thread) justifies a
reset. The trap is the **routine "I changed the agent settings, now Restart to
apply them" workflow**: restarting to pick up a new model/credential env is the
correct, expected action, yet if the last turn is in-flight or errored (which
flipping to an invalid subscription token makes likely) the Restart silently
discards the whole conversation. **Restart on a non-clean last turn is enough to
trigger the bug — with or without the config change.**

## User Stories

### US1 — Restart to apply a settings change keeps my conversation (the incident)
As a user who edited the agent settings (e.g. api_key → subscription) and clicks
**Restart to apply them**, the agent still remembers the whole conversation — the
thread re-attaches, it does not come up blank — even if the last turn was
in-flight or failing when I restarted.

**Acceptance (live, connected Zed — not seeded rows):**
- Given a spec-task session with `config->>'zed_thread_id'` = a non-empty UUID,
  several completed turns, and a **last interaction that is `waiting`/in-flight
  (or errored)**,
- When I click Restart, then (before sending anything) observe reconnect,
- Then `zed_thread_id` is **unchanged**, reconnect sends `open_thread(<thread>)`
  and Zed reloads it (**not** blank), Restart logs `thread_reset=false`; and a
  subsequent message continues on the **same** thread with prior context
  (`last_zed_message_id` keeps climbing, no new `thread_created`).

### US2 — Restart after an auth / provider / transport error or cancel keeps context
As above, but the last turn ended in `error` (401/429/provider 5xx/transport) or
was cancelled. Restart preserves the thread.

### US3 — Restart alone (no config change) does not lose context
As a user who simply clicks Restart — whether the last turn is `complete`,
in-flight, or errored, and **without** any model/provider/credential edit — my
conversation survives (unless the thread is genuinely wedged). This directly
tests the reviewer's suspicion that Restart alone can trigger the loss.

### US4 — Restart after a normal completed turn keeps context (regression)
Last turn `complete` → Restart preserves the thread (existing #2860 behaviour
must not regress).

### US5 — A genuinely wedged thread still resets and recovers (regression guard)
Thread actually wedged — the ACP agent crashed (`Claude Agent process exited` /
`Session not found`) or Zed reports authoritative `no thread found with ID …` —
Restart still resets and the agent recovers cleanly.

## Non-Goals

- Fixing the underlying subscription auth failure (a session owner's subscription
  token being invalid) — sibling concern. We only ensure it can't trip a thread
  reset. (See Open Questions.)
- Changing explicit `/clear` (`session_clear.go`) or `recoverMissingThread`
  (confirmed a red herring here).

## Deliverables

1. A live reproduction of "non-clean last turn (in-flight and/or auth-errored) →
   Restart → thread discarded / blank on reconnect".
2. The fix: Restart preserves the thread on in-flight/auth/provider/transport/
   cancel states; only resets on positive evidence of a genuine thread wedge.
3. Live evidence: reconnect `open_thread` reloads the SAME thread (no blank UI,
   no new `thread_created`), `last_zed_message_id` climbing on the same
   `zed_thread_id` — for US1, US2, US3; plus US4/US5.
4. A PR against `helixml/helix`.

## Open Questions

1. **Reset predicate.** Two shapes: (a) invert the default — on a human-initiated
   restart, **preserve** a thread that has ≥1 completed turn (substantial
   history) and reset **only** on positive wedge evidence (a recorded
   `isAgentCrashError` / `isAuthoritativeMissingThreadError`), recovering lazily
   via `recoverMissingThread` if the reattach genuinely fails; (b) additionally
   classify the last interaction's stored error so auth/rate-limit/provider/
   transport/cancel never count as a wedge. **Recommendation:** (a) as the robust
   primary (it also covers the `waiting`/in-flight trigger, which has no error
   string to classify), with (b) as a refinement. Confirm against the actual
   interaction state/error metadata in the repro.

2. **What decides "genuine wedge" for an in-flight turn?** An in-flight `waiting`
   turn at restart has no terminal event. Under proposal (a) it is *preserved*
   (reattach; if the reattach fails, `recoverMissingThread` cleans up lazily).
   Confirm the reconnect `open_thread` reattach + lazy recovery handles a truly
   dead in-flight turn without leaving the user stuck. This is the crux to prove
   in US5.

3. **Auto-restart-on-crash consistency.** `maybeAutoRestartCrashedAgent` passes
   `resetThread=true` deliberately and is gated on `isAgentCrashError`. Confirm an
   auth error is not `isAgentCrashError` (so autonomous sessions also won't reset
   on auth), and that the new human-restart gate stays consistent with it.

4. **Scope/PR.** Fix is API-side Go only
   (`api/pkg/server/session_handlers.go`), Air hot-reloads, no Zed/sandbox rebuild
   expected → likely no `sandbox-versions.txt` bump. Confirm during
   implementation.
