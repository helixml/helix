# Incident: "Restart agent session" discards a healthy Zed thread → total context loss

**Date:** 2026-07-20
**Severity:** High (silent, total context loss on a routine restart; no error surfaced)
**Status:** Root-caused from live prod logs + DB + container state. Session repaired
(config repointed). Code fix proposed, NOT yet implemented.
**Affected:** spec task `spt_01kxzhec3skvvf7bntk0kdhfc0`
("Add Hypotheses, Connections & Content Calendar Modules to HelixOS"),
planning session `ses_01kxzheqnf0sdsj5xdnx6zp19e`
(`agent_type=zed_external`, `code_agent_runtime=claude_code`, `zed_agent_name=claude`).
Prod = meta.helix.ml (node01).
**Related:** `design/2026-06-19-incident-interrupt-during-boot-context-loss.md`
(same end-state — message in a divorced thread — but a *different* trigger),
`design/2026-06-22-zed-open-thread-send-mismatch.md`,
memory `project_org_worker_stale_zed_thread_ephemeral_claude.md`.

---

## Summary

The user had a healthy 149-message planning conversation in Zed thread
`e6ef9c10-84bf-4aea-a6d2-2478ff08ffee` (Claude Code session, runtime
`claude_code`). The last turn ("merge main in") completed cleanly at 10:58:30.

The user then clicked **Restart** (to pick up a newly-merged `chris-outreach`
repo) and, once back, sent a follow-up: *"i restarted you so you should now see
the chris-outreach git repo…"*.

That follow-up landed in a **brand-new, empty thread**
(`041d0b91-a108-423f-93ae-370e1e013133`) with **zero prior context**. The agent
answered the follow-up with no knowledge of the task. All 149 messages of
context were invisible to it. No error was surfaced.

The context was never actually lost — `e6ef9c10.jsonl` sat intact on the
persistent workspace volume the whole time (the user recovered by manually
reopening it in the Zed AgentPanel). What was destroyed was Helix's **pointer**
to it.

---

## Timeline (UTC, from `helix-api-1` logs + DB + container FS)

| Time | Event |
|---|---|
| 10:34–10:58:30 | 7 interactions run in thread `e6ef9c10`. `last_zed_message_id` climbs 33→43→48→60→73→105→149. `config.zed_thread_id = e6ef9c10`. |
| 10:58:30 | "merge main in" turn completes cleanly (`message_completed`, state `complete`). **No crash, no wedge.** |
| ~10:59:37 | User clicks Restart → `hydra_executor: Stopping dev container`. |
| **10:59:52** | `session_handlers.go:2590` → **`🔄 [HELIX] Restart agent session — recreated container, cleared ZedThreadID (fresh thread), reset crashed prompts`**. `config.zed_thread_id` set to `""` and persisted. |
| 11:00:04 | Zed reconnects. `[CONNECT] Session loaded for reconnect`. Because `ZedThreadID == ""`, the `open_thread` re-attach block (`websocket_external_agent_sync.go:434`) is **skipped** — nothing tells Zed to reopen `e6ef9c10`. |
| 11:00:23 | User's follow-up dispatched: `📤 [QUEUE] Sending queued prompt … acp_thread_id= … first_message=true`. **Empty `acp_thread_id` → Zed forks a new thread.** |
| 11:00:32 | `thread_created acp_thread_id=041d0b91…`; `Storing Zed context ID on existing Helix session` → `config.zed_thread_id = 041d0b91`. New interaction `int_01kxzjy8…` has `last_zed_message_id=8` (fresh count). |
| 11:03:59 | Reconnect. Now `ZedThreadID=041d0b91`, so `open_thread(041d0b91)` IS sent — Helix re-attaches to the *empty* thread. |
| 11:04–11:05 | User manually reopens `e6ef9c10` in Zed; `Thread title changed in Zed acp_thread_id=e6ef9c10` → Helix updates the *session name* from it, but `config.zed_thread_id` stays `041d0b91`. |
| 11:06:10 | Helix still sends a queued prompt into `041d0b91` (opened ≠ what the user sees). |

Net: Helix drove `041d0b91` (empty); the user worked in `e6ef9c10` (real). The
two surfaces diverged with no UI signal.

---

## Root cause

`restartSessionContainer` (`api/pkg/server/session_handlers.go:2530`) is the
single canonical backend for **every** restart surface — the in-chat
`/sessions/{id}/restart-agent` button, the worker-page "Restart agent session"
button, and the spec-task detail page. It **unconditionally** clears the thread:

```go
// session_handlers.go ~2552
if previousThreadID != "" {
    session.Metadata.ZedThreadID = ""            // <-- destroys the pointer
    s.Store.UpdateSessionMetadata(ctx, sessionID, session.Metadata)
}
```

The justification (comment at :2552) is real but narrow: *a crash frequently
poisons the thread (wedged ACP turn, half-written threads.db), and reattaching
reproduces the wedge.* That is true **only for crash recovery**. But "restart"
is overwhelmingly invoked in **non-crash** situations — pick up a new repo,
refresh the environment, kick a stalled desktop — where the thread is perfectly
healthy and the user's whole expectation is *reboot the box, keep my
conversation*. For those, clearing the pointer is pure data loss.

The escape hatch the comment names — "callers that want to restart while keeping
the thread use `/sessions/{id}/resume`" — is not exposed on the surfaces users
actually click. `resume` is what should have run here; `restart` is what the
spec-task page wires up.

This is the same *opened ≠ sent-to* divergence class as the 2026-06-19 and
2026-06-22 incidents, reached by a third trigger: a deliberate, blanket thread
clear on a non-crash restart.

---

## The clear is safe to skip for a healthy thread

For `code_agent_runtime=claude_code`, the thread's entire context lives in
`~/.claude-state/projects/-home-retro-work/<zed_thread_id>.jsonl` on the
**persistent workspace volume**, which survives container recreate. Resuming a
cleanly-completed thread is exactly the healthy `open_thread` → Claude
`--resume` path, proven here by the user manually reopening `e6ef9c10` and it
working. There is no poisoned state to reproduce when the last turn is
`complete`.

---

## Fix (proposed)

**Gate the clear on evidence the thread is actually wedged; preserve it
otherwise.** Concretely, in `restartSessionContainer`, only clear `ZedThreadID`
when the thread is in a bad state:

- last interaction for the session/generation is NOT `complete` (i.e. `waiting`,
  errored, or crashed / mid-turn), **or**
- the previous container never reached `agent_ready` (boot wedge).

When the last interaction is `complete`, **keep** `ZedThreadID` so the reconnect
`open_thread` block re-attaches to the real thread and the next message resumes
it. A crash-recovery caller that genuinely wants a fresh thread keeps its
current behaviour because its last interaction won't be `complete`.

Optional hardening (defence in depth):
- Log at WARN when a restart clears a thread whose last interaction was
  `complete` — that combination should now be impossible and is a red flag.
- Surface a distinct, explicit "Reset conversation" action for the rare case a
  user actually wants a fresh thread, rather than overloading "Restart".

Must be tested against a **live, connected Zed** (per CLAUDE.md lifecycle rule):
create a spec task, let a turn complete, restart, send a follow-up, and assert
the follow-up lands in the SAME thread (`last_zed_message_id` keeps climbing, no
new `thread_created`, `config.zed_thread_id` unchanged).

---

## Session repair applied (2026-07-20)

Repointed Helix's stored thread back to the real one so Helix and Zed agree and
the next Helix-UI message resumes the conversation the user sees:

```sql
UPDATE sessions
SET config = jsonb_set(config, '{zed_thread_id}',
      '"e6ef9c10-84bf-4aea-a6d2-2478ff08ffee"'), updated = now()
WHERE id='ses_01kxzheqnf0sdsj5xdnx6zp19e'
  AND config->>'zed_thread_id' = '041d0b91-a108-423f-93ae-370e1e013133';
```

Rollback value: `041d0b91-a108-423f-93ae-370e1e013133`. No restart required — the
send path reads `ZedThreadID` fresh from the store per dispatch, and the live Zed
already has `e6ef9c10` open.

**Do NOT restart to fix this** — restart re-runs the exact clear-thread code and
would repeat the loss.

Residual: the manual 11:04+ messages the user typed directly into `e6ef9c10` in
Zed did not create Helix interactions (Helix was pointed at `041d0b91` then), so
the Helix chat transcript has a small gap versus the Zed thread. The agent's own
context (the jsonl) is complete and continues correctly.
