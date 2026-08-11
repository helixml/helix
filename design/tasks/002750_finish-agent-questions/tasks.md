# Implementation Tasks: Finish Agent Questions (ACP Elicitations) — OpenAPI, Tests, E2E and Live Verification

## 0. Orient (do not skip — most of this feature already exists)

- [ ] In `helix`: `git log --oneline -3` and `git show --stat 9ddbd5d74`; read `design/2026-08-11-agent-questions-elicitation.md`
- [ ] In `zed`: `git status` → stash if dirty → `git checkout feature/002731-end-to-end-agent` (local checkout is currently on `main`); `git show --stat 8fbf40ad92`
- [ ] Read the four committed Rust files and `api/pkg/server/websocket_external_agent_elicitation.go` before writing anything
- [ ] Start the inner stack; poll `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080` until `200` (5–10 min is normal)

## 1. OpenAPI regeneration (unblocks the frontend build)

- [ ] Run `./stack update_openapi`
- [ ] Verify `grep -c "v1SessionsElicitationsRespondCreate" frontend/src/api/api.ts` ≥ 1 and `TypesElicitationRespondResponse` is exported
- [ ] If the generated method names differ, update `frontend/src/services/elicitationService.ts` to match — never hand-edit `api.ts`
- [ ] `cd frontend && yarn build` — clean
- [ ] Commit and push (`feat(api): regenerate openapi client for elicitation endpoints`)

## 2. memorystore elicitation support (unblocks the e2e)

- [ ] Run `./run_docker_e2e.sh` on the branch **as-is** first, to find out whether the existing queue phases already nil-panic on `HasLiveAgentElicitation` (build `zed-binary` first — see §4)
- [ ] Add an `agentElicitations map[string]*types.AgentElicitation` (+ mutex) to `api/pkg/store/memorystore/memorystore.go`
- [ ] Implement all 8 methods, preserving Postgres semantics: conditional `TransitionAgentElicitation` returning `(false, nil)` on status mismatch; `ReapStaleAgentElicitations` filtering on `LastSeenAt` + live status; `HasLiveAgentElicitation` by `interaction_id`
- [ ] Commit and push

## 3. Go unit tests — `api/pkg/server/websocket_external_agent_elicitation_test.go`

- [ ] `sudo apt-get update && sudo apt-get install -y gcc libc6-dev` (CGo needed for tree-sitter)
- [ ] Scaffold `ElicitationSuite` in the `suite.Suite` + gomock style of `websocket_external_agent_sync_test.go`
- [ ] `handleElicitationRequested`: row upsert + transcript entry + patch published + `agent_question` attention event emitted **once**
- [ ] `handleElicitationRequested` idempotency: heartbeat/reconnect re-announcement does not re-notify
- [ ] `handleElicitationResolved`: each terminal status (`accepted`/`declined`/`cancelled`/`completed`) transitions, mirrors into the entry, clears the notification
- [ ] `handleElicitationResolved` with an unknown id: dropped and logged, no error
- [ ] `handleElicitationResync`: listed ids touched; absent ids **not** reaped inside the grace window; absent ids cancelled with `agent_no_longer_holds` after it (inject the grace via `HELIX_ELICITATION_RESYNC_GRACE_SECONDS` or a synthetic cutoff — do not sleep 60 s)
- [ ] `handleElicitationResponseAck`: `noop` and `not_found` reconcile the row; `accepted` is a local no-op
- [ ] **Reconnect does not cancel**: `handleAgentReady` leaves a pending elicitation pending
- [ ] REST endpoint: unauthorised user → 401/403; unknown elicitation → 404; elicitation belonging to another session → 403/404 (assert actual behaviour); already-terminal → 409; no agent connected → 409
- [ ] **Two clients answer at once**: first transition wins, second gets 409, `sendCommandToExternalAgent` called exactly once
- [ ] **Answer after cancel**: 0 rows affected → 409
- [ ] Send-failure rollback restores `pending`
- [ ] Empty / unmappable `request_id` falls back to `handleMessageAdded`'s resolution
- [ ] `TestAutoWake_SkipsInteractionBlockedOnUserQuestion`
- [ ] `processPromptQueue` dispatches (does not defer) a non-interrupt follow-up while a question is live
- [ ] `CGO_ENABLED=1 go test ./pkg/server/ -count=1` — clean; quote the output
- [ ] Commit and push

## 4. E2E — Phase 18 in `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`

- [ ] Build the Zed binary: `./stack build-zed dev`, then `cp zed-build/zed ../zed/crates/external_websocket_sync/e2e-test/zed-binary` (poll for completion; **no blind sleeps**)
- [ ] Add **Phase 18** (17 is already "Queue interrupt"), appended after `runQueuePhases`, replacing the `d.phase = 17` terminal state
- [ ] Step 1: `ProcessSyncEvent("elicitation_requested")` with a realistic **non-empty** `requested_schema` (`oneOf` options + `_claude/askUserQuestionOption` / `_askUserQuestionCustomAnswer` metadata), the live `acp_thread_id`, an `entry_index`, `status: "pending"`
- [ ] Step 2: assert the row is live and the schema round-tripped non-empty
- [ ] Step 3: issue `respond_elicitation` through the production path; assert the command reaches Zed; tolerate + log the real `elicitation_response_ack{not_found}` (synthetic elicitation is unknown to live Zed)
- [ ] Step 4: `ProcessSyncEvent("elicitation_resolved", accepted)`; assert terminal status and mirrored transcript entry
- [ ] Step 5: send a normal `chat_message` on the same thread and assert `message_completed` — the turn must not be wedged
- [ ] Phase comment states **why it is synthetic**: a CI phase must not depend on the model choosing to call `AskUserQuestion`
- [ ] Update the header phase-list comment block and any phase-count constants/timeouts
- [ ] **Run `./run_docker_e2e.sh` to green** and quote the output — mandatory per `CLAUDE.md`, no exceptions
- [ ] If `run_e2e.sh` needed a local-only model override to run here, **revert that edit** before committing
- [ ] Commit and push in zed

## 5. Live verification in the inner Helix (evidence required)

- [ ] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project)
- [ ] Create a **spec task** (not a bare chat session) on a **Claude Code** agent; confirm liveness via `config->>'zed_thread_id'`
- [ ] Get the agent to call `AskUserQuestion`
- [ ] Screenshot the question rendered in the Helix UI with **every** option (label + description) → `screenshots/01-question-rendered.png`
- [ ] Answer it in Helix; screenshot the resumed turn reflecting that answer → `screenshots/02-answered-turn-resumed.png`
- [ ] **Next operation**: send a normal follow-up message; confirm it is delivered and answered → `screenshots/03-followup.png`
- [ ] Trigger a **second** question in the same session; confirm it works independently → `screenshots/04-second-question.png`
- [ ] Decline / Skip path: confirm the turn continues with empty answers and settles cleanly → `screenshots/05-skip.png`
- [ ] Leave a question unanswered, interrupt from Helix; confirm it settles terminal and stops being answerable (card locks; a further POST returns 409) → `screenshots/06-interrupted.png`
- [ ] Best-effort extras: API-restart-while-pending, custom-answer-only ("Other") path, task-list/header badge, bell notification + dismissal
- [ ] Record any failures plainly in the final report — do **not** report the feature done on the strength of unit tests
- [ ] Commit the screenshots to helix-specs and push

## 6. Ship (CLAUDE.md order — exactly)

- [ ] Commit in `zed`; **do not push yet**
- [ ] `git rev-parse HEAD` in `zed`
- [ ] Bump `ZED_COMMIT` in `helix/sandbox-versions.txt` (from `eae9b051e…`) to that hash; commit
- [ ] Open the **Helix** PR first
- [ ] Push the zed branch; `gh pr create --repo helixml/zed` (the `--repo` flag is mandatory)
- [ ] Check CI yourself on both PRs (`gh pr checks` / Drone MCP tools); fix and re-check until green
- [ ] Merge **zed first**, then helix (regular merge commits, never squash); rebase the `ZED_COMMIT` bump if it moved
- [ ] Give full PR URLs (`https://github.com/helixml/helix/pull/N`), never `owner/repo#N`
