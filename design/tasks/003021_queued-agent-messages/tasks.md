# Implementation Tasks: Make API-Queued Agent Messages Visible in the Prompt Queue

## Primary fix — stamp `spec_task_id`

- [ ] Add `resolveSpecTaskIDForSession(ctx, sessionID) string` to `api/pkg/server/prompt_history_handlers.go`, using `Store.ListSpecTasks` with `PlanningSessionID` filter, `Limit: 2`, `IncludeArchived: true`; return `""` on zero results or on error (log, never fail the enqueue); `log.Warn()` if more than one match
- [ ] In `persistQueuedPrompt`, call the helper only when the passed `specTaskID` is empty, so the two existing callers keep their explicit `task.ID`
- [ ] Confirm `session_handlers.go:2571` now produces a stamped row without changing its call signature
- [ ] Add Go unit tests: session owned by a spec task → id stamped; plain session with no spec task → empty `spec_task_id` and no error; store lookup error → empty `spec_task_id` and enqueue still succeeds

## Secondary bug 1 — soft-deletes

- [ ] Add `deleted_at IS NULL` to the base query in `ListPromptHistory` (`api/pkg/store/store_prompt_history.go:509`), before the `Count` so `total` is also correct
- [ ] Add a unit test asserting a soft-deleted entry is excluded from both `Entries` and `Total`

## Secondary bug 2 — busy-defer must not burn the retry budget

- [ ] Read the comment block at `websocket_external_agent_sync.go:3745-3800` before editing — it encodes the interrupt exemption, the ASC/DESC ordering incident, and the boot-race incident
- [ ] Add the `errPromptBusyDeferred` sentinel and wrap both "became busy … deferring" returns in `sendQueuedPromptToSession` with `%w`, leaving the message text unchanged
- [ ] Add `RevertPromptToPending(ctx, promptID)` to the store: sets `status='pending'`, `next_retry_at=NULL`, `error_message=''`, leaves `retry_count` untouched
- [ ] Add `RevertPromptToPending` to the `Store` interface and regenerate the gomock mocks
- [ ] Branch on `errors.Is(err, errPromptBusyDeferred)` in `processPromptQueue`, `processInterruptPrompt`, and `processAnyPendingPrompt` — call `RevertPromptToPending` and log at Info instead of `MarkPromptAsFailed`
- [ ] Verify `GetNextPendingPrompt` / `GetAnyPendingPrompt` re-select a `pending` row with `next_retry_at IS NULL` immediately
- [ ] Confirm option (a) (a busy pre-check in `processAnyPendingPrompt`) is NOT also implemented — one path only
- [ ] Add unit tests: busy-defer leaves `retry_count` unchanged and status `pending`; a genuine dispatch error still increments `retry_count` and sets backoff; the cap at 20 still applies to genuine failures

## Backfill

- [ ] Add `api/pkg/store/migrations/0010_backfill_prompt_history_spec_task_id.up.sql` with the idempotent `UPDATE … FROM spec_tasks` joining `session_id = planning_session_id`, guarded by `(spec_task_id IS NULL OR spec_task_id = '') AND deleted_at IS NULL`
- [ ] Add the matching `.down.sql` as a documented no-op
- [ ] Count affected rows before and after on the target DB; state in the PR whether the backfill ran and how many rows it touched

## Live verification in the inner Helix (the deliverable)

- [ ] Bring up `localhost:8080`, register/log in as `test@helix.ml` / `helixtest`, complete onboarding, create a spec task, and wait for a LIVE Zed session (`config->>'zed_thread_id'` is a non-empty UUID)
- [ ] With the agent MID-TURN, `POST /api/v1/sessions/{session_id}/messages` with `interrupt: false` — the exact call HelixOS makes
- [ ] Open the spec-task page and confirm the queued message is visible in the prompt-queue UI as pending/waiting
- [ ] Save the screenshot to `screenshots/` in this task folder — this is the deliverable
- [ ] Confirm the message dispatches when the turn completes and the content the agent receives is unchanged
- [ ] With the agent mid-turn, trigger the cancel-ack path repeatedly and assert `retry_count` on the queued prompt does not climb
- [ ] Confirm a plain session with no spec task still enqueues and dispatches
- [ ] Confirm soft-deleting a queued prompt removes it from the UI and it does not reappear

## Wrap-up

- [ ] Run `go build ./...`, `go vet ./...`, and the affected package tests
- [ ] Write the PR description: which of the two option (a)/(b) fixes was chosen and why, and whether the backfill ran
