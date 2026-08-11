# Design: End-to-End Agent Questions via ACP Elicitations

Three layers: Zed sync crate → Helix API/store → Helix frontend. The whole feature is one
new conversation entry type plus one new command, riding the existing sync machinery.

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
| Turn teardown cancels outstanding elicitations | `acp_thread.rs:4065 cancel_outstanding_elicitations` |
| Entry→SyncEvent mapping that must learn about elicitations (three `_ => return`/`continue` arms) | `crates/external_websocket_sync/src/thread_service.rs:914, 1051, 1112` |
| Thread registry lookup by `acp_thread_id` | `thread_service.rs:834 get_thread()` |
| Command→callback→GPUI-task pattern to copy verbatim | `thread_service.rs:1434-1483` (cancel task) + `external_websocket_sync.rs:574 request_cancel_thread` + `websocket_sync.rs:400` dispatch |
| `SyncEvent` enum + `to_outgoing_message()` | `crates/external_websocket_sync/src/types.rs:170-370` |
| `requested_schema` is a typed `acp::ElicitationSchema` (serde-serializable) — "verbatim JSON" means `serde_json::to_value(&mode.requested_schema)`, never a hand-rolled flatten | `agent_ui/src/conversation_view/elicitation.rs:918, 1553` |
| Accept response shape | `agent_ui/src/conversation_view.rs:2974` — `CreateElicitationResponse::new(ElicitationAction::Accept(ElicitationAcceptAction::new().content(content)))` |

Helix (`/home/retro/work/helix`):

| Fact | Location |
|---|---|
| Sync event dispatch switch | `api/pkg/server/websocket_external_agent_sync.go:847-900` |
| Entry accumulator + `ResponseEntry{Type,Content,MessageID,ToolName,ToolStatus}` | `api/pkg/server/wsprotocol/accumulator.go:59` |
| Persisted structured entries on the interaction | `api/pkg/types/types.go:87 Interaction.ResponseEntries` (jsonb) |
| Streaming deltas to the frontend | `types.EntryPatch` (`types.go:1011`), `publishEntryPatchesToFrontend` |
| Outbound command shape + delivery | `types.ExternalAgentCommand{Type,Data}` (`types.go:2281`), `sendCommandToExternalAgent` (`:2327`), `queueOrSend` (`:2635`) |
| Endpoint pattern to copy (auth + swagger + command) | `api/pkg/server/session_handlers.go:2396 cancelSessionTurn` |
| Auto-wake gate to extend | `api/pkg/server/auto_wake_stuck_interactions.go:225 maybeAutoWake` |
| Frontend entry rendering / timeline builder | `frontend/src/components/session/InteractionInference.tsx:20-140` |
| Frontend patch application | `frontend/src/contexts/streaming.tsx:482-540` |
| E2E phases 1-16 (add 17) | `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go:9-23` |

## Wire format

### Zed → Helix

Two new `SyncEvent` variants (plus one ack). `entry_index` is the position in
`AcpThread::entries()` and doubles as the accumulator's `message_id`, so the question lands
in the conversation in the right place with zero new ordering logic.

```jsonc
// event_type: "elicitation_requested"
{
  "acp_thread_id": "…",
  "request_id": "…",              // turn-scoped rid, same rotation rules as message_added
  "entry_index": "7",             // == message_id in the accumulator
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
  "action": "accept" | "decline" | "cancel",
  "content": { "question_0": "Red", "question_1": ["a","b"] }  // accept only
}
```

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
  resolved event is idempotent by design (same id + same terminal status), so double-emit
  is harmless — the Go side treats it as a conditional update.
- The three entry-mapping match arms (`:914` NewEntry, `:1051` EntryUpdated, `:1112`
  Stopped/Error re-flush) currently `_ => return`. Add an `Elicitation` arm so elicitation
  entries are not dropped and are re-sent on the terminal flush.

**Accept.** New command `respond_elicitation`, wired exactly like the cancel path:
`websocket_sync.rs` dispatch (`:400`) → `handle_respond_elicitation` →
`crate::request_elicitation_response(ElicitationResponseRequest{…})` (new global callback in
`external_websocket_sync.rs`, mirroring `GLOBAL_CANCEL_THREAD_CALLBACK` +
`PENDING_*` queue for the not-yet-initialised case) → dedicated GPUI task in
`thread_service.rs` next to the cancel task → `get_thread(&acp_thread_id)` →
`thread.update(cx, |t, cx| t.respond_to_elicitation(&ElicitationEntryId(id.into()), resp, cx))`.

**Idempotency and races.** `respond_to_elicitation` is already a safe no-op for an unknown
id or a non-`Pending` status, so no panic is reachable. To *report* the no-op, snapshot the
status via `thread.elicitation(&id)` before and after the update and emit
`ElicitationResponseAck` with `accepted` / `noop` / `not_found`. A dropped thread (registry
miss) → `not_found`. Never `unwrap()`; never `let _ =` on the send (`.log_err()` per the Zed
CLAUDE.md rules).

## Layer 2 — Helix API

### Storage decision: hybrid (row is authoritative, entry is the record)

**Chosen:** a dedicated `agent_elicitations` row **plus** a new `ResponseEntry` of type
`"elicitation"` inside `Interaction.ResponseEntries`.

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
    Created, Updated time.Time
}
```

Why both, not just the entry:

- **Status races need a conditional write.** Two clients answering at once, or an answer
  racing a Zed-side cancel, is resolved by `UPDATE … SET status=? WHERE id=? AND status=?`
  returning rows-affected. A jsonb array on the interaction gives no such primitive without
  read-modify-write on a blob that the streaming accumulator is concurrently rewriting.
- **Authorisation and lookup are by id.** The answer endpoint must check that
  `elicitation.session_id == {id from URL}`. That is one indexed lookup, not a jsonb scan.
- **"Which tasks are blocked" must be cheap.** The task list badge and the auto-wake gate
  both need `status='pending'` filtered by session/interaction. Indexed column vs. scanning
  every interaction's jsonb.

Why also the entry, not just the row:

- The card must render **in conversation order** and remain in the transcript after it is
  answered ("it is part of the conversation record"). The entries array is exactly that,
  and `entry_index` from Zed gives the ordering for free.
- The existing patch/stream path (`EntryPatch` → `streaming.tsx`) already delivers entries
  live; reusing it means no second live channel.

The row is authoritative for `status`; the entry carries the render payload and a mirrored
status written by the same handler that writes the row. Single writer, so they cannot drift.

`ResponseEntry` and `EntryPatch` both gain one optional field:

```go
Elicitation *ElicitationEntry `json:"elicitation,omitempty"`
// ElicitationEntry: {id, tool_call_id, message, mode, schema, status, content}
```

Everything else about the accumulator is unchanged — an elicitation entry is just an entry
whose `Type` is `"elicitation"` and whose `Content` is the plain-text message (so
`TextFromEntries` and search keep working).

### Handlers

- `processExternalAgentSyncMessage` (`:847`) gains `"elicitation_requested"`,
  `"elicitation_resolved"`, `"elicitation_response_ack"`.
- `handleElicitationRequested`: resolve session + target interaction from `request_id` using
  the same path as `handleMessageAdded` (streaming context, DB fallback); upsert the row;
  upsert the entry into the accumulator; publish entry patches + `interaction_update`. The
  interaction stays `state=waiting` — that is correct, it *is* waiting.
- `handleElicitationResolved`: conditional status update; mirror into the entry; publish.
  Unknown id → log and drop (a question from a thread this API never mapped).
- `handleElicitationResponseAck`: log; on `noop`/`not_found` reconcile the row to a terminal
  status so the UI stops offering to answer.
- `handleAgentReady` (`:4261`) gains reconciliation: on (re)connect, any `pending`/
  `submitting` elicitation for that session is transitioned to `cancelled` — the
  `respond_tx` died with the process, so it can never be answered.

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
4. No external-agent WebSocket for the session → 409 "agent not connected" (fail fast; a
   pending question implies a live blocked agent). Use `sendCommandToExternalAgent`.
5. Return the new status; the authoritative terminal status arrives via
   `elicitation_resolved` and is pushed to the frontend.

### Auto-wake

`maybeAutoWake` (`auto_wake_stuck_interactions.go:225`) gets a **Gate 0**, before the
connection gate: if the interaction has an elicitation with `status IN (pending,
submitting)`, log and return. A turn blocked on a human is not a hung agent, and re-prompting
it would both interrupt the elicitation and confuse the user. One indexed query per stuck
row; the worker already runs at 10 s intervals over ≤50 rows.

## Layer 3 — Frontend

- `types.ts`: `ResponseEntry.type` gains `"elicitation"`; add the `elicitation` payload
  type. `streaming.tsx:482` carries `ep.elicitation` through the patch merge (alongside
  `tool_name`/`tool_status`).
- `elicitationSchema.ts` — a **pure** JSON-Schema → fields parser, unit-testable without a
  DOM:
  - iterate `schema.properties` in order; each property is one field.
  - `oneOf` present → single-select; option `const`/`title` is the label, `description` the
    sub-label.
  - `type: "array"` with `items.anyOf` → multi-select over the same option objects.
  - a property whose `_meta["claudeCode/customAnswer"].questionId` names another property is
    attached to that property as its "Other" free-text input (this is the generic rule —
    **no** matching on the literal string `question_0`).
  - plain `string` → text input; `boolean` → checkbox; anything else → text input.
  - `required` may be absent (the adapter does not set it) — nothing is mandatory unless the
    schema says so.
  - Unknown/degenerate schemas produce a generic form, never a throw. The card is wrapped in
    an error boundary so a hostile schema cannot take down the conversation view.
- `ElicitationCard.tsx` — renders `message`, per-field header chip + question text, options
  as clickable controls with label **and** description, the "Other" input, one **Submit**
  and one **Decline**. After resolution it renders the chosen answer(s) read-only with the
  terminal status. Lucide icons only.
- `InteractionInference.tsx`: `buildActivityTimeline` gets an `elicitation` segment kind —
  it must **not** be folded into the collapsed tool-run segment (the whole point is that it
  is visible).
- Mutation: generated API client + React Query, invalidating the session/interactions
  queries on success. No `setTimeout`, no `setQueryData`.
- Blocked surfacing: transient `waiting_for_user_input bool \`gorm:"-"\`` on `SpecTask`
  (same pattern as `SandboxStatusMessage`, `simple_spec_task.go:242`), populated in the list
  and get handlers from one `status='pending'` query; rendered as a distinct badge in the
  task list/kanban and in the task detail header. If the answer to Open Question 2 is yes,
  additionally raise an `AttentionEvent` of new type `agent_question`, cleared when the
  elicitation reaches a terminal status.

## Status reconciliation rules (Zed panel ↔ Helix UI)

1. **Zed's `ElicitationStore` is the source of truth for status.** Helix never invents a
   terminal status except in case 4.
2. Helix's own `submitting` is an optimistic local state, never sent to Zed and never
   displayed as "answered" — the card shows a spinner until `elicitation_resolved` arrives.
3. Every terminal transition observed in Zed (answered in the panel, declined, cancelled by
   turn teardown, cancelled by an interrupt) is mirrored by `elicitation_resolved`; the
   Helix card stops being answerable on receipt, live, without a refresh.
4. **Only** on agent (re)connect does Helix declare a still-pending elicitation `cancelled`:
   the process that owned the `respond_tx` is gone, so no answer can ever land.
5. Terminal statuses are final. A late event for an already-terminal elicitation is dropped
   (conditional update affects zero rows) and logged.

## Failure modes handled

| Case | Behaviour |
|---|---|
| Answer arrives after cancel/decline | DB conditional update affects 0 rows → 409 to the caller; if the command already went out, Zed's `respond_to_elicitation` no-ops and acks `noop`; UI reconciles to the terminal status. |
| Two clients answer at once | First wins the `pending→submitting` transition; second gets 409 with the current status. Only one `respond_elicitation` command is ever sent. |
| Container/API restart with a question pending | Row + entry persisted, so the transcript survives. On `agent_ready` the row is reconciled to `cancelled`; the card renders "expired — the agent restarted". No zombie answerable question. |
| Turn interrupted from Helix with the question unanswered | Zed's `cancel_outstanding_elicitations` (`acp_thread.rs:4065`) resolves the oneshot with `Cancel` → `elicitation_resolved(cancelled)` → card locked. |
| Elicitation for a thread/request Helix cannot map | Logged and dropped, exactly like an unmappable `message_added`. Never creates an orphan interaction. |
| Unknown schema shape (MCP forwarding, refusal-fallback consent) | Generic field rendering; error boundary; the raw schema is preserved in the row so nothing is lost. |
| Agent disconnected when the user hits Submit | 409 "agent not connected"; the row stays pending until the reconnect reconciliation cancels it. |
| Elicitation answered in the Zed panel | Mirrored to Helix via `elicitation_resolved`. Implemented, but **untestable** here — the panel draws no controls (see the attached screenshot). Not a test path. |

## Cross-repo merge order (from helix `CLAUDE.md`)

1. Commit in the zed repo (do **not** push).
2. `git rev-parse HEAD` → update `ZED_COMMIT` in `sandbox-versions.txt`.
3. Open the **Helix** PR first.
4. Push the zed branch, open its PR — `gh pr create` **must** pass `--repo helixml/zed`.
5. Merge zed first, then helix. Rebase the `ZED_COMMIT` bump if it moved.

## Testing strategy

- **Rust**: unit tests in `external_websocket_sync` for the entry→event mapping and the
  respond-command no-op/not-found paths (mock service via `WebSocketSyncService::new_test`).
- **Go**: table-driven suite additions to `websocket_external_agent_sync_test.go`
  (gomock store, `suite.Suite` pattern per CLAUDE.md) for requested/resolved/ack handling,
  the endpoint's auth + 409 paths, and `TestAutoWake_SkipsInteractionBlockedOnUserQuestion`.
- **Frontend**: unit tests for `elicitationSchema.ts` against the verified adapter schema
  (single question, 4 questions, multi-select, custom-answer field, junk schema).
- **E2E**: new Phase 17 in `helix-ws-test-server/main.go` — prompt the agent to call
  `AskUserQuestion`, assert `elicitation_requested` with a non-empty schema, send
  `respond_elicitation`, assert `elicitation_resolved(accepted)` and a subsequent
  `message_completed` for the same turn. **The full dockerized e2e must be run and seen
  green** (`run_docker_e2e.sh`) — this is an absolute rule in `CLAUDE.md`.
- **Live**: the inner-Helix run described in requirements' Definition of Done, with
  screenshots.

## Notes for future agents

- The `_ => return` arms in `thread_service.rs`'s subscribe handler are where any new
  `AgentThreadEntry` variant silently dies. If a new entry kind ever appears in Zed, this is
  the first place to look.
- `message_id` in this protocol is the **entry index** in `AcpThread::entries()`. That is
  why a new entry type slots into the accumulator's ordering for free.
- `request_id` rotation (turn-scoped, not the global `THREAD_REQUEST_MAP`) is load-bearing;
  copy `turn_request_id.borrow()` usage from the neighbouring arms rather than reaching for
  `crate::get_thread_request_id`.
- The Zed agent panel's elicitation card is known-broken by design decision, not by
  accident. Do not "fix" it as a debugging aid.
