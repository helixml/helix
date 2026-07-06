# Requirements: Unify All Agent Message Sending on the Session-Scoped Prompt Queue

## Background

Helix has **two parallel implementations** of "create a waiting interaction and
send a `chat_message` to the Zed/ACP agent." Both bottom out on the same
`sendCommandToExternalAgent` primitive, but only one carries the busy/idle
discipline.

- **GOOD — the prompt-queue path** (`prompt_history_entries` rows →
  `processPendingPromptsForIdleSessions` → `processPromptQueue` /
  `processInterruptPrompt` → `sendQueuedPromptToSession`). Honors `interrupt`
  first-class: `interrupt=true` cancels the current turn (respecting the
  thread-establishment boot barrier) and sends; `interrupt=false` is **held
  until the latest interaction is not `waiting`** (deferred until idle). Also
  carries the orphaned-waiting reap guard (PR #2808). Empirically reliable.
  **But it is spec-task-scoped**: `prompt_history_entries.SpecTaskID` is
  `NOT NULL` and the poller entry point is keyed on a spec task.
- **POOR — the direct path** (`sendChatMessageToExternalAgent` →
  `sendMessageToSession` → `sendMessageToSpecTaskAgent`). Creates the waiting
  interaction and emits the `chat_message` **immediately, with no busy-check and
  no deferral**. `interrupt` here only chooses cancel-first (true) vs send-now
  (false); **neither value defers until idle**.

Everything that is **not** a spec-task queue message uses the poor path — and
that includes two important classes:

1. **Automated spec-task senders** (CI, post-merge push, rebase, approval) that
   pass `interrupt=false` believing it queues. It doesn't — they fire a
   concurrent `session/prompt` mid-turn, stacking a second empty `waiting`
   interaction. This caused a production incident (attachment
   `2026-07-06-ci-notification-concurrent-turn-mid-turn.md`).
2. **Org "bot" / general session sends.** The org runtime's
   `inProcHelixClient.SendMessage` → `POST /sessions/{id}/messages` →
   `sendMessageToSession` sends with `interrupt=false` (never set → zero value),
   on the poor path. Bot activations are serialised per Worker only at
   *dispatch*, not per *turn* (`SendMessage` is fire-and-forget), so a Worker's
   next activation can dispatch while its previous turn is still streaming — the
   same concurrent-mid-turn failure. **Strongly suspected root cause of "bots
   are unreliable"** (to be confirmed with a live repro).

## Goal

Make the session-scoped prompt queue the **single way** to send a message to an
agent. `interrupt` means the same thing everywhere:
- `interrupt=false` → **defer until the agent is idle** (bots, general sends,
  push, rebase, approval);
- `interrupt=true` → **cancel the current turn (boot-safe) then send** (CI
  results, design-review comments, org transitions, reviewer revision).

Then **delete the poor direct-dispatch path entirely** — no fallbacks, no dead
code (repo rules). No automated sender or bot ever fires a concurrent
`session/prompt` mid-turn again.

## User Stories

### US1 — CI results interrupt cleanly (no concurrent empty interaction)
When CI passes/fails mid-turn, the notification is delivered immediately as a
proper interrupt (cancel current turn, respecting the boot barrier, then send),
never as a second empty `waiting` card behind the running one.

**Acceptance criteria**
- CI results enqueue with `interrupt=true`; delivered via `processInterruptPrompt`.
- During boot (thread not established) the interrupt defers and lands in the same
  thread once established (no divorced thread).
- No coalescing — each CI transition is its own interrupt.

### US2 — Post-merge push/rebase & approval respect the queue
These enqueue with `interrupt=false` and defer while the agent is busy, delivered
in order once idle.

**Acceptance criteria**
- Push, rebase, approval are enqueued interrupt=false and deferred while busy.

### US3 — Bot / general session sends defer until idle
A message sent via `POST /sessions/{id}/messages` (the endpoint org bots use)
while the agent is mid-turn is **held until the current turn completes**, then
delivered as the next turn — not dispatched concurrently.

**Acceptance criteria**
- `sendSessionMessage` enqueues onto the session-scoped queue instead of the
  direct path.
- A send to a mid-turn session creates **no** concurrent second `waiting`
  interaction; it is delivered once idle.
- If the desktop is stopped/offline, the queued message boots it (existing queue
  → autostart path) and delivers on reconnect.
- Org bots (which call this endpoint) inherit this behaviour with no org-runtime
  code change.

### US4 — Deliberate interrupts still interrupt (via the queue)
Design-review comment reply, org status transition, and reviewer revision remain
immediate interrupts — now delivered through the queue's `interrupt=true` path.

**Acceptance criteria**
- These paths enqueue `interrupt=true` and preempt the current turn.
- Design-review comment **response routing still works**: the streamed response
  is attributed to the commenter and finalized onto the comment (re-plumbed to
  resolve via the interaction's `PromptID` rather than a synchronously-returned
  request/interaction id).
- The interrupt-during-boot barrier is not regressed
  (`design/2026-06-19-incident-interrupt-during-boot-context-loss.md`).

### US5 — Exactly one path; direct path deleted
There is one way to message an agent. The direct path and its wrappers are gone.

**Acceptance criteria**
- `sendChatMessageToExternalAgent`, `sendMessageToSession`,
  `sendMessageToSpecTaskAgent`, `MessageSenderCINotifier`, and the unused
  instruction methods (`SendImplementationReviewRequest`,
  `SendRevisionInstruction`, `SendMergeInstruction`) + their now-unused helpers
  are deleted.
- `CGO_ENABLED=0 go build ./...` passes with no unused symbols.

## Non-Goals

- Rewriting the org-graph runtime. We only change the transport underneath its
  existing `SendMessage`.
- Changing agent/Zed-side ACP behaviour.
- A user-facing prompt-history UI for non-spec-task sessions (the rows are just
  the queue mechanism there).

## Decisions (resolved at review)

1. **One PR, full unification.** The comment-reply reroute (finalize via
   `Interaction.PromptID`) ships in **this** PR — genuine "one way." It is the
   highest-risk piece and must be tested live end-to-end (see design Risks).
2. **`POST /sessions/{id}/messages` becomes async.** It returns the queue-entry
   id instead of `{request_id, interaction_id}`; regenerate the API client +
   update the CLI. (Org runtime ignores the return; CLI to be updated.)
3. **`SpecTaskID` nullable migration** proceeds; add an explicit column alter if
   AutoMigrate won't relax `NOT NULL`.
4. **User attribution:** spec task → `CreatedBy`/`Owner`; general session →
   `session.Owner`.
5. **Confirm bot causation live** during implementation: reproduce a bot
   mid-turn overlap (empty concurrent interaction) before, show it gone after —
   to prove this is *the* cause, not just *a* cause.

## Open Questions

None (all resolved at review).
