# Requirements: Make Spec-Task Prompt Queue Org-Global

## Background

A spec-task's prompt queue is currently filtered by **prompt ownership**: the store's
`ListPromptHistory` hard-filters `WHERE user_id = <viewer>`. But prompts are stamped with
`UserID = session.Owner`, and spec-task sessions dispatched by HelixOS are owned by a single
service account (`helix-os-v2`, Kai `109…`), not the human clicking in the UI (luke `101…`).
Result: a human who typed review comments cannot see them after refresh — the queue looks empty
because his own comments are filed under the service account.

The product requirement is that the queue is **org-global**: everyone in an org who is authorized
to view a task must see **all** prompts in that task's queue, regardless of which user or service
account authored each one. This is not "world-readable" — a user with no access to the project
must still get 403.

Two secondary bugs surfaced in the same incident (see below).

Verified against the codebase:
- `api/pkg/store/store_prompt_history.go:517` — `ListPromptHistory` starts with `.Where("user_id = ?", userID)`.
- `api/pkg/server/prompt_history_handlers.go:634` — handler calls `Store.ListPromptHistory(ctx, user.ID, listReq)` with **no** authorization check on the task/session.
- `api/pkg/server/prompt_history_handlers.go:437` — `persistQueuedPrompt` stamps `UserID: session.Owner`, `NotifyUserID: notifyUserID`.
- `api/pkg/server/spec_task_design_review_handlers.go:108-124` — the correct, already-shipped auth pattern (`GetSpecTask` → creator bypass → `authorizeUserToProjectByID(..., ActionGet)`).
- `api/pkg/services/agent_instruction_service.go:596-601` — approval/implementation kickoff enqueued with `interrupt=false` (bug b).

## User Stories

### Story 1 — Org member sees the whole queue (primary)
As an **org member authorized to view a spec task**, I want to see every prompt in that task's
queue (review comments, service-account messages, kickoff prompts) regardless of who authored them,
so that the queue reflects the real conversation instead of appearing empty.

**Acceptance Criteria**
- Given a spec task whose session is owned by user/service-account A, and a review comment enqueued
  under A, when authorized org member B (≠ session owner, ≠ prompt owner) opens the queue, then B
  sees that comment.
- Given a user C with **no** access to the task's project/org, when C requests the queue, then the
  API returns **403** and no prompts.
- Given the `session_id`-scoped path (org-chat / bot session with no spec task), the same
  authorization applies: authorized session/project members see all prompts; unauthorized users get 403.
- The `spec_task_id` path authorizes against `specTask.ProjectID`; the creator always has access.
- The store no longer filters visibility by `user_id`. Scope remains `spec_task_id` / `session_id`.

### Story 2 — Author is shown per prompt (primary)
As a viewer, I want each prompt in the queue to display **who authored it** (name/email/avatar,
with a "system"/service label for the service account), so that in a global queue I can tell my
own comments from others' and from machine-generated prompts.

**Acceptance Criteria**
- Each queue entry resolves `user_id` → a human-readable author label and avatar.
- The service account renders as a clear "system"/service label, not a blank or confusing id.
- Author display is consistent with how review-comment authorship is shown elsewhere.
- The frontend uses the generated API client (no raw fetch), per repo rules.

### Story 3 (secondary, bug b) — Implementation kickoff always reaches the agent
As a user approving a spec, I want the "Begin Implementation" kickoff to reliably reach the agent
even while review-comment interrupts are streaming in, so that approval is never silently abandoned.

**Acceptance Criteria**
- Approving a spec while the agent is mid-turn on an interrupt still results in the implementation
  prompt (content starting `## CURRENT PHASE: IMPLEMENTATION`) being delivered — an interaction with
  that content reaches `waiting`.
- The phase-transition kickoff is not starved to `failed` past `defaultMaxPromptQueueRetries` (20)
  while the session is alive; it is root-caused, not papered over.

### Story 4 (secondary, bug c) — No cross-task contamination / no wedged `sending` prompts
As a user, I want a comment about task X to land only in task X's session queue, and I want prompts
never to wedge the queue if they get stuck in `sending`.

**Acceptance Criteria**
- A comment/prompt is routed to the correct session/spec-task; a prompt for task 002314 must not
  appear in task 002322's session queue.
- Prompts orphaned in `sending` (never transition) are reaped/handled so they don't hold
  `queue_position` indefinitely.

## Non-Goals
- Fixing HelixOS to impersonate the real human (separate repo `helixos`, out of scope). It is only
  the *reason* the queue must be global.
- Changing pin/tag ownership semantics (those still verify `prompt.UserID == user.ID`); only queue
  **visibility** becomes org-global.

## Open Questions
1. **Commit split & PR:** Primary fix (1) and secondary (b)/(c) must be separate commits so (1) can
   merge independently. Confirm you also want (b) and (c) attempted end-to-end in this same PR, or
   just (1) with (b)/(c) documented/flagged if they prove risky.
2. **Bug (b) approach:** Two options — (i) enqueue the kickoff as `interrupt=true` so it preempts, or
   (ii) exempt phase-transition prompts from the retry cap / give them idle-drain priority. Preference?
   Option (i) is simpler but changes preemption semantics; (ii) is more surgical. Recommend (ii) unless
   you want the kickoff to jump ahead of pending review comments.
3. **Bug (c) root cause:** The brief hypothesizes a stale/misrouted session id in the frontend
   optimistic send (`RobustPromptInput` / `usePromptHistory` / `promptHistoryService`). Confirm we
   should chase and fix the routing bug, not just add the orphaned-`sending` reaper.
4. **Author resolution source:** Is there an existing user-lookup/resolution endpoint or hook the
   queue should reuse for `user_id` → name/email/avatar, or should author fields be denormalized onto
   the prompt-history response server-side? (Prefer server-side enrichment to avoid N lookups.)
5. The primary fix keeps the `userID` parameter on `ListPromptHistory` for signature stability but
   stops using it to filter. Confirm that's acceptable vs. removing the parameter and updating callers.
