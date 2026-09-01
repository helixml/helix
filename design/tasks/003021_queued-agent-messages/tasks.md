# Implementation Tasks: Make API-Queued Agent Messages Visible in the Prompt Queue

## Primary fix — stamp `spec_task_id`

- [~] Add `resolveSpecTaskIDForSession(ctx, sessionID) string` to `api/pkg/server/prompt_history_handlers.go`, using `Store.ListSpecTasks` with `PlanningSessionID` filter, `Limit: 2`, `IncludeArchived: true`; return `""` on zero results or on error (log, never fail the enqueue); `log.Warn()` if more than one match
- [~] In `persistQueuedPrompt`, call the helper only when the passed `specTaskID` is empty, so the two existing callers keep their explicit `task.ID`
- [~] Confirm `session_handlers.go:2571` now produces a stamped row without changing its call signature
- [~] Add Go unit tests: session owned by a spec task → id stamped; plain session with no spec task → empty `spec_task_id` and no error; store lookup error → empty `spec_task_id` and enqueue still succeeds

## Secondary bug 1 — soft-deletes

- [x] Add `deleted_at IS NULL` to the base query in `ListPromptHistory` (`api/pkg/store/store_prompt_history.go:509`), before the `Count` so `total` is also correct
- [ ] NOT DONE — unit test for soft-delete exclusion. Skipped per user direction

## Secondary bug 2 — busy-defer must not burn the retry budget

- [x] Read the comment block at `websocket_external_agent_sync.go:3745-3800` before editing — it encodes the interrupt exemption, the ASC/DESC ordering incident, and the boot-race incident
- [x] Add the `errPromptBusyDeferred` sentinel and wrap both "became busy … deferring" returns in `sendQueuedPromptToSession` with `%w`, leaving the message text unchanged
- [x] Add `RevertPromptToPending(ctx, promptID)` to the store: sets `status='pending'`, `next_retry_at=NULL`, `error_message=''`, leaves `retry_count` untouched
- [x] Add `RevertPromptToPending` to the `Store` interface and regenerate the gomock mocks
- [x] Branch on `errors.Is(err, errPromptBusyDeferred)` in `processPromptQueue`, `processInterruptPrompt`, and `processAnyPendingPrompt` — call `RevertPromptToPending` and log at Info instead of `MarkPromptAsFailed`
- [x] Verify `GetNextPendingPrompt` / `GetAnyPendingPrompt` re-select a `pending` row with `next_retry_at IS NULL` immediately
- [x] Confirm option (a) (a busy pre-check in `processAnyPendingPrompt`) is NOT also implemented — one path only
- [ ] NOT DONE — unit tests for busy-defer retry-count behaviour. Skipped per user direction

## Show the whole agent queue, not just the viewer's own prompts

- [x] Add `UserID` to `types.PromptHistoryListRequest` (server-set only, json `-`) and change the store signature to `ListPromptHistory(ctx, req)`, applying the user filter only when non-empty
- [x] Add a store guard: error out if `SpecTaskID`, `SessionID` and `UserID` are all empty, so the query can never run unscoped
- [x] Update the `Store` interface (`store.go:877`), regenerate `store_mocks.go`, and update `memorystore`
- [x] In the `listPromptHistory` handler, authorize before querying: `spec_task_id` → GetSpecTask → GetProject → `authorizeUserToProject(…, types.ActionGet)`; `session_id` → GetSession → `authorizeUserToSession(…, types.ActionGet)`; then query with no user filter
- [ ] Make the failure path narrow: if the task/project/session lookup fails, fall back to owner-scoped results, never to a wider set
- [x] Apply the same authz check to the `syncPromptHistory` handler
- [x] Drop `user_id` from `SyncPromptHistory`'s trailing "return all entries" query so list and sync are scoped identically (leave the create branch stamping `UserID: userID`)
- [x] Add the foreign-row guard to `SyncPromptHistory`'s update branch: skip when `existingEntry.UserID != userID`
- [x] Leave `deletePromptHistoryEntry`'s owner-only 403 unchanged
- [x] Add `userId` to the frontend `PromptHistoryEntry` type and map it in `backendToLocal`
- [x] ~~Exclude foreign-owned entries from the sync push payload~~ — **not needed**: backend entries are already marked `syncedToBackend: true` and the push filters on `!syncedToBackend`. The one leak (interrupt toggle) is closed by hiding the affordance. See design.md note 4
- [x] Render `OrganizationUserAvatar` per entry when the queue has more than one distinct owner (suppressed for the single-owner case), with the display name on hover — **in `RobustPromptInput.tsx`/`SortableQueueItem`, NOT `SessionPromptQueue.tsx`** (that one is for sessions with no spec task; see design.md note 1)
- [x] Hide the delete affordance on entries the current user does not own
- [ ] NOT DONE — Go tests for the new authz + de-scoped read. Skipped per user direction. **This is the highest-value gap: the read-hole check is untested**
- [ ] NOT DONE — frontend test for the owner indicator. Skipped per user direction

## Backfill

- [x] Add `api/pkg/store/migrations/0010_backfill_prompt_history_spec_task_id.up.sql` with the idempotent `UPDATE … FROM spec_tasks` joining `session_id = planning_session_id`, guarded by `(spec_task_id IS NULL OR spec_task_id = '') AND deleted_at IS NULL`
- [x] Add the matching `.down.sql` as a documented no-op
- [x] Stated in the PR: the migration is committed and will run on deploy; it has NOT been run against any live DB from here (no stack), so no row count is claimed

## Live verification in the inner Helix — NOT DONE (stack unavailable, then descoped by user)

The session startup script failed: the Zed build was OOM-killed, so no containers came
up. `postgres`/`api`/`frontend` were started by hand but a LIVE Zed session was not
reachable. The user then directed merging without testing. See design.md.


- [ ] Bring up `localhost:8080`, register/log in as `test@helix.ml` / `helixtest`, complete onboarding, create a spec task, and wait for a LIVE Zed session (`config->>'zed_thread_id'` is a non-empty UUID)
- [ ] With the agent MID-TURN, `POST /api/v1/sessions/{session_id}/messages` with `interrupt: false` — the exact call HelixOS makes
- [ ] Open the spec-task page and confirm the queued message is visible in the prompt-queue UI as pending/waiting
- [ ] Save the screenshot to `screenshots/` in this task folder — this is the deliverable
- [ ] Confirm the message dispatches when the turn completes and the content the agent receives is unchanged
- [ ] With the agent mid-turn, trigger the cancel-ack path repeatedly and assert `retry_count` on the queued prompt does not climb
- [ ] Confirm a plain session with no spec task still enqueues and dispatches
- [ ] Confirm soft-deleting a queued prompt removes it from the UI and it does not reappear
- [ ] Create a second account in the same org/project, queue a prompt from it on the same spec task, and confirm both accounts see both prompts with identity indicators — screenshot from each
- [ ] Confirm the single-owner case shows no avatars (no noise regression)
- [ ] With a third account that has no access to the project, confirm `GET /api/v1/prompt-history?spec_task_id=…` returns 403/empty — this is the read-hole gate
- [ ] Confirm the second account's client does not clobber or delete the first account's queued prompt (no delete button; sync round trip leaves the row unchanged)

## Wrap-up

- [x] `CGO_ENABLED=0 go build ./...` exit 0; frontend `tsc --noEmit` exit 0, zero diagnostics; `go test ./pkg/server/ ./pkg/types/ ./pkg/store/memorystore/` all pass. (`gcc`+`libc6-dev` must be installed for the CGo/tree-sitter test build; with CGo on, `go build ./...` then needs `pkg-config` for the GStreamer bindings — pre-existing, unrelated)
- [x] Write the PR description: which of the two option (a)/(b) fixes was chosen and why, and whether the backfill ran
