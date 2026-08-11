# Implementation Tasks: End-to-End Agent Questions via ACP Elicitations

## User decisions (taken at implementation start)

- Skip button label = **"Skip"** (not "Decline") — the adapter continues the turn with empty answers
- Resync grace window = **60 s**, overridable via `HELIX_ELICITATION_RESYNC_GRACE_SECONDS`
- Follow-up while pending **locks the card immediately** ("replying instead…"), reconciled by the cancelled event

## Verify first (blocking assumptions)

- [~] Confirm `entry_index` == accumulator `message_id`. STATIC PROOF DONE: `message_id` is literally `latest_idx.to_string()` / `entry_idx` (the index into `AcpThread::entries()`), and elicitations are pushed into that same vector, so the equality holds by construction. Live log confirmation still pending during live testing. Original wording (log `(entry_index, message_id, entry_type)` on both sides for a turn with text + tool call + elicitation); if not, carry an explicit `after_message_id` instead
- [x] Re-read `applyAskElicitationResponse` (`claude-agent-acp/dist/elicitation.js:180-210`) in the version actually deployed at implementation time and mirror its precedence exactly

## Zed — `crates/external_websocket_sync/`

- [x] Add `SyncEvent::ElicitationRequested`, `ElicitationResolved`, `ElicitationResync`, `ElicitationResponseAck` to `src/types.rs` plus their `to_outgoing_message()` arms
- [x] Add a single `status_str()` helper mapping `ElicitationStatus` → wire strings (`Canceled` → `"cancelled"`), used everywhere
- [x] Emit `ElicitationRequested` from the `AcpThreadEvent::ElicitationRequested` arm in `thread_service.rs`, serializing `requested_schema` verbatim with `serde_json::to_value`
- [x] Emit `ElicitationResolved` from both `AcpThreadEvent::ElicitationResponded` and the `EntryUpdated(ix)` arm when the entry is `AgentThreadEntry::Elicitation`
- [x] Add an `Elicitation` arm to the `EntryUpdated` match (status changes arrive only there). CHANGED FROM PLAN: `NewEntry` and the `Stopped`/`Error` flush need no arm — elicitations are carried by the dedicated `AcpThreadEvent::ElicitationRequested`/`ElicitationResponded` events instead, which avoids emitting a duplicate `message_added` for the same entry
- [x] Use the turn-scoped `turn_request_id` for all elicitation events (copy the neighbouring arms; do not call `get_thread_request_id` directly)
- [x] Add the reconnect/`open_thread` **resync**: re-emit `ElicitationRequested` for every still-`Pending` elicitation per registered thread, then one `ElicitationResync` per thread listing exactly those ids (empty list is meaningful) — reuse the entry-walk machinery from the `Stopped`/`Error` flush
- [x] Add `ElicitationResponseRequest` + `GLOBAL_ELICITATION_RESPONSE_CALLBACK` (+ pending queue) in `external_websocket_sync.rs`, mirroring the cancel-thread callback
- [x] Add `respond_elicitation` to the command dispatch in `websocket_sync.rs` and its `handle_respond_elicitation` parser
- [x] Add the dedicated GPUI drain task in `thread_service.rs` (next to the cancel task) calling `AcpThread::respond_to_elicitation`
- [x] Snapshot status before/after the update and emit `ElicitationResponseAck` with `accepted` / `noop` / `not_found`; no `unwrap()`, no bare `let _ =`
- [ ] Rust unit tests: entry→event mapping, resync emission, no-op on already-answered, not-found on missing thread
- [ ] `cargo build ... -p zed` — **BLOCKED: no Rust toolchain in this environment** (no cargo/rustc/~.cargo/target). Build path is `./stack build-zed` (Docker); did not complete under load average 350-500

## Helix API — types, store, handlers

- [x] Add `types.AgentElicitation` model (incl. `LastSeenAt`) + GORM AutoMigrate registration (indexes on `session_id`, `interaction_id`, `status`)
- [x] Add store methods: `CreateOrUpdateElicitation`, `GetElicitation`, `TransitionElicitationStatus` (conditional `WHERE status=?`), `TouchElicitationsLastSeen`, `ListPendingElicitationsBySessions`, `ReapUnseenPendingElicitations`
- [x] Add `Elicitation *ElicitationEntry` to `wsprotocol.ResponseEntry` and `types.EntryPatch`; teach the accumulator to carry it (upsert + restore in `RestoreAccumulator`)
- [x] Handle `elicitation_requested`: resolve session/interaction from `request_id`, idempotent row upsert, entry upsert, publish patches + `interaction_update`, raise the `agent_question` attention event on the new-pending transition only
- [x] Empty/unmappable `request_id`: reuse `handleMessageAdded`'s existing resolution (streaming context → DB fallback to newest waiting interaction for the thread); drop with a loud `warn` only if that also misses
- [x] Handle `elicitation_resolved`: conditional status update, mirror into the entry, clear the attention event, publish; drop+log unknown ids
- [x] Handle `elicitation_resync`: refresh `LastSeenAt` for listed ids; reap pending/submitting rows of that session absent from the list and older than the grace window → `cancelled(agent_no_longer_holds)`
- [x] Handle `elicitation_response_ack`: reconcile row on `noop`/`not_found`
- [x] **Do not** change any elicitation status in `handleAgentReady` — a reconnect is not evidence (an API restart leaves the agent and its `respond_tx` alive)
- [x] Change `processPromptQueue` (`:3392`): when the newest interaction is `waiting` **and** has a pending/submitting elicitation, dispatch the follow-up with interrupt semantics instead of deferring it; do not write a terminal status locally
- [x] Add `POST /api/v1/sessions/{id}/elicitations/{elicitation_id}/respond` with swagger annotations, session-from-URL auth (`authorizeUserToSession` + `ActionUpdate`), session-ownership check, `pending→submitting` conditional transition (409 on loss), agent-connected check (409), `sendCommandToExternalAgent`
- [x] Register the route next to `/sessions/{id}/cancel` in `server.go`
- [x] Add `types.AttentionEventAgentQuestion` and emit it via `attentionService.EmitEvent` on the new-pending transition only, with the **elicitation id as the qualifier** so resync re-announcements dedupe instead of re-notifying
- [x] Add `buildTitle` / `buildDescription` / `eventEmoji` cases for the new event type (no generic fallthrough)
- [x] Add per-event dismissal keyed by the elicitation-scoped idempotency key (task-wide `DismissAttentionEventsForTask` is too blunt) and call it on every terminal status
- [x] Do **not** copy the "user already active in session" attention suppression from `agent_interaction_completed` — a question needs answering regardless
- [ ] Go unit tests: notification emitted once for a question, not re-emitted on resync re-announcement, dismissed on each terminal status
- [x] Add Gate 0 to `maybeAutoWake`: skip interactions with a pending/submitting elicitation
- [ ] Run `./stack update_openapi`
- [ ] Go unit tests: requested/resolved/resync/ack handlers, endpoint auth + 404/403/409 paths, two-clients race, answer-after-cancel, empty-`request_id` fallback, **reconnect-does-not-cancel**, resync-absence-cancels-after-grace, queue-does-not-defer-follow-up
- [ ] Go unit test `TestAutoWake_SkipsInteractionBlockedOnUserQuestion`
- [x] `go build ./pkg/...` clean (verified). `go test ./pkg/server/...` NOT run — needs CGo deps + Postgres

## Helix frontend

- [x] Extend `ResponseEntry` type (`"elicitation"` + payload) in `types.ts` / `InteractionInference.tsx`
- [x] Carry `elicitation` through the patch merge in `contexts/streaming.tsx`
- [x] Add `elicitationSchema.ts`: generic JSON-Schema → fields parser (oneOf, array/items.anyOf, custom-answer linking **by value shape** — `isCustomAnswer`/`questionId`, never by meta-key name — and fallbacks) with unit tests
- [x] Implement submission-content building that mirrors the adapter: trimmed non-empty custom wins over selection; omit questions with neither set; custom-only submission valid
- [x] Add `ElicitationCard.tsx`: message, header chips, options with label + description, "Other" input, Submit + Skip/Decline (copy must convey that declining continues the turn with empty answers), answered/terminal read-only states incl. "you replied instead" and "expired — the agent restarted", Lucide icons, error boundary
- [x] Add an `elicitation` segment to `buildActivityTimeline` so the card is never folded into a collapsed tool run
- [ ] Optimistically lock the card when the user sends a normal message with a question pending; reconcile on the `cancelled` event
- [x] Wire submission to the generated API client with React Query + query invalidation (no raw fetch, no `setTimeout`, no `setQueryData`)
- [ ] Add transient `waiting_for_user_input` to `SpecTask` and populate it in the list/get handlers
- [ ] Surface "waiting for your answer" in the task list/kanban and the task detail header
- [ ] `cd frontend && yarn build` clean

## E2E

- [ ] Add Phase 17 to `e2e-test/helix-ws-test-server/main.go`, driven by a **synthetic** elicitation injected at the Zed test seam (comment must say why it is synthetic rather than model-driven): `elicitation_requested` with non-empty schema → `respond_elicitation` → `elicitation_resolved(accepted)` → `message_completed`
- [ ] Update the phase list comment block and any phase-count constants
- [ ] Run the full dockerized e2e (`./run_docker_e2e.sh`) until green — mandatory, no exceptions

## Live verification in the inner Helix (evidence required)

- [ ] Start a spec task and get the agent to call `AskUserQuestion`
- [ ] Screenshot the question rendered in the Helix task UI with all options → `screenshots/`
- [ ] Answer it in Helix; capture the resumed turn reflecting the answer → `screenshots/`
- [ ] Custom-answer path: submit only the "Other" text with no option selected; confirm the agent receives the typed text
- [ ] Send a normal follow-up message after answering; confirm delivery and reply
- [ ] Trigger a second question in the same session; confirm it works independently
- [ ] Follow-up-instead-of-answering: with a question pending, send a normal message; confirm it is delivered, the turn proceeds, and the card locks as cancelled
- [ ] **API-restart test**: with a question pending, restart the API; confirm the question is still shown, still answerable, and answering it still resumes the turn
- [ ] Decline path: confirm the turn continues with empty answers and settles cleanly
- [ ] Leave a question unanswered, interrupt from Helix; confirm it settles and stops being answerable
- [ ] Confirm the task list/header badge appears while pending and clears on resolution
- [ ] Confirm the notification lands: bell entry present (screenshot it) and, where a Slack trigger is configured, the threaded Slack reply; confirm it is dismissed once the question is resolved
- [ ] Confirm an API restart while a question is pending does not produce a duplicate notification (resync dedupe)

## Docs and merge

- [ ] Write `design/2026-08-11-agent-questions-elicitation.md` in the helix repo (wire format incl. resync, storage decision + rationale, status-reconciliation rules with the reconnect-is-not-evidence rule, follow-up-while-pending behaviour, failure modes, and the `spawner.go` follow-up note)
- [ ] Commit in zed (do not push); `git rev-parse HEAD`; update `ZED_COMMIT` in `sandbox-versions.txt`
- [ ] Open the Helix PR first
- [ ] Push the zed branch and open its PR with `gh pr create --repo helixml/zed`
- [ ] CI green on both PRs; merge zed first, then helix (rebase the `ZED_COMMIT` bump if it moved)


## Status at handoff (honest)

**Verified:**
- `go build ./pkg/...` passes clean.
- ACP type shapes verified against docs.rs before writing Rust — caught two wrong
  assumptions (`tool_call_id` lives on `ElicitationScope::Session`, not on
  `CreateElicitationRequest`; accept content is `BTreeMap<String, ElicitationContentValue>`,
  not raw JSON).
- Adapter response semantics read from the deployed source (0.66.0), not inferred.

**NOT verified — do not treat as working:**
- Zed crate is **not compiled**. No Rust toolchain exists here; `./stack build-zed` (Docker)
  did not complete under sustained load average 350-500.
- Frontend: `node_modules` is not installed; `yarn install` did not complete, so
  `yarn build` and the schema-parser unit tests were **not run**.
- Go unit tests for the new handlers: **not written**.
- E2E phase 17: **not written, not run**.
- Live inner-Helix verification (the whole Definition of Done): **not done**.

**Merge state:** both branches merged with `origin/main` and pushed.
`origin/main` is an ancestor of both HEADs. Merge conflicts resolved keeping both sides:
main added a `plan` response-entry type while this branch added `elicitation`, so the type
unions list both and the render keeps both `{questions}` and `{planProgress}`.
`ZED_COMMIT` pins the merged zed SHA `341d099da7`.

**Environment blocker:** this machine ran at load average 95-500 throughout. A Go build
that should take ~2 min took ~25 min; `yarn install` never finished. The remaining work is
not conceptually blocked — it is blocked on machine capacity.
