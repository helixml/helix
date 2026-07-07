# CI notification injects a concurrent turn mid-turn → second interaction sits empty

**Date:** 2026-07-06
**Spec task:** `spt_01kwrt1nvtrvnsrxgteagg46mp` ("Multi-Tenant, Multi-Brand Modular HelixOS")
**Helix session:** `ses_01kwrt1nwpdam8k9y9fte0h90h`
**ACP thread:** `ccc26138-d4dc-4cb5-9126-55424d500609`
**Instance:** meta.helix.ml (localhost dev stack)

## Symptom (user report)

> "The agent is busy running long-running slow compile steps but the interaction
> in Helix has not been updated at all."

User also noted, crucially, that this is **not** the classic off-by-one:
> "last time both interactions landed in approximately the same second but that
> didn't happen this time … this new interaction was sent by the automatic CI
> notification system so maybe that's relevant."

That instinct is correct. This is a different failure from the
`request_id`-desync off-by-one family
(`2026-03-15-interaction-routing-fifo-queue.md`,
`2026-04-25-zed-claude-async-event-flush-on-user-input.md`,
`2026-04-28-stale-request-id-rebind-loses-zed-updates.md`).

## Evidence

Two `waiting` interactions, **2 minutes apart** (not same-second), each an
**independent CI notification**:

```
int_01kwwbav  18:34:02  waiting  req_68aa13e7  resp GROWING 15k→37k→…  msg 491→555+
              prompt="CI failed for PR #2809 (helix). … investigate and push a fix."
int_01kwwbef  18:36:01  waiting  req_7aa78f60  resp_len=0  last_zed_message_id=(none)
              prompt="CI passed for PR #35 (helixos). https://…/pull/35/checks"
```

- `int_01kwwbav` is streaming **live and correctly** — Zed.log shows the agent
  running slow CGo builds / `go test` chasing PR #2809's CI failure, all under
  `req_68aa13e7`, bound to `int_01kwwbav`. At 18:48 it was at 37KB / message 555
  and still going (a genuine ~14-min turn).
- `req_7aa78f60` (int_01kwwbef's id) appears **0 times** in either Zed log — the
  agent has never emitted a token for it.

**Ruled out — this is NOT the off-by-one bug:**
- Zero stale-`request_id` / mapping-consumed / DB-fallback ("most recent waiting")
  events in the API log for this window. Only clean
  `Matched interaction by request_id mapping`.
- The content in `int_01kwwbav` genuinely belongs to `int_01kwwbav` (its own
  `req_68aa13e7`), not misrouted from `int_01kwwbef`.

So Helix's FIFO routing is working: queue = `[int_01kwwbav, int_01kwwbef]`,
stream goes to `queue[0]` = the running turn ✓, and `int_01kwwbef` waits behind.

## Root cause

The CI notifier (`api/pkg/services/spec_task_ci_notifier.go`) delivers results
via `SpecTaskMessageSender` with `interrupt=false`. Its comment states the
intent:

> "interrupt is always false: CI results aren't urgent enough to interrupt
> mid-turn — the agent picks them up at the next message-pump tick."

But the sender it uses — `sendMessageToSession` /
`sendMessageToSpecTaskAgent` (`spec_task_design_review_handlers.go:~1791`,
"✅ Sent message to session agent via WebSocket") — **dispatches immediately**.
It pre-creates the interaction and sends a `session/prompt` (fresh `request_id`)
over the WebSocket **regardless of whether a turn is already running**. It does
**not** consult the prompt-queue busy-check
(`processPendingPromptsForIdleSessions`, which is the path that actually defers
user-typed Helix messages while `latest interaction == waiting`).

`interrupt` only controls whether a `cancel` is sent first — it does **not**
control queue-vs-immediate dispatch. So `interrupt=false` does not mean "wait
until idle"; it means "don't cancel, just send now."

Sequence:
1. 18:34:02 — CI notify "PR #2809 failed" → `sendMessageToSession` → creates
   `int_01kwwbav`, dispatches `req_68aa13e7`. Agent (idle then) starts the turn.
2. 18:36:01 — CI notify "PR #35 passed" → `sendMessageToSession` → creates
   `int_01kwwbef`, dispatches `req_7aa78f60` **while the agent is still mid-turn
   on `req_68aa13e7`**.

`int_01kwwbav` is not cancelled (it keeps streaming), so Zed did not run its
`run_turn()` cancel-then-restart; the second prompt is buffered at the
Zed/ACP message-pump and has not been handed to the ACP thread yet. Hence
`int_01kwwbef` is empty and `req_7aa78f60` is silent.

## Expected resolution

When `int_01kwwbav`'s turn completes, Zed's message pump should hand
`req_7aa78f60` to the ACP thread; Helix pops the FIFO to `[int_01kwwbef]` and
routes the response there. So it **should self-resolve** when the long PR #2809
turn finishes — which matches the user's "might resolve itself as soon as it
finishes."

**Residual risk:** if the wrapper mishandles the queued second prompt after a
long turn (claude-agent-acp #551 family — next prompt bounces with empty
`end_turn`), `int_01kwwbef` could complete empty. If that happens, a tiny nudge
message flushes it (the documented manual mitigation). Watch whether
`int_01kwwbef` gets content shortly after `int_01kwwbav` completes.

## The actual defect vs. UX wrinkle

- **Functionally**, Helix FIFO is correct and the message is (probably) queued.
- **The defect** is that the CI notifier's *intent* ("queue, deliver when idle")
  is not honored by its *sender* (immediate dispatch). Firing a second
  `session/prompt` mid-turn is exactly the condition that pokes the fragile ACP
  async paths, and it produces a confusing UI: a new `waiting` card with no
  content while the real work streams into the previous card.

## Fix direction (not yet implemented)

Make automated, non-urgent injections (CI results, and any other
system-generated notifications) actually respect the busy-check instead of
immediate-dispatch:

- Route them through the same queue discipline as user queue-mode messages:
  enqueue a `prompt_history_entries` row (interrupt=false) and let
  `processPendingPromptsForIdleSessions` dispatch it **only when the latest
  interaction is not `waiting`**. This delivers the notification cleanly as the
  next turn once the agent is idle — which is what the notifier comment already
  claims happens — and never sends a concurrent `session/prompt` mid-turn.
- Alternatively, have the notifier check busy-state and hold/coalesce CI results
  until idle (multiple CI transitions during one long turn could be batched into
  a single "here's what happened while you were working" message).

Preferred: the first — reuse the existing queue rather than add a second
deferral mechanism. This also composes with the desktop-resume reap fix from
PR #2808 (a queued CI notification on a stopped desktop would then boot it
cleanly rather than stranding a concurrently-dispatched turn).

## This is systemic: four automated senders share the same wrong assumption

`sendMessageToSession` → `sendChatMessageToExternalAgent(sessionID, message,
requestID, interrupt)` has **no busy-check and no queue routing**. The only
"queue" it has is the *offline* path: if there is **no WS connected**, it returns
`ErrNoExternalAgentWS`, the interaction is persisted, and `pickupWaitingInteraction`
delivers it on reconnect. When the agent is **online but mid-turn**, the message
is dispatched as a concurrent `session/prompt` immediately. `interrupt` only
chooses cancel-first (true) vs. send-now (false) — **neither value defers until
idle**.

Multiple automated callers pass `interrupt=false` in the documented belief that
it queues behind the in-flight turn. It does not. Grep for the tell-tale comments:

| Site | Message | Comment (all wrong about "queue") | Mid-turn risk |
|---|---|---|---|
| `spec_task_ci_notifier.go:50` | CI pass/fail result | *"the agent picks them up at the next message-pump tick"* | **High** — CI transitions fire from the PR poll loop while the agent works (this incident) |
| `spec_task_workflow_handlers.go:213` | post-merge **push** instruction | *"let it queue behind any in-flight agent turn"* | **Med-High** — fires on PR merge; agent may be mid-turn |
| `spec_task_workflow_handlers.go:314` | post-merge-failure **rebase** instruction | *"system-driven follow-up"* | **Med** — same trigger family |
| `agent_instruction_service.go:673` (`SendApprovalInstruction`) | approval kickoff | *"begins a new phase with an idle agent; respect the queue"* | **Low** — usually idle at approval, but not guaranteed |

The `interrupt=true` callers are a different category — deliberate interrupts
that *intend* to cut into the current turn (immediate dispatch is correct there,
modulo the separate boot-race hazards in
`2026-06-19-incident-interrupt-during-boot-context-loss.md`):
`spec_driven_task_service.go:1457` (reviewer revision), `spec_tasks_org_wiring.go:34`
(org status transition), `spec_task_design_review_handlers.go:403,1251` (design-review
submit / comment reply), `session_handlers.go:2324` (user send endpoint, user-controlled).

Also worth auditing but not confirmed to dispatch mid-turn:
`AgentInstructionService.SendImplementationReviewRequest / SendRevisionInstruction /
SendMergeInstruction` (759/779/799) — they build prompts; confirm their send path.

### Unifying fix

Give `interrupt=false` a real meaning: **enqueue + defer until idle**. Route the
four automated senders through the same `prompt_history_entries` +
`processPendingPromptsForIdleSessions` busy-check that user queue-mode messages
use, instead of immediate `sendChatMessageToExternalAgent`. Then the comments
become true, no automated notification ever dispatches a concurrent
`session/prompt` mid-turn, and multiple CI transitions during one long turn
naturally coalesce as the queue drains. One fix corrects all four sites.

## Artifacts captured (session scratchpad)

- `zed.log.current` / `zed.log.old` — ACP wire trace (rotates fast; the 18:36:01
  dispatch moment was already rotated out — coverage starts 18:36:49).
- `snap.txt`, `recheck1.txt` — interaction-state snapshots over time.
- `desktop-ps.txt` — process tree (claude-agent-acp, gopls, live compile).
- `fingerprint.txt` — proof of absence of stale-request_id events.
