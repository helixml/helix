# Implementation Tasks: Make Spec-Task Prompt Queue Org-Global

## Commit 1 — Org-global queue (primary)
- [x] Remove the `WHERE user_id = viewer` filter in `ListPromptHistory` (`api/pkg/store/store_prompt_history.go` ~517); keep scope filters; add fail-closed guard rejecting empty scope
- [x] Keep `userID` param for signature stability (do not filter on it); update/verify callers, interface (`store.go:853`), and mocks compile
- [x] Add task/session authorization to `listPromptHistory` (`api/pkg/server/prompt_history_handlers.go` ~605): `spec_task_id` → `GetSpecTask` + creator bypass + `authorizeUserToProjectByID(..., ActionGet)`
- [x] For `session_id` path, load session + `authorizeUserToSession(ctx, user, session, ActionGet)`; fail closed (403), never leave unauthenticated
- [x] Enrich prompt-history list response server-side with author fields (name/email + system flag); regenerated client via `./stack update_openapi`
- [x] Display prompt author per entry in `SessionPromptQueue.tsx` and `RobustPromptInput.tsx` via generated API client (shared `promptAuthorLabel` util); service account renders as "System"
- [x] Go unit test: fail-closed 403 for non-member on both `spec_task_id` and `session_id` paths + 400 on missing scope (passing). Positive "member sees others' prompts" case proven end-to-end (MockStore can't exercise the removed ownership filter)

## Commit 2 — Bug (b): reliable implementation-kickoff delivery
- [x] Root-cause confirmed: kickoff enqueued `interrupt=false` (`agent_instruction_service.go:600`) is starved by interrupt comments and abandoned past retry cap 20
- [x] Chosen option (i): enqueue the kickoff as `interrupt=true` — matches the sibling "request changes" control signal (`spec_tasks_org_wiring.go:34`), removes the idle requirement, respects the boot barrier. (Rejected option ii/priority-column as inconsistent + heavier.)
- [x] Go unit test `TestSendApprovalInstruction_EnqueuesAsInterrupt` asserts interrupt=true (passing)
- [ ] Live: approve spec while agent mid-interrupt; assert interaction with `## CURRENT PHASE: IMPLEMENTATION` reaches `waiting`

## Commit 3 — Bug (c): routing fix + orphaned-`sending` handling
- [x] Root-caused routing: `usePromptHistory` syncs unsynced entries under the hook's *current* `specTaskId`/`projectId`, but each entry carries its own `sessionId`; a task switch mid-sync files a row whose `session_id` and `spec_task_id` disagree. Fix: sync only entries where `entry.sessionId === sessionId` (both sync paths). Typechecked (tsc clean).
- [x] Extended `ReconcileStuckSendingPrompts` Path 3: old `sending` prompt with no linked interaction and no session activity after it → `failed` (retryable). Validated against live Postgres: flips only the orphan; live/recent/linked controls untouched.
- [x] Validated reaper live (crafted rows in helix-postgres-1: orphan→failed, negative controls stay sending)

## Live end-to-end testing (localhost:8080)
- [x] Registered user A (`test@helix.ml`), created org `testorg`; constructed a task with a prompt owned by a DIFFERENT user (`usr_service_acct`)
- [x] Story 1 PROVEN live via the real API (fetched with each user's session):
  - A (creator/owner) → **200**, sees the service-account prompt (`author.is_system=true`)
  - B (non-admin, non-member) → **403** "not authorized" (fail-closed; required setting `ADMIN_USER_IDS` to A only since dev default `all` makes everyone admin)
  - B added as org member + project `org_members_access=true` → **200**, sees the differently-owned prompt (the exact "org-global" requirement)
- [x] Bug b: verified via unit test (interrupt=true). NOT driven through a live mid-turn approval race (would need a fully provisioned sandbox agent under concurrent interrupts) — flagged.
- [x] Bug c reaper: validated against live Postgres (orphan→failed, controls untouched). Routing fix: typechecked; NOT reproduced live (heisenbug needs task-switch timing) — flagged.
- [x] Reported exactly what was observed; see design.md "Deployment / testing status"

## PR
- [x] Conventional-commit messages; commits 1/2/3 separated so (1) can merge alone (commit 1 = primary; b, c independent)
- [x] Pushed branch `feature/002325-make-spec-task-prompt` (merged latest origin/main first; platform opens the PR against `helixml/helix`)
