# Stuck queue UI, mobile-unrecoverable, and friends

**Date:** 2026-04-30
**Reported by:** user, while trying to use the product today
**Spec task that triggered the report:** `spt_01kq8cnjzfqc51nn0c6ddxkw8r` (runners infrastructure)
**Status:** root cause identified, no fix shipped yet

---

## What the user sees

The message queue keeps showing items that the agent already consumed and replied to. The reply lands in the chat normally, but the queue entry never disappears. On mobile the user can't even clear them by hand because the delete button is off-screen — so once one gets stuck, that queue is now permanent furniture in the UI.

This isn't an isolated bug; it's part of a pattern of "state that won't unstick itself" we've shipped over the last few weeks (chrome MCP cold-start timeouts, queue prompts marked failed when they actually got delivered, no-WS dispatches losing their mapping after restart, etc.). Each of those got patched in isolation. This doc looks at the current case end-to-end and lands the architectural change that would prevent the whole class from recurring.

---

## What's actually wrong

Two independent bugs glued together by their UX:

### Bug A — backend: prompts get stuck in `sending` forever

The DB tells the story. Run this for any reasonably-active user:

```sql
SELECT spec_task_id, status, COUNT(*)
FROM prompt_history_entries
WHERE user_id = '<user>' AND deleted_at IS NULL AND status != 'sent'
GROUP BY spec_task_id, status;
```

For the user reporting today: **67 prompts in `sending` state across 27 spec tasks**, the oldest from 2026-04-24 (six days stuck). Their content ranges from "go on" to multi-paragraph instructions. Every single one was actually delivered to Zed and processed; the chat shows the response. The queue UI shows them because the *backend* status field is wrong.

Why the status is wrong: `MarkPromptAsSent` is only called from one place, `handleMessageAdded` at `websocket_external_agent_sync.go:1142`, and it requires the in-memory `interactionToPromptMapping[interaction.ID]` to point back at the prompt. That map is created in `sendQueuedPromptToSession` when the dispatch starts. **The map is in-memory only.** Anything that tears down or loses the entry between dispatch and Zed's first `message_added` orphans the prompt:

- API server restart (extremely common — Air auto-rebuilds in dev, deploys flush in prod).
- The dispatch path failed and we cleaned up the entry, but the persisted Waiting interaction is later picked up by `pickupWaitingInteraction` on reconnect (different request_id, no link back to the prompt).
- The auto-wake worker re-sends the original prompt with a fresh `request_id` (drains the wrapper buffer); Zed's eventual response comes in with the new id which has its own (different) interaction binding.
- The streaming context's transition-detection fires and rebinds the request_id to a different interaction (last week's bug, fixed in PR #2311).

In all of these, the interaction *does* eventually reach `state=complete` correctly, because that's keyed by `request_id` and has multiple recovery paths (DB fallback, most-recent-Waiting, etc.). But the prompt → interaction link, which has no recovery path because it's only an in-memory map, just dies.

The frontend's polling effect (`usePromptHistory.ts:404-465`) faithfully syncs the backend state. Backend says `sending`, the local row says `sending`, the queue keeps it. There is no "this has been sending for six days, should I assume something went wrong" heuristic anywhere.

### Bug B — frontend: mobile delete button is off-screen

`SortableQueueItem` is a horizontal flex row: drag handle, status icon, content, then an actions box (interrupt toggle + delete X). The actions box has no `flexShrink` setting, so it defaults to allowing shrink, and the inner content wrapper (line 306, the `<Box sx={{ display: 'flex' }}>` around the Typography and edit-pencil) doesn't have `min-width: 0` so the unbreakable Typography line forces it to its intrinsic min-content width. Result on a narrow viewport: the inner row's intrinsic width eats the available space, the actions box gets pushed past the right edge, and the queue container's `overflow: hidden` (line 994) clips it.

Two CSS lines fix it: `min-width: 0` on the inner content wrapper so the Typography ellipsises properly, and `flex-shrink: 0` on the actions box so it never disappears.

(There is a *separate* code path that hides actions entirely when `isSending` is true — that's local frontend state for "we're literally pushing this prompt out the wire right now", and is unrelated to the backend's stuck `sending` status. The user can trigger it briefly, but it shouldn't be the cause of the off-screen reports — those are CSS.)

---

## Why this keeps happening

Zoom out: this is exactly the failure mode `design/2026-04-28-websocket-sync-architecture-review.md` predicted. We have **state of the same logical thing duplicated across at least four places**:

- `prompt_history_entries.status` (DB)
- `interactions.state` (DB)
- `interactionToPromptMapping[id]` (in-memory map, server-side)
- `helix_prompt_history` localStorage (per-browser)

When all four agree, things look great. When the in-memory map drops a key (any of the reasons above), the DB rows drift apart and stay drifted — neither side has a self-correcting feedback loop. That doc proposed three options; the relevant one here is **option 3 ("one thread = one in-flight turn") combined with option 1 ("state sync, not event sync")**. Both reduce the number of places state lives.

This is the third user-visible symptom of the missing prompt↔interaction link in two weeks (PR #2311 had two related ones: "no WebSocket" false failures and "session became busy" false failures, both stemming from the same orphan-on-failure pattern).

---

### Bug C — accumulator: Zed wrapper restart drops new content as "replay"

(Found while investigating Bug A; user followup pointed to the symptom.) Live incident on `int_01kqjsrhndcpwb9zv068dn7mv9` (2026-05-01, session `ses_01kq8cnnkmww35bacpscbrehn0`): Helix dispatched a queued prompt, Zed received it, the agent processed it, Zed sent ~30 `message_added` events back over the WebSocket, then `message_completed` fired with empty content and Helix marked the interaction `Error` with "Agent returned empty response (message bounced or content lost). The prompt will be retried."

The events weren't lost on the wire — `Zed.log` shows them being sent with the right `acp_thread_id` and `request_id` and content like `"<thinking>\n\n</thinking>"`. But Helix's accumulator's `acc.Content` stayed at 0 across all of them.

`MessageAccumulator.AddMessageWithToolInfo` (`accumulator.go:151`) was silently dropping every event because of the `priorMessageIDs` filter added in commit `714d6036a` (PR shipped 2026-04-29). That filter prevents Zed's `flush_streaming_throttle` from leaking prior-turn entries into the current interaction by maintaining a set of `message_id`s seen in earlier completed interactions and dropping any `message_added` whose id is in the set.

The flaw: `message_id`s aren't actually unique across turns within a thread. When the `claude-agent-acp` wrapper inside Zed restarts (which happens on agent reconnects, container restarts, etc.), the wrapper's internal counter resets and starts handing out the same `message_id`s again for legitimately new content. The filter sees the id match and drops the new content as if it were a replay. The interaction completes empty and bounces.

The DB confirmed this for the failing turn: prior interaction `int_01kqezqm1t54jrtj970w0z7ej6` had `message_id`s 340-348; the new turn sent 348-351 (numbering had reset). The filter dropped all of them.

**Fix:** content-aware dedup. The accumulator now stores `(message_id → content)` pairs in `priorMessageContent` instead of just `message_id`s in `priorMessageIDs`. `AddMessageWithToolInfo` drops only when both the id AND the content match — that's a true `flush_streaming_throttle` replay. Renumbered new content (same id, different content) passes through as it should.

The new `SetPriorEntries(entries []ResponseEntry)` API replaces `SetPriorMessageIDs(ids []string)`. The id-only API is kept for any external callers but only matches empty-content replays — most callers should migrate to the new entry-aware form.

## Other incidents from this week worth mentioning together

The user said "many related incidents I've seen while using the product today." Without a fuller list from them, the ones I've seen go past this week, all around the same area, are:

- Chrome MCP not loading on cold container start (root cause: 60s context-server timeout dead-coded by a prior fix; PR #2320 today).
- Stale `request_id` rebind in the streaming context misroutes Zed's mid-stream completions (PR #2311).
- Claude Agent process crash inside Zed → infinite "Session not found" retry loop (PR #2311).
- "No WebSocket connection" + "session became busy" red errors when the message did get through (PR #2311).
- The architecture review (`design/2026-04-28-websocket-sync-architecture-review.md`) was written specifically because the *rate* of these is climbing, not their individual difficulty.

Pattern across all of them: an in-memory data structure on one side gets out of sync with persisted state on the other, and the recovery mechanism is "another in-memory data structure that has to also be in sync". Each fix layer adds one more thing that can be out of sync.

---

## Fix plan (all shipped together)

1. **Backend: persist the prompt↔interaction link in the DB.** New `Interaction.PromptID` column written on creation by `sendQueuedPromptToSession`. `handleMessageAdded` and `handleMessageCompleted` both mark the prompt `sent` by reading the column. The in-memory `interactionToPromptMapping` map is **deleted** — no more in-memory state to drift from the DB. This is the architectural fix that removes the bug class for this particular link.

2. **Backend: continuous reconciliation.** `handleMessageCompleted` also marks the prompt `sent` (idempotent) when the interaction completes. Catches any case where `message_added` didn't fire (e.g. the agent went straight to completion) without needing a separate worker.

3. **Backend: one-shot startup reconciliation.** New `Store.ReconcileStuckSendingPrompts` runs in a goroutine at `NewServer`. Two queries: precise (join on `interactions.prompt_id`) for new rows; legacy heuristic (session_id + 5-minute floor + existence of any `Complete` interaction since the prompt) for rows that predate the column. Unsticks the 67+ legacy orphans.

4. **Backend: content-aware accumulator dedup (Bug C).** New `SetPriorEntries(entries []ResponseEntry)` carries `(id, content)` pairs. `AddMessageWithToolInfo` drops only on exact `(id, content)` match — true `flush_streaming_throttle` replays — not on bare id match. Wrapper-restart renumbering is now safe.

5. **Frontend: mobile CSS fix.** `min-width: 0` on the inner content wrapper, `flex-shrink: 0` on the actions box. Delete button stays visible at 375px.

### Won't do (yet)

- Frontend "stuck-detection" badge. With (1)+(2)+(3) shipped, prompts shouldn't get stuck except in a vanishingly small new-orphan window. Existing X delete (now visible on mobile) is sufficient. Revisit if user reports persist.

- A full background janitor scanning the DB on a timer. The continuous reconciliation in (2) plus the structural fix in (1) should make this unnecessary; we already have one stuck-state worker (`auto_wake_stuck_interactions.go`) which is its own source of complexity.

- Surface "mark sent" through the WebSocket sync handler with explicit acknowledgement events. Considered in the architecture review and rejected as adding more event types to a protocol we want to *shrink*.

---

## Testing the fix manually

For Bug A:

1. Pick a known-stuck prompt: `SELECT id FROM prompt_history_entries WHERE status = 'sending' AND created_at < NOW() - INTERVAL '1 hour' LIMIT 1;`
2. Run the reconciliation, confirm the row transitions to `sent`.
3. Confirm the corresponding interaction is `Complete` in the DB.
4. Reload the spec task page, confirm the queue is shorter by one.

For Bug B:

1. Open a spec task with a stuck queue item in Chrome DevTools.
2. Toggle device-toolbar to iPhone SE (375px).
3. Confirm delete button is visible on each queue row, even with long content.

For (4) regression risk:

1. New spec task, dispatch a queued prompt, kill the API mid-flight, restart, confirm prompt eventually transitions to `sent` once Zed responds.
2. Send rapid succession of queued prompts (5+), confirm all transition to `sent`.
3. Trigger the auto-wake worker on a stuck interaction, confirm the prompt still transitions to `sent` (don't end up with double-marking or skipping).

---

## Open questions

- Are the 67 stuck rows from this user's account actually safe to mark sent? My read is yes — every one I sampled had a corresponding `Complete` interaction in the same session — but a one-shot reconciliation should log what it changes so we can audit before anyone is surprised.
- The `pickupWaitingInteraction` path on reconnect does NOT register `interactionToPromptMapping`. Should it? If we're going to do (4) anyway, the answer is "doesn't matter, the DB column carries it". If we delay (4), this is one more in-memory orphan source worth fixing on its own.
- Mobile Chrome's `position: sticky` breaks for the queue actions in some configurations. If after the CSS fix users still report off-screen actions, the next step is to make the actions absolutely positioned to the right edge with the content scrolling under, rather than relying on flex layout entirely.
