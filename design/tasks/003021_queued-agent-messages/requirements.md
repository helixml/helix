# Requirements: Make API-Queued Agent Messages Visible in the Prompt Queue

## Background

`POST /api/v1/sessions/{id}/messages` (used by HelixOS and other org bots) enqueues a
row in `prompt_history_entries` with an **empty `spec_task_id`**, because
`session_handlers.go:2571` hardcodes `""` as the last argument to
`enqueueAgentMessage`. The spec-task prompt-queue UI queries by `spec_task_id`, so
those rows are filtered out and the user sees nothing. On 2026-09-01, 53 approvals
Luke made in HelixOS were correctly queued and correctly `pending` — and completely
invisible.

Two related defects are in scope because they sit in the same code path:

- `ListPromptHistory` has no `deleted_at IS NULL` predicate, so soft-deleted prompts
  can reappear in the queue UI.
- `processAnyPendingPrompt` has no busy pre-check, so every cancel-ack claims the
  head-of-queue prompt, gets a busy-defer from `sendQueuedPromptToSession`, and calls
  `MarkPromptAsFailed` — burning the 20-attempt retry budget. At 20 the selectors stop
  picking the row up and the message is **silently dropped forever**.

## User Stories

### US-1: Approvals sent through the API are visible in the queue
**As** a user who approves cards in HelixOS (or any caller of the session-messages API),
**I want** the queued message to appear in the spec-task prompt-queue UI as pending,
**so that** I can see my action landed and is waiting rather than vanishing.

Acceptance criteria:
- [ ] A message sent via `POST /api/v1/sessions/{id}/messages` while the agent is
      mid-turn appears in the prompt-queue UI on that session's spec-task page, shown
      as pending/waiting.
- [ ] The created `prompt_history_entries` row has `spec_task_id` set to the spec task
      whose `planning_session_id` equals the message's session id.
- [ ] The message still dispatches when the current turn completes, and the content the
      agent receives is byte-for-byte unchanged.
- [ ] A session with **no** owning spec task (plain org-chat session) still enqueues and
      dispatches successfully, with `spec_task_id` left empty. No error, no behaviour change.
- [ ] `ListPromptHistory`'s `spec_task_id` filter is NOT widened (no `OR spec_task_id = ''`)
      and its `user_id` scoping is NOT removed.

### US-2: Existing invisible queue rows become visible
**As** Luke, with 53 already-queued approvals stuck invisible,
**I want** the existing rows repaired,
**so that** my current queue shows up without re-sending anything.

Acceptance criteria:
- [ ] A one-off backfill stamps `spec_task_id` on existing rows where `session_id`
      matches a spec task's `planning_session_id` and `spec_task_id` is empty/NULL.
- [ ] The backfill is idempotent — running it twice changes nothing the second time.
- [ ] It never overwrites a non-empty `spec_task_id`.
- [ ] Whether the backfill ran is stated explicitly in the PR description.

### US-3: Soft-deleted prompts stay deleted
**As** a user who deleted a queued prompt,
**I want** it to stay out of the queue,
**so that** the UI reflects what I actually deleted.

Acceptance criteria:
- [ ] `ListPromptHistory` excludes rows with `deleted_at` set, matching
      `ListPromptHistoryBySpecTask` and `ListPromptHistoryBySession`.
- [ ] The `total` count returned also excludes deleted rows.

### US-4: Busy-defers don't consume the retry budget
**As** a user who interrupts the agent while messages are queued,
**I want** my queued messages to survive,
**so that** they are not silently dropped after ~20 interrupts.

Acceptance criteria:
- [ ] Repeatedly triggering the cancel-ack path while the agent is mid-turn does NOT
      increase `retry_count` on the head-of-queue prompt.
- [ ] After the defers, the prompt is still `pending` (or `failed` only for genuine
      errors) and still dispatches normally when the session goes idle.
- [ ] Genuine failures (interaction creation errors, WS send errors) still increment
      `retry_count` and still respect the `defaultMaxPromptQueueRetries = 20` cap.
- [ ] Interrupt prompts keep their busy-check exemption, and the boot-race exception
      (empty `ZedThreadID`) still defers them — see the comment block at
      `websocket_external_agent_sync.go:3745-3800`.
- [ ] Exactly ONE of the two candidate fixes is implemented, not both.

### US-5: Verified live, not just compiled
**As** a reviewer,
**I want** browser evidence,
**so that** I know the visible-queue outcome actually works end-to-end.

Acceptance criteria:
- [ ] Tested in the inner Helix at `localhost:8080` against a LIVE Zed session
      (`config->>'zed_thread_id'` is a non-empty UUID).
- [ ] A screenshot of the spec-task prompt queue containing the API-sent message is
      attached to the task's `screenshots/` folder. This is the deliverable.
- [ ] Go unit tests cover session→spec-task resolution (found and not-found cases) and
      the busy-defer retry-count behaviour.

## Out of Scope

- The known hazard that a queue owned by one user is invisible to another viewer of the
  same project (the `user_id` scoping on `ListPromptHistory`). Not this bug — on the live
  session the prompt owner and viewer are the same account. Leave it alone.
- Surfacing retry-cap exhaustion to the user in the UI.
- Any change to the frontend queue rendering — it is correct today, it was only ever
  being handed an empty result set.

## Open Questions

1. **Backfill delivery mechanism.** The plan is a numbered SQL migration
   (`0010_backfill_prompt_history_spec_task_id`) so it runs automatically on every
   deploy including meta. Alternative is a hand-run one-off UPDATE against the meta DB.
   Migration is assumed — confirm that is acceptable for prod, and that a one-shot
   backfill baked into migrations (rather than a manual DBA step) is the house style.
   The `down` migration cannot restore the previous empty values without a snapshot, so
   it is planned as a documented no-op.
2. **Where to resolve the spec task.** The design puts the lookup inside
   `persistQueuedPrompt` (fires only when the caller passed an empty `specTaskID`), so
   every present and future caller benefits, rather than only patching the one handler
   at `session_handlers.go:2571`. This costs one extra indexed query per API-path enqueue.
   Assumed fine; say if you want it confined to the handler instead.
3. **Multiple spec tasks per planning session.** `planning_session_id` is assumed
   effectively unique across `spec_tasks`. If it is not, the design takes the single
   result and logs a warning when more than one matches. Confirm there is no legitimate
   fan-out case.
4. **Retry-count reset on defer.** The chosen fix reverts a busy-deferred prompt to
   `pending` and clears `next_retry_at`/`error_message` while leaving `retry_count`
   untouched. Assumed correct — a defer should neither charge nor forgive the budget.
   Alternative would be zeroing `retry_count` on a clean defer; not chosen because it
   would let a genuinely failing prompt launder its history through one defer.
