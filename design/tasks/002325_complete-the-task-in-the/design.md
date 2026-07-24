# Design: Make Spec-Task Prompt Queue Org-Global

## Overview

Move prompt-queue visibility from **ownership-based** (`WHERE user_id = viewer`) to
**scope-based + authorization-based**: the queue is scoped by `spec_task_id` / `session_id`
(already applied), and the *handler* authorizes the viewer against the task's project/org before
returning any rows. This mirrors the already-shipped auth in the sibling design-review handler.

Three commits, clearly separated so the primary can merge independently:

- **Commit 1 (primary):** org-global queue — store filter drop + handler authorization + frontend author display.
- **Commit 2 (bug b):** reliable implementation-kickoff delivery.
- **Commit 3 (bug c):** cross-task routing fix + orphaned-`sending` handling.

## Commit 1 — Org-global queue (primary)

### 1a. Store: stop filtering by owner
`api/pkg/store/store_prompt_history.go` — `ListPromptHistory` (line ~517).
- Remove the leading `.Where("user_id = ?", userID)`. Keep `spec_task_id` / `project_id` /
  `session_id` / `since` filters — those define the scope.
- Keep the `userID` parameter in the signature for stability (or remove and update callers — see
  Open Question 5); do **not** use it to filter. Add a comment explaining the queue is now
  scope-authorized, not owner-filtered.
- Callers: only the handler at `prompt_history_handlers.go:634` passes a real viewer id; the store
  interface (`store.go:853`) and mocks (`store_mocks.go`) stay compatible if the param is retained.

**Fail-closed guard:** because the store no longer filters by user, it MUST reject a call with an
empty scope (no `spec_task_id` and no `session_id`) rather than returning the entire table. The
handler already 400s on empty scope (`prompt_history_handlers.go:608`); add a defensive check in the
store too so a future caller can't accidentally list everything.

### 1b. Handler: authorize by task/session
`api/pkg/server/prompt_history_handlers.go` — `listPromptHistory` (~line 605-650).
Before calling the store, authorize the viewer for the requested scope. Two paths:

- **`spec_task_id` path** — copy the proven pattern from
  `spec_task_design_review_handlers.go:108-124`:
  ```go
  specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
  if err != nil { /* 404/500 */ }
  if user.ID != specTask.CreatedBy {
      if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionGet); err != nil {
          http.Error(w, "Not authorized", http.StatusForbidden); return
      }
  }
  ```
- **`session_id` path** — do NOT leave unauthenticated. Load the session and authorize with the
  existing `authorizeUserToSession(ctx, user, session, types.ActionGet)` helper
  (`external_agent_handlers.go` uses it extensively; `authz.go` defines the project/resource helpers).
  This covers owner + org/project access and fails closed.

If a `project_id` filter is also supplied, it's an additional narrowing filter, not an auth
substitute — auth is always by task or session.

### 1c. Frontend: display prompt author
Prompts can now be authored by someone other than the viewer, so the queue must show who sent each.
- Component: `frontend/src/components/session/SessionPromptQueue.tsx` (renders `e.interrupt` etc.).
  Add an author label/avatar per entry, resolved from the prompt's `user_id`.
- **Preferred:** enrich the prompt-history list response server-side with author display fields
  (name/email/avatar + an `isSystem`/service flag) so the frontend needs no extra lookups. If added,
  regenerate the API client (`./stack update_openapi`) and consume via the generated client — no raw
  fetch (CLAUDE.md:182).
- Render the service account (`helix-os-v2`, owner `109…`) as a clear "system"/service label, matching
  how review-comment authorship is displayed elsewhere.

## Commit 2 — Bug (b): reliable implementation-kickoff delivery

Root cause: the approval/implementation kickoff (content `## CURRENT PHASE: IMPLEMENTATION`,
`agent_instruction_service.go:596-601`) is enqueued `interrupt=false`. Review comments are
`interrupt=true` and jump the queue, so under a stream of comments the session is never idle, the
kickoff loses every race ("session … became busy … deferring queue prompt",
`websocket_external_agent_sync.go:~3396`), is marked `failed`, retries on backoff, and past
`defaultMaxPromptQueueRetries = 20` (`store_prompt_history.go:24`) `GetNextPendingPrompt` stops
selecting it — the phase transition is silently abandoned.

The kickoff is a **control signal that must be delivered**, not an ordinary chat message.
**Recommended fix (option ii):** treat phase-transition prompts as priority — exempt them from the
retry cap and/or give them precedence in the idle drain so they are never abandoned while the session
is alive. Alternative (option i): enqueue as `interrupt=true` so it preempts (simpler, but jumps
ahead of pending review comments — semantic change). Decide per Open Question 2. Whichever is chosen,
add a targeted test proving the kickoff reaches `waiting` under concurrent interrupts.

## Commit 3 — Bug (c): routing fix + orphaned-`sending` handling

Observed: a comment about task 002314 landed in task 002322's session queue, stuck in `sending` at
`queue_position=1`, with a **client-generated optimistic id** (`Date.now()-random`, not a server
`prompt_…` id) and stamped with the *real* user (opposite of the review-comment path).

Two sub-fixes:
- **Routing:** trace the frontend optimistic send (`RobustPromptInput.tsx`,
  `hooks/usePromptHistory.ts`, `services/promptHistoryService.ts`, `utils/optimisticSessionStarting.ts`)
  for a stale/misrouted `sessionId`/`specTaskId` captured across task switches, or an id collision.
  Fix so a prompt is enqueued against the currently-active session/task only.
- **Orphan reaper:** there is already a `sending`-timeout reap in the store (the `NOW() - INTERVAL '5
  minutes'` block near `store_prompt_history.go:470`) and `ResetCrashedPromptsForSession`. Confirm
  orphaned optimistic-id `sending` rows are caught by it; extend/fix if the client-generated-id case
  slips through, so a wedged prompt can't hold `queue_position` indefinitely.

## Key Decisions & Rationale
- **Auth in handler, scope in store** — matches the existing separation for design-review comments;
  keeps store methods dumb/scope-only and centralizes authorization where the `*HelixAPIServer`
  helpers live. Avoids duplicating org/project logic in the store.
- **Reuse `authorizeUserToProjectByID` / `authorizeUserToSession`** — the shipped, tested helpers;
  no new authz surface. Fail-closed by default.
- **Server-side author enrichment over client lookups** — one query, no N+1, and keeps the frontend
  on the generated client.
- **Separate commits** — the brief explicitly requires (1) to be mergeable even if (b)/(c) need more
  discussion.

## Risks / Gotchas
- **Do not open a hole on the `session_id` path.** Closing the per-user filter without adding session
  auth would make org-chat/bot queues world-readable. Negative test required.
- Pin/tag endpoints still check `prompt.UserID == user.ID` — leave them; only list visibility changes.
- Regenerating the API client requires `./stack update_openapi`; frontend hot-reloads (Vite, port 8081).
- Go is fail-fast (CLAUDE.md) — return errors, don't swallow; 403 on unauthorized, 404 on missing task.

## Testing (live, inner Helix at localhost:8080)
- **Story 1:** create a spec task; enqueue a comment owned by a different user_id / service account;
  confirm a *different* authorized org member sees it, and a non-member gets **403**. Prove the exact
  bug is gone: a viewer who is not the session owner sees prompts owned by the session owner.
- **Story 2:** confirm each queue entry shows the correct author and the service account shows a
  system label.
- **Bug b:** approve a spec while the agent is mid-interrupt; confirm an interaction with
  `CURRENT PHASE: IMPLEMENTATION` reaches `waiting`.
- **Bug c:** reproduce/verify no cross-task contamination and no wedged `sending` prompt.
- Report what was actually observed; do not claim untested confidence (CLAUDE.md).

## Implementation Notes (Commit 1 — landed)

- **Store filter drop** (`store_prompt_history.go`): removed `.Where("user_id = ?", userID)`.
  Kept the `userID` parameter for signature/interface stability (the `Store` interface + gomock
  `store_mocks.go` are unchanged) but it is now `_`. Added a fail-closed guard: an unscoped call
  (no `spec_task_id` and no `session_id`) returns an error instead of listing the whole table.
- **Handler authorization** (`prompt_history_handlers.go` `listPromptHistory`): mirrors
  `spec_task_design_review_handlers.go`. spec_task path → `GetSpecTask` + creator bypass +
  `authorizeUserToProjectByID(..., types.ActionGet)`. session path → `GetSession` +
  `authorizeUserToSession(..., types.ActionGet)` (the shipped helper; covers owner + org member +
  org owner + `OrgMembersAccess`). Both fail closed with 403.
- **Author enrichment**: added non-persisted `types.PromptAuthor` (`gorm:"-"`) on
  `PromptHistoryEntry` plus `resolvePromptAuthors` in the handler — batches unique `UserID`s via
  `Store.GetUser` (no N+1). Unresolvable owner or `types.OwnerTypeSystem` → `IsSystem=true`. Note:
  in the real incident the service account is a normal keycloak user (Kai) that owns the
  `helix-os-v2` api key, so it resolves to that human's name/email; a truly system/unresolvable
  owner is what renders as "System". This is honest — the prompt row only carries `session.Owner`,
  not which api key enqueued it.
- **API client**: ran `./stack update_openapi` — `swag` must be on PATH
  (`export PATH="$PATH:$HOME/go/bin"`; the bare `./stack update_openapi` fails with
  `swag: command not found`). Generated `TypesPromptAuthor` + `author?` on
  `TypesPromptHistoryEntry`.
- **Frontend**: shared `utils/promptAuthor.ts#promptAuthorLabel` used by both queue views
  (`SessionPromptQueue.tsx` and `RobustPromptInput.tsx`'s `SortableQueueItem`). Threaded `author`
  through the local `PromptHistoryEntry`/`LocalPromptHistoryEntry` types, `backendToLocal`, and
  `reconcileEntry` (backend value wins so optimistic entries pick up the resolved author on sync).
- **Tests**: `CGO_ENABLED=1` needs `gcc` + `libc6-dev` (tree-sitter). Unit tests cover the
  fail-closed 403s and the 400-no-scope; the positive org-member-sees-others case is deferred to
  live inner-Helix testing because a MockStore returns canned rows and cannot exercise the real SQL
  ownership filter that was removed.
- **Gotcha**: `frontend/yarn build` fails at the very end with `EACCES mkdir dist/external-libs`
  (the `dist` bind mount is container-owned) — this is NOT a code error; all 21712 modules
  transform first. Use `tsc --noEmit` for a clean local type-check instead.

## Implementation Notes (Commit 2 — bug b, landed)

- **Chosen option (i), not (ii).** Planning recommended (ii) (priority column + retry-cap
  exemption). Implementation analysis flipped the decision to (i) — enqueue the kickoff as
  `interrupt=true` — because:
  - The sibling control signal `RequestChanges` already delivers via
    `enqueueSpecTaskAgentMessage(..., interrupt=true)` (`spec_tasks_org_wiring.go:34`). The approval
    kickoff is the same kind of control signal; matching the pattern is consistent and maintainable.
  - Interrupt delivery already "respects the boot barrier" (per `enqueueAgentMessage` doc + the
    2026-06-19 interrupt-during-boot incident fix), so option (i) does not reintroduce boot races.
  - Interrupt prompts are delivered even when the session is busy (they cancel the current turn), so
    they never enter the non-interrupt "busy → defer → fail → retry" loop that was the actual root
    cause of the ~8-min starvation. Option (ii) would have added a schema column + selector changes
    for a problem the interrupt path already solves.
- **Change:** one line at `agent_instruction_service.go:600` (`false`→`true`) + explanatory comment.
- **Test:** `agent_instruction_service_test.go#TestSendApprovalInstruction_EnqueuesAsInterrupt` — a
  task with empty `ProjectID` short-circuits all guideline/repo store lookups, so the method makes
  no store calls; the capturing enqueuer asserts `interrupt=true`, the IMPLEMENTATION prompt content,
  and the approver carried as `notifyUserID`. Passing.
- Note the retry cap (`retry_count < ?`) also exists in `GetNextInterruptPrompt`, so an interrupt
  that genuinely fails 20× is still dropped — but that is the same accepted behavior as every other
  interrupt control signal, and the busy-defer failure mode (the real cause here) no longer applies.

## Implementation Notes (Commit 3 — bug c, landed)

- **Routing root cause (frontend).** `usePromptHistory`'s immediate-sync path is consistent
  (entry + specTaskId captured in the same render), but the debounced `syncToBackend` re-syncs
  `history.filter(!syncedToBackend)` under the hook's *current* `specTaskId`/`projectId`. Each entry
  carries its own `sessionId`. If the user switches tasks while an entry is still unsynced (immediate
  sync offline/failed), the debounced sync files it under the NEW task while keeping the OLD
  `session_id` → the row's `session_id` and `spec_task_id` disagree, i.e. a comment for task A lands
  in task B's session queue. Fix: both sync paths now skip entries whose `sessionId !== sessionId`
  (the current view). `sessionId` and `specTaskId` are a consistent pair per view, so every synced
  row is internally consistent. Minimal + low-risk; tsc clean.
- **Orphan reaper (backend).** `ReconcileStuckSendingPrompts` previously only marked orphaned
  `sending` rows as `sent` (paths 1/2, requiring evidence of processing). A misrouted/never-delivered
  prompt has no such evidence, so it stayed `sending` forever (never re-selected by
  `GetNextPendingPrompt`). New Path 3 flips to `failed` (retryable) when: `status='sending'`,
  `created_at < NOW()-5min`, no interaction links it (`prompt_id`), and no interaction in its session
  exists at/after its creation. Conservative guards avoid racing a genuinely in-flight dispatch.
- **Live validation (real Postgres, helix-postgres-1).** Crafted four rows: `test_orphan` (old,
  no activity) → flipped to `failed` with the message; `test_live` (interaction created after it),
  `test_recent` (<5min old), `test_linked` (interaction with `prompt_id`) all correctly stayed
  `sending`. The deployed scheduled reaper independently flipped `test_orphan` too, confirming the
  new code path is live. Rows cleaned up afterwards.

## Deployment / testing status

- Stack came up mid-implementation (`localhost:8080` → 200, api/frontend/postgres healthy). Air
  hot-reloaded the Go changes (proven by the live reaper flipping `test_orphan`); Vite HMR serves the
  frontend changes.
- Verified: Go builds (`pkg/store`, `pkg/server`, `pkg/services`), frontend `tsc --noEmit` clean,
  Go unit tests pass (authz 403s + no-scope 400; approval-kickoff interrupt), bug (c) reaper
  validated against live Postgres.
- NOT yet run: the full two-user org-global browser e2e (register → onboard → create task → second
  user/service-account comment → different authorized member sees it → non-member 403). This is the
  brief's mandated Story-1 live test and remains to be driven through the inner-Helix UI.
