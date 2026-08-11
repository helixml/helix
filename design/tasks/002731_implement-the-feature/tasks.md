# Implementation Tasks: End-to-End Agent Questions via ACP Elicitations

## Zed — `crates/external_websocket_sync/`

- [ ] Add `SyncEvent::ElicitationRequested`, `ElicitationResolved`, `ElicitationResponseAck` to `src/types.rs` plus their `to_outgoing_message()` arms
- [ ] Add a single `status_str()` helper mapping `ElicitationStatus` → wire strings (`Canceled` → `"cancelled"`), used everywhere
- [ ] Emit `ElicitationRequested` from the `AcpThreadEvent::ElicitationRequested` arm in `thread_service.rs`, serializing `requested_schema` verbatim with `serde_json::to_value`
- [ ] Emit `ElicitationResolved` from both `AcpThreadEvent::ElicitationResponded` and the `EntryUpdated(ix)` arm when the entry is `AgentThreadEntry::Elicitation`
- [ ] Add `Elicitation` arms to the three entry-mapping matches (`NewEntry` ~:914, `EntryUpdated` ~:1051, `Stopped/Error` flush ~:1112) so elicitation entries are no longer dropped
- [ ] Use the turn-scoped `turn_request_id` for all elicitation events (copy the neighbouring arms; do not call `get_thread_request_id` directly)
- [ ] Add `ElicitationResponseRequest` + `GLOBAL_ELICITATION_RESPONSE_CALLBACK` (+ pending queue) in `external_websocket_sync.rs`, mirroring the cancel-thread callback
- [ ] Add `respond_elicitation` to the command dispatch in `websocket_sync.rs` and its `handle_respond_elicitation` parser
- [ ] Add the dedicated GPUI drain task in `thread_service.rs` (next to the cancel task) calling `AcpThread::respond_to_elicitation`
- [ ] Snapshot status before/after the update and emit `ElicitationResponseAck` with `accepted` / `noop` / `not_found`; no `unwrap()`, no bare `let _ =`
- [ ] Rust unit tests: entry→event mapping, no-op on already-answered, not-found on missing thread
- [ ] `cargo build --features external_websocket_sync -p zed` and `./script/clippy` clean

## Helix API — types, store, handlers

- [ ] Add `types.AgentElicitation` model + GORM AutoMigrate registration (indexes on `session_id`, `interaction_id`, `status`)
- [ ] Add store methods: `CreateOrUpdateElicitation`, `GetElicitation`, `TransitionElicitationStatus` (conditional `WHERE status=?`), `ListPendingElicitationsBySessions`, `CancelPendingElicitationsForSession`
- [ ] Add `Elicitation *ElicitationEntry` to `wsprotocol.ResponseEntry` and `types.EntryPatch`; teach the accumulator to carry it (upsert + restore in `RestoreAccumulator`)
- [ ] Handle `elicitation_requested` in `processExternalAgentSyncMessage`: resolve session/interaction from `request_id`, persist row, upsert entry, publish patches + `interaction_update`
- [ ] Handle `elicitation_resolved`: conditional status update, mirror into the entry, publish; drop+log unknown ids
- [ ] Handle `elicitation_response_ack`: reconcile row on `noop`/`not_found`
- [ ] Reconcile on reconnect in `handleAgentReady`: pending/submitting elicitations for the session → `cancelled`
- [ ] Add `POST /api/v1/sessions/{id}/elicitations/{elicitation_id}/respond` handler with swagger annotations, session-from-URL auth (`authorizeUserToSession` + `ActionUpdate`), session-ownership check, `pending→submitting` conditional transition (409 on loss), agent-connected check (409), `sendCommandToExternalAgent`
- [ ] Register the route next to `/sessions/{id}/cancel` in `server.go`
- [ ] Add Gate 0 to `maybeAutoWake`: skip interactions with a pending/submitting elicitation
- [ ] Run `./stack update_openapi`
- [ ] Go unit tests in `websocket_external_agent_sync_test.go` style: requested/resolved/ack handlers, endpoint auth + 404/403/409 paths, two-clients race, answer-after-cancel
- [ ] Go unit test `TestAutoWake_SkipsInteractionBlockedOnUserQuestion`
- [ ] `go build ./pkg/...` and `go test ./pkg/server/...` clean

## Helix frontend

- [ ] Extend `ResponseEntry` type (`"elicitation"` + payload) in `types.ts` / `InteractionInference.tsx`
- [ ] Carry `elicitation` through the patch merge in `contexts/streaming.tsx`
- [ ] Add `elicitationSchema.ts`: generic JSON-Schema → fields parser (oneOf, array/items.anyOf, `_meta` custom-answer linking, fallbacks) with unit tests
- [ ] Add `ElicitationCard.tsx`: message, header chips, options with label + description, "Other" input, Submit + Decline, answered/terminal read-only states, Lucide icons, error boundary
- [ ] Add an `elicitation` segment to `buildActivityTimeline` so the card is never folded into a collapsed tool run
- [ ] Wire submission to the generated API client with React Query + query invalidation (no raw fetch, no `setTimeout`, no `setQueryData`)
- [ ] Add transient `waiting_for_user_input` to `SpecTask` and populate it in the list/get handlers
- [ ] Surface "waiting for your answer" in the task list/kanban and the task detail header
- [ ] (Pending Open Question 2) Raise/clear an `agent_question` attention event
- [ ] `cd frontend && yarn build` clean

## E2E

- [ ] Add Phase 17 to `e2e-test/helix-ws-test-server/main.go`: prompt → `elicitation_requested` (assert non-empty schema) → `respond_elicitation` → `elicitation_resolved(accepted)` → `message_completed`
- [ ] Update the phase list comment block and any phase-count constants
- [ ] Run the full dockerized e2e (`./run_docker_e2e.sh`) until green — mandatory, no exceptions

## Live verification in the inner Helix (evidence required)

- [ ] Start a spec task and get the agent to call `AskUserQuestion`
- [ ] Screenshot the question rendered in the Helix task UI with all options → `screenshots/`
- [ ] Answer it in Helix; capture the resumed turn reflecting the answer → `screenshots/`
- [ ] Send a normal follow-up message in the same thread; confirm delivery and reply
- [ ] Trigger a second question in the same session; confirm it works independently
- [ ] Exercise the decline path; confirm the turn settles cleanly
- [ ] Leave a question unanswered, interrupt from Helix; confirm it settles and stops being answerable
- [ ] Confirm the task list/header shows "blocked on human answer" while pending

## Docs and merge

- [ ] Write `design/2026-08-11-agent-questions-elicitation.md` in the helix repo (wire format, storage decision + rationale, status-reconciliation rules, failure modes)
- [ ] Commit in zed (do not push); `git rev-parse HEAD`; update `ZED_COMMIT` in `sandbox-versions.txt`
- [ ] Open the Helix PR first
- [ ] Push the zed branch and open its PR with `gh pr create --repo helixml/zed`
- [ ] CI green on both PRs; merge zed first, then helix (rebase the `ZED_COMMIT` bump if it moved)
