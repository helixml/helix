# Agent questions in Helix (ACP elicitations)

An ACP agent can ask the user a question mid-turn. Claude Code does this with its built-in
`AskUserQuestion` tool; `claude-agent-acp` turns that into an ACP elicitation
(`session/request_elicitation`, mode `form`) and **blocks the turn** until the client
answers.

Before this change that question never reached a Helix user. `crates/external_websocket_sync/`
had no concept of elicitations — its entry mapping matched only `UserMessage |
AssistantMessage | ToolCall` — so `AgentThreadEntry::Elicitation` was silently dropped and
Helix showed only the dead tool-call stub the adapter emitted just before it. The
interaction sat in `waiting` until someone interrupted it, which killed the turn with
`Tool permission request failed: Tool use aborted`.

## Wire format

Zed → Helix (new `SyncEvent` variants in `crates/external_websocket_sync/src/types.rs`):

| Event | Purpose |
|---|---|
| `elicitation_requested` | A question was asked. Carries `elicitation_id`, `entry_index`, `tool_call_id`, `mode`, `message`, the **verbatim** `requested_schema`, and `status`. |
| `elicitation_resolved` | It reached a terminal status (`accepted`/`declined`/`cancelled`/`completed`). |
| `elicitation_resync` | Completeness marker: every elicitation this thread still holds. An empty list is meaningful. |
| `elicitation_response_ack` | Result of applying an answer: `accepted` / `noop` / `not_found`. |

Helix → Zed: `ExternalAgentCommand{Type: "respond_elicitation"}` with `acp_thread_id`,
`elicitation_id`, `action` (`accept`/`decline`), and `content`.

`entry_index` is the position in `AcpThread::entries()`, which is also what the accumulator
uses as `message_id` — that is what puts the question in the right place in the transcript
with no new ordering logic.

`ElicitationStatus::Canceled` (Zed's spelling) maps to wire `cancelled` in exactly one
place, `elicitation_status_str()`.

## Storage: a row *and* a transcript entry

`agent_elicitations` is authoritative for status; a mirror lives inline in the
interaction's `ResponseEntries` as a new entry type `elicitation`. Both are written by the
same handler, so they cannot drift.

The row exists because three things need it and a jsonb blob cannot provide them:

- **Status races need a conditional write.** Two clients answering at once, an answer
  racing a cancel, and duplicate resolved events are all decided by
  `UPDATE … WHERE id = ? AND status IN (…)` returning rows-affected — not by whoever
  writes last.
- **Authorisation is by id.** The answer endpoint checks `elicitation.session_id` against
  the session in the URL. That is one indexed lookup.
- **"Which tasks are blocked on a human" must be cheap.** The auto-wake gate, the prompt
  queue and the task badge all query by indexed status.

The entry exists because the card must render **in conversation order** and stay in the
transcript after it is answered, and because the existing `EntryPatch` → `streaming.tsx`
path already delivers entries live.

## Status reconciliation: a reconnect proves nothing

The single most important rule here, and the one the first draft of this design got wrong:

> **`agent_ready` is not evidence that the agent is gone.** It fires on *every* WebSocket
> reconnect, and the commonest cause is the Helix API restarting (an Air rebuild) while
> the desktop container, the Zed process and its `respond_tx` all survive untouched.

Deriving "the question is dead" from a reconnect would silently kill live questions on
every API restart. So nothing in the Go handlers resolves a question because of a
reconnect.

Instead, **the thread holding a pending question re-affirms it every 15 s**
(`ELICITATION_HEARTBEAT_INTERVAL`), refreshing `last_seen_at`. The reaper cancels only
questions whose affirmations have stopped for longer than the grace window (60 s default,
`HELIX_ELICITATION_RESYNC_GRACE_SECONDS`). A real container restart is still handled — just
on evidence rather than assumption. A reconnect additionally triggers a **full**
re-announcement (payloads, not just ids) so a Helix that lost its state can rebuild.

A heartbeat rather than a reconnect-only resync is required for a second reason: with
reconnect-only, a question the user takes ten minutes to answer produces no fresh statement
and looks identical to a dead one, while a container restart that registers no threads
emits no resync at all and so yields no evidence to reap on.

Other rules:

1. Zed's `ElicitationStore` is the source of truth for status.
2. Helix's `submitting` is optimistic and local — never sent to Zed, never shown as
   "answered".
3. Terminal statuses are final; a late event affects zero rows and is logged.

## Follow-up while a question is pending

Zed's model is unambiguous: `AcpThread::run_turn` unconditionally calls
`cancel_inner(RequestPermissionOutcome::InterruptedByFollowUp)`, which calls
`cancel_outstanding_elicitations` *before* the `running_turn` check. A follow-up therefore
cancels the outstanding question and the new turn proceeds. Helix mirrors that rather than
inventing a third behaviour.

One Helix change was needed: `processPromptQueue` defers any non-interrupt prompt while the
newest interaction is `waiting`. A pending question holds that state indefinitely and
auto-wake now deliberately leaves it alone, so the user's message would have sat in the
queue forever with no feedback. It now dispatches instead, and lets the agent report the
resulting `cancelled` status back.

## The schema is data, and its `_meta` keys have already moved

The frontend builds the form generically from the JSON Schema. It does **not** match on
`question_0`, and it does **not** match on any literal `_meta` key name — verified against
the deployed adapter (claude-agent-acp **0.66.0**), the real keys are
`_askUserQuestionCustomAnswer` and `_claude/askUserQuestionOption`, not the
`claudeCode/*` names the original brief quoted. Custom-answer fields are found by the
*shape* of their metadata (`isCustomAnswer`, or a `questionId` naming a sibling), which
survives another rename.

Answer folding mirrors `applyAskElicitationResponse` exactly:

- A custom answer that is non-empty after `.trim()` wins over that question's selection,
  and the selection is not sent at all. Submitting **only** a custom answer is valid — the
  "none of the above" case.
- A question with neither is omitted. Nothing is `required`; partial answers are legal.
- **`decline` is not an abort**: it yields `{action:"answered", answers:{}}`, the tool call
  succeeds and the turn continues. The UI therefore labels it **"Skip"**, not "Decline".

## Failure modes

| Case | Behaviour |
|---|---|
| API restart with a question pending | Row + entry persisted; Zed still holds the `respond_tx`; the heartbeat refreshes `last_seen_at`; the question stays answerable. |
| Container/agent restart | Affirmations stop; after the grace window the reaper cancels it with reason `agent_no_longer_holds`; the card reads "expired — the agent restarted". |
| User sends a message instead of answering | Queue dispatches it; Zed cancels the question; card locks as "you replied instead". |
| Answer after cancel/decline | Conditional update affects 0 rows → 409; if the command already went out, Zed no-ops and acks `noop`. |
| Two clients answer at once | First wins `pending → submitting`; second gets 409. Only one command is ever sent. |
| Send fails after claiming | The claim is rolled back to `pending` so the user can retry. |
| Unmappable `request_id` | Falls back to `handleMessageAdded`'s existing resolution; dropped with a loud `error` log only if that also misses. |
| Unknown schema shape | Generic field rendering; the card offers Skip so the turn can continue. |

## Notification

A pending question raises an `agent_question` attention event via
`AttentionService.EmitEvent`, which fans out to the in-app bell, the project's Slack
thread, and the org event sink from one call. The **elicitation id is the idempotency
qualifier**, so the heartbeat and reconnect re-announcements cannot re-notify. It is
dismissed per-event (not task-wide) on any terminal status.

Deliberately *not* suppressed when the user is active in the session, unlike
`agent_interaction_completed` — that heuristic is right for "your turn finished" and wrong
for a question, which needs an answer either way.

## Known follow-up (out of scope here)

`api/pkg/org/infrastructure/runtime/helix/spawner.go` has its own activation timeout that
can spawn a decoy `waiting` interaction on top of a healthy session. It does not know about
blocked-on-human either. Only `auto_wake_stuck_interactions.go` was gated in this change.
