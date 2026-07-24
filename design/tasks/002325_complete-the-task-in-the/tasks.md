# Implementation Tasks: Make Spec-Task Prompt Queue Org-Global

## Commit 1 — Org-global queue (primary)
- [x] Remove the `WHERE user_id = viewer` filter in `ListPromptHistory` (`api/pkg/store/store_prompt_history.go` ~517); keep scope filters; add fail-closed guard rejecting empty scope
- [x] Keep `userID` param for signature stability (do not filter on it); update/verify callers, interface (`store.go:853`), and mocks compile
- [x] Add task/session authorization to `listPromptHistory` (`api/pkg/server/prompt_history_handlers.go` ~605): `spec_task_id` → `GetSpecTask` + creator bypass + `authorizeUserToProjectByID(..., ActionGet)`
- [x] For `session_id` path, load session + `authorizeUserToSession(ctx, user, session, ActionGet)`; fail closed (403), never leave unauthenticated
- [x] Enrich prompt-history list response server-side with author fields (name/email + system flag); regenerated client via `./stack update_openapi`
- [x] Display prompt author per entry in `SessionPromptQueue.tsx` and `RobustPromptInput.tsx` via generated API client (shared `promptAuthorLabel` util); service account renders as "System"
- [ ] Go unit test: authorized non-owner org member sees owner's prompts; non-member gets 403 (both `spec_task_id` and `session_id` paths)

## Commit 2 — Bug (b): reliable implementation-kickoff delivery
- [ ] Root-cause confirm: kickoff enqueued `interrupt=false` (`agent_instruction_service.go:596`) is starved by interrupt comments and abandoned past retry cap 20
- [ ] Make phase-transition kickoff a priority/guaranteed control signal (chosen: exempt from retry cap + idle-drain priority, OR enqueue as interrupt — per review decision)
- [ ] Test: approve spec while agent mid-interrupt; assert interaction with `## CURRENT PHASE: IMPLEMENTATION` reaches `waiting`

## Commit 3 — Bug (c): routing fix + orphaned-`sending` handling
- [ ] Trace frontend optimistic send (`RobustPromptInput.tsx`, `usePromptHistory.ts`, `promptHistoryService.ts`, `optimisticSessionStarting.ts`) for stale/misrouted session/task id; fix so prompts enqueue against the active session only
- [ ] Verify/extend the `sending`-orphan reaper so client-optimistic-id rows stuck in `sending` are cleared and don't wedge `queue_position`
- [ ] Test: comment for one task never lands in another task's queue; wedged `sending` prompt is recovered

## Live end-to-end testing (localhost:8080)
- [ ] Register/login (`test@helix.ml` / `helixtest`), onboard, create a spec task, drive spec-review + approve
- [ ] Story 1: second user / service-account comment visible to a different authorized member; non-member gets 403
- [ ] Bug b: verify kickoff reaches agent under concurrent interrupts
- [ ] Bug c: verify no cross-task contamination and no wedged prompt
- [ ] Report exactly what was observed (no unearned confidence)

## PR
- [ ] Conventional-commit messages; keep commits 1/2/3 separated so (1) can merge alone
- [ ] Open PR against `helixml/helix` with full GitHub URLs
