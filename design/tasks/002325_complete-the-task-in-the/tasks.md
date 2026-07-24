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
- [ ] Register/login (`test@helix.ml` / `helixtest`), onboard, create a spec task, drive spec-review + approve
- [ ] Story 1: second user / service-account comment visible to a different authorized member; non-member gets 403
- [ ] Bug b: verify kickoff reaches agent under concurrent interrupts
- [ ] Bug c: verify no cross-task contamination and no wedged prompt
- [ ] Report exactly what was observed (no unearned confidence)

## PR
- [ ] Conventional-commit messages; keep commits 1/2/3 separated so (1) can merge alone
- [ ] Open PR against `helixml/helix` with full GitHub URLs
