# Make the spec-task prompt queue org-global (+ two queue-reliability fixes)

## Summary

The spec-task prompt queue was filtered by **prompt ownership**
(`Store.ListPromptHistory` did `WHERE user_id = <viewer>`), but prompts are stamped with
`session.Owner`. For spec-task sessions dispatched by a service account (e.g. HelixOS's shared
`HELIX_API_KEY`), the session owner is the service account — not the human clicking in the UI — so a
user who typed review comments saw an **empty queue** after refresh (their comments were filed under
the service account).

This makes queue visibility **org-global**: scoped by `spec_task_id` / `session_id` and authorized by
project/org access, not by prompt ownership. Any org member authorized to view the task sees **all**
prompts in its queue, whoever authored them — while a non-member still gets 403 (org-wide, not
world-readable).

Three independent commits so the primary fix can merge even if the follow-ups need more discussion.

## Commit 1 — Org-global queue (primary)

- **Store** (`store_prompt_history.go`): drop the `user_id = viewer` filter in `ListPromptHistory`;
  keep the `spec_task_id` / `session_id` scope. Fail closed — reject an unscoped call so no caller can
  list the whole table.
- **Handler** (`prompt_history_handlers.go` `listPromptHistory`): authorize before returning prompts,
  mirroring `spec_task_design_review_handlers.go`. `spec_task_id` path → `GetSpecTask` + creator
  bypass + `authorizeUserToProjectByID(..., ActionGet)`; `session_id` path → `GetSession` +
  `authorizeUserToSession(..., ActionGet)`. Both 403 for the unauthorized.
- **Author display**: a prompt can now be authored by someone other than the viewer, so the list
  response resolves each `user_id` to a `PromptAuthor` (name/email, `is_system` for service/unknown
  owners), batched to avoid N+1. Both queue views (`SessionPromptQueue.tsx`,
  `RobustPromptInput.tsx`) render the author via the generated API client.
- Tests: fail-closed 403 on both scope paths + 400 on missing scope.

## Commit 2 — Reliable implementation kickoff (bug b)

The "Begin Implementation" phase-transition prompt was enqueued as a non-interrupt queue prompt, so
it required an idle session. Under a stream of interrupt review comments the session was never idle,
so the kickoff lost every race ("session became busy, deferring queue prompt"), was marked failed,
retried on backoff, and past the retry cap was silently abandoned — leaving the task stuck in
`implementation` with the agent never told to start. It is now enqueued as an **interrupt** (cancel
current turn, respect the boot barrier), matching the sibling "request changes" control signal.
Interrupts don't require idle, so the kickoff is never starved. Test asserts `interrupt=true`.

## Commit 3 — Cross-task contamination + orphaned sends (bug c)

- **Routing** (`usePromptHistory.ts`): the debounced sync filed unsynced entries under the hook's
  *current* `specTaskId`/`projectId`, while each entry keeps its own `sessionId`. A task switch
  mid-sync produced a row whose `session_id` and `spec_task_id` disagreed — a comment for one task
  landing in another's queue. Now only entries whose `sessionId` matches the current view are synced,
  so every row is internally consistent.
- **Orphan reaper** (`store_prompt_history.go`): a prompt stuck in `sending` that was never delivered
  is never re-selected, so it showed as perpetually in-flight and wedged the queue. Extend
  `ReconcileStuckSendingPrompts` with a conservative third path — an old `sending` prompt with no
  linked interaction and no session activity after it is flipped to `failed` (retryable), so it
  recovers or surfaces instead of hanging. Validated against live Postgres.

## Testing

- Go builds; unit tests pass (authz 403/400, approval-kickoff interrupt).
- Frontend `tsc --noEmit` clean; generated API client regenerated (`./stack update_openapi`).
- **Primary fix verified live** in the inner Helix against the real endpoint: an authorized org
  member who is NOT the prompt owner sees a service-account-owned prompt (200); a non-member gets 403;
  an org-shared-project member sees it (200). Author renders as "System" for the service account.
- Bug (c) reaper validated against live Postgres (orphan → failed; live/recent/linked controls
  untouched).
- Not driven live: the full mid-turn approval race for (b) (needs a provisioned agent under
  concurrent interrupts) and the (c) routing race (timing-dependent) — both covered by unit test /
  typecheck and flagged in the task's design.md.
