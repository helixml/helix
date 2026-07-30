# Fix: switching an agent's model/provider/credential must NOT discard the Zed conversation thread

## Problem (real incident, meta prod, 2026-07-21)

A long-running spec-task session (`spt_01kvtnrkgp5t2a7n4pwcv2cb8j`, "LinkedIn
Outreach", owned by Chris) had a healthy Claude Code (ACP) conversation thread
`bd5abc10-…` — a 569 MB jsonl at
`~/.claude-state/projects/-home-retro-work/<thread>.jsonl` on the workspace
volume, ~869 messages deep. A user opened the agent settings and switched it from
Anthropic **API key** mode to **subscription** mode (and picked a different
model, opus 4.8). Immediately after, the session's `config.zed_thread_id` pointer
was **cleared to empty**. The next messages therefore dispatched with an empty
`acp_thread_id` / `first_message=true`, so Zed **forked brand-new empty threads**
(`2c1b6724`, `1151c086`) — total, silent loss of the agent's working context (the
Helix UI transcript still showed the old messages, but the agent had forgotten
everything). Recovery required manually repointing `config.zed_thread_id` back to
`bd5abc10` in Postgres.

## Root cause to confirm and fix

There are multiple code paths that set `session.Metadata.ZedThreadID = ""`:
- `api/pkg/server/session_handlers.go` `restartSessionContainer` — **already fixed**
  by PR #2860 (https://github.com/helixml/helix/pull/2860): it now preserves a
  healthy thread via `lastInteractionCompletedCleanly()` and only resets when the
  thread looks wedged.
- `api/pkg/server/session_switch_agent_handlers.go:~237` — still sets
  `session.Metadata.ZedThreadID = ""` **unconditionally**. Comment at ~line 231
  ("Repoint the session's agent in place. Clearing ZedThreadID makes the …")
  and ~325 ("a successful switch always ends with a fresh thread id").
- `api/pkg/server/session_clear.go` — the explicit /clear (leave as-is; that's
  intentional).

**Your first job: confirm exactly which path fired during a
provider/model/credential change** (add logging / reproduce). The restart-preserve
fix (#2860) was already deployed on meta, and the last interaction before the loss
was `complete` (healthy), so a plain restart would have *preserved* the thread —
which means the clear almost certainly came from the **switch-agent path** (or an
app-config-edit path that re-provisions the session), not the restart path.
Reproduce it: create a zed_external claude_code spec task, send a couple of
messages so a thread exists (`config->>'zed_thread_id'` is a non-empty UUID), then
change the agent's model / provider / credential_type in settings and observe
whether `zed_thread_id` gets cleared.

## The fix

Switching the **LLM model, provider, or credential type** (api_key ⇄ subscription)
within the **same** `code_agent_runtime` (e.g. claude_code → claude_code) must
**preserve** the existing Zed thread — the conversation is model-agnostic ACP
state and there is no reason to discard it. Apply the same principle as #2860:
only clear `ZedThreadID` when either
1. the **agent kind genuinely changes** such that the old thread state is
   incompatible (e.g. `zed-agent` ⇄ `claude_code`, different ACP agent binaries /
   thread stores), OR
2. the thread is **wedged** (last interaction not in a clean terminal state — reuse
   `lastInteractionCompletedCleanly` or equivalent).

For a pure model/provider/credential change, keep the thread and let the reconnect
`open_thread` (`websocket_external_agent_sync.go:~439`) re-attach. Note the token
is injected at desktop-start (`external_agent_handlers.go` `subscriptionEnvForSession`),
so a model/provider switch that needs new env should still recreate the desktop —
but **preserve the thread pointer** across that recreate.

## Acceptance criteria (must test live in the inner Helix — this is a lifecycle change)

Per the repo's testing rules, lifecycle changes MUST be tested against a LIVE,
connected Zed, not seeded DB rows. Create a spec task, get a live thread
(`config->>'zed_thread_id'` = non-empty UUID), then:
1. Change **model** (e.g. opus → sonnet) → send a message → agent still has prior
   context, `zed_thread_id` unchanged, no new empty thread forked.
2. Change **credential type** (api_key ⇄ subscription) → same: context preserved.
3. Change **agent kind** (claude_code ⇄ zed-agent) → thread MAY reset (that's
   allowed) — verify it comes up cleanly.
4. Regression: a genuinely **wedged** thread (kill the ACP agent mid-turn) still
   resets and recovers.

Report the exact clear-path you found, the gate you added, and paste the live
test output (the `last_zed_message_id` climbing on the SAME thread across a
model switch is the key evidence). Do NOT claim "covered by unit tests" — unit
tests that assert the field value are not evidence the conversation survived.

Related design docs: `design/2026-07-20-restart-clears-zed-thread-context-loss.md`
(the #2860 writeup — mirror its approach).
