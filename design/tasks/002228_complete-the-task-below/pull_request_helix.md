# Unify all agent message sending on the session-scoped prompt queue

## Summary

There were two parallel ways to send a `chat_message` to a Zed/ACP agent. Only
one — the prompt-history queue — carried the busy/idle discipline (`interrupt=false`
= defer until idle, `interrupt=true` = cancel-then-send, plus the thread-boot
barrier and the PR https://github.com/helixml/helix/pull/2808 orphaned-waiting
reap). The other — the direct `sendChatMessageToExternalAgent` path and its
wrappers — dispatched immediately with **no busy-check**, so `interrupt=false`
meant "send now anyway", firing a concurrent `session/prompt` mid-turn and
stranding an empty second `waiting` interaction. That was the CI-notification
incident (see `design/tasks/002228_.../attachments/2026-07-06-ci-notification-concurrent-turn-mid-turn.md`).

Every automated sender (CI, post-merge push/rebase, approval), every reactive
interrupt sender (design-review comment/submit, org transition, reviewer
revision), **and** the general session-send endpoint used by org bots rode that
direct path. Org bots in particular send `interrupt=false` and their per-Worker
activations serialise only at *dispatch*, not per *turn*, so a Worker's next
activation could fire while its previous turn was still streaming — the same
concurrent-mid-turn failure. This is the likely-systemic cause behind "bots are
unreliable".

This PR makes the **session-scoped prompt queue the single way to message an
agent** and deletes the duplicate.

## Changes

- **Queue is now session-scoped.** `prompt_history_entries.spec_task_id` is
  nullable; delivery keys on `session_id`. Extracted
  `processPendingPromptsForSession(sessionID)` from the spec-task poller so both
  the spec-task path and general/bot sends share the same busy-defer, boot
  barrier, and desktop-resume reap.
- **Single enqueue entry point** `enqueueAgentMessage(...)` (+ `persistQueuedPrompt`
  / `nudgeSessionQueue`) and a `SpecTaskMessageEnqueuer` callback for
  `pkg/services`. New store primitives `CreatePromptHistoryEntry`,
  `ListPromptHistoryBySession`, `GetCommentByPromptID`.
- **Migrated every sender** onto the queue:
  - CI results → `interrupt=true` (surface pass/fail immediately; **no
    coalescing**, per review).
  - post-merge push / rebase / approval → `interrupt=false` (defer until idle).
  - design-review comment / submit, org transition, reviewer revision →
    `interrupt=true`.
  - `POST /sessions/{id}/messages` (org bots) → enqueue; bots inherit busy-defer
    with **no org-runtime change**.
- **Design-review comment responses** keep working via a dispatch-time backfill:
  the comment stores `prompt_id`, and `sendQueuedPromptToSession` backfills
  `request_id`/`interaction_id` from it — so the existing finalize / streaming /
  timeout / reconcile machinery is untouched. A placeholder `request_id` is set
  at enqueue to preserve the comment queue's in-flight guard.
- **Deleted dead/duplicate code:** the direct wrappers `sendMessageToSession` /
  `sendMessageToSpecTaskAgent`; `MessageSenderCINotifier`; the unused
  `SendImplementationReviewRequest` / `SendRevisionInstruction` /
  `SendMergeInstruction` + `AgentInstructionService.sendMessage` and their
  now-unused prompt builders/templates; the `messageSender` field.
  `sendChatMessageToExternalAgent` is retained ONLY as the low-level primitive
  the cross-repo Zed WS-sync e2e harness drives (production is now unreachable
  from it) and documented as such.
- **API contract:** `POST /sessions/{id}/messages` is now async and returns
  `{ prompt_id }` instead of `{ request_id, interaction_id }`. Regenerated the
  OpenAPI/TypeScript client.

## Testing

- `CGO_ENABLED=0 go build ./...` clean.
- Unit tests (gomock): enqueue row correctness; `processPendingPromptsForSession`
  idle→dispatch, busy→defer, interrupt busy+established→interrupt,
  busy+not-established→defer (boot barrier); updated the session-message and
  websocket-sync suites for the new async contract. `go test ./pkg/server/`
  green; `./pkg/services/` green (one pre-existing env-only git-identity test
  failure, unrelated — fails identically on `origin/main`).
- Live E2E in the inner Helix: see PR discussion / design doc.

## Also in this PR

- **Org-chat queue visibility:** the queue is now session-scoped, so
  `POST /prompt-history` list/sync accept a `session_id` (not just `spec_task_id`).
  A read-only `SessionPromptQueue` strip in the org-bot chat shows what's queued
  for the agent. (Kept `usePromptHistory`/`RobustPromptInput` untouched to avoid
  regressing the spec-task queue.)
- **zed WebSocket-sync e2e — production-path coverage:** added a real in-memory
  prompt queue to `memorystore` (+ fixed `ListInteractions` to honor `Order desc`,
  which the busy-check relies on) and an `EnqueueQueuedPrompt` test helper, so the
  cross-repo Zed e2e can drive the production queue path. Two new phases
  (queue busy-defer, queue interrupt) run green against a real Zed. `sandbox-versions.txt`
  `ZED_COMMIT` is bumped to the companion zed commit.

Companion zed PR: adds the two e2e phases (see pull_request_zed.md).

Composes with https://github.com/helixml/helix/pull/2808.
