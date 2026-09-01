# Make API-queued agent messages visible in the prompt queue

## Summary

Messages queued through `POST /api/v1/sessions/{id}/messages` — the call HelixOS makes on
every card approval — were written with an empty `spec_task_id`, because the generic
session-messages handler had no spec task to hand and passed `""`. The prompt-queue UI
queries *by* `spec_task_id`, so those rows were filtered out: 53 approvals were correctly
queued, correctly `pending`, and completely invisible. The queue was working; only its
visibility was broken.

`persistQueuedPrompt` now resolves the owning spec task itself (via the existing
`ListSpecTasks` `PlanningSessionID` filter) when the caller didn't supply one. The two
callers that already pass `task.ID` are unchanged, and a session with no spec task still
enqueues and dispatches with an empty `spec_task_id` — that is legitimate there.

Three related defects in the same path are fixed alongside it.

**Busy-defers no longer burn the retry budget (silent data loss).** `processAnyPendingPrompt`
claims the head-of-queue prompt with no busy pre-check; `sendQueuedPromptToSession` then
rejects it as busy and the caller called `MarkPromptAsFailed`, incrementing `retry_count`.
That drain fires on *every acknowledged cancellation* — every time a user interrupts the
agent — and on the live session one prompt reached `retry_count = 11` in 16 minutes purely
from defers. At the cap of 20 the `retry_count < ?` predicate stops selecting the row and
the message is dropped forever, with nothing surfaced to the user.

Of the two options in the brief this takes **(b)**, distinguishing the defer from a real
error, not (a) a busy pre-check in `processAnyPendingPrompt`. Reasons: that drain also
claims *interrupt* prompts, which are deliberately exempt from the busy check, so a blanket
pre-check would break the interrupt path and a conditional one would need to inspect the
prompt before the atomic claim; and (b) additionally covers the boot-race case where an
interrupt is deferred because `ZedThreadID` is not yet set, which (a) never reaches. A new
`ErrPromptBusyDeferred` sentinel wraps the existing busy return, and a single shared
`requeueUndispatchedPrompt` helper makes the defer-vs-failure decision for all three drain
paths, so it cannot drift between them. Deferred prompts go back to `pending` with
`retry_count` untouched; genuine failures still increment it and still respect the cap. The
comment block encoding the past interrupt/boot-race incidents is untouched — only the error
*value* changed, not the control flow.

**The queue now shows every prompt waiting for the agent, not just the viewer's own.** The
queue belongs to the agent, not to whoever typed the message: a teammate's or a bot's
queued prompt is going to run on the session you are looking at, so hiding it is the same
class of lie as the original bug. `ListPromptHistory` and the sync response are now scoped
by spec task / session instead of `user_id`.

That predicate *was* the authorization on those endpoints — there was no other check — so
it is replaced with a real one: `authorizeUserToPromptQueue` resolves the spec task's
project and calls `authorizeUserToProject`, or `authorizeUserToSession` for session-scoped
queues, before any rows are returned. The store additionally refuses to run a request that
carries no scope at all. Writes stay owner-attributed: `SyncPromptHistory` skips updates to
rows owned by someone else, and the UI hides the edit / interrupt-toggle / delete
affordances on foreign entries (`deletePromptHistoryEntry`'s owner-only 403 is unchanged).
Entries carry an owner avatar with a "Queued by …" hover label, shown only when the queue
actually holds more than one distinct owner so the common single-user case is unchanged.

**`ListPromptHistory` ignored soft-deletes.** It had no `deleted_at IS NULL` predicate,
unlike `ListPromptHistoryBySpecTask` and `ListPromptHistoryBySession`, so deleted prompts
could reappear in the queue. Added, before the count so `total` agrees.

**Backfill.** `0010_backfill_prompt_history_spec_task_id` stamps `spec_task_id` on existing
rows where `session_id` matches a spec task's `planning_session_id`. It is idempotent (the
guard stops matching once stamped), never overwrites a non-empty value, and follows the
`0006_backfill_spec_task_assignees` precedent including the `to_regclass`/column guards. The
`down` is a documented no-op — the previous values were empty strings written by the bug.
**It has not been run against any live database from this branch**, so no row count is
claimed; it will run on deploy.

## Testing

**The required live browser verification was NOT performed, and this change is unverified
at runtime.**

The session's startup script failed before the stack came up: it builds Zed from source and
`rustc` was OOM-killed (`signal: 9, SIGKILL`) compiling the `project` crate, so the script
exited 1 with `❌ Error: Failed to build Zed IDE` and started no containers. I brought up
`postgres`, `api` and `frontend` by hand, but the test plan needs a LIVE Zed session
(`config->>'zed_thread_id'` a non-empty UUID), which requires the desktop image that depends
on that Zed binary. The task was then explicitly descoped to "get it merged, don't worry
about testing".

What was actually run:

- `go build ./...` — passes, exit 0.
- Frontend `tsc --noEmit -p tsconfig.json` (TypeScript 5.9.3) — passes, exit 0, zero
  diagnostics. Run by mounting the source into the `helix-frontend` image, since
  `frontend/node_modules` is not installed on this machine.

What was **not** run, and should be before this is trusted in production:

- The whole end-to-end deliverable: send a message mid-turn via the session-messages API
  and confirm it appears in the spec-task prompt queue. **There is no screenshot.**
- That the message still dispatches unchanged when the turn completes.
- That `retry_count` stays flat across repeated cancel-acks.
- That a plain session with no spec task still enqueues and dispatches.
- **The read-hole check** — that an unauthorized account gets 403 for another project's
  `spec_task_id`. This is the highest-risk gap: de-scoping the read and adding the authz
  check landed in the same commit, but only the code was reviewed, not its behaviour.
- Go unit tests for the session→spec-task resolution, the soft-delete exclusion, the
  retry-count behaviour, and the new authorization; and a frontend test for the owner
  indicator. None were written.
