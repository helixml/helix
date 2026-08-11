# Design: End-to-End Agent Questions via ACP Elicitations

Three layers: Zed sync crate → Helix API/store → Helix frontend. The whole feature is one
new conversation entry type, one new command, and one resync, riding the existing sync
machinery.

## Codebase facts established during design (read these before coding)

Zed (`/home/retro/work/zed`):

| Fact | Location |
|---|---|
| `AgentThreadEntry::Elicitation(ElicitationEntryId)` — the dropped variant | `crates/acp_thread/src/acp_thread.rs:407` |
| `Elicitation { id, request: acp::CreateElicitationRequest, status }` | `acp_thread.rs:416` |
| `ElicitationStatus::{Pending{respond_tx}, Accepted, Declined, Canceled, Completed}` | `acp_thread.rs:423` |
| `AcpThreadEvent::ElicitationRequested/ElicitationResponded` | `acp_thread.rs:2197` |
| `AcpThread::elicitation(&id) -> Option<(usize, &Elicitation)>` | `acp_thread.rs:3607` |
| `AcpThread::respond_to_elicitation(&id, response, cx)` — already no-ops when not `Pending` or id unknown | `acp_thread.rs:3548` |
| Status changes emit `EntryUpdated(ix)` (not a dedicated event) | `acp_thread.rs:3560, 3592` |
| **Every new turn cancels outstanding elicitations**: `run_turn` → `cancel_inner(InterruptedByFollowUp)` → `cancel_outstanding_elicitations` | `acp_thread.rs:3785`, `:3987`, `:4065` |
| Entry→SyncEvent mapping that must learn about elicitations (three `_ => return`/`continue` arms) | `crates/external_websocket_sync/src/thread_service.rs:914, 1051, 1112` |
| Thread registry lookup by `acp_thread_id` | `thread_service.rs:834 get_thread()` |
| Command→callback→GPUI-task pattern to copy verbatim | `thread_service.rs:1434-1483` (cancel task) + `external_websocket_sync.rs:574 request_cancel_thread` + `websocket_sync.rs:400` dispatch |
| `SyncEvent` enum + `to_outgoing_message()` | `crates/external_websocket_sync/src/types.rs:170-370` |
| `requested_schema` is a typed `acp::ElicitationSchema` (serde-serializable) — "verbatim JSON" means `serde_json::to_value(&mode.requested_schema)`, never a hand-rolled flatten | `agent_ui/src/conversation_view/elicitation.rs:918, 1553` |
| Accept response shape | `agent_ui/src/conversation_view.rs:2974` — `CreateElicitationResponse::new(ElicitationAction::Accept(ElicitationAcceptAction::new().content(content)))` |

Adapter, `claude-agent-acp` **0.66.0**, read from
`.zed-state/…/@agentclientprotocol/claude-agent-acp/dist/elicitation.js` (all three copies
on this machine are 0.66.0):

| Fact | Location |
|---|---|
| Schema builder (`question_<i>`, `question_<i>_custom`, titles, descriptions, option objects, nothing `required`) | `elicitation.js:115-166` |
| Option preview meta key is **`_claude/askUserQuestionOption`** — the brief's `claudeCode/optionPreview` is wrong | `elicitation.js:91` |
| Custom-answer meta key is **`_askUserQuestionCustomAnswer`** — the brief's `claudeCode/customAnswer` is wrong | `elicitation.js:98` |
| Response folding + precedence | `elicitation.js:180-210 applyAskElicitationResponse` |

Helix (`/home/retro/work/helix`):

| Fact | Location |
|---|---|
| Sync event dispatch switch | `api/pkg/server/websocket_external_agent_sync.go:847-900` |
| Entry accumulator + `ResponseEntry{Type,Content,MessageID,ToolName,ToolStatus}` | `api/pkg/server/wsprotocol/accumulator.go:59` |
| Persisted structured entries on the interaction | `api/pkg/types/types.go:87 Interaction.ResponseEntries` (jsonb) |
| Streaming deltas to the frontend | `types.EntryPatch` (`types.go:1011`), `publishEntryPatchesToFrontend` |
| Outbound command shape + delivery | `types.ExternalAgentCommand{Type,Data}` (`types.go:2281`), `sendCommandToExternalAgent` (`:2327`), `queueOrSend` (`:2635`) |
| **Queue defers any non-interrupt prompt while the newest interaction is `waiting`** — the follow-up trap | `websocket_external_agent_sync.go:3392 processPromptQueue` |
| `agent_ready` handler (fires on **every** reconnect, including a plain API restart) | `websocket_external_agent_sync.go:4261` |
| Endpoint pattern to copy (auth + swagger + command) | `api/pkg/server/session_handlers.go:2396 cancelSessionTurn` |
| Auto-wake gate to extend | `api/pkg/server/auto_wake_stuck_interactions.go:225 maybeAutoWake` |
| Attention event machinery to reuse | `api/pkg/types/attention_event.go` (`org_message` is the existing "ask a human" precedent) |
| Frontend entry rendering / timeline builder | `frontend/src/components/session/InteractionInference.tsx:20-140` |
| Frontend patch application | `frontend/src/contexts/streaming.tsx:482-540` |
| E2E phases 1-16 (add 17) | `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go:9-23` |

## Wire format

### Zed → Helix

`entry_index` is the position in `AcpThread::entries()` and is intended to double as the
accumulator's `message_id`, so the question lands in the conversation in the right place
with no new ordering logic. **This equality is load-bearing and is an assumption until
proven** — see "Assumption to verify first" below.

```jsonc
// event_type: "elicitation_requested"
{
  "acp_thread_id": "…",
  "request_id": "…",              // turn-scoped rid, same rotation rules as message_added
  "entry_index": "7",             // intended == message_id in the accumulator
  "elicitation_id": "uuid",       // ElicitationEntryId — opaque, Zed-owned
  "tool_call_id": "toolu_…",      // may be empty
  "mode": "form",                 // "form" | "url" | other (forward-compat)
  "message": "Which colour should I use?",
  "requested_schema": { /* verbatim serde_json of acp::ElicitationSchema */ },
  "status": "pending",
  "timestamp": 1770000000
}

// event_type: "elicitation_resolved"
{
  "acp_thread_id": "…", "request_id": "…", "entry_index": "7",
  "elicitation_id": "uuid",
  "status": "accepted" | "declined" | "cancelled" | "completed",
  "content": { "question_0": "Red" },   // null unless accepted
  "timestamp": 1770000001
}

// event_type: "elicitation_resync"  — completeness marker, see reconciliation
{ "acp_thread_id": "…", "elicitation_ids": ["uuid", …], "timestamp": 1770000002 }

// event_type: "elicitation_response_ack"  (reply to a respond_elicitation command)
{ "elicitation_id": "uuid", "status": "accepted" | "noop" | "not_found", "error": "" }
```

`ElicitationStatus::Canceled` (Zed spelling) maps to wire `"cancelled"` in exactly one
place — the `status_str()` helper in `types.rs`. Do not let the two spellings spread.

### Helix → Zed

```jsonc
// ExternalAgentCommand{Type:"respond_elicitation", Data:{…}}
{
  "acp_thread_id": "…",
  "elicitation_id": "uuid",
  "action": "accept" | "decline",     // "cancel" reserved for teardown, never a user button
  "content": { "question_0": "Red", "question_1": ["a","b"] }  // accept only
}
```

## Assumption to verify first (before building on it)

`entry_index == accumulator message_id` is the reason ordering comes free. Confirm it
**empirically on a live thread** before the rest of the entry work: log
`(entry_index, message_id, entry_type)` on both sides for a turn containing text, a tool
call and an elicitation, and check they line up. If they do not, fall back to carrying an
explicit `after_message_id` in the event and inserting relative to it — do not paper over a
mismatch with a sort.

## Layer 1 — Zed (`crates/external_websocket_sync/`)

**Emit.** In the `cx.subscribe(thread_entity, …)` handler (`thread_service.rs:903`):

- `AcpThreadEvent::ElicitationRequested(id)` → read `thread.elicitation(&id)`, build
  `SyncEvent::ElicitationRequested` (schema serialized verbatim), send with the current
  turn's `rid` (`turn_request_id.borrow()` — same rule as `message_added`, so the question
  routes to the right interaction).
- `AcpThreadEvent::ElicitationResponded(id)` and the `EntryUpdated(ix)` arm when the entry
  at `ix` is `AgentThreadEntry::Elicitation` → `SyncEvent::ElicitationResolved` with the
  current status. Both paths must be handled: `respond_to_elicitation`/`cancel_elicitation`
  emit `EntryUpdated`, while the oneshot completing emits `ElicitationResponded`. The
  resolved event is idempotent by design, so double-emit is harmless — the Go side treats it
  as a conditional update.
- The three entry-mapping match arms (`:914` NewEntry, `:1051` EntryUpdated, `:1112`
  Stopped/Error re-flush) currently `_ => return`. Add an `Elicitation` arm so elicitation
  entries are not dropped and are re-sent on the terminal flush.

**Resync (new, and load-bearing for Correction 1).** On WebSocket (re)connect and on
`open_thread`, Zed walks each registered thread's entries, re-emits
`ElicitationRequested` for every elicitation still `Pending`, and then emits one
`ElicitationResync` per thread listing exactly those ids (empty list allowed and
meaningful). This reuses the same "walk entries and re-send" machinery as the
`Stopped`/`Error` flush at `thread_service.rs:1112`. Helix uses the list as a completeness
marker: anything pending on the Helix side and absent from it is gone on the agent side.

**Accept.** New command `respond_elicitation`, wired exactly like the cancel path:
`websocket_sync.rs` dispatch (`:400`) → `handle_respond_elicitation` →
`crate::request_elicitation_response(ElicitationResponseRequest{…})` (new global callback in
`external_websocket_sync.rs`, mirroring `GLOBAL_CANCEL_THREAD_CALLBACK` + `PENDING_*` queue
for the not-yet-initialised case) → dedicated GPUI task in `thread_service.rs` next to the
cancel task → `get_thread(&acp_thread_id)` → `thread.update(cx, |t, cx|
t.respond_to_elicitation(&ElicitationEntryId(id.into()), resp, cx))`.

**Idempotency and races.** `respond_to_elicitation` is already a safe no-op for an unknown
id or a non-`Pending` status, so no panic is reachable. To *report* the no-op, snapshot the
status via `thread.elicitation(&id)` before and after the update and emit
`ElicitationResponseAck` with `accepted` / `noop` / `not_found`. A dropped thread (registry
miss) → `not_found`. Never `unwrap()`; never bare `let _ =` on a fallible send (`.log_err()`
per the Zed CLAUDE.md rules).

## Layer 2 — Helix API

### Storage decision: hybrid (row is authoritative, entry is the record) — confirmed in review

```go
type AgentElicitation struct {
    ID            string         `gorm:"primaryKey;size:255"` // Zed ElicitationEntryId
    SessionID     string         `gorm:"index;size:255"`
    InteractionID string         `gorm:"index;size:255"`
    RequestID     string         `gorm:"size:255"`
    AcpThreadID   string         `gorm:"size:255"`
    ToolCallID    string         `gorm:"size:255"`
    EntryIndex    string         `gorm:"size:64"`             // == accumulator message_id
    Message       string         `gorm:"type:text"`
    Mode          string         `gorm:"size:32"`
    Schema        datatypes.JSON `gorm:"type:jsonb"`          // verbatim requestedSchema
    Status        string         `gorm:"size:32;index"`       // pending|submitting|accepted|declined|cancelled|completed
    Content       datatypes.JSON `gorm:"type:jsonb"`          // the answer, once accepted
    LastSeenAt    time.Time      // refreshed by resync; drives the grace window
    Created, Updated time.Time
}
```

Why both, not just the entry:

- **Status races need a conditional write.** Two clients answering at once, or an answer
  racing a Zed-side cancel, is resolved by `UPDATE … SET status=? WHERE id=? AND status=?`
  returning rows-affected. A jsonb array on the interaction gives no such primitive without
  read-modify-write on a blob the streaming accumulator is concurrently rewriting.
- **Authorisation and lookup are by id.** The answer endpoint must check that
  `elicitation.session_id == {id from URL}`. One indexed lookup, not a jsonb scan.
- **"Which tasks are blocked" must be cheap.** The task-list badge, the attention event and
  the auto-wake gate all need `status='pending'` filtered by session/interaction.

Why also the entry, not just the row:

- The card must render **in conversation order** and remain in the transcript after it is
  answered. The entries array is exactly that, and `entry_index` gives the ordering.
- The existing patch/stream path (`EntryPatch` → `streaming.tsx`) already delivers entries
  live; reusing it means no second live channel.

The row is authoritative for `status`; the entry carries the render payload and a mirrored
status written by **the same handler** that writes the row. Single writer, so they cannot
drift. `ResponseEntry` and `EntryPatch` each gain one optional field:

```go
Elicitation *ElicitationEntry `json:"elicitation,omitempty"`
// ElicitationEntry: {id, tool_call_id, message, mode, schema, status, content}
```

Otherwise the accumulator is unchanged — an elicitation entry is just an entry whose `Type`
is `"elicitation"` and whose `Content` is the plain-text message, so `TextFromEntries` and
search keep working.

### Handlers

- `processExternalAgentSyncMessage` (`:847`) gains `"elicitation_requested"`,
  `"elicitation_resolved"`, `"elicitation_resync"`, `"elicitation_response_ack"`.
- `handleElicitationRequested`: resolve session + target interaction from `request_id` using
  the same path as `handleMessageAdded` (streaming context, then DB fallback); upsert the row
  (idempotent — a resync re-announcement of a known id only refreshes `LastSeenAt`); upsert
  the entry; publish entry patches + `interaction_update`; raise the `agent_question`
  attention event on the pending→(new) transition only. The interaction stays `state=waiting`
  — that is correct, it *is* waiting.
- **Empty or unmappable `request_id`**: do not invent a third rule. Use exactly the
  resolution `handleMessageAdded` already uses (streaming context → DB fallback to the
  newest `waiting` interaction for that session/thread; there is existing coverage for this
  in `TestMessageAdded_ContextMappingMiss_DBFallback`). Only if that also misses is the
  event dropped, with a `warn` naming the thread and elicitation id — dropping a question
  is the bug we are fixing, so it must be loud.
- `handleElicitationResolved`: conditional status update; mirror into the entry; clear the
  attention event; publish. Unknown id → log and drop.
- `handleElicitationResync`: for the named thread, refresh `LastSeenAt` on every listed id;
  for pending/submitting rows of that session **not** listed and whose `LastSeenAt` is older
  than the grace window, transition to `cancelled` (reason `agent_no_longer_holds`).
- `handleElicitationResponseAck`: log; on `noop`/`not_found` reconcile the row to a terminal
  status so the UI stops offering to answer.
- `handleAgentReady` (`:4261`): **no status changes here.** Reconnect is not evidence.

### Follow-up while a question is pending (Correction 2)

Zed's own model is unambiguous: `run_turn` unconditionally calls
`cancel_inner(RequestPermissionOutcome::InterruptedByFollowUp)` (`acp_thread.rs:3785`) which
calls `cancel_outstanding_elicitations` (`:3987`, `:4065`) — note this runs *before* the
`running_turn` check, so it fires whether or not a turn is live. A follow-up therefore
cancels the outstanding elicitation and the new turn proceeds. Helix mirrors that; it does
not invent a third behaviour.

The one Helix-side change needed: `processPromptQueue` (`:3392`) currently defers any
non-interrupt prompt while the newest interaction is `state=waiting`. A pending question
holds that state indefinitely and auto-wake is now deliberately gated off, so the user's
message would sit in the queue forever with no feedback. Fix: when the newest interaction is
`waiting` **and** has a pending/submitting elicitation, do not defer — dispatch with
interrupt semantics. Helix does **not** write a terminal status itself; Zed's resulting
`elicitation_resolved(cancelled)` is the authority, and the card reads "you replied instead".

### REST endpoint

```
POST /api/v1/sessions/{id}/elicitations/{elicitation_id}/respond
body: {"action":"accept"|"decline", "content": {…}}
```

Registered next to `/sessions/{id}/cancel` (`server.go:1057`), `system.Wrapper`, swagger
annotations in the `cancelSessionTurn` style, then `./stack update_openapi` and the
**generated** TS client from the frontend.

Authorisation and safety:
1. `GetSession(ctx, vars["id"])` → 404; `authorizeUserToSession(ctx, user, session,
   types.ActionUpdate)` → 403. The session comes from the URL only; the body carries no
   session identifier and none would be trusted.
2. Load the elicitation by id; if `elicitation.SessionID != sessionID` → 404 (do not leak
   existence).
3. Conditional transition `pending → submitting`; zero rows affected → 409 with the current
   status (this is the two-clients-at-once resolution — the loser gets a clean error).
4. No external-agent WebSocket for the session → 409 "agent not connected". Otherwise
   `sendCommandToExternalAgent`.
5. Return the new status; the authoritative terminal status arrives via
   `elicitation_resolved` and is pushed to the frontend.

### Auto-wake

`maybeAutoWake` (`auto_wake_stuck_interactions.go:225`) gets a **Gate 0**, before the
connection gate: if the interaction has an elicitation with `status IN (pending,
submitting)`, log and return. A turn blocked on a human is not a hung agent, and
re-prompting it would both cancel the question and confuse the user. One indexed query per
stuck row; the worker already runs at 10 s intervals over ≤50 rows.

**Known follow-up, out of scope here:** the org-layer activation spawner
(`api/pkg/org/infrastructure/runtime/helix/spawner.go`) has its own timeout that can spawn a
decoy `waiting` interaction on top of a healthy session. It does not know about
blocked-on-human either. Recorded so it is not lost.

## Layer 3 — Frontend

- `types.ts`: `ResponseEntry.type` gains `"elicitation"`; add the payload type.
  `streaming.tsx:482` carries `ep.elicitation` through the patch merge (alongside
  `tool_name`/`tool_status`).
- `elicitationSchema.ts` — a **pure** JSON-Schema → fields parser, unit-testable without a
  DOM:
  - iterate `schema.properties` in order; each property is one field.
  - `oneOf` present → single-select; option `const`/`title` is the label, `description` the
    sub-label.
  - `type: "array"` with `items.anyOf` → multi-select over the same option objects.
  - a property whose `_meta` contains **any** entry with `isCustomAnswer === true` or a
    `questionId` naming a sibling property is attached to that sibling as its "Other"
    free-text input. Key off the **value shape, not the meta-key name** — that name has
    already changed once (`_askUserQuestionCustomAnswer` in 0.66.0, not the
    `claudeCode/customAnswer` the brief claims).
  - plain `string` → text input; `boolean` → checkbox; anything else → text input.
  - `required` is absent in practice — nothing is mandatory unless the schema says so, and
    submitting nothing for a question is legal.
  - Unknown/degenerate schemas produce a generic form, never a throw. The card is wrapped in
    an error boundary so a hostile schema cannot take down the conversation view.
- **Submission content must mirror `applyAskElicitationResponse` exactly** (`elicitation.js:
  180-210`): send `question_<i>_custom` when non-empty after trim (the adapter then ignores
  that question's selection), send `question_<i>` otherwise, omit the question entirely when
  neither is set. Custom-only submission with the `oneOf` field unset is valid and is the
  "none of the above" path — it must be reachable in the UI.
- `ElicitationCard.tsx` — renders `message`, per-field header chip + question text, options
  as clickable controls with label **and** description, the "Other" input, one **Submit**
  and one **Skip/Decline**. Decline is not an abort: per the adapter it yields empty answers
  and the turn continues, so the copy must say so. After resolution the card renders the
  chosen answer(s) read-only with the terminal status (including "you replied instead" for a
  follow-up cancel and "expired — the agent restarted" for `agent_no_longer_holds`). Lucide
  icons only.
- `InteractionInference.tsx`: `buildActivityTimeline` gets an `elicitation` segment kind — it
  must **not** be folded into the collapsed tool-run segment.
- Mutation: generated API client + React Query, invalidating the session/interactions
  queries on success. No `setTimeout`, no `setQueryData`.
- Blocked surfacing: transient `waiting_for_user_input bool \`gorm:"-"\`` on `SpecTask`
  (same pattern as `SandboxStatusMessage`, `simple_spec_task.go:242`), populated in the list
  and get handlers from one `status='pending'` query; distinct badge in the task
  list/kanban and the task detail header; plus the `agent_question` attention event.

## Status reconciliation rules (Zed panel ↔ Helix UI)

1. **Zed's `ElicitationStore` is the source of truth for status.** Helix never invents a
   terminal status except by rule 5.
2. Helix's `submitting` is an optimistic local state, never sent to Zed and never displayed
   as "answered" — the card shows a spinner until `elicitation_resolved` arrives.
3. Every terminal transition observed in Zed (answered in the panel, declined, cancelled by
   turn teardown, cancelled by an interrupt, cancelled by a follow-up prompt) is mirrored by
   `elicitation_resolved`; the Helix card stops being answerable on receipt, live.
4. **A reconnect is not evidence of anything.** `agent_ready` fires on every WebSocket
   reconnect, and the commonest cause is the Helix API restarting (Air rebuild) while the
   desktop container, the Zed process and the `respond_tx` all survive untouched. Nothing
   about the status changes on reconnect.
5. Helix declares a pending elicitation `cancelled` only on **positive evidence from Zed**:
   (a) it is absent from that thread's `elicitation_resync` list and its `LastSeenAt` is
   older than the grace window, or (b) a `respond_elicitation` for it is acked `not_found`.
   A genuinely dead question (real container restart, thread torn down) is still reaped —
   just by evidence rather than assumption.
6. Terminal statuses are final. A late event for an already-terminal elicitation is dropped
   (conditional update affects zero rows) and logged.

## Failure modes handled

| Case | Behaviour |
|---|---|
| **API restart with a question pending** | Row + entry persisted; Zed still holds the `respond_tx`; the post-reconnect resync re-announces the id, `LastSeenAt` refreshes, status stays `pending`. The question is still shown and still answerable. (This is the case the first draft of this design got wrong.) |
| Container/agent restart with a question pending | Zed no longer holds it, so it is absent from the resync; after the grace window the row goes `cancelled(agent_no_longer_holds)` and the card reads "expired — the agent restarted". |
| User sends a normal message instead of answering | Queue dispatches it with interrupt semantics instead of deferring; Zed's `run_turn` cancels the elicitation; `elicitation_resolved(cancelled)` locks the card with "you replied instead". Never silently queued. |
| Answer arrives after cancel/decline | DB conditional update affects 0 rows → 409 to the caller; if the command already went out, Zed's `respond_to_elicitation` no-ops and acks `noop`; UI reconciles to the terminal status. |
| Two clients answer at once | First wins `pending→submitting`; second gets 409 with the current status. Only one `respond_elicitation` is ever sent. |
| Turn interrupted from Helix with the question unanswered | `cancel_outstanding_elicitations` (`acp_thread.rs:4065`) resolves the oneshot with `Cancel` → `elicitation_resolved(cancelled)` → card locked. |
| Elicitation with an empty/unmappable `request_id` | Falls back to `handleMessageAdded`'s existing resolution (streaming context → newest waiting interaction for the thread); dropped with a loud `warn` only if that also misses. |
| Unknown schema shape (MCP forwarding, refusal-fallback consent) | Generic field rendering; error boundary; the raw schema is preserved in the row so nothing is lost. |
| Agent disconnected when the user hits Submit | 409 "agent not connected"; the row stays pending, and the resync decides its fate on reconnect. |
| Elicitation answered in the Zed panel | Mirrored to Helix via `elicitation_resolved`. Implemented, but **untestable** here — the panel draws no controls (see the attached screenshot). Not a test path. |

## Cross-repo merge order (from helix `CLAUDE.md`)

1. Commit in the zed repo (do **not** push).
2. `git rev-parse HEAD` → update `ZED_COMMIT` in `sandbox-versions.txt`.
3. Open the **Helix** PR first.
4. Push the zed branch, open its PR — `gh pr create` **must** pass `--repo helixml/zed`.
5. Merge zed first, then helix. Rebase the `ZED_COMMIT` bump if it moved.

## Testing strategy

- **Rust**: unit tests in `external_websocket_sync` for the entry→event mapping, the resync
  emission, and the respond-command no-op/not-found paths (mock service via
  `WebSocketSyncService::new_test`).
- **Go**: suite additions to `websocket_external_agent_sync_test.go` (gomock store,
  `suite.Suite` per CLAUDE.md) for requested/resolved/resync/ack handling, the endpoint's
  auth + 409 paths, the empty-`request_id` fallback, **reconnect-does-not-cancel**,
  resync-absence-does-cancel-after-grace, the queue not deferring a follow-up while a
  question is pending, and `TestAutoWake_SkipsInteractionBlockedOnUserQuestion`.
- **Frontend**: unit tests for `elicitationSchema.ts` against the verified 0.66.0 schema
  (single question, 4 questions, multi-select, custom-answer field found by value shape,
  junk schema) and for the submission-content precedence rules.
- **E2E**: new Phase 17 in `helix-ws-test-server/main.go`, driven by a **synthetic**
  elicitation injected at the Zed test seam — a required CI phase must not depend on the
  model choosing to call a tool. Say so in the phase comment. Assert
  `elicitation_requested` with a non-empty schema → `respond_elicitation` →
  `elicitation_resolved(accepted)` → `message_completed` for the same turn. **The full
  dockerized e2e must be run and seen green** (`run_docker_e2e.sh`) — absolute rule in
  `CLAUDE.md`.
- **Live**: the inner-Helix runs in the requirements' Definition of Done, including the
  API-restart test and the follow-up-instead-of-answering test, with screenshots.

## Notes for future agents

- The `_ => return` arms in `thread_service.rs`'s subscribe handler are where any new
  `AgentThreadEntry` variant silently dies. First place to look when an entry kind never
  reaches Helix.
- `message_id` in this protocol is the **entry index** in `AcpThread::entries()`.
- `request_id` rotation (turn-scoped, not the global `THREAD_REQUEST_MAP`) is load-bearing;
  copy `turn_request_id.borrow()` usage from the neighbouring arms.
- **`agent_ready` is not a lifecycle signal about the agent process.** It fires on every
  WebSocket reconnect, including a plain API restart with the container untouched. Never
  derive "the agent died" from it.
- The adapter's `_meta` key names have already changed between versions. Parse elicitation
  schemas by value shape, never by meta-key string.
- The Zed agent panel's elicitation card is known-broken by decision, not by accident. Do
  not "fix" it as a debugging aid.
